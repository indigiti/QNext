package main

import "testing"

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
