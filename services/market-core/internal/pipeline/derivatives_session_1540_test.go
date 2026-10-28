package pipeline

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestSyntheticContinuesAfterCashCloseInCanonical5SecondPipeline(t *testing.T) {
	pipe, err := New(candle.New("session-v3"), nil, []string{"15s", "30s", "1m", "5m"})
	if err != nil { t.Fatal(err) }
	cashClose, _ := time.Parse(time.RFC3339, "2026-10-28T15:30:00+05:30")
	price := func(id string, at time.Time) []domain.Bar {
		t.Helper()
		out, err := pipe.ApplyTick(domain.Tick{
			InstrumentID: id, EventTime: at, Price: 25000,
			Provider: "test", Quality: domain.QualityGood,
		})
		if err != nil { t.Fatal(err) }
		return out
	}
	if got := price("NSE:NIFTY50", cashClose); len(got) != 0 {
		t.Fatalf("cash index fabricated post-close price: %+v", got)
	}
	if got := price("QNEXT:NIFTY-SYN", cashClose.Add(5*time.Second)); len(got) == 0 {
		t.Fatal("NIFTY-SYN stopped at 15:30")
	}
	if got := price("QNEXT:NIFTY-SYN+", cashClose.Add(9*time.Minute+50*time.Second)); len(got) == 0 {
		t.Fatal("NIFTY-SYN+ stopped before 15:40")
	}
	if got := price("QNEXT:NIFTY-SYN", cashClose.Add(10*time.Minute)); len(got) != 0 {
		t.Fatalf("option-derived price extended past F&O close: %+v", got)
	}
}
