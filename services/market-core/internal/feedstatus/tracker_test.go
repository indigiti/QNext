package feedstatus

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestTrackerObservesProviderInstrumentAndSyntheticStatus(t *testing.T) {
	tracker := New()
	at := time.Date(2026, 9, 25, 7, 0, 0, 0, time.UTC)
	tracker.Observe(domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "upstox",
		Price:        25123.45,
		EventTime:    at,
		ReceivedTime: at.Add(20 * time.Millisecond),
		Quality:      domain.QualityGood,
	})
	tracker.SetSyntheticStatus(func() any {
		return map[string]any{"atm": 25100.0, "active_legs": 6}
	})

	snapshot := tracker.Snapshot()
	if snapshot.Providers["upstox"].Observed != 1 {
		t.Fatalf("provider observed=%d", snapshot.Providers["upstox"].Observed)
	}
	if snapshot.Instruments["NSE:NIFTY50"].Price != 25123.45 {
		t.Fatalf("price=%v", snapshot.Instruments["NSE:NIFTY50"].Price)
	}
	if snapshot.Synthetic == nil {
		t.Fatal("synthetic status missing")
	}
}

func TestTrackerReportsMultipleSyntheticStatuses(t *testing.T) {
	tracker := New()
	tracker.SetSyntheticStatusFor("QNEXT:NIFTY-SYN", func() any {
		return map[string]any{"atm": 25100.0}
	})
	tracker.SetSyntheticStatusFor("QNEXT:BANKNIFTY-SYN", func() any {
		return map[string]any{"atm": 55100.0}
	})

	snapshot := tracker.Snapshot()
	if len(snapshot.Synthetics) != 2 {
		t.Fatalf("expected two synthetic statuses, got %+v", snapshot.Synthetics)
	}
}
