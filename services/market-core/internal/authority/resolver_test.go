package authority

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func TestResolverChoosesHighestPriorityEligibleProvider(t *testing.T) {
	registry := testRegistry(t)
	resolver, err := NewResolver(registry, map[string][]Preference{
		"NSE:NIFTY50": {
			{Provider: "upstox", Priority: 10, MaxStaleness: 2 * time.Second},
			{Provider: "backup", Priority: 20, MaxStaleness: 2 * time.Second},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	if err := resolver.UpdateState("NSE:NIFTY50", "upstox", ProviderState{
		Healthy: true, Entitled: true, LastEventTime: now.Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := resolver.UpdateState("NSE:NIFTY50", "backup", ProviderState{
		Healthy: true, Entitled: true, LastEventTime: now,
	}); err != nil {
		t.Fatal(err)
	}

	selection, ok := resolver.Resolve("NSE:NIFTY50", now)
	if !ok || selection.Provider != "upstox" {
		t.Fatalf("expected upstox authority, got ok=%v selection=%+v", ok, selection)
	}
	if !resolver.Accept("NSE:NIFTY50", "upstox", now) {
		t.Fatal("expected authoritative provider to be accepted")
	}
	if resolver.Accept("NSE:NIFTY50", "backup", now) {
		t.Fatal("expected non-authoritative provider to be rejected")
	}
}

func TestResolverFailsOverWhenPrimaryIsStale(t *testing.T) {
	registry := testRegistry(t)
	resolver, err := NewResolver(registry, map[string][]Preference{
		"NSE:NIFTY50": {
			{Provider: "upstox", Priority: 10, MaxStaleness: time.Second},
			{Provider: "backup", Priority: 20, MaxStaleness: 2 * time.Second},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	_ = resolver.UpdateState("NSE:NIFTY50", "upstox", ProviderState{
		Healthy: true, Entitled: true, LastEventTime: now.Add(-2 * time.Second),
	})
	_ = resolver.UpdateState("NSE:NIFTY50", "backup", ProviderState{
		Healthy: true, Entitled: true, LastEventTime: now,
	})

	selection, ok := resolver.Resolve("NSE:NIFTY50", now)
	if !ok || selection.Provider != "backup" {
		t.Fatalf("expected backup authority, got ok=%v selection=%+v", ok, selection)
	}
}

func TestResolverFailsClosedWithoutEligibleAuthority(t *testing.T) {
	registry := testRegistry(t)
	resolver, err := NewResolver(registry, map[string][]Preference{
		"NSE:NIFTY50": {
			{Provider: "upstox", Priority: 10, MaxStaleness: time.Second},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	_ = resolver.UpdateState("NSE:NIFTY50", "upstox", ProviderState{
		Healthy: false, Entitled: true, LastEventTime: now,
	})

	if _, ok := resolver.Resolve("NSE:NIFTY50", now); ok {
		t.Fatal("expected no authority when the only provider is unhealthy")
	}
}

func TestResolverTieBreakIsDeterministic(t *testing.T) {
	registry := testRegistry(t)
	resolver, err := NewResolver(registry, map[string][]Preference{
		"NSE:NIFTY50": {
			{Provider: "upstox", Priority: 10, MaxStaleness: time.Second},
			{Provider: "backup", Priority: 10, MaxStaleness: time.Second},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	for _, provider := range []string{"upstox", "backup"} {
		_ = resolver.UpdateState("NSE:NIFTY50", provider, ProviderState{
			Healthy: true, Entitled: true, LastEventTime: now,
		})
	}

	selection, ok := resolver.Resolve("NSE:NIFTY50", now)
	if !ok || selection.Provider != "backup" {
		t.Fatalf("expected lexical tie-break to backup, got ok=%v selection=%+v", ok, selection)
	}
}

func testRegistry(t *testing.T) *symbol.Registry {
	t.Helper()
	registry := symbol.NewRegistry()
	instrument := symbol.Instrument{
		ID:         "NSE:NIFTY50",
		Symbol:     "NIFTY",
		Name:       "Nifty 50",
		AssetClass: "INDEX",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
	}
	if err := registry.Register(instrument); err != nil {
		t.Fatal(err)
	}
	for provider, key := range map[string]string{
		"upstox": "NSE_INDEX|Nifty 50",
		"backup": "NIFTY50",
	} {
		if err := registry.RegisterProvider(symbol.ProviderInstrument{
			Provider: provider, InstrumentID: instrument.ID, ProviderKey: key,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return registry
}

func TestResolverCanAddDynamicInstrumentPolicy(t *testing.T) {
	registry := testRegistry(t)
	option := symbol.Instrument{
		ID:         "NSE:NIFTY:2026-09-29:25100:CE",
		Symbol:     "NIFTY26SEP25100CE",
		Name:       "NIFTY option",
		AssetClass: "OPTION",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
	}
	if err := registry.Register(option); err != nil {
		t.Fatal(err)
	}
	for provider, key := range map[string]string{
		"upstox": "NSE_FO|12345",
		"backup": "NSE_FNO|54321|OPTIDX",
	} {
		if err := registry.RegisterProvider(symbol.ProviderInstrument{
			Provider: provider, InstrumentID: option.ID, ProviderKey: key,
		}); err != nil {
			t.Fatal(err)
		}
	}

	resolver, err := NewResolver(registry, map[string][]Preference{
		"NSE:NIFTY50": {
			{Provider: "upstox", Priority: 10, MaxStaleness: time.Second},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.SetPolicy(option.ID, []Preference{
		{Provider: "upstox", Priority: 10, MaxStaleness: time.Second},
		{Provider: "backup", Priority: 20, MaxStaleness: time.Second},
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	if err := resolver.UpdateState(option.ID, "upstox", ProviderState{
		Healthy: true, Entitled: true, LastEventTime: now.Add(-2 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := resolver.UpdateState(option.ID, "backup", ProviderState{
		Healthy: true, Entitled: true, LastEventTime: now,
	}); err != nil {
		t.Fatal(err)
	}
	selection, ok := resolver.Resolve(option.ID, now)
	if !ok || selection.Provider != "backup" {
		t.Fatalf("expected dynamic option to fail over to backup, got ok=%v selection=%+v", ok, selection)
	}
}
