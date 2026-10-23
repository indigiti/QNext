package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/research"
)

func TestMultiIndexSynPlusVolumeObserversAreIsolated(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_ENABLED_MARKETS", "NIFTY,BANKNIFTY")
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:NIFTY-SYN+")
	t.Setenv("QNEXT_MARKET_CONFIG", "")
	broker := NewBroker(64, 16)
	handler := NewWebSocketHandler(broker).(*WebSocketHandler)
	if len(handler.shadows) != 2 || handler.shadow == nil {
		t.Fatalf("expected independent NIFTY and BANKNIFTY shadows, got %d", len(handler.shadows))
	}
	at := time.Date(2026, 10, 23, 3, 45, 0, 0, time.UTC)
	testMarkets := []struct {
		id     string
		source string
		value  float64
		volume float64
	}{
		{"QNEXT:NIFTY-SYN+", "NSE:NIFTY50", 25100, 100},
		{"QNEXT:BANKNIFTY-SYN+", "NSE:BANKNIFTY", 51500, 900},
	}
	for _, market := range testMarkets {
		broker.PublishBar(domain.Bar{
			InstrumentID: market.source, Timeframe: "15s",
			OpenTime: at, CloseTime: at.Add(15*time.Second), Volume: market.volume,
		})
		if err := handler.shadows[market.id].observe(research.SynPlusSnapshot{
			Schema: research.SynPlusSnapshotSchema, InstrumentID: market.id,
			SnapshotAtMS: at.Add(time.Second).UnixMilli(), Value: market.value, Quality: "GOOD",
		}); err != nil {
			t.Fatal(err)
		}
		bar, ok := broker.LatestBar(market.id, "15s")
		if !ok || bar.Volume != market.volume || bar.Close != market.value {
			t.Fatalf("incorrect %s SYN+ snapshot: %+v", market.id, bar)
		}
	}
	// A BANKNIFTY source update must not overwrite NIFTY volume or prices.
	broker.PublishBar(domain.Bar{
		InstrumentID: "NSE:BANKNIFTY", Timeframe: "15s",
		OpenTime: at, CloseTime: at.Add(15*time.Second), Volume: 1100,
	})
	bank, _ := broker.LatestBar("QNEXT:BANKNIFTY-SYN+", "15s")
	nifty, _ := broker.LatestBar("QNEXT:NIFTY-SYN+", "15s")
	if bank.Volume != 1100 || bank.Close != 51500 || nifty.Volume != 100 || nifty.Close != 25100 {
		t.Fatalf("cross-market volume leakage: bank=%+v nifty=%+v", bank, nifty)
	}

	// NIFTY changes remain independent of the newly enabled BANKNIFTY bridge.
	broker.PublishBar(domain.Bar{
		InstrumentID: "NSE:NIFTY50", Timeframe: "15s",
		OpenTime: at, CloseTime: at.Add(15*time.Second), Volume: 250,
	})
	nifty, _ = broker.LatestBar("QNEXT:NIFTY-SYN+", "15s")
	bank, _ = broker.LatestBar("QNEXT:BANKNIFTY-SYN+", "15s")
	if nifty.Volume != 250 || bank.Volume != 1100 {
		t.Fatalf("NIFTY update lost independent observer: nifty=%+v bank=%+v", nifty, bank)
	}
}

func TestDisabledSynPlusMarketDoesNotCreateShadow(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_ENABLED_MARKETS", "NIFTY")
	t.Setenv("QNEXT_MARKET_CONFIG", "")
	handler := NewWebSocketHandler(NewBroker(8, 8)).(*WebSocketHandler)
	if handler.shadows["QNEXT:BANKNIFTY-SYN+"] != nil ||
		handler.shadows["QNEXT:NIFTY-SYN+"] == nil {
		t.Fatalf("disabled market created a shadow: %+v", handler.shadows)
	}
}
