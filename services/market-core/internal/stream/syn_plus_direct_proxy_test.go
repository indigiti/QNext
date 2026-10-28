package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/research"
)

func TestSynPlusFuturesVolumeWorksWhenIndexSourceCandleIsMissing(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_ENABLED_MARKETS", "NIFTY")
	t.Setenv("QNEXT_MARKET_CONFIG", "")
	broker := NewBroker(64, 16)
	handler := NewWebSocketHandler(broker).(*WebSocketHandler)
	at := time.Date(2026, 10, 28, 3, 45, 0, 0, time.UTC)
	observe := func(at time.Time, price float64) {
		t.Helper()
		if err := handler.shadow.observe(research.SynPlusSnapshot{
			Schema:       research.SynPlusSnapshotSchema,
			InstrumentID: "QNEXT:NIFTY-SYN+",
			Version:      "direct-proxy-test",
			SnapshotAtMS: at.UnixMilli(),
			Quality:      "GOOD", Value: price,
		}); err != nil {
			t.Fatal(err)
		}
	}
	trade := func(at time.Time, qty float64) {
		t.Helper()
		broker.PublishProxyVolumeTick(domain.Tick{
			InstrumentID: "NSE:NIFTY50",
			Provider:     "upstox-futures-proxy",
			EventTime:    at,
			Quantity:     qty,
		})
	}
	check := func(timeframe string, open time.Time, want float64) {
		t.Helper()
		got, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", timeframe)
		if !ok || !got.OpenTime.Equal(open) || got.Volume != want ||
			(want > 0 && got.VolumeStatus != "OBSERVED") {
			t.Fatalf("%s incorrect direct futures volume: %+v ok=%v want=%v at=%v", timeframe, got, ok, want, open)
		}
		if got.Open != 25100 || got.Close != 25100 {
			t.Fatalf("direct volume unexpectedly changed synthetic price: %+v", got)
		}
	}

	observe(at.Add(time.Second), 25100)
	trade(at.Add(2*time.Second), 12)
	trade(at.Add(7*time.Second), 18)
	// The index-price pipeline publishes NO index source candles at any TF.
	for _, timeframe := range []string{"15s", "30s", "1m"} {
		check(timeframe, at, 30)
	}
	// When source candles eventually arrive, they must not double-count
	// the same futures deltas and must not cause a false volume reset.
	broker.PublishBar(domain.Bar{
		InstrumentID: "NSE:NIFTY50", Timeframe: "15s",
		OpenTime: at, CloseTime: at.Add(15 * time.Second),
		Volume: 30, VolumeSource: "FUTURES_PROXY", VolumeStatus: "OBSERVED",
	})
	check("15s", at, 30)
	// A sparse 1m price feed and a delayed source bar must not stop future
	// directly observed volume or rewind prior candle revisions.
	trade(at.Add(10*time.Second), 4)
	check("15s", at, 34)
	check("1m", at, 34)
}

func TestSynPlusDirectVolumeBeforePriceSnapshot(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_ENABLED_MARKETS", "NIFTY")
	t.Setenv("QNEXT_MARKET_CONFIG", "")
	broker := NewBroker(32, 16)
	handler := NewWebSocketHandler(broker).(*WebSocketHandler)
	at := time.Date(2026, 10, 28, 3, 45, 0, 0, time.UTC)
	broker.PublishProxyVolumeTick(domain.Tick{
		InstrumentID: "NSE:NIFTY50", Provider: "upstox-futures-proxy",
		EventTime: at.Add(time.Second), Quantity: 25,
	})
	err := handler.shadow.observe(research.SynPlusSnapshot{
		Schema:       research.SynPlusSnapshotSchema,
		InstrumentID: "QNEXT:NIFTY-SYN+",
		SnapshotAtMS: at.Add(2 * time.Second).UnixMilli(),
		Value:        25110, Quality: "GOOD",
	})
	if err != nil {
		t.Fatal(err)
	}
	bar, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", "15s")
	if !ok || bar.Volume != 25 || bar.VolumeSource != "FUTURES_PROXY" {
		t.Fatalf("early futures volume not mirrored when shadow price arrived: %+v ok=%v", bar, ok)
	}
}
