package stream

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

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
	PublishedAt    time.Time
}

type Broker struct {
	mu                    sync.Mutex
	retention             int
	subscriberBuffer      int
	streams               map[string]*streamState
	nextSubscriberID      uint64
	webSocketConnections  atomic.Int64
	publishLatency        sampledLatencyHistogram
	writeLatency          sampledLatencyHistogram
	tickHandleLatency     sampledLatencyHistogram
	publishToWriteLatency sampledLatencyHistogram
	compactedStreams      uint64
	reclaimedReplaySlots  uint64
	evictedEmptyStreams   uint64
	barObservers          map[string]func(domain.Bar)
	proxyVolumeObservers  map[string]func(domain.Tick)
	syntheticVolume       map[string]string
	volumeTargets         map[string][]string
	volumeSources         map[string][]domain.Bar
	syntheticBars         map[string][]domain.Bar
}

type streamState struct {
	id                string
	seq               uint64
	replay            *replayRing
	subs              map[uint64]*subscriberState
	lastPublishedAtMS int64
	idleSinceMS       int64
	coalescedForming  uint64
	droppedForming    uint64
	slowDisconnects   uint64
	latestBar         domain.Bar
	hasLatestBar      bool
}

type replayRing struct {
	items []BarEvent
	start int
	size  int
	limit int
}

type subscriberState struct {
	mu           sync.Mutex
	out          chan BarEvent
	wake         chan struct{}
	done         chan struct{}
	once         sync.Once
	queue        []BarEvent
	queueStorage []BarEvent
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
	ReplaySlots       int    `json:"replay_slots"`
	LastPublishedAtMS int64  `json:"last_published_at_ms"`
	LastBarOpenTimeMS int64  `json:"last_bar_open_time_ms,omitempty"`
	LastBarFinal      bool   `json:"last_bar_final"`
	CoalescedForming  uint64 `json:"coalesced_forming"`
	DroppedForming    uint64 `json:"dropped_forming"`
	SlowDisconnects   uint64 `json:"slow_disconnects"`
}

type BrokerStats struct {
	Streams               int                 `json:"streams"`
	ActiveStreams         int                 `json:"active_streams"`
	IdleStreams           int                 `json:"idle_streams"`
	Subscribers           int                 `json:"subscribers"`
	WebSocketConnections  int                 `json:"websocket_connections"`
	ReplayEvents          int                 `json:"replay_events"`
	ReplaySlots           int                 `json:"replay_slots"`
	ReplayBytesEstimated  int64               `json:"replay_bytes_estimated"`
	CompactedStreams      uint64              `json:"compacted_streams"`
	ReclaimedReplaySlots  uint64              `json:"reclaimed_replay_slots"`
	EvictedEmptyStreams   uint64              `json:"evicted_empty_streams"`
	PublishLatency        LatencyStats        `json:"publish_latency"`
	WSWriteLatency        LatencyStats        `json:"ws_write_latency"`
	TickHandleLatency     LatencyStats        `json:"tick_handle_latency"`
	PublishToWriteLatency LatencyStats        `json:"publish_to_ws_write_latency"`
	CoalescedForming      uint64              `json:"coalesced_forming"`
	DroppedForming        uint64              `json:"dropped_forming"`
	SlowDisconnects       uint64              `json:"slow_disconnects"`
	Details               []BrokerStreamStats `json:"details"`
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
		retention:            retention,
		subscriberBuffer:     subscriberBuffer,
		streams:              make(map[string]*streamState),
		barObservers:         make(map[string]func(domain.Bar)),
		proxyVolumeObservers: make(map[string]func(domain.Tick)),
	}
	observability.SetBrokerSource(func() any { return broker.Stats() })
	return broker
}

// SetBarObserver registers an independent observer for a canonical source
// instrument. Registering another index does not evict existing subscribers;
// callbacks run outside the broker lock and may publish derived bars safely.
// No additional upstream provider subscription is created.
func (b *Broker) SetBarObserver(instrumentID string, observe func(domain.Bar)) {
	if b == nil || strings.TrimSpace(instrumentID) == "" {
		return
	}
	b.mu.Lock()
	if observe == nil {
		delete(b.barObservers, instrumentID)
	} else {
		b.barObservers[instrumentID] = observe
	}
	b.mu.Unlock()
}

