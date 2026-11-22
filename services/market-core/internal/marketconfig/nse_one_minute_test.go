package marketconfig

import (
	"strings"
	"testing"
)

func TestNSEOneMinuteWatchlistValidationAndSubscriptionIsolation(t *testing.T) {
	c := Config{
		Timeframes: []string{"1m"},
		NSEOneMinute: []NSEMinuteSymbol{
			{Symbol: "RELIANCE", Name: "Reliance Industries", ProviderKey: "NSE_EQ|INE002A01018"},
			{Symbol: "NIFTY100", Name: "Nifty 100", ProviderKey: "NSE_INDEX|Nifty 100"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.NSEOneMinute[0].InstrumentID() != "NSE:RELIANCE" ||
		c.NSEOneMinute[1].InstrumentID() != "NSE:NIFTY100" ||
		!c.NSEOneMinute[1].IsIndex() || c.NSEOneMinute[0].IsIndex() {
		t.Fatal("NSE one minute symbol identity not stable")
	}
	for _, key := range c.ProviderKeys() {
		if strings.Contains(key, "INE002A01018") || strings.Contains(key, "Nifty 100") {
			t.Fatal("historical NSE watchlist key leaked into options/futures live subscriptions")
		}
	}

	for _, invalid := range []NSEMinuteSymbol{
		{Symbol: "BAD", Name: "Wrong exchange", ProviderKey: "BSE_EQ|INE000A01011"},
		{Symbol: "BAD", Name: "Empty instrument key", ProviderKey: "NSE_EQ|"},
		{Symbol: "BAD/123", Name: "Invalid ticker", ProviderKey: "NSE_EQ|INE000A01011"},
		{Symbol: "NIFTY", Name: "Duplicate configured market", ProviderKey: "NSE_INDEX|Nifty 50"},
		{Symbol: "RELIANCE", Name: "Duplicate ticker", ProviderKey: "NSE_EQ|INE000A01011"},
		{Symbol: "TCS", Name: "Duplicate provider key", ProviderKey: "NSE_EQ|INE002A01018"},
	} {
		copyConfig := c
		copyConfig.NSEOneMinute = append(append([]NSEMinuteSymbol(nil), c.NSEOneMinute...), invalid)
		if err := copyConfig.Validate(); err == nil {
			t.Fatalf("invalid symbol was accepted: %+v", invalid)
		}
	}
	c.NSEOneMinute = make([]NSEMinuteSymbol, 13)
	if err := c.Validate(); err == nil {
		t.Fatal("unbounded NSE 1m subscription list accepted")
	}
}
