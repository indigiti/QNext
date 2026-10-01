package upstox

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type recoveryMemoryHistory struct {
	bars []domain.Bar
}

func (h *recoveryMemoryHistory) AppendBar(bar domain.Bar) error {
	h.bars = append(h.bars, bar)
	return nil
}

func (h *recoveryMemoryHistory) LoadRange(
	instrumentID string,
	timeframe string,
	from time.Time,
	to time.Time,
) ([]domain.Bar, error) {
	latest := map[int64]domain.Bar{}
	for _, bar := range h.bars {
		if bar.InstrumentID != instrumentID || bar.Timeframe != timeframe {
			continue
		}
		if bar.OpenTime.Before(from) || bar.OpenTime.After(to) {
			continue
		}
		key := bar.OpenTime.UnixMilli()
		previous, ok := latest[key]
		if !ok || bar.Revision >= previous.Revision {
			latest[key] = bar
		}
	}
	result := make([]domain.Bar, 0, len(latest))
	for _, bar := range latest {
		result = append(result, bar)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].OpenTime.Before(result[j].OpenTime)
	})
	return result, nil
}

func TestIntradayRecoveryRebuildsDerivedBarsFromRepairedCanonicalMinute(t *testing.T) {
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID:         "NSE:NIFTY50",
		Symbol:     "NIFTY",
		Name:       "Nifty 50",
		AssetClass: "INDEX",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
		CalendarID: "NSE_EQ",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider: ProviderName, InstrumentID: "NSE:NIFTY50", ProviderKey: "NSE_INDEX|Nifty 50",
	}); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC) // NSE regular open 09:15 IST.
	history := &recoveryMemoryHistory{}
	resync := &fakeResyncPublisher{}
	status := NewGapRecoveryTracker(
		[]string{"1m"},
		[]string{"15s", "30s", "3m", "5m"},
	)
	recovery := &IntradayRecovery{
		Client: fakeIntradayFetcher{candles: []ProviderCandle{
			{OpenTime: base, Open: 25100, High: 25102, Low: 25099, Close: 25101, Volume: 100},
			{OpenTime: base.Add(time.Minute), Open: 25101, High: 25104, Low: 25100, Close: 25103, Volume: 200},
			{OpenTime: base.Add(2 * time.Minute), Open: 25103, High: 25105, Low: 25102, Close: 25104, Volume: 300},
		}},
		AccessToken: "token",
		Registry:    registry,
		History:     history,
		Calendars:   marketcalendar.DefaultRegistry(),
		Timeframes:  []string{"1m"},
		Resync:      resync,
		Status:      status,
	}

	if err := recovery.Recover(context.Background(), RecoveryRequest{
		Provider:       ProviderName,
		InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		From:           base.Add(-time.Second),
		To:             base.Add(3 * time.Minute),
		Cause:          "disconnect",
	}); err != nil {
		t.Fatal(err)
	}

	oneMinute, err := history.LoadRange("NSE:NIFTY50", "1m", base, base.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(oneMinute) != 3 {
		t.Fatalf("expected 3 repaired canonical minutes, got %+v", oneMinute)
	}
	threeMinute, err := history.LoadRange("NSE:NIFTY50", "3m", base, base.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(threeMinute) != 1 {
		t.Fatalf("expected one rebuilt 3m candle, got %+v", threeMinute)
	}
	bar := threeMinute[0]
	if bar.Open != 25100 || bar.High != 25105 || bar.Low != 25099 || bar.Close != 25104 || bar.Volume != 600 {
		t.Fatalf("unexpected rebuilt 3m candle: %+v", bar)
	}
	if !bar.Recovered || !bar.Final || bar.Quality != domain.QualityRecovered {
		t.Fatalf("rebuilt 3m candle must be final recovered history: %+v", bar)
	}

	fiveMinute, err := history.LoadRange("NSE:NIFTY50", "5m", base, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(fiveMinute) != 0 {
		t.Fatalf("incomplete 5m bucket must not be fabricated: %+v", fiveMinute)
	}

	seen := map[string]bool{}
	for _, call := range resync.calls {
		seen[call.timeframe] = true
	}
	if !seen["1m"] || !seen["3m"] {
		t.Fatalf("expected 1m and 3m resync after repair, got %+v", resync.calls)
	}
	if seen["15s"] || seen["30s"] || seen["5m"] {
		t.Fatalf("must not resync unrecovered streams: %+v", resync.calls)
	}

	snapshot := status.Snapshot()
	if snapshot.RecoveredBars != 4 {
		t.Fatalf("expected 3 canonical + 1 derived recovered bars, got %+v", snapshot)
	}
	if !timeframeListed(snapshot.RecoveredTimeframes, "3m") || !timeframeListed(snapshot.RecoveredTimeframes, "5m") {
		t.Fatalf("derived minute rollups should be classified recoverable: %+v", snapshot)
	}
	if !timeframeListed(snapshot.NonExactTimeframes, "15s") || !timeframeListed(snapshot.NonExactTimeframes, "30s") {
		t.Fatalf("seconds must remain explicitly non-exact: %+v", snapshot)
	}
}
