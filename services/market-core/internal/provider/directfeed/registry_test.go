package directfeed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func TestRegistryDiscoveryAndAtomicMapping(t *testing.T) {
	reg := NewRegistry()
	symbols := symbol.NewRegistry()
	cal := marketcalendar.DefaultRegistry()
	calls := 0
	adapter := func(_ context.Context, s Spec) (Binding, error) {
		calls++
		if s.ProviderKey == "FAIL" {
			return Binding{}, errors.New("discovery failed")
		}
		return Binding{
			Instrument: symbol.Instrument{ID: s.InstrumentID, Symbol: s.InstrumentID, Name: s.InstrumentID,
				AssetClass: "FUTURES", Exchange: "TEST", Currency: "USD", Timezone: "UTC",
				CalendarID: "OKX_CRYPTO_24_7", DataMode: "stream", Visible: true},
			Provider: "fixture", Key: s.ProviderKey,
			History: func(context.Context, string, time.Time, time.Time) ([]domain.Bar, error) { return nil, nil },
			Run: func(context.Context, func(domain.Tick) error, func(context.Context) error) error { return nil },
		}, nil
	}
	if err := reg.Register("fixture", adapter); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register("fixture", adapter); err == nil {
		t.Fatal("duplicate adapter accepted")
	}
	prepare := func(id, key, name string, enabled bool) error {
		_, err := reg.Prepare(context.Background(), Spec{Adapter: name, InstrumentID: id, ProviderKey: key, Enabled: enabled}, symbols, cal)
		return err
	}
	if prepare("TEST:A", "KEY", "fixture", false) == nil || prepare("TEST:A", "KEY", "missing", true) == nil || calls != 0 {
		t.Fatal("disabled or unknown adapter reached discovery")
	}
	if err := prepare("TEST:A", "KEY", "fixture", true); err != nil {
		t.Fatal(err)
	}
	if prepare("TEST:B", "KEY", "fixture", true) == nil || prepare("TEST:C", "FAIL", "fixture", true) == nil {
		t.Fatal("collision or failed discovery accepted")
	}
	if len(symbols.ListVisible()) != 1 {
		t.Fatal("failed mapping leaked a visible symbol")
	}
	if got, ok := symbols.ResolveProviderKey("fixture", "KEY"); !ok || got.ID != "TEST:A" {
		t.Fatal("existing mapping changed")
	}
	if err := prepare("TEST:B", "OTHER", "fixture", true); err != nil {
		t.Fatal(err)
	}
	if len(symbols.ListVisible()) != 2 {
		t.Fatal("second independent symbol not registered")
	}
}

func TestLoadDirectFeedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feeds.json")
	for _, tc := range []struct {
		raw string
		ok  bool
	}{
		{`{"feeds":[{"adapter":"okx-linear-swap","instrument_id":"OKX:BTC-USDT-SWAP","provider_key":"BTC-USDT-SWAP","enabled":false}]}`, true},
		{`{"feeds":[]}`, true},
		{`{"feeds":[],"unexpected":true}`, false},
		{`{"feeds":[]} {}`, false},
		{`{"feeds":[{"adapter":"okx","instrument_id":"OKX:../X"}]}`, false},
		{`{"feeds":[{"adapter":"okx","instrument_id":"OKX:X"},{"adapter":"okx","instrument_id":"OKX:X"}]}`, false},
	} {
		if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); (err == nil) != tc.ok {
			t.Fatalf("config %s: %v", tc.raw, err)
		}
	}
}
