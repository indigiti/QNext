package marketconfig

import "testing"

func validConfig() Config {
	legs := []Leg{
		{"CE25000", "CE25000KEY", 25000, "CALL"}, {"PE25000", "PE25000KEY", 25000, "PUT"},
		{"CE25050", "CE25050KEY", 25050, "CALL"}, {"PE25050", "PE25050KEY", 25050, "PUT"},
		{"CE25100", "CE25100KEY", 25100, "CALL"}, {"PE25100", "PE25100KEY", 25100, "PUT"},
		{"CE25150", "CE25150KEY", 25150, "CALL"}, {"PE25150", "PE25150KEY", 25150, "PUT"},
		{"CE25200", "CE25200KEY", 25200, "CALL"}, {"PE25200", "PE25200KEY", 25200, "PUT"},
	}
	return Config{
		Timeframes: []string{"30s", "1m", "3m", "5m"},
		Nifty:      Instrument{InstrumentID: "NSE:NIFTY50", ProviderKey: "NSE_INDEX|Nifty 50"},
		Synthetic: SyntheticConfig{
			InstrumentID:           "QNEXT:NIFTY-SYN",
			Version:                "nifty-syn-v1",
			MinimumValidCandidates: 3,
			MaxLegAgeMS:            2000,
			MaxLegTimeSkewMS:       1000,
			Legs:                   legs,
		},
	}
}

func TestConfigValidatesFiveStrikeSynthetic(t *testing.T) {
	config := validConfig()
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	keys := config.ProviderKeys()
	if len(keys) != 16 {
		t.Fatalf("expected 16 provider keys across six markets, got %d", len(keys))
	}
	recoverable := config.RecoverableTimeframes()
	if len(recoverable) != 1 || recoverable[0] != "1m" {
		t.Fatalf("expected canonical 1m recovery only, got %+v", recoverable)
	}
}

func TestConfigRejectsMissingLeg(t *testing.T) {
	config := validConfig()
	config.Synthetic.Legs = config.Synthetic.Legs[:9]
	if err := config.Validate(); err == nil {
		t.Fatal("expected missing leg validation failure")
	}
}

func TestLegacyConfigExpandsToSixMarketPairs(t *testing.T) {
	config := validConfig()
	markets := config.EffectiveMarkets()
	if len(markets) != 6 {
		t.Fatalf("expected six market pairs, got %d", len(markets))
	}

	want := []string{"NIFTY", "BANKNIFTY", "MIDCPNIFTY", "FINNIFTY", "SENSEX", "BANKEX"}
	for i, symbol := range want {
		if markets[i].Symbol != symbol {
			t.Fatalf("market %d symbol=%q want %q", i, markets[i].Symbol, symbol)
		}
		if markets[i].Synthetic.InstrumentID == "" {
			t.Fatalf("market %s missing synthetic instrument", symbol)
		}
	}
	if markets[0].Synthetic.Version != "nifty-syn-v1" {
		t.Fatalf("legacy NIFTY synthetic should be preserved, got %q", markets[0].Synthetic.Version)
	}
}

func TestDefaultMarketsUseExpectedIndexProviderKeys(t *testing.T) {
	markets := DefaultMarkets()
	want := map[string]string{
		"NIFTY":      "NSE_INDEX|Nifty 50",
		"BANKNIFTY":  "NSE_INDEX|Nifty Bank",
		"MIDCPNIFTY": "NSE_INDEX|NIFTY MID SELECT",
		"FINNIFTY":   "NSE_INDEX|Nifty Fin Service",
		"SENSEX":     "BSE_INDEX|SENSEX",
		"BANKEX":     "BSE_INDEX|BANKEX",
	}
	for _, market := range markets {
		if got := market.Underlying.ProviderKey; got != want[market.Symbol] {
			t.Fatalf("%s provider key=%q want %q", market.Symbol, got, want[market.Symbol])
		}
	}
}

func TestConfigRequiresCanonicalOneMinute(t *testing.T) {
	config := validConfig()
	config.Timeframes = []string{"15s", "30s", "3m", "5m"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected configuration without 1m to fail")
	}
}

func TestDefaultEnabledTimeframesMatchOpsDefaults(t *testing.T) {
	got := DefaultEnabledTimeframes()
	want := []string{"15s", "30s", "1m", "2m", "3m", "5m", "15m", "30m", "1h", "1D"}
	if len(got) != len(want) {
		t.Fatalf("defaults=%+v want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("defaults=%+v want %+v", got, want)
		}
	}
}
