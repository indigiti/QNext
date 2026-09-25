package stream

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
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
	seq    uint64
	replay []BarEvent
	subs   map[uint64]*subscriberState
}

type subscriberState struct {
	events      chan BarEvent
	closeReason chan string
}

type BrokerSnapshot struct {
	Streams     int
	Subscribers int
}

type Subscription struct {
	StreamID       string
	CurrentSeq     uint64
	Replay         []BarEvent
	Events         <-chan BarEvent
	ResyncRequired bool
	closeReason    <-chan string

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
	return &Broker{
		retention:        retention,
		subscriberBuffer: subscriberBuffer,
		streams:          make(map[string]*streamState),
	}
}

func (b *Broker) PublishBar(bar domain.Bar) {
	key := streamKey(bar.InstrumentID, bar.Timeframe)

	b.mu.Lock()
	defer b.mu.Unlock()

	state := b.streams[key]
	if state == nil {
		state = &streamState{subs: make(map[uint64]*subscriberState)}
		b.streams[key] = state
	}
	state.seq++
	event := BarEvent{
		StreamID: streamID(bar.InstrumentID, bar.Timeframe),
		Seq:      state.seq,
		Bar:      bar,
	}
	state.replay = append(state.replay, event)
	if len(state.replay) > b.retention {
		state.replay = append([]BarEvent(nil), state.replay[len(state.replay)-b.retention:]...)
	}

	for id, subscriber := range state.subs {
		select {
		case subscriber.events <- event:
		default:
			closeSlowSubscriber(state, id, subscriber)
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
	event := BarEvent{
		StreamID:       streamID(instrumentID, timeframe),
		ResyncRequired: true,
		Reason:         reason,
	}
	for id, subscriber := range state.subs {
		select {
		case subscriber.events <- event:
		default:
			closeSlowSubscriber(state, id, subscriber)
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
		state = &streamState{subs: make(map[uint64]*subscriberState)}
		b.streams[key] = state
	}

	sub := &Subscription{
		StreamID:   id,
		CurrentSeq: state.seq,
		broker:     b,
		key:        key,
	}

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
	subscriber := &subscriberState{
		events:      make(chan BarEvent, b.subscriberBuffer),
		closeReason: make(chan string, 1),
	}
	state.subs[b.nextSubscriberID] = subscriber
	sub.id = b.nextSubscriberID
	sub.Events = subscriber.events
	sub.closeReason = subscriber.closeReason
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
		if subscriber, ok := state.subs[s.id]; ok {
			close(subscriber.events)
			close(subscriber.closeReason)
			delete(state.subs, s.id)
		}
	})
}

func (b *Broker) Snapshot() BrokerSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()

	snapshot := BrokerSnapshot{Streams: len(b.streams)}
	for _, state := range b.streams {
		snapshot.Subscribers += len(state.subs)
	}
	return snapshot
}

func closeSlowSubscriber(state *streamState, id uint64, subscriber *subscriberState) {
	select {
	case subscriber.closeReason <- "slow_consumer":
	default:
	}
	close(subscriber.events)
	close(subscriber.closeReason)
	delete(state.subs, id)
}

func streamKey(instrumentID, timeframe string) string {
	return instrumentID + "\x00" + timeframe
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
