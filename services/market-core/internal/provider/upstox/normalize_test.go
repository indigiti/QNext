package upstox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func TestNormalizeLTPCEnvelope(t *testing.T) {
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID:         "NSE:NIFTY50",
		Symbol:     "NIFTY",
		Name:       "Nifty 50",
		AssetClass: "INDEX",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider:     ProviderName,
		InstrumentID: "NSE:NIFTY50",
		ProviderKey:  "NSE_INDEX|Nifty 50",
	}); err != nil {
		t.Fatal(err)
	}

	normalizer, err := NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "ltpc_nifty_v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope DecodedEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}

	sequence := uint64(40)
	ticks, err := normalizer.NormalizeEnvelope(
		envelope,
		time.UnixMilli(1740729566045).UTC(),
		func() uint64 {
			sequence++
			return sequence
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(ticks) != 1 {
		t.Fatalf("expected one tick, got %d", len(ticks))
	}

	tick := ticks[0]
	if tick.InstrumentID != "NSE:NIFTY50" || tick.Provider != ProviderName {
		t.Fatalf("unexpected identity: %+v", tick)
	}
	if tick.Price != 219.3 {
		t.Fatalf("unexpected price: %+v", tick)
	}
	if tick.Sequence != 41 {
		t.Fatalf("unexpected sequence: %d", tick.Sequence)
	}
	if tick.EventTime.UnixMilli() != 1740729552723 {
		t.Fatalf("unexpected event time: %v", tick.EventTime)
	}
	if tick.ReceivedTime.UnixMilli() != 1740729566039 {
		t.Fatalf("unexpected received time: %v", tick.ReceivedTime)
	}
}

func TestNormalizeRejectsUnknownProviderKey(t *testing.T) {
	registry := symbol.NewRegistry()
	normalizer, err := NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}

	_, err = normalizer.NormalizeEnvelope(
		DecodedEnvelope{
			Type:      "live_feed",
			CurrentTS: "1740729566039",
			Feeds: map[string]Feed{
				"UNKNOWN": {LTPC: &LTPC{LTP: 1, LTT: "1740729552723"}},
			},
		},
		time.UnixMilli(1740729566045).UTC(),
		func() uint64 { return 1 },
	)
	if err == nil {
		t.Fatal("expected unknown provider key to fail")
	}
}
