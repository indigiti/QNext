package pipeline

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestConfiguredContinuousSessionAcrossPipeline(t *testing.T) {
	ids := map[string]bool{"FIXTURE:BTC-USDT": true, "OTHER:ETH-USDT": true}
	engine := candle.NewWithUTCInstruments("fixture", ids)
	delete(ids, "FIXTURE:BTC-USDT")
	p, err := New(engine, nil, []string{"5s", "15s", "1m", "2m"})
	if err != nil {
		t.Fatal(err)
	}
	// Weekend UTC midnight, outside India's cash session.
	start := time.Date(2026, 11, 28, 23, 58, 0, 0, time.UTC)
	for _, id := range []string{"FIXTURE:BTC-USDT", "OTHER:ETH-USDT"} {
		var finalMinute, finalTwo bool
		for i := 0; i <= 24; i++ {
			tick := domain.Tick{InstrumentID: id, Provider: "fixture", Price: 100, Quantity: 1,
				EventTime: start.Add(time.Duration(i) * 5 * time.Second), Quality: domain.QualityGood}
			bars, err := p.ApplyTick(tick)
			if err != nil {
				t.Fatal(err)
			}
			for _, bar := range bars {
				if bar.InstrumentID != id {
					t.Fatalf("cross-instrument bar: %+v", bar)
				}
				if bar.Final && bar.OpenTime.Equal(start) && bar.Timeframe == "1m" {
					finalMinute = true
				}
				if bar.Final && bar.OpenTime.Equal(start) && bar.Timeframe == "2m" {
					finalTwo = true
				}
			}
		}
		if !finalMinute || !finalTwo {
			t.Fatalf("%s: continuous UTC rollup not finalized", id)
		}
	}
	if engine.InInstrumentSession("NSE:NIFTY50", start) {
		t.Fatal("legacy NSE session changed")
	}
}
