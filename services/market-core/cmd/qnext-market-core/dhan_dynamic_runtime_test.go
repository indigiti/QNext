package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/authority"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
	"github.com/indigiti/QNext/services/market-core/internal/provider/dhan"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/resilience"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type fakeDhanOptionChain struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeDhanOptionChain) Chain(
	_ context.Context,
	_ string,
	_ string,
	underlying dhan.InstrumentKey,
	expiry string,
) ([]dhan.OptionChainLeg, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if underlying.SecurityID != 13 || expiry != "2026-09-29" {
		return nil, context.Canceled
	}
	return []dhan.OptionChainLeg{
		{Strike: 25100, Side: "CE", SecurityID: 42529},
		{Strike: 25100, Side: "PE", SecurityID: 42530},
	}, nil
}

func (f *fakeDhanOptionChain) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestAuthorityInstrumentIDsCoverEveryActiveUnderlying(t *testing.T) {
	config := marketconfig.Config{
		Timeframes: []string{"1m"},
		Markets:    marketconfig.DefaultMarkets(),
	}
	ids := authorityInstrumentIDs(config)
	if len(ids) != 6 {
		t.Fatalf("expected six active auto-market underlyings, got %d: %+v", len(ids), ids)
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range []string{
		"NSE:NIFTY50",
		"NSE:BANKNIFTY",
		"NSE:MIDCPNIFTY",
		"NSE:FINNIFTY",
		"BSE:SENSEX",
		"BSE:BANKEX",
	} {
		if !seen[id] {
			t.Fatalf("authority coverage missing active underlying %s: %+v", id, ids)
		}
	}
}

func TestRegisterDhanMappingsAllowsPartialMultiMarketCoverage(t *testing.T) {
	config := marketconfig.Config{
		Timeframes: []string{"1m"},
		Markets:    marketconfig.DefaultMarkets(),
	}
	registry, err := buildRegistry(&config)
	if err != nil {
		t.Fatal(err)
	}
	resilienceConfig := testResilienceConfig()
	if err := registerDhanMappings(config, resilienceConfig, registry); err != nil {
		t.Fatal(err)
	}

	if _, ok := registry.ProviderMapping(dhan.ProviderName, "NSE:NIFTY50"); !ok {
		t.Fatal("configured NIFTY Dhan mapping should be registered")
	}
	if _, ok := registry.ProviderMapping(dhan.ProviderName, "NSE:BANKNIFTY"); ok {
		t.Fatal("unconfigured BANKNIFTY must remain Upstox-only instead of inventing Dhan coverage")
	}
}

func TestDynamicDhanMapperRegistersOptionAuthorityAndCachesChain(t *testing.T) {
	market := marketconfig.DefaultMarkets()[0]
	config := marketconfig.Config{
		Timeframes: []string{"1m"},
		Markets:    []marketconfig.MarketConfig{market},
	}
	registry, err := buildRegistry(&config)
	if err != nil {
		t.Fatal(err)
	}
	resilienceConfig := testResilienceConfig()
	if err := registerDhanMappings(config, resilienceConfig, registry); err != nil {
		t.Fatal(err)
	}

	resolver, err := authority.NewResolver(registry, map[string][]authority.Preference{
		market.Underlying.InstrumentID: {
			{Provider: upstox.ProviderName, Priority: 10, MaxStaleness: time.Second},
			{Provider: dhan.ProviderName, Priority: 20, MaxStaleness: time.Second},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	chain := &fakeDhanOptionChain{}
	keys := dhan.NewKeyRegistry([]dhan.InstrumentKey{{
		ExchangeSegment: "IDX_I",
		SecurityID:      13,
		Instrument:      "INDEX",
	}})
	mapper, err := newDhanDynamicOptionMapper(
		config,
		resilienceConfig,
		chain,
		"token",
		"client",
		registry,
		resolver,
		keys,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	callID := "NSE:NIFTY:2026-09-29:25100:CE"
	putID := "NSE:NIFTY:2026-09-29:25100:PE"
	for _, pair := range []struct {
		id  string
		key string
	}{
		{callID, "NSE_FO|CALL"},
		{putID, "NSE_FO|PUT"},
	} {
		if err := registry.Ensure(symbol.Instrument{
			ID:         pair.id,
			Symbol:     pair.id,
			Name:       pair.id,
			AssetClass: "OPTION",
			Exchange:   "NSE",
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
		}); err != nil {
			t.Fatal(err)
		}
		if err := registry.EnsureProvider(symbol.ProviderInstrument{
			Provider:     upstox.ProviderName,
			InstrumentID: pair.id,
			ProviderKey:  pair.key,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if !mapper.Eligible(callID) {
		t.Fatal("NIFTY dynamic option should be eligible when underlying Dhan mapping exists")
	}
	if err := mapper.resolve(context.Background(), callID); err != nil {
		t.Fatal(err)
	}
	if err := mapper.resolve(context.Background(), putID); err != nil {
		t.Fatal(err)
	}
	if chain.Calls() != 1 {
		t.Fatalf("same underlying/expiry should use one cached Dhan option-chain request, got %d", chain.Calls())
	}
	if !mapper.Covered(callID) || !mapper.Covered(putID) {
		t.Fatal("resolved dynamic option legs should report Dhan coverage")
	}

	callMapping, ok := registry.ProviderMapping(dhan.ProviderName, callID)
	if !ok || callMapping.ProviderKey != "NSE_FNO|42529|OPTIDX" {
		t.Fatalf("unexpected Dhan call mapping: ok=%v mapping=%+v", ok, callMapping)
	}
	if len(keys.Snapshot()) != 3 {
		t.Fatalf("expected underlying plus two dynamic option quote keys, got %+v", keys.Snapshot())
	}

	now := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	if err := resolver.UpdateState(callID, upstox.ProviderName, authority.ProviderState{
		Healthy: true, Entitled: true, LastEventTime: now.Add(-3 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := resolver.UpdateState(callID, dhan.ProviderName, authority.ProviderState{
		Healthy: true, Entitled: true, LastEventTime: now,
	}); err != nil {
		t.Fatal(err)
	}
	selection, ok := resolver.Resolve(callID, now)
	if !ok || selection.Provider != dhan.ProviderName {
		t.Fatalf("expected dynamic option authority to fail over to Dhan, got ok=%v selection=%+v", ok, selection)
	}
}

func testResilienceConfig() resilience.Config {
	return resilience.Config{
		Version:                resilience.ConfigVersion,
		MaxStalenessMS:         2000,
		GapRecoveryMS:          5000,
		DhanPollIntervalMS:     1000,
		FailoverPendingMS:      0,
		FailbackPendingMS:      0,
		AuthorityCooldownMS:    0,
		AuthorityPolicyVersion: "q3-authority-v2-dynamic-options",
		Instruments: []resilience.InstrumentMapping{{
			InstrumentID:    "NSE:NIFTY50",
			DhanProviderKey: "IDX_I|13|INDEX",
			Recoverable:     true,
		}},
	}
}
