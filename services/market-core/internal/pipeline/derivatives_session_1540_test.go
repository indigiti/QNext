package pipeline

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestSyntheticContinuesAfterCashCloseInCanonical5SecondPipeline(t *testing.T) {
	pipe, err := New(candle.New("session-v3"), nil, []string{"15s", "30s", "1m", "5m"})
	if err != nil {
		t.Fatal(err)
	}
	cashClose, _ := time.Parse(time.RFC3339, "2026-10-28T15:30:00+05:30")
	price := func(id string, at time.Time) []domain.Bar {
		t.Helper()
		out, err := pipe.ApplyTick(domain.Tick{
			InstrumentID: id, EventTime: at, Price: 25000,
			Provider: "test", Quality: domain.QualityGood,
		})
		if err != nil {
			t.Fatal(err)
		}
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

func TestLastSyntheticMinuteFinalizesOnClockAt1540(t *testing.T) {
	pipe, err := New(candle.New("final-1540"), nil, []string{"15s", "30s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse(time.RFC3339, "2026-10-28T15:39:00+05:30")
	for i := 0; i < 12; i++ {
		at := start.Add(time.Duration(i) * 5 * time.Second)
		if _, err := pipe.ApplyTick(domain.Tick{
			InstrumentID: "QNEXT:NIFTY-SYN+", EventTime: at,
			Provider: "test", Price: 25100 + float64(i),
			Quality: domain.QualityGood,
		}); err != nil {
			t.Fatal(err)
		}
	}
	lastClose := start.Add(time.Minute)
	updates, err := pipe.AdvanceClock(lastClose, func(string, domain.Tick, time.Time) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool)
	for _, bar := range updates {
		if bar.Final && bar.CloseTime.Equal(lastClose) {
			got[bar.Timeframe] = true
		}
	}
	for _, timeframe := range []string{"5s", "15s", "30s", "1m"} {
		if !got[timeframe] {
			t.Fatalf("missing %s final at 15:40: %+v", timeframe, updates)
		}
	}
}
