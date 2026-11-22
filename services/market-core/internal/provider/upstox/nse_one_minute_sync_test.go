package upstox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

type nseMinuteTestFetcher struct {
	candles map[string][]ProviderCandle
	fail    bool
	calls   int
}

func (f *nseMinuteTestFetcher) FetchRange(_ context.Context, _, key string, _, _ time.Time) ([]ProviderCandle, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("provider unavailable")
	}
	return append([]ProviderCandle(nil), f.candles[key]...), nil
}

type nseMinuteTestHistory struct{ bars map[string][]domain.Bar }

func (h *nseMinuteTestHistory) AppendBar(bar domain.Bar) error {
	if h.bars == nil {
		h.bars = make(map[string][]domain.Bar)
	}
	key := bar.InstrumentID + "|" + bar.Timeframe
	h.bars[key] = append(h.bars[key], bar)
	return nil
}
func (h *nseMinuteTestHistory) LoadRange(id, tf string, from, to time.Time) ([]domain.Bar, error) {
	latest := make(map[int64]domain.Bar)
	for _, bar := range h.bars[id+"|"+tf] {
		if bar.OpenTime.Before(from) || !bar.OpenTime.Before(to) {
			continue
		}
		if prev, exists := latest[bar.OpenTime.UnixMilli()]; !exists || bar.Revision >= prev.Revision {
			latest[bar.OpenTime.UnixMilli()] = bar
		}
	}
	out := make([]domain.Bar, 0, len(latest))
	for _, bar := range latest {
		out = append(out, bar)
	}
	return out, nil
}

type nseMinuteTestResync struct{ ids []string }

func (p *nseMinuteTestResync) PublishResync(id, tf, reason string) {
	p.ids = append(p.ids, id+"|"+tf+"|"+reason)
}

func TestNSEOneMinutePollPreservesEquityVolumeAndIndexPriceOnly(t *testing.T) {
	ist := time.FixedZone("IST", 5*60*60+30*60)
	now := time.Date(2026, 11, 20, 11, 20, 0, 0, ist).UTC()
	minute := time.Date(2026, 11, 20, 10, 0, 0, 0, ist).UTC()
	late := now.Add(-20 * time.Second)
	fetch := &nseMinuteTestFetcher{candles: map[string][]ProviderCandle{
		"NSE_EQ|INE002A01018": {
			{OpenTime: minute, Open: 100, High: 104, Low: 99, Close: 103, Volume: 800},
			{OpenTime: late, Open: 110, High: 110, Low: 110, Close: 110, Volume: 1},
		},
		"NSE_INDEX|Nifty 100": {
			{OpenTime: minute, Open: 25000, High: 25050, Low: 24990, Close: 25030, Volume: 123456},
		},
	}}
	history := &nseMinuteTestHistory{}
	resync := &nseMinuteTestResync{}
	syncer := &NSEMinuteSync{
		Client: fetch, History: history, Resync: resync, AccessToken: "test-token",
		Now: func() time.Time { return now },
		Symbols: []marketconfig.NSEMinuteSymbol{
			{Symbol: "RELIANCE", Name: "Reliance Industries", ProviderKey: "NSE_EQ|INE002A01018"},
			{Symbol: "NIFTY100", Name: "Nifty 100", ProviderKey: "NSE_INDEX|Nifty 100"},
		},
	}
	if err := syncer.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	stock, _ := history.LoadRange("NSE:RELIANCE", "1m", minute.Add(-time.Minute), now)
	index, _ := history.LoadRange("NSE:NIFTY100", "1m", minute.Add(-time.Minute), now)
	if len(stock) != 1 || !stock[0].Final || stock[0].Volume != 800 ||
		stock[0].VolumeStatus != "OBSERVED" || stock[0].VolumeSource != "UPSTOX_EQUITY_HISTORICAL" {
		t.Fatalf("invalid stock historical minute: %+v", stock)
	}
	if len(index) != 1 || index[0].Volume != 0 || index[0].VolumeStatus != "UNKNOWN" {
		t.Fatalf("provider index volume wrongly treated as tradable: %+v", index)
	}
	if len(resync.ids) != 2 {
		t.Fatalf("both symbols must resync once: %+v", resync.ids)
	}

	if err := syncer.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(resync.ids) != 2 || syncer.Snapshot().Symbols[0].Unchanged != 1 {
		t.Fatalf("identical history caused duplicate revisions: %+v", syncer.Snapshot())
	}

	fetch.candles["NSE_EQ|INE002A01018"][0].Volume = 860
	if err := syncer.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	stock, _ = history.LoadRange("NSE:RELIANCE", "1m", minute.Add(-time.Minute), now)
	if len(stock) != 1 || stock[0].Revision != 1 || stock[0].Volume != 860 ||
		syncer.Snapshot().Symbols[0].Corrected != 1 || len(resync.ids) != 3 {
		t.Fatalf("historical correction did not update canonical minute: %+v / %+v", stock, syncer.Snapshot())
	}
}

func TestNSEMinutePollingFailureIsVisibleWithoutInventingCandles(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	fetch := &nseMinuteTestFetcher{fail: true}
	history := &nseMinuteTestHistory{}
	syncer := &NSEMinuteSync{
		Client: fetch, History: history, AccessToken: "test-token",
		Now: func() time.Time { return now },
		Symbols: []marketconfig.NSEMinuteSymbol{{
			Symbol: "RELIANCE", Name: "Reliance", ProviderKey: "NSE_EQ|INE002A01018",
		}},
	}
	err := syncer.SyncOnce(context.Background())
	if err == nil {
		t.Fatal("failed provider fetch was reported as success")
	}
	snap := syncer.Snapshot()
	if snap.Enabled != true || snap.Mode != "historical_polling_not_live_ticks" ||
		len(snap.Symbols) != 1 || snap.Symbols[0].LastError == "" ||
		snap.Symbols[0].LastSuccessMS != 0 || snap.Running {
		t.Fatalf("provider failure is invisible: %+v", snap)
	}
	if len(history.bars) != 0 {
		t.Fatal("failed provider fetch invented historical data")
	}
}

func TestNSEMinutePollingRequiresPreselectedInstruments(t *testing.T) {
	syncer := &NSEMinuteSync{Client: &nseMinuteTestFetcher{}, History: &nseMinuteTestHistory{}, AccessToken: "a"}
	if err := syncer.SyncOnce(context.Background()); err == nil {
		t.Fatal("unbounded NSE history syncing accepted an empty watchlist")
	}
	syncer.Symbols = make([]marketconfig.NSEMinuteSymbol, 13)
	if err := syncer.SyncOnce(context.Background()); err == nil {
		t.Fatal("unbounded NSE history syncing accepted more than 12 instruments")
	}
}
