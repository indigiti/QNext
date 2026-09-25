package upstox

import (
	"context"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type fakeIntradayFetcher struct {
	candles []ProviderCandle
}

func (f fakeIntradayFetcher) Fetch(context.Context, string, string, string) ([]ProviderCandle, error) {
	return f.candles, nil
}

type recoveredCollector struct {
	bars []domain.Bar
}

func (c *recoveredCollector) AppendBar(bar domain.Bar) error {
	c.bars = append(c.bars, bar)
	return nil
}

func TestIntradayRecoveryPersistsOnlyClosedGapBars(t *testing.T) {
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID: "NSE:NIFTY50", Symbol: "NIFTY", Name: "Nifty 50",
		AssetClass: "INDEX", Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider: ProviderName, InstrumentID: "NSE:NIFTY50", ProviderKey: "NSE_INDEX|Nifty 50",
	}); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	collector := &recoveredCollector{}
	recovery := &IntradayRecovery{
		Client: fakeIntradayFetcher{candles: []ProviderCandle{
			{OpenTime: base, Open: 25100, High: 25102, Low: 25099, Close: 25101, Volume: 100},
			{OpenTime: base.Add(time.Minute), Open: 25101, High: 25103, Low: 25100, Close: 25102, Volume: 200},
		}},
		AccessToken: "token",
		Registry:    registry,
		History:     collector,
		Timeframes:  []string{"1m"},
	}

	err := recovery.Recover(context.Background(), RecoveryRequest{
		Provider:       ProviderName,
		InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		From:           base.Add(-time.Second),
		To:             base.Add(90 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(collector.bars) != 1 {
		t.Fatalf("expected only fully closed bar, got %+v", collector.bars)
	}
	bar := collector.bars[0]
	if !bar.Recovered || bar.Quality != domain.QualityRecovered || bar.Close != 25101 {
		t.Fatalf("unexpected recovered bar: %+v", bar)
	}
}

type resyncCall struct {
	instrumentID string
	timeframe    string
	reason       string
}

type fakeResyncPublisher struct {
	calls []resyncCall
}

func (p *fakeResyncPublisher) PublishResync(instrumentID, timeframe, reason string) {
	p.calls = append(p.calls, resyncCall{
		instrumentID: instrumentID,
		timeframe:    timeframe,
		reason:       reason,
	})
}

func TestIntradayRecoveryUsesInstrumentCursorAndSignalsResync(t *testing.T) {
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID: "NSE:NIFTY50", Symbol: "NIFTY", Name: "Nifty 50",
		AssetClass: "INDEX", Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider: ProviderName, InstrumentID: "NSE:NIFTY50", ProviderKey: "NSE_INDEX|Nifty 50",
	}); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	collector := &recoveredCollector{}
	resync := &fakeResyncPublisher{}
	status := NewGapRecoveryTracker([]string{"1m"}, []string{"15s", "30s"})
	recovery := &IntradayRecovery{
		Client: fakeIntradayFetcher{candles: []ProviderCandle{
			{OpenTime: base, Open: 25100, High: 25102, Low: 25099, Close: 25101, Volume: 100},
			{OpenTime: base.Add(time.Minute), Open: 25101, High: 25103, Low: 25100, Close: 25102, Volume: 200},
		}},
		AccessToken: "token",
		Registry:    registry,
		History:     collector,
		Timeframes:  []string{"1m"},
		Resync:      resync,
		Status:      status,
	}

	err := recovery.Recover(context.Background(), RecoveryRequest{
		Provider:       ProviderName,
		InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		From:           base.Add(-10 * time.Minute),
		FromByInstrumentID: map[string]time.Time{
			"NSE:NIFTY50": base.Add(30 * time.Second),
		},
		To:    base.Add(150 * time.Second),
		Cause: "watchdog",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(collector.bars) != 1 || !collector.bars[0].OpenTime.Equal(base.Add(time.Minute)) {
		t.Fatalf("per-instrument cursor not applied: %+v", collector.bars)
	}
	if len(resync.calls) != 1 {
		t.Fatalf("expected one resync, got %+v", resync.calls)
	}
	if call := resync.calls[0]; call.instrumentID != "NSE:NIFTY50" || call.timeframe != "1m" || call.reason != "provider_gap_recovered" {
		t.Fatalf("unexpected resync: %+v", call)
	}
	snapshot := status.Snapshot()
	if snapshot.Attempts != 1 || snapshot.Successes != 1 || snapshot.Failures != 0 || snapshot.RecoveredBars != 1 {
		t.Fatalf("unexpected recovery status: %+v", snapshot)
	}
	if snapshot.ExactTickReplay {
		t.Fatal("Upstox candle recovery must not claim exact tick replay")
	}
}
