package symbol

import "testing"

func TestRegistryResolvesProviderKeyAndSearch(t *testing.T) {
	registry := NewRegistry()
	nifty := Instrument{
		ID:         "NSE:NIFTY50",
		Symbol:     "NIFTY",
		Name:       "Nifty 50",
		AssetClass: "INDEX",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
		Aliases:    []string{"NIFTY 50"},
	}
	if err := registry.Register(nifty); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(ProviderInstrument{
		Provider:     "upstox",
		InstrumentID: nifty.ID,
		ProviderKey:  "NSE_INDEX|Nifty 50",
	}); err != nil {
		t.Fatal(err)
	}

	resolved, ok := registry.ResolveProviderKey("upstox", "NSE_INDEX|Nifty 50")
	if !ok || resolved.ID != nifty.ID {
		t.Fatalf("unexpected provider resolution: ok=%v instrument=%+v", ok, resolved)
	}

	found := registry.Search("nifty")
	if len(found) != 1 || found[0].ID != nifty.ID {
		t.Fatalf("unexpected search result: %+v", found)
	}
}
