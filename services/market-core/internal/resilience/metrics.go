package resilience

import (
	"sort"
	"sync"
	"time"
)

type ProviderSnapshot struct {
	Received            uint64 `json:"received"`
	Accepted            uint64 `json:"accepted"`
	DroppedNonAuthority uint64 `json:"dropped_non_authority"`
	Errors              uint64 `json:"errors"`
	LastEventTimeMS     int64  `json:"last_event_time_ms"`
}

type SwitchEvent struct {
	InstrumentID  string `json:"instrument_id"`
	From          string `json:"from"`
	To            string `json:"to"`
	Reason        string `json:"reason"`
	PolicyVersion string `json:"authority_policy_version"`
	AtMS          int64  `json:"at_ms"`
	GapMS         int64  `json:"gap_ms"`
}

type Snapshot struct {
	ActiveAuthorities map[string]string           `json:"active_authorities"`
	AuthorityStates   map[string]string           `json:"authority_states"`
	Provider          map[string]ProviderSnapshot `json:"providers"`
	AuthoritySwitches uint64                      `json:"authority_switches"`
	GapDetections     uint64                      `json:"gap_detections"`
	RecoveryAttempts  uint64                      `json:"recovery_attempts"`
	RecoverySuccesses uint64                      `json:"recovery_successes"`
	RecoveryFailures  uint64                      `json:"recovery_failures"`
	RecentSwitches    []SwitchEvent               `json:"recent_switches"`
}

type Metrics struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

func NewMetrics() *Metrics {
	return &Metrics{snapshot: Snapshot{
		ActiveAuthorities: map[string]string{},
		AuthorityStates:   map[string]string{},
		Provider:          map[string]ProviderSnapshot{},
	}}
}

func (m *Metrics) received(provider string, at time.Time) {
	m.mu.Lock()
	p := m.snapshot.Provider[provider]
	p.Received++
	if at.UnixMilli() > p.LastEventTimeMS {
		p.LastEventTimeMS = at.UnixMilli()
	}
	m.snapshot.Provider[provider] = p
	m.mu.Unlock()
}

func (m *Metrics) accepted(provider string) {
	m.mu.Lock()
	p := m.snapshot.Provider[provider]
	p.Accepted++
	m.snapshot.Provider[provider] = p
	m.mu.Unlock()
}

func (m *Metrics) dropped(provider string) {
	m.mu.Lock()
	p := m.snapshot.Provider[provider]
	p.DroppedNonAuthority++
	m.snapshot.Provider[provider] = p
	m.mu.Unlock()
}

func (m *Metrics) ObserveError(provider string) {
	m.mu.Lock()
	p := m.snapshot.Provider[provider]
	p.Errors++
	m.snapshot.Provider[provider] = p
	m.mu.Unlock()
}

func (m *Metrics) setAuthority(id, provider string) {
	m.mu.Lock()
	m.snapshot.ActiveAuthorities[id] = provider
	m.mu.Unlock()
}

func (m *Metrics) setState(id string, state AuthorityState) {
	m.mu.Lock()
	m.snapshot.AuthorityStates[id] = string(state)
	m.mu.Unlock()
}

func (m *Metrics) switched(event SwitchEvent, state AuthorityState) {
	m.mu.Lock()
	m.snapshot.AuthoritySwitches++
	m.snapshot.ActiveAuthorities[event.InstrumentID] = event.To
	m.snapshot.AuthorityStates[event.InstrumentID] = string(state)
	m.snapshot.RecentSwitches = append(m.snapshot.RecentSwitches, event)
	if len(m.snapshot.RecentSwitches) > 32 {
		m.snapshot.RecentSwitches = append([]SwitchEvent(nil), m.snapshot.RecentSwitches[len(m.snapshot.RecentSwitches)-32:]...)
	}
	m.mu.Unlock()
}

func (m *Metrics) gap() {
	m.mu.Lock()
	m.snapshot.GapDetections++
	m.mu.Unlock()
}

func (m *Metrics) recovery(ok bool) {
	m.mu.Lock()
	m.snapshot.RecoveryAttempts++
	if ok {
		m.snapshot.RecoverySuccesses++
	} else {
		m.snapshot.RecoveryFailures++
	}
	m.mu.Unlock()
}

func (m *Metrics) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.snapshot
	s.ActiveAuthorities = cloneMap(m.snapshot.ActiveAuthorities)
	s.AuthorityStates = cloneMap(m.snapshot.AuthorityStates)
	s.Provider = make(map[string]ProviderSnapshot, len(m.snapshot.Provider))
	for k, v := range m.snapshot.Provider {
		s.Provider[k] = v
	}
	s.RecentSwitches = append([]SwitchEvent(nil), m.snapshot.RecentSwitches...)
	sort.Slice(s.RecentSwitches, func(i, j int) bool { return s.RecentSwitches[i].AtMS < s.RecentSwitches[j].AtMS })
	return s
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
