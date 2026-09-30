package feedstatus

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestTrackerObservesProviderInstrumentAndSyntheticStatus(t *testing.T) {
	tracker := New()
	at := time.Date(2026, 9, 25, 7, 0, 0, 0, time.UTC)
	trade := at.Add(-2 * time.Second)
	processed := at.Add(35 * time.Millisecond)
	tracker.Observe(domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "upstox",
		Price:        25123.45,
		EventTime:    at,
		TradeTime:    trade,
		ReceivedTime: at.Add(20 * time.Millisecond),
		ProcessedTime: processed,
		Quality:      domain.QualityGood,
	})
	tracker.SetSyntheticStatus(func() any {
		return map[string]any{"atm": 25100.0, "active_legs": 6}
	})

	snapshot := tracker.Snapshot()
	provider := snapshot.Providers["upstox"]
	if provider.Observed != 1 {
		t.Fatalf("provider observed=%d", provider.Observed)
	}
	if provider.LastTradeTimeMS != trade.UnixMilli() {
		t.Fatalf("trade time=%d want=%d", provider.LastTradeTimeMS, trade.UnixMilli())
	}
	instrument := snapshot.Instruments["NSE:NIFTY50"]
	if instrument.Price != 25123.45 {
		t.Fatalf("price=%v", instrument.Price)
	}
	if instrument.LastProcessedTimeMS != processed.UnixMilli() {
		t.Fatalf("processed time=%d want=%d", instrument.LastProcessedTimeMS, processed.UnixMilli())
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
