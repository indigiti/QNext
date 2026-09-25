package upstox

import (
	"context"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

type fakeHistoricalRangeFetcher struct {
	minutes []ProviderCandle
	weekly  []ProviderCandle
	monthly []ProviderCandle
}

func (f fakeHistoricalRangeFetcher) FetchRange(
	context.Context,
	string,
	string,
	time.Time,
	time.Time,
) ([]ProviderCandle, error) {
	return append([]ProviderCandle(nil), f.minutes...), nil
}

func (f fakeHistoricalRangeFetcher) FetchWeeklyRange(
	context.Context,
	string,
	string,
	time.Time,
	time.Time,
) ([]ProviderCandle, error) {
	return append([]ProviderCandle(nil), f.weekly...), nil
}

func (f fakeHistoricalRangeFetcher) FetchMonthlyRange(
	context.Context,
	string,
	string,
	time.Time,
	time.Time,
) ([]ProviderCandle, error) {
	return append([]ProviderCandle(nil), f.monthly...), nil
}

type repairResyncCollector struct {
	calls []string
}

func (r *repairResyncCollector) PublishResync(instrumentID, timeframe, reason string) {
	r.calls = append(r.calls, instrumentID+"|"+timeframe+"|"+reason)
}

func TestHistoricalRepairBuildsExpandedIntervalsAndIsIdempotent(t *testing.T) {
	ist := time.FixedZone("IST", 5*60*60+30*60)
	sessionStart := time.Date(2026, 9, 24, 9, 15, 0, 0, ist)
	minutes := make([]ProviderCandle, 0, 375)
	for i := 0; i < 375; i++ {
		price := 25000 + float64(i)/10
		minutes = append(minutes, ProviderCandle{
			OpenTime: sessionStart.Add(time.Duration(i) * time.Minute).UTC(),
			Open:     price,
			High:     price + 2,
			Low:      price - 2,
			Close:    price + 1,
			Volume:   float64(i + 1),
		})
	}

	store := history.New(t.TempDir())
	resync := &repairResyncCollector{}
	repairer := &HistoricalRepairer{
		Client: fakeHistoricalRangeFetcher{minutes: minutes},
		AccessToken: "token",
		History: store,
		Calendars: marketcalendar.DefaultRegistry(),
		Markets: []marketconfig.MarketConfig{marketconfig.DefaultMarkets()[0]},
		Resync: resync,
		Now: func() time.Time {
			return time.Date(2026, 9, 25, 16, 0, 0, 0, ist).UTC()
		},
	}

	first, err := repairer.Repair(context.Background(), HistoricalRepairRequest{
		Days: 3,
		Reason: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := first.Markets[0].Timeframes
	for _, timeframe := range []string{
		"1m", "2m", "3m", "5m", "10m", "15m", "30m", "45m",
		"1h", "2h", "3h", "4h", "1D",
	} {
		counts, ok := got[timeframe]
		if !ok || counts.Missing == 0 {
			t.Fatalf("%s should be repaired on first run: %+v", timeframe, counts)
		}
	}
	if got["4h"].Missing != 2 {
		t.Fatalf("expected two session-clamped 4h bars, got %+v", got["4h"])
	}

	second, err := repairer.Repair(context.Background(), HistoricalRepairRequest{Days: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, timeframe := range []string{"1m", "10m", "1h", "4h", "1D"} {
		counts := second.Markets[0].Timeframes[timeframe]
		if counts.Missing != 0 || counts.Corrected != 0 || counts.Unchanged == 0 {
			t.Fatalf("%s should be idempotent on second run: %+v", timeframe, counts)
		}
	}
}
