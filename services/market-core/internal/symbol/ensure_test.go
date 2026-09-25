package symbol

import "testing"

func TestEnsureProviderIsIdempotentAndRejectsRemap(t *testing.T) {
	registry := NewRegistry()
	instrument := Instrument{
		ID: "NSE:NIFTY:2026-09-29:25100:CE", Symbol: "NIFTY CE", Name: "NIFTY CE",
		AssetClass: "OPTION", Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata",
	}
	if err := registry.Ensure(instrument); err != nil {
		t.Fatal(err)
	}
	mapping := ProviderInstrument{
		Provider: "upstox", InstrumentID: instrument.ID, ProviderKey: "NSE_FO|123",
	}
	if err := registry.EnsureProvider(mapping); err != nil {
		t.Fatal(err)
	}
	if err := registry.EnsureProvider(mapping); err != nil {
		t.Fatalf("exact mapping should be idempotent: %v", err)
	}
	if err := registry.EnsureProvider(ProviderInstrument{
		Provider: "upstox", InstrumentID: instrument.ID, ProviderKey: "NSE_FO|456",
	}); err == nil {
		t.Fatal("expected provider remap to fail closed")
	}
}
