package main

import "testing"

func TestConfiguredMarketSelectsAnyConfiguredSynPlusIndex(t *testing.T) {
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
	bank, bankTimeframes, err := configuredMarket()
	if err != nil {
		t.Fatal(err)
	}
	if bank.Symbol != "BANKNIFTY" || bank.Underlying.InstrumentID != "NSE:BANKNIFTY" || len(bankTimeframes) == 0 {
		t.Fatalf("unexpected BANKNIFTY SYN+ market: %+v timeframes=%+v", bank, bankTimeframes)
	}
	t.Setenv("QNEXT_DATA_COLLECTOR_MARKET", "UNKNOWN")
	if _, _, err := configuredMarket(); err == nil {
		t.Fatal("expected an unconfigured SYN+ market to be rejected")
	}
}
