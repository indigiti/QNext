package stream

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/demand"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/observability"
)

type BarEvent struct {
	StreamID       string
	Seq            uint64
	Bar            domain.Bar
	ResyncRequired bool
	Reason         string
}

type Broker struct {
	mu               sync.Mutex
	retention        int
	subscriberBuffer int
	streams          map[string]*streamState
	nextSubscriberID uint64
}

type streamState struct {
	seq               uint64
	replay            *replayRing
	subs              map[uint64]*subscriberState
	lastPublishedAtMS int64
	coalescedForming  uint64
	droppedForming    uint64
	slowDisconnects   uint64
}

type replayRing struct {
	items []BarEvent
	start int
	size  int
}

type subscriberState struct {
	mu           sync.Mutex
	out          chan BarEvent
	wake         chan struct{}
	done         chan struct{}
	once         sync.Once
	queue        []BarEvent
	maxQueued    int
	maxProtected int
}

type enqueueResult struct {
	coalesced      uint64
	droppedForming uint64
	overflow       bool
}

type BrokerStreamStats struct {
	StreamID          string `json:"stream_id"`
	InstrumentID      string `json:"instrument_id"`
	Timeframe         string `json:"timeframe"`
	Seq               uint64 `json:"seq"`
	Subscribers       int    `json:"subscribers"`
	ReplayEvents      int    `json:"replay_events"`
	LastPublishedAtMS int64  `json:"last_published_at_ms"`
	LastBarOpenTimeMS int64  `json:"last_bar_open_time_ms,omitempty"`
	LastBarFinal      bool   `json:"last_bar_final"`
	CoalescedForming  uint64 `json:"coalesced_forming"`
	DroppedForming    uint64 `json:"dropped_forming"`
	SlowDisconnects   uint64 `json:"slow_disconnects"`
}

type BrokerStats struct {
	Streams          int                 `json:"streams"`
	Subscribers      int                 `json:"subscribers"`
	ReplayEvents     int                 `json:"replay_events"`
	CoalescedForming uint64              `json:"coalesced_forming"`
	DroppedForming   uint64              `json:"dropped_forming"`
	SlowDisconnects  uint64              `json:"slow_disconnects"`
	Details          []BrokerStreamStats `json:"details"`
}

type Subscription struct {
	StreamID       string
	CurrentSeq     uint64
	Replay         []BarEvent
	Events         <-chan BarEvent
	ResyncRequired bool

	broker           *Broker
	key              string
	id               uint64
	demandInstrument string
	once             sync.Once
}

func NewBroker(retention, subscriberBuffer int) *Broker {
	if retention <= 0 {
		retention = 256
	}
	if subscriberBuffer <= 0 {
		subscriberBuffer = 64
	}
	broker := &Broker{
		retention:        retention,
		subscriberBuffer: subscriberBuffer,
		streams:          make(map[string]*streamState),
	}
	observability.SetBrokerSource(func() any { return broker.Stats() })
	return broker
}

func (b *Broker) PublishBar(bar domain.Bar) {
	key := streamKey(bar.InstrumentID, bar.Timeframe)

	b.mu.Lock()
	state := b.streams[key]
	if state == nil {
		state = b.newStreamState()
		b.streams[key] = state
	}
	state.seq++
	state.lastPublishedAtMS = time.Now().UTC().UnixMilli()
	event := BarEvent{
		StreamID: streamID(bar.InstrumentID, bar.Timeframe),
		Seq:      state.seq,
		Bar:      bar,
	}
	state.replay.Append(event)

	var released int
	for id, subscriber := range state.subs {
		result := subscriber.enqueue(event)
		state.coalescedForming += result.coalesced
		state.droppedForming += result.droppedForming
		if !result.overflow {
			continue
		}
		subscriber.stop()
		delete(state.subs, id)
		state.slowDisconnects++
		released++
	}
	b.mu.Unlock()

	for i := 0; i < released; i++ {
		demand.Default().Release(bar.InstrumentID)
	}
}

func (b *Broker) PublishResync(instrumentID, timeframe, reason string) {
	if instrumentID == "" || timeframe == "" {
		return
	}
	key := streamKey(instrumentID, timeframe)
	b.mu.Lock()
	state := b.streams[key]
	if state == nil {
		b.mu.Unlock()
		return
	}
	event := BarEvent{StreamID: streamID(instrumentID, timeframe), ResyncRequired: true, Reason: reason}
	var released int
	for id, subscriber := range state.subs {
		result := subscriber.enqueue(event)
		state.coalescedForming += result.coalesced
		state.droppedForming += result.droppedForming
		if !result.overflow {
			continue
		}
		subscriber.stop()
		delete(state.subs, id)
		state.slowDisconnects++
		released++
	}
	b.mu.Unlock()

	for i := 0; i < released; i++ {
		demand.Default().Release(instrumentID)
	}
}

