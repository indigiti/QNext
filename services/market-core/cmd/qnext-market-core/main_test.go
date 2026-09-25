package main

import (
	"testing"
	"time"
)

func TestBuildRegistryExposesSixIndexAndSyntheticPairsWithoutLiveConfig(t *testing.T) {
	registry, err := buildRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}

	visible := registry.ListVisible()
	if len(visible) != 12 {
		t.Fatalf("expected twelve visible workspace symbols, got %d: %+v", len(visible), visible)
	}

	seen := make(map[string]bool, len(visible))
	for _, instrument := range visible {
		seen[instrument.Symbol] = true
	}
	for _, symbol := range []string{
		"NIFTY", "NIFTY-SYN",
		"BANKNIFTY", "BANKNIFTY-SYN",
		"MIDCPNIFTY", "MIDCPNIFTY-SYN",
		"FINNIFTY", "FINNIFTY-SYN",
		"SENSEX", "SENSEX-SYN",
		"BANKEX", "BANKEX-SYN",
	} {
		if !seen[symbol] {
			t.Fatalf("workspace catalog missing %s: %+v", symbol, visible)
		}
	}
}

func TestRegularMarketSessionActive(t *testing.T) {
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	if !regularMarketSessionActive(time.Date(2026, 9, 25, 10, 0, 0, 0, ist)) {
		t.Fatal("expected weekday market session to be active")
	}
	if regularMarketSessionActive(time.Date(2026, 9, 25, 16, 0, 0, 0, ist)) {
		t.Fatal("expected after-hours watchdog to be inactive")
	}
	if regularMarketSessionActive(time.Date(2026, 9, 26, 10, 0, 0, 0, ist)) {
		t.Fatal("expected weekend watchdog to be inactive")
	}
	if regularMarketSessionActive(time.Date(2026, 10, 2, 10, 0, 0, 0, ist)) {
		t.Fatal("expected exchange holiday watchdog to be inactive")
	}
}


func TestMarketSessionResolverUsesInstrumentCalendarAuthority(t *testing.T) {
	registry, err := buildRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	calendars := marketcalendar.DefaultRegistry()
	resolver := marketSessionResolver(registry, calendars)
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}

	window, active, err := resolver(
		"NSE:NIFTY50",
		time.Date(2026, 9, 25, 10, 0, 0, 0, ist),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("expected NIFTY regular session to be active")
	}
	if got := window.Open.In(ist); got.Hour() != 9 || got.Minute() != 15 {
		t.Fatalf("unexpected session open: %v", got)
	}
	if got := window.Close.In(ist); got.Hour() != 15 || got.Minute() != 30 {
		t.Fatalf("unexpected session close: %v", got)
	}

	_, active, err = resolver(
		"NSE:NIFTY50",
		time.Date(2026, 10, 2, 10, 0, 0, 0, ist),
	)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("holiday tick must be outside the certified session")
	}

	if _, _, err := resolver(
		"NSE:NIFTY50",
		time.Date(2027, 1, 2, 10, 0, 0, 0, ist),
	); err == nil {
		t.Fatal("resolver must fail closed outside the certified calendar window")
	}
}
