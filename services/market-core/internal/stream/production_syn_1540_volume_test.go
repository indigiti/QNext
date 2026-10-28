package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestProductionSYNVolumeContinuesAfterCashCloses(t *testing.T) {
	b := NewBroker(64, 16)
	b.ConfigureSyntheticVolume(map[string]string{
		"QNEXT:NIFTY-SYN": "NSE:NIFTY50",
		"QNEXT:BANKNIFTY-SYN": "NSE:BANKNIFTY",
	})
	at, _ := time.Parse(time.RFC3339, "2026-10-28T15:35:05+05:30")
	syn := domain.Bar{
		InstrumentID: "QNEXT:NIFTY-SYN", Timeframe: "15s",
		OpenTime: at.Add(-5 * time.Second), CloseTime: at.Add(10 * time.Second),
		Open: 25100, High: 25105, Low: 25099, Close: 25102,
	}
	b.PublishBar(syn)
	b.PublishProxyVolumeTick(domain.Tick{
		InstrumentID: "NSE:NIFTY50", Provider: "upstox-futures-proxy",
		EventTime: at, Quantity: 40,
	})
	b.PublishProxyVolumeTick(domain.Tick{
		InstrumentID: "NSE:NIFTY50", Provider: "upstox-futures-proxy",
		EventTime: at.Add(2 * time.Second), Quantity: 15,
	})
	got, ok := b.LatestBar(syn.InstrumentID, syn.Timeframe)
	if !ok || got.Volume != 55 || got.VolumeStatus != "OBSERVED" ||
		got.Open != syn.Open || got.Close != syn.Close ||
		got.Revision < 2 {
		t.Fatalf("postclose SYN proxy volume was not delivered: %+v ok=%v", got, ok)
	}
	// A new synthetic price update must retain the previously mirrored proxy.
	syn.Close = 25104
	b.PublishBar(syn)
	got, ok = b.LatestBar(syn.InstrumentID, syn.Timeframe)
	if !ok || got.Volume != 55 || got.Close != 25104 {
		t.Fatalf("SYN OHLC or proxy volume was overwritten: %+v", got)
	}
	// No cash-index price is fabricated by the proxy update.
	if _, ok := b.LatestBar("NSE:NIFTY50", "15s"); ok {
		t.Fatal("direct futures volume created a synthetic cash-index price bar")
	}
	b.PublishBar(domain.Bar{
		InstrumentID: "QNEXT:BANKNIFTY-SYN", Timeframe: "15s",
		OpenTime: syn.OpenTime, CloseTime: syn.CloseTime,
		Open: 55000, Close: 55000,
	})
	bank, ok := b.LatestBar("QNEXT:BANKNIFTY-SYN", "15s")
	if !ok || bank.Volume != 0 {
		t.Fatalf("NIFTY futures volume leaked into BANKNIFTY: %+v", bank)
	}
}
