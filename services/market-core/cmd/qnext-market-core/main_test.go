package main

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

func TestBuildRegistryExposesSixIndexAndSyntheticPairsPlusNiftyShadowWithoutLiveConfig(t *testing.T) {
	registry, err := buildRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}

	visible := registry.ListVisible()
	if len(visible) != 13 {
		t.Fatalf("expected twelve production workspace symbols plus NIFTY-SYN+ shadow, got %d: %+v", len(visible), visible)
	}

	seen := make(map[string]bool, len(visible))
	for _, instrument := range visible {
		seen[instrument.Symbol] = true
	}
	for _, symbol := range []string{
		"NIFTY", "NIFTY-SYN", "NIFTY-SYN+",
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

func TestContinuousRecordingCoversEveryDefaultIndexAndSynthetic(t *testing.T) {
	config := marketconfig.Config{
		Timeframes: marketconfig.DefaultEnabledTimeframes(),
		Markets:    marketconfig.DefaultMarkets(),
	}

	providerKeys := config.ProviderKeys()
	if len(providerKeys) != 6 {
		t.Fatalf("expected all six index underlyings to stay provider-subscribed, got %d: %v", len(providerKeys), providerKeys)
	}

	syntheticIDs := continuousSyntheticInstrumentIDs(config)
	if len(syntheticIDs) != 6 {
		t.Fatalf("expected all six INDEX-SYN engines to stay demand-active, got %d: %v", len(syntheticIDs), syntheticIDs)
	}
	seen := make(map[string]bool, len(syntheticIDs))
	for _, instrumentID := range syntheticIDs {
		seen[instrumentID] = true
	}
	for _, instrumentID := range []string{
		"QNEXT:NIFTY-SYN",
		"QNEXT:BANKNIFTY-SYN",
		"QNEXT:MIDCPNIFTY-SYN",
		"QNEXT:FINNIFTY-SYN",
		"QNEXT:SENSEX-SYN",
		"QNEXT:BANKEX-SYN",
	} {
		if !seen[instrumentID] {
			t.Fatalf("continuous recording missing %s: %v", instrumentID, syntheticIDs)
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