func (b *Broker) LatestBar(instrumentID, timeframe string) (domain.Bar, bool) {
	if instrumentID == "" || timeframe == "" {
		return domain.Bar{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.streams[streamKey(instrumentID, timeframe)]
	if state == nil {
		return domain.Bar{}, false
	}
	event, ok := state.replay.Last()
	if !ok {
		return domain.Bar{}, false
	}
	return event.Bar, true
}

func (b *Broker) Stats() BrokerStats {
	if b == nil {
		return BrokerStats{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	stats := BrokerStats{Streams: len(b.streams)}
	stats.Details = make([]BrokerStreamStats, 0, len(b.streams))
	for key, state := range b.streams {
		instrumentID, timeframe := splitStreamKey(key)
		streamStats := BrokerStreamStats{
			StreamID:          streamID(instrumentID, timeframe),
			InstrumentID:      instrumentID,
			Timeframe:         timeframe,
			Seq:               state.seq,
			Subscribers:       len(state.subs),
			ReplayEvents:      state.replay.Len(),
			LastPublishedAtMS: state.lastPublishedAtMS,
			CoalescedForming:  state.coalescedForming,
			DroppedForming:    state.droppedForming,
			SlowDisconnects:   state.slowDisconnects,
		}
		if event, ok := state.replay.Last(); ok {
			streamStats.LastBarOpenTimeMS = event.Bar.OpenTime.UTC().UnixMilli()
			streamStats.LastBarFinal = event.Bar.Final
		}
		stats.Subscribers += streamStats.Subscribers
		stats.ReplayEvents += streamStats.ReplayEvents
		stats.CoalescedForming += streamStats.CoalescedForming
		stats.DroppedForming += streamStats.DroppedForming
		stats.SlowDisconnects += streamStats.SlowDisconnects
		stats.Details = append(stats.Details, streamStats)
	}
	return stats
}

func (b *Broker) Subscribe(instrumentID, timeframe string, afterSeq *uint64) (*Subscription, error) {
	if instrumentID == "" || timeframe == "" {
		return nil, errors.New("instrument and timeframe are required")
	}
	return b.subscribe(streamKey(instrumentID, timeframe), streamID(instrumentID, timeframe), afterSeq)
}

func (b *Broker) SubscribeByID(id string, afterSeq uint64) (*Subscription, error) {
	instrumentID, timeframe, err := parseStreamID(id)
	if err != nil {
		return nil, err
	}
	return b.subscribe(streamKey(instrumentID, timeframe), id, &afterSeq)
}

func (b *Broker) subscribe(key, id string, afterSeq *uint64) (*Subscription, error) {
	b.mu.Lock()
	state := b.streams[key]
	if state == nil {
		state = b.newStreamState()
		b.streams[key] = state
	}
	instrumentID, _ := splitStreamKey(key)
	sub := &Subscription{
		StreamID:         id,
		CurrentSeq:       state.seq,
		broker:           b,
		key:              key,
		demandInstrument: instrumentID,
	}
	if afterSeq != nil {
		if *afterSeq > state.seq {
			sub.ResyncRequired = true
			b.mu.Unlock()
			return sub, nil
		}
		if *afterSeq < state.seq {
			oldest, ok := state.replay.First()
			if !ok || *afterSeq+1 < oldest.Seq {
				sub.ResyncRequired = true
				b.mu.Unlock()
				return sub, nil
			}
			for _, event := range state.replay.Slice() {
				if event.Seq > *afterSeq {
					sub.Replay = append(sub.Replay, event)
				}
			}
		}
	}
	b.nextSubscriberID++
	subscriber := newSubscriberState(b.subscriberBuffer)
	state.subs[b.nextSubscriberID] = subscriber
	sub.id = b.nextSubscriberID
	sub.Events = subscriber.out
	b.mu.Unlock()

	demand.Default().Acquire(instrumentID)
	go subscriber.run()
	return sub, nil
}

func (s *Subscription) Cancel() {
	if s == nil || s.broker == nil || s.id == 0 {
		return
	}
	s.once.Do(func() {
		removed := false
		s.broker.mu.Lock()
		state := s.broker.streams[s.key]
		if state != nil {
			if subscriber, ok := state.subs[s.id]; ok {
				subscriber.stop()
				delete(state.subs, s.id)
				removed = true
			}
		}
		s.broker.mu.Unlock()
		if removed {
			demand.Default().Release(s.demandInstrument)
		}
	})
}

func (b *Broker) newStreamState() *streamState {
	return &streamState{
		replay: newReplayRing(b.retention),
		subs:   make(map[uint64]*subscriberState),
	}
}

func newReplayRing(capacity int) *replayRing {
	if capacity <= 0 {
		capacity = 1
	}
	return &replayRing{items: make([]BarEvent, capacity)}
}

func (r *replayRing) Append(event BarEvent) {
	if r.size < len(r.items) {
		index := (r.start + r.size) % len(r.items)
		r.items[index] = event
		r.size++
		return
	}
	r.items[r.start] = event
	r.start = (r.start + 1) % len(r.items)
}

func (r *replayRing) Len() int {
	if r == nil {
		return 0
	}
	return r.size
}

func (r *replayRing) First() (BarEvent, bool) {
	if r == nil || r.size == 0 {
		return BarEvent{}, false
	}
	return r.items[r.start], true
}

func (r *replayRing) Last() (BarEvent, bool) {
	if r == nil || r.size == 0 {
		return BarEvent{}, false
	}
	index := (r.start + r.size - 1) % len(r.items)
	return r.items[index], true
}

func (r *replayRing) Slice() []BarEvent {
	if r == nil || r.size == 0 {
		return nil
	}
	result := make([]BarEvent, r.size)
	for i := 0; i < r.size; i++ {
		result[i] = r.items[(r.start+i)%len(r.items)]
	}
	return result
}

func newSubscriberState(buffer int) *subscriberState {
	if buffer <= 0 {
		buffer = 1
	}
	protectedBurst := buffer / 2
	if protectedBurst < 8 {
		protectedBurst = 8
	}
	return &subscriberState{
		out:          make(chan BarEvent),
		wake:         make(chan struct{}, 1),
		done:         make(chan struct{}),
		queue:        make([]BarEvent, 0, buffer+protectedBurst),
		maxQueued:    buffer,
		maxProtected: buffer + protectedBurst,
	}
}

func (s *subscriberState) enqueue(event BarEvent) enqueueResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.done:
		return enqueueResult{overflow: true}
	default:
	}

	if len(s.queue) < s.maxQueued {
		s.queue = append(s.queue, event)
		s.signal()
		return enqueueResult{}
	}

	if isProtectedEvent(event) {
		kept := s.queue[:0]
		var dropped uint64
		for _, queued := range s.queue {
			if !isProtectedEvent(queued) && len(s.queue)-int(dropped) >= s.maxQueued {
				dropped++
				continue
			}
			kept = append(kept, queued)
		}
		s.queue = kept
		if len(s.queue) >= s.maxProtected {
			return enqueueResult{droppedForming: dropped, overflow: true}
		}
		s.queue = append(s.queue, event)
		s.signal()
		return enqueueResult{droppedForming: dropped}
	}

	for index := len(s.queue) - 1; index >= 0; index-- {
		queued := s.queue[index]
		if isProtectedEvent(queued) {
			continue
		}
		if queued.Bar.OpenTime.Equal(event.Bar.OpenTime) {
			s.queue[index] = event
			s.signal()
			return enqueueResult{coalesced: 1}
		}
	}
	return enqueueResult{droppedForming: 1}
}

func (s *subscriberState) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *subscriberState) run() {
	defer close(s.out)
	for {
		event, ok := s.next()
		if !ok {
			return
		}
		select {
		case s.out <- event:
		case <-s.done:
			return
		}
	}
}

func (s *subscriberState) next() (BarEvent, bool) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			event := s.queue[0]
			s.queue[0] = BarEvent{}
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return event, true
		}
		s.mu.Unlock()

		select {
		case <-s.wake:
		case <-s.done:
			return BarEvent{}, false
		}
	}
}

