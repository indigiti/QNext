package pipeline

import (
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type BucketIntegrity string

const (
	IntegrityGood         BucketIntegrity = "GOOD"
	IntegrityCarryForward BucketIntegrity = "CARRY_FORWARD"
	IntegrityRecovered    BucketIntegrity = "RECOVERED"
	IntegrityMissing      BucketIntegrity = "MISSING"
	IntegrityDegraded     BucketIntegrity = "DEGRADED"
)

type IntegrityEntry struct {
	InstrumentID string          `json:"instrument_id"`
	OpenTimeMS   int64           `json:"open_time_ms"`
	CloseTimeMS  int64           `json:"close_time_ms"`
	State        BucketIntegrity `json:"state"`
	Reason       string          `json:"reason,omitempty"`
}

type IntegritySnapshot struct {
	Counts map[BucketIntegrity]uint64 `json:"counts"`
	Latest map[string]IntegrityEntry  `json:"latest,omitempty"`
}

type integrityLedger struct {
	counts map[BucketIntegrity]uint64
	latest map[string]IntegrityEntry
}

func newIntegrityLedger() *integrityLedger {
	return &integrityLedger{
		counts: make(map[BucketIntegrity]uint64),
		latest: make(map[string]IntegrityEntry),
	}
}

func (l *integrityLedger) observeBar(bar domain.Bar) {
	if l == nil || !bar.Final || bar.Timeframe != "5s" {
		return
	}
	state := IntegrityGood
	reason := ""
	switch {
	case bar.CarryForward:
		state = IntegrityCarryForward
		reason = "NO_UPDATE"
	case bar.Recovered:
		state = IntegrityRecovered
	case bar.Quality == domain.QualityDegraded || bar.Quality == domain.QualityStale || bar.Quality == domain.QualityInvalid:
		state = IntegrityDegraded
	}
	l.record(bar.InstrumentID, bar.OpenTime, bar.CloseTime, state, reason)
}

func (l *integrityLedger) observeMissing(instrumentID string, openTime, closeTime time.Time, reason string) {
	if l == nil || instrumentID == "" || openTime.IsZero() || !closeTime.After(openTime) {
		return
	}
	l.record(instrumentID, openTime, closeTime, IntegrityMissing, reason)
}

func (l *integrityLedger) record(instrumentID string, openTime, closeTime time.Time, state BucketIntegrity, reason string) {
	l.counts[state]++
	l.latest[instrumentID] = IntegrityEntry{
		InstrumentID: instrumentID,
		OpenTimeMS:   openTime.UTC().UnixMilli(),
		CloseTimeMS:  closeTime.UTC().UnixMilli(),
		State:        state,
		Reason:       reason,
	}
}

func (l *integrityLedger) snapshot() IntegritySnapshot {
	out := IntegritySnapshot{
		Counts: make(map[BucketIntegrity]uint64, len(l.counts)),
		Latest: make(map[string]IntegrityEntry, len(l.latest)),
	}
	for key, value := range l.counts {
		out.Counts[key] = value
	}
	for key, value := range l.latest {
		out.Latest[key] = value
	}
	return out
}
