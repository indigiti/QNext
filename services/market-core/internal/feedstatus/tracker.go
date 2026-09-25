package feedstatus

import (
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type ProviderSnapshot struct {
	Observed           uint64  `json:"observed"`
	LastEventTimeMS    int64   `json:"last_event_time_ms"`
	LastReceivedTimeMS int64   `json:"last_received_time_ms"`
	LastPrice          float64 `json:"last_price,omitempty"`
}

type InstrumentSnapshot struct {
	Provider           string         `json:"provider"`
	Price              float64        `json:"price"`
	LastEventTimeMS    int64          `json:"last_event_time_ms"`
	LastReceivedTimeMS int64          `json:"last_received_time_ms"`
	Quality            domain.Quality `json:"quality"`
	SyntheticVersion   string         `json:"synthetic_version,omitempty"`
}

type Snapshot struct {
	Providers   map[string]ProviderSnapshot   `json:"providers"`
	Instruments map[string]InstrumentSnapshot `json:"instruments"`
	Synthetic   any                           `json:"synthetic,omitempty"`
	Synthetics  map[string]any                `json:"synthetics,omitempty"`
}

type Tracker struct {
	mu                sync.RWMutex
	providers         map[string]ProviderSnapshot
	instruments       map[string]InstrumentSnapshot
	syntheticStatus   func() any
	syntheticStatuses map[string]func() any
}

func New() *Tracker {
	return &Tracker{
		providers:         map[string]ProviderSnapshot{},
		instruments:       map[string]InstrumentSnapshot{},
		syntheticStatuses: map[string]func() any{},
	}
}

func (t *Tracker) Observe(tick domain.Tick) {
	if t == nil || tick.InstrumentID == "" || tick.Provider == "" || tick.Price <= 0 || tick.EventTime.IsZero() {
		return
	}

	eventMS := tick.EventTime.UTC().UnixMilli()
	received := tick.ReceivedTime.UTC()
	if received.IsZero() {
		received = tick.EventTime.UTC()
	}
	receivedMS := received.UnixMilli()

	t.mu.Lock()
	provider := t.providers[tick.Provider]
	provider.Observed++
	if eventMS >= provider.LastEventTimeMS {
		provider.LastEventTimeMS = eventMS
		provider.LastPrice = tick.Price
	}
	if receivedMS > provider.LastReceivedTimeMS {
		provider.LastReceivedTimeMS = receivedMS
	}
	t.providers[tick.Provider] = provider

	current := t.instruments[tick.InstrumentID]
	if eventMS >= current.LastEventTimeMS {
		t.instruments[tick.InstrumentID] = InstrumentSnapshot{
			Provider:           tick.Provider,
			Price:              tick.Price,
			LastEventTimeMS:    eventMS,
			LastReceivedTimeMS: receivedMS,
			Quality:            tick.Quality,
			SyntheticVersion:   tick.SyntheticVersion,
		}
	}
	t.mu.Unlock()
}

func (t *Tracker) SetSyntheticStatus(status func() any) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.syntheticStatus = status
	t.mu.Unlock()
}

func (t *Tracker) SetSyntheticStatusFor(instrumentID string, status func() any) {
	if t == nil || instrumentID == "" || status == nil {
		return
	}
	t.mu.Lock()
	t.syntheticStatuses[instrumentID] = status
	t.mu.Unlock()
}

func (t *Tracker) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{Providers: map[string]ProviderSnapshot{}, Instruments: map[string]InstrumentSnapshot{}}
	}

	t.mu.RLock()
	providers := make(map[string]ProviderSnapshot, len(t.providers))
	for key, value := range t.providers {
		providers[key] = value
	}
	instruments := make(map[string]InstrumentSnapshot, len(t.instruments))
	for key, value := range t.instruments {
		instruments[key] = value
	}
	status := t.syntheticStatus
	statuses := make(map[string]func() any, len(t.syntheticStatuses))
	for key, value := range t.syntheticStatuses {
		statuses[key] = value
	}
	t.mu.RUnlock()

	var synthetic any
	if status != nil {
		synthetic = status()
	}
	synthetics := make(map[string]any, len(statuses))
	for key, statusFn := range statuses {
		synthetics[key] = statusFn()
	}
	return Snapshot{
		Providers:   providers,
		Instruments: instruments,
		Synthetic:   synthetic,
		Synthetics:  synthetics,
	}
}

func AgeMS(atMS int64, now time.Time) int64 {
	if atMS <= 0 {
		return -1
	}
	age := now.UTC().UnixMilli() - atMS
	if age < 0 {
		return 0
	}
	return age
}
