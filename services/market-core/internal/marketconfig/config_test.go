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
	if len(keys) != 11 {
		t.Fatalf("expected 11 provider keys, got %d", len(keys))
	}
	recoverable := config.RecoverableTimeframes()
	if len(recoverable) != 3 {
		t.Fatalf("expected 3 recoverable timeframes, got %+v", recoverable)
	}
}

func TestConfigRejectsMissingLeg(t *testing.T) {
	config := validConfig()
	config.Synthetic.Legs = config.Synthetic.Legs[:9]
	if err := config.Validate(); err == nil {
		t.Fatal("expected missing leg validation failure")
	}
}
