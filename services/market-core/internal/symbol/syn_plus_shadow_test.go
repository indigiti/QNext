package symbol

import "testing"

func TestNiftySynPlusVirtualSymbol(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:NIFTY-SYN+")
	registry := NewRegistry()
	if err := registry.Register(Instrument{
		ID:         "QNEXT:NIFTY-SYN",
		Symbol:     "NIFTY-SYN",
		Name:       "QNext Nifty 50 Synthetic",
		AssetClass: "INDEX",
		Exchange:   "QNEXT",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
		CalendarID: "NSE_EQ",
		Synthetic:  true,
		Visible:    true,
	}); err != nil {
		t.Fatal(err)
	}

	instrument, ok := registry.Instrument("QNEXT:NIFTY-SYN+")
	if !ok {
		t.Fatal("expected virtual NIFTY-SYN+ instrument")
	}
	if instrument.Symbol != "NIFTY-SYN+" || !instrument.Synthetic || !instrument.Visible {
		t.Fatalf("unexpected virtual instrument: %+v", instrument)
	}

	visible := registry.ListVisible()
	found := false
	for _, candidate := range visible {
		if candidate.ID == "QNEXT:NIFTY-SYN+" {
			found = true
		}
	}
	if !found {
		t.Fatalf("NIFTY-SYN+ missing from visible symbols: %+v", visible)
	}

	matches := registry.Search("syn plus")
	if len(matches) != 1 || matches[0].ID != "QNEXT:NIFTY-SYN+" {
		t.Fatalf("unexpected search matches: %+v", matches)
	}
}