// SetProxyVolumeObserver listens to the authoritative routed futures deltas,
// even when the index-price pipeline cannot finalize a 15s/30s source candle.
// Each index has an independent observer and no extra provider subscriptions.
func (b *Broker) SetProxyVolumeObserver(instrumentID string, observe func(domain.Tick)) {
	if b == nil || strings.TrimSpace(instrumentID) == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if observe == nil {
		delete(b.proxyVolumeObservers, instrumentID)
	} else {
		b.proxyVolumeObservers[instrumentID] = observe
	}
}

// PublishProxyVolumeTick invokes the registered SYN+ volume bridge outside
// the broker lock; it publishes no synthetic price candle of its own.
func (b *Broker) PublishProxyVolumeTick(tick domain.Tick) {
	if b == nil || tick.Quantity <= 0 || tick.Provider != "upstox-futures-proxy" {
		return
	}
	b.mu.Lock()
	observer := b.proxyVolumeObservers[tick.InstrumentID]
	patches := b.alignDerivativeProxyTickLocked(tick)
	b.mu.Unlock()
	for _, patch := range patches {
		b.PublishBar(patch)
	}
	if observer != nil {
		observer(tick)
	}
}

func (b *Broker) PublishBar(bar domain.Bar) {
	started := time.Now()
	key := streamKey(bar.InstrumentID, bar.Timeframe)

	b.mu.Lock()
	bar, patches := b.alignVolumeLocked(bar)
	state := b.streams[key]
	if state == nil {
		state = b.newStreamState(bar.InstrumentID, bar.Timeframe)
		b.streams[key] = state
	}
	state.seq++
	state.lastPublishedAtMS = started.UnixMilli()
	// Older revision events may be emitted after a newer candle has opened.
	// Preserve chronological latest for REST live overlay and volume mirroring.
	if !state.hasLatestBar || !bar.OpenTime.Before(state.latestBar.OpenTime) {
		state.latestBar = bar
		state.hasLatestBar = true
	}
	event := BarEvent{
		StreamID:    state.id,
		Seq:         state.seq,
		Bar:         bar,
		PublishedAt: started,
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
	if len(state.subs) == 0 && state.idleSinceMS == 0 {
		state.idleSinceMS = time.Now().UnixMilli()
	}
	observer := b.barObservers[bar.InstrumentID]
	b.mu.Unlock()

	if observer != nil {
		observer(bar)
	}
	for _, patch := range patches {
		b.PublishBar(patch)
	}
	for i := 0; i < released; i++ {
		demand.Default().Release(bar.InstrumentID)
	}
	b.publishLatency.observe(time.Since(started))
}

// ObserveTickHandleLatency measures the canonical MarketSink.Handle path,
// including synthetic processing, persistence enqueue and bar publishing.
// It is not upstream exchange latency or browser render latency.
func (b *Broker) ObserveTickHandleLatency(elapsed time.Duration) {
	if b != nil {
		b.tickHandleLatency.observe(elapsed)
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
	event := BarEvent{StreamID: state.id, ResyncRequired: true, Reason: reason}
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
	if len(state.subs) == 0 && state.idleSinceMS == 0 {
		state.idleSinceMS = time.Now().UnixMilli()
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
	if !state.hasLatestBar {
		return domain.Bar{}, false
	}
	return state.latestBar, true
}

func (b *Broker) Stats() BrokerStats {
	if b == nil {
		return BrokerStats{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	stats := BrokerStats{
		Streams:               len(b.streams),
		WebSocketConnections:  int(b.webSocketConnections.Load()),
		CompactedStreams:      b.compactedStreams,
		ReclaimedReplaySlots:  b.reclaimedReplaySlots,
		EvictedEmptyStreams:   b.evictedEmptyStreams,
		PublishLatency:        b.publishLatency.snapshot(),
		WSWriteLatency:        b.writeLatency.snapshot(),
		TickHandleLatency:     b.tickHandleLatency.snapshot(),
		PublishToWriteLatency: b.publishToWriteLatency.snapshot(),
	}
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
			ReplaySlots:       len(state.replay.items),
			LastPublishedAtMS: state.lastPublishedAtMS,
			CoalescedForming:  state.coalescedForming,
			DroppedForming:    state.droppedForming,
			SlowDisconnects:   state.slowDisconnects,
		}
		if event, ok := state.replay.Last(); ok {
			streamStats.LastBarOpenTimeMS = event.Bar.OpenTime.UTC().UnixMilli()
			streamStats.LastBarFinal = event.Bar.Final
		}
		if streamStats.Subscribers > 0 {
			stats.ActiveStreams++
		} else {
			stats.IdleStreams++
		}
		stats.Subscribers += streamStats.Subscribers
		stats.ReplayEvents += streamStats.ReplayEvents
		stats.ReplaySlots += streamStats.ReplaySlots
		stats.CoalescedForming += streamStats.CoalescedForming
		stats.DroppedForming += streamStats.DroppedForming
		stats.SlowDisconnects += streamStats.SlowDisconnects
		stats.Details = append(stats.Details, streamStats)
	}
	// This is a shallow allocated-slot estimate: BarEvent contains referenced
	// strings, so it is not total heap/RSS. Query it only on Stats(), not on publish.
	stats.ReplayBytesEstimated = int64(stats.ReplaySlots) * int64(unsafe.Sizeof(BarEvent{}))
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
	instrumentID, timeframe := splitStreamKey(key)
	b.mu.Lock()
	state := b.streams[key]
	if state == nil {
		state = b.newStreamState(instrumentID, timeframe)
		b.streams[key] = state
	}
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
	state.idleSinceMS = 0
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
				if len(state.subs) == 0 {
					state.idleSinceMS = time.Now().UnixMilli()
				}
				removed = true
			}
		}
		s.broker.mu.Unlock()
		if removed {
			demand.Default().Release(s.demandInstrument)
		}
	})
}

