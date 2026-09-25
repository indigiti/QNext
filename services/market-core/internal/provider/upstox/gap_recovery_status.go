package upstox

import (
	"sync"
	"time"
)

type GapRecoverySnapshot struct {
	Attempts            uint64   `json:"attempts"`
	Successes           uint64   `json:"successes"`
	Failures            uint64   `json:"failures"`
	RecoveredBars       uint64   `json:"recovered_bars"`
	LastRecoveredBars   uint64   `json:"last_recovered_bars"`
	LastFromMS          int64    `json:"last_from_ms,omitempty"`
	LastToMS            int64    `json:"last_to_ms,omitempty"`
	LastCause           string   `json:"last_cause,omitempty"`
	LastError           string   `json:"last_error,omitempty"`
	ExactTickReplay     bool     `json:"exact_tick_replay"`
	RecoveredTimeframes []string `json:"recovered_timeframes"`
	NonExactTimeframes  []string `json:"non_exact_timeframes"`
}

type GapRecoveryTracker struct {
	mu       sync.RWMutex
	snapshot GapRecoverySnapshot
}

func NewGapRecoveryTracker(recoveredTimeframes, nonExactTimeframes []string) *GapRecoveryTracker {
	return &GapRecoveryTracker{
		snapshot: GapRecoverySnapshot{
			ExactTickReplay:     false,
			RecoveredTimeframes: append([]string(nil), recoveredTimeframes...),
			NonExactTimeframes:  append([]string(nil), nonExactTimeframes...),
		},
	}
}

func (t *GapRecoveryTracker) Observe(request RecoveryRequest, recoveredBars uint64, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.snapshot.Attempts++
	t.snapshot.LastRecoveredBars = recoveredBars
	t.snapshot.RecoveredBars += recoveredBars
	t.snapshot.LastCause = request.Cause
	t.snapshot.LastFromMS = millisOrZero(request.From)
	t.snapshot.LastToMS = millisOrZero(request.To)
	t.snapshot.LastError = ""

	if err != nil {
		t.snapshot.Failures++
		t.snapshot.LastError = err.Error()
		return
	}
	t.snapshot.Successes++
}

func (t *GapRecoveryTracker) Snapshot() GapRecoverySnapshot {
	if t == nil {
		return GapRecoverySnapshot{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := t.snapshot
	out.RecoveredTimeframes = append([]string(nil), t.snapshot.RecoveredTimeframes...)
	out.NonExactTimeframes = append([]string(nil), t.snapshot.NonExactTimeframes...)
	return out
}

func millisOrZero(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UTC().UnixMilli()
}
