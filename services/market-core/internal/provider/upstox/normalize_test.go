package upstox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func testNiftyRegistry(t *testing.T) *symbol.Registry {
	t.Helper()
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
	return registry
}

func TestNormalizeLTPCEnvelope(t *testing.T) {
	registry := testNiftyRegistry(t)

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
	if tick.Quantity != 75 {
		t.Fatalf("unexpected last-traded quantity: %+v", tick)
	}
	if tick.Sequence != 41 {
		t.Fatalf("unexpected sequence: %d", tick.Sequence)
	}
	if tick.EventTime.UnixMilli() != 1740729566039 {
		t.Fatalf("unexpected market event time: %v", tick.EventTime)
	}
	if tick.TradeTime.UnixMilli() != 1740729552723 {
		t.Fatalf("unexpected trade time: %v", tick.TradeTime)
	}
	if tick.ReceivedTime.UnixMilli() != 1740729566039 {
		t.Fatalf("unexpected received time: %v", tick.ReceivedTime)
	}
}

func TestNormalizeAdvancesSubMinuteCandlesWhenLTTIsFrozen(t *testing.T) {
	registry := testNiftyRegistry(t)
	normalizer, err := NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}
	engine := candle.New("test-market-clock")
	sequence := uint64(0)
	nextSeq := func() uint64 {
		sequence++
		return sequence
	}

	base := time.Date(2026, 9, 30, 9, 15, 0, 0, time.FixedZone("IST", 5*60*60+30*60)).UTC()
	tradeMS := base.Add(2 * time.Second).UnixMilli()
	marketTimes := []time.Time{
		base.Add(3 * time.Second),
		base.Add(16 * time.Second),
		base.Add(31 * time.Second),
	}

	var opens15 []int64
	var opens30 []int64
	for i, marketAt := range marketTimes {
		envelope := DecodedEnvelope{
			Type:      "live_feed",
			CurrentTS: formatMillis(marketAt),
			Feeds: map[string]Feed{
				"NSE_INDEX|Nifty 50": {
					LTPC: &LTPC{
						LTP: 22000 + float64(i),
						LTT: formatMillis(time.UnixMilli(tradeMS)),
					},
			},
		}
		ticks, normalizeErr := normalizer.NormalizeEnvelope(envelope, marketAt.Add(time.Millisecond), nextSeq)
		if normalizeErr != nil {
			t.Fatal(normalizeErr)
		}
		if len(ticks) != 1 {
			t.Fatalf("expected one tick, got %d", len(ticks))
		}
		if ticks[0].TradeTime.UnixMilli() != tradeMS {
			t.Fatalf("trade time changed while LTT was frozen: %v", ticks[0].TradeTime)
		}

		bars15, candleErr := engine.Apply(ticks[0], "15s")
		if candleErr != nil {
			t.Fatal(candleErr)
		}
		opens15 = append(opens15, bars15[len(bars15)-1].OpenTime.UnixMilli())

		bars30, candleErr := engine.Apply(ticks[0], "30s")
		if candleErr != nil {
			t.Fatal(candleErr)
		}
		opens30 = append(opens30, bars30[len(bars30)-1].OpenTime.UnixMilli())
	}

	if opens15[0] == opens15[1] || opens15[1] == opens15[2] {
		t.Fatalf("15s candle did not advance across provider market-time buckets: %v", opens15)
	}
	if opens30[0] == opens30[2] {
		t.Fatalf("30s candle did not advance across provider market-time bucket: %v", opens30)
	}
	if opens30[0] != opens30[1] {
		t.Fatalf("30s candle advanced too early: %v", opens30)
	}
}

func formatMillis(at time.Time) string {
	return time.UnixMilli(at.UnixMilli()).UTC().Format("150405.000")[:0] + millisString(at.UnixMilli())
}

func millisString(value int64) string {
	return fmt.Sprintf("%d", value)
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
		time.UnixMilli(1740729566045).UTC(),
		func() uint64 { return 1 },
	)
	if err == nil {
		t.Fatal("expected unknown provider key to fail")
	}
}