// Cache the canonical stream ID once rather than encoding it on every
// forming-bar revision. The identity is stable across subscribers and resume.
func (b *Broker) newStreamState(instrumentID, timeframe string) *streamState {
	return &streamState{
		id:          streamID(instrumentID, timeframe),
		replay:      newReplayRing(b.retention),
		subs:        make(map[uint64]*subscriberState),
		idleSinceMS: time.Now().UnixMilli(),
	}
}

// Grow replay capacity only as a stream becomes active. Streams with a short
// lifetime should not allocate the entire retention window eagerly. Once
// full, the ring still preserves the exact same sequence/resume semantics.
func newReplayRing(capacity int) *replayRing {
	if capacity <= 0 {
		capacity = 1
	}
	initial := capacity
	if initial > 16 {
		initial = 16
	}
	return &replayRing{items: make([]BarEvent, initial), limit: capacity}
}

func (r *replayRing) Append(event BarEvent) {
	if r.size == len(r.items) && r.size < r.limit {
		nextCapacity := len(r.items) * 2
		if nextCapacity > r.limit {
			nextCapacity = r.limit
		}
		next := make([]BarEvent, nextCapacity)
		for i := 0; i < r.size; i++ {
			next[i] = r.items[(r.start+i)%len(r.items)]
		}
		r.items = next
		r.start = 0
	}
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
	storage := make([]BarEvent, 0, buffer+protectedBurst)
	return &subscriberState{
		out:          make(chan BarEvent),
		wake:         make(chan struct{}, 1),
		done:         make(chan struct{}),
		queue:        storage,
		queueStorage: storage,
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
		s.appendQueued(event)
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
		s.appendQueued(event)
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

// Queue consumption advances the slice window. Compact only when its tail
// is exhausted, reusing the original bounded backing array rather than
// allocating a new array on every long-running subscription.
func (s *subscriberState) appendQueued(event BarEvent) {
	if len(s.queue) == cap(s.queue) && len(s.queue) < cap(s.queueStorage) {
		copy(s.queueStorage[:len(s.queue)], s.queue)
		s.queue = s.queueStorage[:len(s.queue)]
	}
	s.queue = append(s.queue, event)
	s.signal()
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
			if len(s.queue) == 0 {
				s.queue = s.queueStorage[:0]
			}
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
