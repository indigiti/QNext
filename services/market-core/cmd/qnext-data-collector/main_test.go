package main

import "testing"

func TestConfiguredMarketKeepsSynPlusNiftyOnly(t *testing.T) {
	t.Setenv("QNEXT_MARKET_CONFIG", "")
	t.Setenv("QNEXT_DATA_COLLECTOR_MARKET", "NIFTY")
	market, timeframes, err := configuredMarket()
	if err != nil {
		t.Fatal(err)
	}
	if market.Symbol != "NIFTY" {
		t.Fatalf("expected NIFTY SYN+ market, got %s", market.Symbol)
	}
	if len(timeframes) == 0 {
		t.Fatal("expected canonical candle timeframes for SYN+")
	}

	t.Setenv("QNEXT_DATA_COLLECTOR_MARKET", "BANKNIFTY")
	if _, _, err := configuredMarket(); err == nil {
		t.Fatal("expected non-NIFTY SYN+ market selection to be rejected")
	}
}
