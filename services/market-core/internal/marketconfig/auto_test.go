package marketconfig

import "testing"

func TestAutoSyntheticConfigValidatesWithoutFixedLegs(t *testing.T) {
	config := validConfig()
	config.Synthetic.Legs = nil
	config.Synthetic.Auto = &AutoLegConfig{
		StrikeInterval:      50,
		ActiveStrikes:       5,
		WarmStrikes:         7,
		ATMHysteresisPoints: 5,
		ATMConfirmationMS:   750,
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	keys := config.ProviderKeys()
	want := []string{
		"BSE_INDEX|BANKEX",
		"BSE_INDEX|SENSEX",
		"NSE_INDEX|NIFTY MID SELECT",
		"NSE_INDEX|Nifty 50",
		"NSE_INDEX|Nifty Bank",
		"NSE_INDEX|Nifty Fin Service",
	}
	if len(keys) != len(want) {
		t.Fatalf("auto mode should bootstrap six index provider keys, got %+v", keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("provider key %d=%q want %q; all keys=%+v", i, keys[i], want[i], keys)
		}
	}
	if !config.AutoLegsEnabled() {
		t.Fatal("expected auto-leg mode")
	}
}

func TestAutoSyntheticConfigRejectsMixedFixedAndDynamicLegs(t *testing.T) {
	config := validConfig()
	config.Synthetic.Auto = &AutoLegConfig{
		StrikeInterval:      50,
		ActiveStrikes:       5,
		WarmStrikes:         7,
		ATMHysteresisPoints: 5,
		ATMConfirmationMS:   750,
	}
	if err := config.Validate(); err == nil {
		t.Fatal("expected mixed fixed/auto configuration to fail")
	}
}
