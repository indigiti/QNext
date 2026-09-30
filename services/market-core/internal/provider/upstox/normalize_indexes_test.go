package upstox

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type indexClockCase struct {
	name         string
	instrumentID string
	providerKey  string
	exchange     string
}

func configuredIndexClockCases() []indexClockCase {
	return []indexClockCase{
		{name: "NIFTY", instrumentID: "NSE:NIFTY50", providerKey: "NSE_INDEX|Nifty 50", exchange: "NSE"},
		{name: "BANKNIFTY", instrumentID: "NSE:BANKNIFTY", providerKey: "NSE_INDEX|Nifty Bank", exchange: "NSE"},
		{name: "MIDCPNIFTY", instrumentID: "NSE:MIDCPNIFTY", providerKey: "NSE_INDEX|NIFTY MID SELECT", exchange: "NSE"},
		{name: "FINNIFTY", instrumentID: "NSE:FINNIFTY", providerKey: "NSE_INDEX|Nifty Fin Service", exchange: "NSE"},
		{name: "SENSEX", instrumentID: "BSE:SENSEX", providerKey: "BSE_INDEX|SENSEX", exchange: "BSE"},
		{name: "BANKEX", instrumentID: "BSE:BANKEX", providerKey: "BSE_INDEX|BANKEX", exchange: "BSE"},
	}
}

func testConfiguredIndexRegistry(t *testing.T) *symbol.Registry {
	t.Helper()

	registry := symbol.NewRegistry()
	for _, tc := range configuredIndexClockCases() {
		if err := registry.Register(symbol.Instrument{
			ID:         tc.instrumentID,
			Symbol:     tc.name,
			Name:       tc.name,
			AssetClass: "INDEX",
			Exchange:   tc.exchange,
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
		}); err != nil {
			t.Fatalf("register %s: %v", tc.name, err)
		}
		if err := registry.RegisterProvider(symbol.ProviderInstrument{
			Provider:     ProviderName,
			InstrumentID: tc.instrumentID,
			ProviderKey:  tc.providerKey,
		}); err != nil {
			t.Fatalf("register provider %s: %v", tc.name, err)
		}
	}
	return registry
}

func TestNormalizeConfiguredIndexesAdvanceSubMinuteCandlesWhenLTTIsFrozen(t *testing.T) {
	registry := testConfiguredIndexRegistry(t)
	normalizer, err := NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 30, 9, 15, 0, 0, time.FixedZone("IST", 5*60*60+30*60)).UTC()
	tradeMS := base.Add(2 * time.Second).UnixMilli()
	marketTimes := []time.Time{
		base.Add(3 * time.Second),
		base.Add(16 * time.Second),
		base.Add(31 * time.Second),
	}

	for _, tc := range configuredIndexClockCases() {
		t.Run(tc.name, func(t *testing.T) {
			engine := candle.New("test-" + tc.name + "-market-clock")
			sequence := uint64(0)
			nextSeq := func() uint64 {
				sequence++
				return sequence
			}

			var opens15 []int64
			var opens30 []int64
			for i, marketAt := range marketTimes {
				envelope := DecodedEnvelope{
					Type:      "live_feed",
					CurrentTS: formatMillis(marketAt),
					Feeds: map[string]Feed{
						tc.providerKey: {
							LTPC: &LTPC{
								LTP: 10000 + float64(i),
								LTT: formatMillis(time.UnixMilli(tradeMS)),
							},
						},
					},
				}

				ticks, normalizeErr := normalizer.NormalizeEnvelope(
					envelope,
					marketAt.Add(time.Millisecond),
					nextSeq,
				)
				if normalizeErr != nil {
					t.Fatal(normalizeErr)
				}
				if len(ticks) != 1 {
					t.Fatalf("expected one tick, got %d", len(ticks))
				}

				tick := ticks[0]
				if tick.InstrumentID != tc.instrumentID {
					t.Fatalf("unexpected instrument: %s", tick.InstrumentID)
				}
				if tick.EventTime.UnixMilli() != marketAt.UnixMilli() {
					t.Fatalf("event time did not use provider market clock: %v", tick.EventTime)
				}
				if tick.TradeTime.UnixMilli() != tradeMS {
					t.Fatalf("trade time changed while LTT was frozen: %v", tick.TradeTime)
				}

				bars15, candleErr := engine.Apply(tick, "15s")
				if candleErr != nil {
					t.Fatal(candleErr)
				}
				opens15 = append(opens15, bars15[len(bars15)-1].OpenTime.UnixMilli())

				bars30, candleErr := engine.Apply(tick, "30s")
				if candleErr != nil {
					t.Fatal(candleErr)
				}
				opens30 = append(opens30, bars30[len(bars30)-1].OpenTime.UnixMilli())
			}

			if opens15[0] == opens15[1] || opens15[1] == opens15[2] {
				t.Fatalf("15s candle did not advance across market-time buckets: %v", opens15)
			}
			if opens30[0] != opens30[1] {
				t.Fatalf("30s candle advanced too early: %v", opens30)
			}
			if opens30[0] == opens30[2] {
				t.Fatalf("30s candle did not advance across market-time bucket: %v", opens30)
			}
		})
	}
}
