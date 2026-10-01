package upstox

import (
	"math"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func testOptionRegistry(t *testing.T) *symbol.Registry {
	t.Helper()
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID:         "NSE:OPT:NIFTY:22600:CE",
		Symbol:     "NIFTY22600CE",
		Name:       "NIFTY 22600 CE",
		AssetClass: "OPTION",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider: ProviderName, InstrumentID: "NSE:OPT:NIFTY:22600:CE", ProviderKey: "NSE_FO|NIFTY22600CE",
	}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func normalizeOptionFeed(t *testing.T, feed Feed) float64 {
	t.Helper()
	normalizer, err := NewNormalizer(testOptionRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	marketAt := time.Date(2026, 10, 1, 3, 0, 1, 0, time.UTC)
	ticks, err := normalizer.NormalizeEnvelope(DecodedEnvelope{
		Type:      "live_feed",
		CurrentTS: formatMillis(marketAt),
		Feeds: map[string]Feed{
			"NSE_FO|NIFTY22600CE": feed,
		},
	}, marketAt.Add(time.Millisecond), func() uint64 { return 1 })
	if err != nil {
		t.Fatal(err)
	}
	if len(ticks) != 1 {
		t.Fatalf("expected one option tick, got %d", len(ticks))
	}
	if ticks[0].Quantity != 0 && feed.FullFeed != nil && len(feed.FullFeed.Depth) > 0 {
		t.Fatalf("quote-driven option tick must not replay LTQ as volume: %+v", ticks[0])
	}
	return ticks[0].Price
}

func TestNormalizeOptionUsesMicropriceBeforeMidpointAndLTP(t *testing.T) {
	ltpc := &LTPC{LTP: 99, LTT: "1790823600000", LTQ: "50"}
	state := &MarketState{
		LTPC:  ltpc,
		Depth: []Quote{{BidP: 100, AskP: 102, BidQ: 30, AskQ: 10}},
	}
	got := normalizeOptionFeed(t, Feed{LTPC: ltpc, FullFeed: state, RequestMode: ModeFull})
	want := 101.5
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("expected microprice %.2f, got %.6f", want, got)
	}
}

func TestNormalizeOptionFallsBackToMidpointWithoutDepthQuantity(t *testing.T) {
	ltpc := &LTPC{LTP: 99, LTT: "1790823600000", LTQ: "50"}
	state := &MarketState{
		LTPC:  ltpc,
		Depth: []Quote{{BidP: 100, AskP: 102}},
	}
	got := normalizeOptionFeed(t, Feed{LTPC: ltpc, FullFeed: state, RequestMode: ModeFull})
	if got != 101 {
		t.Fatalf("expected midpoint 101, got %.6f", got)
	}
}

func TestNormalizeOptionFallsBackToLTPForInvalidBook(t *testing.T) {
	ltpc := &LTPC{LTP: 99, LTT: "1790823600000", LTQ: "50"}
	state := &MarketState{
		LTPC:  ltpc,
		Depth: []Quote{{BidP: 102, AskP: 100, BidQ: 10, AskQ: 10}},
	}
	normalizer, err := NewNormalizer(testOptionRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	marketAt := time.Date(2026, 10, 1, 3, 0, 1, 0, time.UTC)
	ticks, err := normalizer.NormalizeEnvelope(DecodedEnvelope{
		Type:      "live_feed",
		CurrentTS: formatMillis(marketAt),
		Feeds: map[string]Feed{
			"NSE_FO|NIFTY22600CE": {LTPC: ltpc, FullFeed: state, RequestMode: ModeFull},
		},
	}, marketAt.Add(time.Millisecond), func() uint64 { return 1 })
	if err != nil {
		t.Fatal(err)
	}
	if len(ticks) != 1 || ticks[0].Price != 99 || ticks[0].Quantity != 50 {
		t.Fatalf("expected LTP fallback with trade quantity, got %+v", ticks)
	}
}

func TestNormalizeOptionQuoteCanAdvanceWithoutTradeTimestamp(t *testing.T) {
	ltpc := &LTPC{LTP: 0, LTT: "", LTQ: ""}
	state := &MarketState{
		LTPC:  ltpc,
		Depth: []Quote{{BidP: 100, AskP: 102, BidQ: 10, AskQ: 10}},
	}
	got := normalizeOptionFeed(t, Feed{LTPC: ltpc, FullFeed: state, RequestMode: ModeFull})
	if got != 101 {
		t.Fatalf("expected quote-driven price without a trade timestamp, got %.6f", got)
	}
}