func (s *subscriberState) stop() {
	s.once.Do(func() { close(s.done) })
}

func isProtectedEvent(event BarEvent) bool {
	return event.ResyncRequired || event.Bar.Final
}

func streamKey(instrumentID, timeframe string) string { return instrumentID + "\x00" + timeframe }
func splitStreamKey(key string) (string, string) {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) != 2 {
		return key, ""
	}
	return parts[0], parts[1]
}
func streamID(instrumentID, timeframe string) string {
	encode := base64.RawURLEncoding.EncodeToString
	return "bars." + encode([]byte(instrumentID)) + "." + encode([]byte(timeframe))
}
func parseStreamID(id string) (string, string, error) {
	parts := strings.Split(id, ".")
	if len(parts) != 3 || parts[0] != "bars" {
		return "", "", errors.New("invalid stream id")
	}
	decode := base64.RawURLEncoding.DecodeString
	instrument, err := decode(parts[1])
	if err != nil {
		return "", "", errors.New("invalid stream instrument")
	}
	timeframe, err := decode(parts[2])
	if err != nil {
		return "", "", errors.New("invalid stream timeframe")
	}
	if len(instrument) == 0 || len(timeframe) == 0 {
		return "", "", errors.New("invalid stream id")
	}
	return string(instrument), string(timeframe), nil
}
