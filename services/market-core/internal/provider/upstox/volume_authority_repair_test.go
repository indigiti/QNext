package upstox

import (
	"context"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func TestIndexHistoryRepairProtectsAuthoritativeFuturesProxyVolume(t *testing.T) {
	store := history.New(t.TempDir())
	at := time.Date(2026, 10, 22, 3, 45, 0, 0, time.UTC)
	original := domain.Bar{
		InstrumentID: "NSE:NIFTY50", Timeframe: "1m", OpenTime: at, CloseTime: at.Add(time.Minute),
		Open: 25200, High: 25210, Low: 25190, Close: 25203, Volume: 430, Final: true, Quality: domain.QualityGood,
	}
	if err := store.AppendBar(original); err != nil {
		t.Fatal(err)
	}
	cashSource := original
	cashSource.Close = 25204
	cashSource.Volume = 0 // Cash-index history has no traded futures volume.
	repairer := &HistoricalRepairer{History: store}
	counts, changed, err := repairer.repairIndexSourceBars(
		at.Add(-time.Minute), at.Add(2*time.Minute), []domain.Bar{cashSource},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || counts.Corrected != 1 {
		t.Fatalf("index price correction was not applied: %+v", counts)
	}
	revised, err := store.LoadRange("NSE:NIFTY50", "1m", at, at.Add(time.Minute))
	if err != nil || len(revised) != 1 {
		t.Fatalf("cannot load revised bar: %v %+v", err, revised)
	}
	if revised[0].Volume != 430 || revised[0].Close != 25204 || revised[0].Revision != 1 {
		t.Fatalf("historical index repair erased authoritative proxy volume: %+v", revised[0])
	}

	// Re-running the same provider correction should not create another
	// revision or accidentally erase proxy volume.
	counts, changed, err = repairer.repairIndexSourceBars(
		at.Add(-time.Minute), at.Add(2*time.Minute), []domain.Bar{cashSource},
	)
	if err != nil || changed || counts.Unchanged != 1 {
		t.Fatalf("index repair not idempotent: %+v changed=%v err=%v", counts, changed, err)
	}
}

func TestIndexHistoryRepairDoesNotTreatProviderVolumeAsTradedIndexVolume(t *testing.T) {
	store := history.New(t.TempDir())
	at := time.Date(2026, 10, 22, 3, 45, 0, 0, time.UTC)
	source := domain.Bar{
		InstrumentID: "NSE:NIFTY50", Timeframe: "1m", OpenTime: at, CloseTime: at.Add(time.Minute),
		Open: 25200, High: 25210, Low: 25190, Close: 25203, Volume: 999, Final: true,
	}
	repairer := &HistoricalRepairer{History: store}
	_, _, err := repairer.repairIndexSourceBars(at.Add(-time.Minute), at.Add(2*time.Minute), []domain.Bar{source})
	if err != nil {
		t.Fatal(err)
	}
	out, err := store.LoadRange(source.InstrumentID, "1m", at, at.Add(time.Minute))
	if err != nil || len(out) != 1 || out[0].Volume != 0 {
		t.Fatalf("provider index history incorrectly labelled as native volume: %+v err=%v", out, err)
	}

	tradable := source
	tradable.InstrumentID = "NSE:FUT:NIFTY:2026-10-29"
	_, _, err = repairer.repairBars(at.Add(-time.Minute), at.Add(2*time.Minute), []domain.Bar{tradable})
	if err != nil {
		t.Fatal(err)
	}
	out, err = store.LoadRange(tradable.InstrumentID, "1m", at, at.Add(time.Minute))
	if err != nil || len(out) != 1 || out[0].Volume != 999 {
		t.Fatalf("legitimate tradable volume was lost: %+v err=%v", out, err)
	}
}

func TestIntradayIndexRecoveryPreservesProxyButCorrectsPrice(t *testing.T) {
	at := time.Date(2026, 10, 22, 3, 45, 0, 0, time.UTC)
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID: "NSE:NIFTY50", Symbol: "NIFTY", Name: "Nifty 50", AssetClass: "INDEX",
		Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider: ProviderName, InstrumentID: "NSE:NIFTY50", ProviderKey: "NSE_INDEX|Nifty 50",
	}); err != nil {
		t.Fatal(err)
	}
	store := history.New(t.TempDir())
	if err := store.AppendBar(domain.Bar{
		InstrumentID: "NSE:NIFTY50", Timeframe: "1m", OpenTime: at, CloseTime: at.Add(time.Minute),
		Open: 25100, High: 25103, Low: 25099, Close: 25101, Volume: 125, Final: true,
	}); err != nil {
		t.Fatal(err)
	}
	recovery := &IntradayRecovery{
		Client: fakeIntradayFetcher{candles: []ProviderCandle{{
			OpenTime: at, Open: 25100, High: 25103, Low: 25099, Close: 25102, Volume: 0,
		}}},
		AccessToken: "token", Registry: registry, History: store, Timeframes: []string{"1m"},
	}
	if err := recovery.Recover(context.Background(), RecoveryRequest{
		Provider:       ProviderName,
		InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		From:           at.Add(-time.Second), To: at.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	out, err := store.LoadRange("NSE:NIFTY50", "1m", at, at.Add(time.Minute))
	if err != nil || len(out) != 1 || out[0].Close != 25102 || out[0].Volume != 125 {
		t.Fatalf("intraday source repair erased futures volume: %+v err=%v", out, err)
	}
}
