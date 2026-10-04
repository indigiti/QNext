package upstox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

func testFutureRegistry(t *testing.T) *symbol.Registry {
	t.Helper()
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID:         "NSE_FO:NIFTY-FUT",
		Symbol:     "NIFTY-FUT",
		Name:       "Nifty Futures",
		AssetClass: "FUTURE",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider:     ProviderName,
		InstrumentID: "NSE_FO:NIFTY-FUT",
		ProviderKey:  "NSE_FO|99999",
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

func TestNormalizeUsesPositiveVTTDeltaForVolume(t *testing.T) {
	registry := testFutureRegistry(t)
	normalizer, err := NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 5, 9, 15, 0, 0, time.FixedZone("IST", 5*60*60+30*60)).UTC()
	sequence := uint64(0)
	nextSeq := func() uint64 {
		sequence++
		return sequence
	}

	cases := []struct {
		name string
		at   time.Time
		vtt  int64
		want float64
	}{
		{name: "baseline", at: base.Add(time.Second), vtt: 1000, want: 0},
		{name: "positive delta", at: base.Add(2 * time.Second), vtt: 1025, want: 25},
		{name: "duplicate cumulative volume", at: base.Add(3 * time.Second), vtt: 1025, want: 0},
		{name: "provider reset", at: base.Add(4 * time.Second), vtt: 5, want: 0},
		{name: "delta after reset", at: base.Add(5 * time.Second), vtt: 12, want: 7},
	}

	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ltpc := &LTPC{
				LTP: 22000 + float64(index),
				LTT: formatMillis(tc.at),
				LTQ: "999",
			}
			envelope := DecodedEnvelope{
				Type:      "live_feed",
				CurrentTS: formatMillis(tc.at),
				Feeds: map[string]Feed{
					"NSE_FO|99999": {
						LTPC:     ltpc,
						FullFeed: &MarketState{LTPC: cloneLTPC(ltpc), VTT: tc.vtt},
					},
				},
			}
			ticks, normalizeErr := normalizer.NormalizeEnvelope(
				envelope,
				tc.at.Add(time.Millisecond),
				nextSeq,
			)
			if normalizeErr != nil {
				t.Fatal(normalizeErr)
			}
			if len(ticks) != 1 {
				t.Fatalf("expected one tick, got %d", len(ticks))
			}
			if ticks[0].Quantity != tc.want {
				t.Fatalf("unexpected VTT-derived quantity: got %v want %v", ticks[0].Quantity, tc.want)
			}
		})
	}
}

func TestNormalizeResetsVTTBaselineAcrossSessionDate(t *testing.T) {
	registry := testFutureRegistry(t)
	normalizer, err := NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}

	location := time.FixedZone("IST", 5*60*60+30*60)
	first := time.Date(2026, 9, 5, 15, 29, 0, 0, location).UTC()
	second := time.Date(2026, 9, 6, 9, 15, 1, 0, location).UTC()
	sequence := uint64(0)
	nextSeq := func() uint64 {
		sequence++
		return sequence
	}

	normalize := func(at time.Time, vtt int64) float64 {
		ltpc := &LTPC{LTP: 22000, LTT: formatMillis(at), LTQ: "10"}
		ticks, normalizeErr := normalizer.NormalizeEnvelope(
			DecodedEnvelope{
				Type:      "live_feed",
				CurrentTS: formatMillis(at),
				Feeds: map[string]Feed{
					"NSE_FO|99999": {
						LTPC:     ltpc,
						FullFeed: &MarketState{LTPC: cloneLTPC(ltpc), VTT: vtt},
					},
				},
			},
			at.Add(time.Millisecond),
			nextSeq,
		)
		if normalizeErr != nil {
			t.Fatal(normalizeErr)
		}
		return ticks[0].Quantity
	}

	if got := normalize(first, 50000); got != 0 {
		t.Fatalf("first session baseline should not emit volume: %v", got)
	}
	if got := normalize(first.Add(time.Second), 50020); got != 20 {
		t.Fatalf("unexpected first-session delta: %v", got)
	}
	if got := normalize(second, 100); got != 0 {
		t.Fatalf("new session must establish a fresh baseline: %v", got)
	}
}

func TestNormalizeDeduplicatesLTQFallbackByTradeTime(t *testing.T) {
	registry := testFutureRegistry(t)
	normalizer, err := NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 5, 9, 15, 0, 0, time.FixedZone("IST", 5*60*60+30*60)).UTC()
	tradeAt := base.Add(time.Second)
	sequence := uint64(0)
	nextSeq := func() uint64 {
		sequence++
		return sequence
	}

	normalize := func(marketAt, lastTradeAt time.Time, ltq string) float64 {
		ticks, normalizeErr := normalizer.NormalizeEnvelope(
			DecodedEnvelope{
				Type:      "live_feed",
				CurrentTS: formatMillis(marketAt),
				Feeds: map[string]Feed{
					"NSE_FO|99999": {
						LTPC: &LTPC{
							LTP: 22000,
							LTT: formatMillis(lastTradeAt),
							LTQ: ltq,
						},
					},
				},
			},
			marketAt.Add(time.Millisecond),
			nextSeq,
		)
		if normalizeErr != nil {
			t.Fatal(normalizeErr)
		}
		return ticks[0].Quantity
	}

	if got := normalize(base.Add(2*time.Second), tradeAt, "25"); got != 25 {
		t.Fatalf("first LTQ observation should count once: %v", got)
	}
	if got := normalize(base.Add(3*time.Second), tradeAt, "25"); got != 0 {
		t.Fatalf("repeated LTT/LTQ must not double-count volume: %v", got)
	}
	if got := normalize(base.Add(4*time.Second), tradeAt.Add(time.Second), "30"); got != 30 {
		t.Fatalf("new trade timestamp should count LTQ: %v", got)
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
	return strconv.FormatInt(at.UnixMilli(), 10)
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
