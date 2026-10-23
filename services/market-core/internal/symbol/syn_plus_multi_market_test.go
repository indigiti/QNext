package symbol

import "testing"

func TestSynPlusVirtualSymbolsRespectEnabledMarkets(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_ENABLED_MARKETS", "NIFTY,BANKNIFTY")
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:NIFTY-SYN+")
	registry := NewRegistry()
	for _, market := range []struct {
		symbol string
		id     string
	}{
		{"NIFTY", "QNEXT:NIFTY-SYN"},
		{"BANKNIFTY", "QNEXT:BANKNIFTY-SYN"},
		{"SENSEX", "QNEXT:SENSEX-SYN"},
	} {
		if err := registry.Register(Instrument{
			ID: market.id, Symbol: market.symbol + "-SYN", Name: market.symbol + " Synthetic",
			AssetClass: "INDEX", Exchange: "QNEXT", Currency: "INR",
			Timezone: "Asia/Kolkata", CalendarID: "NSE_EQ", Synthetic: true, Visible: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"QNEXT:NIFTY-SYN+", "QNEXT:BANKNIFTY-SYN+"} {
		instrument, ok := registry.Instrument(id)
		if !ok || !instrument.Visible || !instrument.Synthetic {
			t.Fatalf("missing explicitly enabled shadow symbol: %s %+v", id, instrument)
		}
	}
	if _, ok := registry.Instrument("QNEXT:SENSEX-SYN+"); ok {
		t.Fatal("SENSEX SYN+ must not appear without activation")
	}
	matches := registry.Search("banknifty syn plus")
	if len(matches) != 1 || matches[0].ID != "QNEXT:BANKNIFTY-SYN+" {
		t.Fatalf("incorrect BANKNIFTY lookup: %+v", matches)
	}
}
