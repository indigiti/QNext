package symbol

import "testing"

func TestListVisibleReturnsOnlyWorkspaceSymbols(t *testing.T) {
	registry := NewRegistry()
	for _, instrument := range []Instrument{
		{
			ID: "QNEXT:NIFTY-SYN", Symbol: "NIFTY-SYN", Name: "QNext Nifty Synthetic",
			AssetClass: "INDEX", Exchange: "QNEXT", Currency: "INR", Timezone: "Asia/Kolkata",
			CalendarID: "NSE_EQ", Synthetic: true, Visible: true,
		},
		{
			ID: "NSE:NIFTY50", Symbol: "NIFTY", Name: "Nifty 50",
			AssetClass: "INDEX", Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata",
			CalendarID: "NSE_EQ", Visible: true,
		},
		{
			ID: "NIFTY:25000:CE", Symbol: "NIFTY:25000:CE", Name: "NIFTY:25000:CE",
			AssetClass: "OPTION", Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata",
			CalendarID: "NSE_EQ", Visible: false,
		},
	} {
		if err := registry.Register(instrument); err != nil {
			t.Fatal(err)
		}
	}

	visible := registry.ListVisible()
	if len(visible) != 2 {
		t.Fatalf("expected two visible instruments, got %+v", visible)
	}
	if visible[0].ID != "NSE:NIFTY50" || visible[1].ID != "QNEXT:NIFTY-SYN" {
		t.Fatalf("unexpected visible ordering: %+v", visible)
	}
}
