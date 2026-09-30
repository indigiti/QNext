package stream

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

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
	replay            []BarEvent
	subs              map[uint64]chan BarEvent
	lastPublishedAtMS int64
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
}

type BrokerStats struct {
	Streams      int                 `json:"streams"`
	Subscribers  int                 `json:"subscribers"`
	ReplayEvents int                 `json:"replay_events"`
	Details      []BrokerStreamStats `json:"details"`
}

type Subscription struct {
	StreamID       string
	CurrentSeq     uint64
	Replay         []BarEvent
	Events         <-chan BarEvent
	ResyncRequired bool

	broker *Broker
	key    string
	id     uint64
	once   sync.Once
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
	defer b.mu.Unlock()

	state := b.streams[key]
	if state == nil {
		state = &streamState{
			replay: make([]BarEvent, 0, b.retention),
			subs:   make(map[uint64]chan BarEvent),
		}
		b.streams[key] = state
	}
	state.seq++
	state.lastPublishedAtMS = time.Now().UTC().UnixMilli()
	event := BarEvent{
		StreamID: streamID(bar.InstrumentID, bar.Timeframe),
		Seq:      state.seq,
		Bar:      bar,
	}
	if len(state.replay) < b.retention {
		state.replay = append(state.replay, event)
	} else {
		copy(state.replay, state.replay[1:])
		state.replay[len(state.replay)-1] = event
	}
	for id, ch := range state.subs {
		select {
		case ch <- event:
		default:
			close(ch)
			delete(state.subs, id)
		}
	}
}

func (b *Broker) PublishResync(instrumentID, timeframe, reason string) {
	if instrumentID == "" || timeframe == "" {
		return
	}
	key := streamKey(instrumentID, timeframe)
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.streams[key]
	if state == nil {
		return
	}
	event := BarEvent{StreamID: streamID(instrumentID, timeframe), ResyncRequired: true, Reason: reason}
	for id, ch := range state.subs {
		select {
		case ch <- event:
		default:
			close(ch)
			delete(state.subs, id)
		}
	}
}

func (b *Broker) LatestBar(instrumentID, timeframe string) (domain.Bar, bool) {
	if instrumentID == "" || timeframe == "" {
		return domain.Bar{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.streams[streamKey(instrumentID, timeframe)]
	if state == nil || len(state.replay) == 0 {
		return domain.Bar{}, false
	}
	return state.replay[len(state.replay)-1].Bar, true
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
			ReplayEvents:      len(state.replay),
			LastPublishedAtMS: state.lastPublishedAtMS,
		}
		if len(state.replay) > 0 {
			bar := state.replay[len(state.replay)-1].Bar
			streamStats.LastBarOpenTimeMS = bar.OpenTime.UTC().UnixMilli()
			streamStats.LastBarFinal = bar.Final
		}
		stats.Subscribers += streamStats.Subscribers
		stats.ReplayEvents += streamStats.ReplayEvents
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
	defer b.mu.Unlock()
	state := b.streams[key]
	if state == nil {
		state = &streamState{replay: make([]BarEvent, 0, b.retention), subs: make(map[uint64]chan BarEvent)}
		b.streams[key] = state
	}
	sub := &Subscription{StreamID: id, CurrentSeq: state.seq, broker: b, key: key}
	if afterSeq != nil {
		if *afterSeq > state.seq {
			sub.ResyncRequired = true
			return sub, nil
		}
		if *afterSeq < state.seq {
			if len(state.replay) == 0 || *afterSeq+1 < state.replay[0].Seq {
				sub.ResyncRequired = true
				return sub, nil
			}
			for _, event := range state.replay {
				if event.Seq > *afterSeq {
					sub.Replay = append(sub.Replay, event)
				}
			}
		}
	}
	b.nextSubscriberID++
	ch := make(chan BarEvent, b.subscriberBuffer)
	state.subs[b.nextSubscriberID] = ch
	sub.id = b.nextSubscriberID
	sub.Events = ch
	return sub, nil
}

func (s *Subscription) Cancel() {
	if s == nil || s.broker == nil || s.id == 0 {
		return
	}
	s.once.Do(func() {
		s.broker.mu.Lock()
		defer s.broker.mu.Unlock()
		state := s.broker.streams[s.key]
		if state == nil {
			return
		}
		if ch, ok := state.subs[s.id]; ok {
			close(ch)
			delete(state.subs, s.id)
		}
	})
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
