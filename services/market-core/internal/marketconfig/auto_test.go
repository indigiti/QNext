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
	if len(keys) != 1 || keys[0] != "NSE_INDEX|Nifty 50" {
		t.Fatalf("auto mode should bootstrap only the NIFTY provider key, got %+v", keys)
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
