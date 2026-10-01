package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/authority"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
	"github.com/indigiti/QNext/services/market-core/internal/provider/dhan"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/resilience"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func authorityInstrumentIDs(config marketconfig.Config) []string {
	seen := make(map[string]bool)
	ids := make([]string, 0)
	for _, market := range config.EffectiveMarkets() {
		if !seen[market.Underlying.InstrumentID] {
			seen[market.Underlying.InstrumentID] = true
			ids = append(ids, market.Underlying.InstrumentID)
		}
		if market.Synthetic.Auto != nil {
			continue
		}
		for _, leg := range market.Synthetic.Legs {
			if !seen[leg.InstrumentID] {
				seen[leg.InstrumentID] = true
				ids = append(ids, leg.InstrumentID)
			}
		}
	}
	return ids
}

func registerDhanMappings(marketConfig marketconfig.Config, config resilience.Config, registry *symbol.Registry) error {
	active := make(map[string]bool)
	for _, instrumentID := range authorityInstrumentIDs(marketConfig) {
		active[instrumentID] = true
	}
	for _, mapping := range config.Instruments {
		if !active[mapping.InstrumentID] {
			continue
		}
		key, err := dhan.ParseInstrumentKey(mapping.DhanProviderKey)
		if err != nil {
			return fmt.Errorf("invalid Dhan mapping for %s: %w", mapping.InstrumentID, err)
		}
		if _, ok := registry.Instrument(mapping.InstrumentID); !ok {
			return fmt.Errorf("Dhan mapping references unknown canonical instrument: %s", mapping.InstrumentID)
		}
		if err := registry.EnsureProvider(symbol.ProviderInstrument{
			Provider:     dhan.ProviderName,
			InstrumentID: mapping.InstrumentID,
			ProviderKey:  key.String(),
		}); err != nil {
			return fmt.Errorf("register Dhan mapping for %s: %w", mapping.InstrumentID, err)
		}
	}
	return nil
}

func runResilientMarket(
	ctx context.Context,
	marketConfig marketconfig.Config,
	config resilience.Config,
	upstoxAccessToken string,
	dhanClientID string,
	dhanAccessToken string,
	registry *symbol.Registry,
	store *history.Store,
	upstoxRunner upstox.StreamRunner,
	upstoxRecovery upstox.GapRecovery,
	upstoxRequest upstox.SubscriptionRequest,
	upstoxRequestSnapshot func() upstox.SubscriptionRequest,
	downstream resilience.TickSink,
	metrics *resilience.Metrics,
) error {
	if registry == nil || store == nil || upstoxRunner == nil || downstream == nil {
		return errors.New("resilience runtime requires registry, history, Upstox runner, and downstream sink")
	}
	if metrics == nil {
		metrics = resilience.NewMetrics()
	}

	policy := make(map[string][]authority.Preference)
	recoverable := make(map[string]bool)
	dhanKeys := make([]dhan.InstrumentKey, 0, len(config.Instruments))
	for _, instrumentID := range authorityInstrumentIDs(marketConfig) {
		preferences := []authority.Preference{
			{Provider: upstox.ProviderName, Priority: 10, MaxStaleness: config.MaxStaleness()},
		}
		if mapping, ok := config.Mapping(instrumentID); ok {
			if _, registered := registry.ProviderMapping(dhan.ProviderName, instrumentID); registered {
				key, err := dhan.ParseInstrumentKey(mapping.DhanProviderKey)
				if err != nil {
					return err
				}
				dhanKeys = append(dhanKeys, key)
				preferences = append(preferences, authority.Preference{
					Provider: dhan.ProviderName, Priority: 20, MaxStaleness: config.MaxStaleness(),
				})
				recoverable[instrumentID] = mapping.Recoverable
			}
		}
		policy[instrumentID] = preferences
	}

	resolver, err := authority.NewResolver(registry, policy)
	if err != nil {
		return err
	}
	dhanQuoteKeys := dhan.NewKeyRegistry(dhanKeys)
	dynamicMapper, err := newDhanDynamicOptionMapper(
		marketConfig,
		config,
		dhan.OptionChainClient{},
		dhanAccessToken,
		dhanClientID,
		registry,
		resolver,
		dhanQuoteKeys,
		func(err error) {
			metrics.ObserveError(dhan.ProviderName)
			log.Printf("Dhan dynamic option mapping: %v", err)
		},
	)
	if err != nil {
		return err
	}
	go dynamicMapper.Run(ctx)
	dhanRecovery := &dhan.Recovery{
		Client:      dhan.HistoryClient{},
		AccessToken: dhanAccessToken,
		Registry:    registry,
		History:     store,
		Timeframes:  marketConfig.RecoverableTimeframes(),
	}
	recoverer := resilience.RecoveryFunc(func(ctx context.Context, gap resilience.Gap) error {
		switch gap.Provider {
		case dhan.ProviderName:
			return dhanRecovery.Recover(ctx, gap.InstrumentID, gap.From, gap.To)
		case upstox.ProviderName:
			if upstoxRecovery == nil {
				return errors.New("Upstox recovery is unavailable")
			}
			mapping, ok := registry.ProviderMapping(upstox.ProviderName, gap.InstrumentID)
			if !ok {
				return errors.New("Upstox recovery mapping is not registered")
			}
			return upstoxRecovery.Recover(ctx, upstox.RecoveryRequest{
				Provider:       upstox.ProviderName,
				InstrumentKeys: []string{mapping.ProviderKey},
				From:           gap.From,
				To:             gap.To,
				Cause:          "authority_failover",
			})
		default:
			return fmt.Errorf("unsupported recovery authority %q", gap.Provider)
		}
	})

	router, err := resilience.NewRouterWithPolicy(
		resolver,
		downstream,
		metrics,
		config.GapRecovery(),
		recoverable,
		recoverer,
		config.TransitionPolicy(upstox.ProviderName),
	)
	if err != nil {
		return err
	}
	router.SetTransitionSink(resilience.NewTransitionStore(env("QNEXT_STORAGE_ROOT", "./storage")).Append)
	onTick := func(tick domain.Tick) error {
		if marketConfig.AutoLegsEnabled() && tick.Provider == upstox.ProviderName {
			if _, staticallyManaged := policy[tick.InstrumentID]; !staticallyManaged {
				// Preserve the live Upstox path while Dhan security-id discovery runs
				// asynchronously. Once mapped, this exact canonical option leg joins
				// the same authority router used by underlyings and fixed legs.
				dynamicMapper.Observe(tick.InstrumentID)
				if !dynamicMapper.Covered(tick.InstrumentID) {
					return downstream(tick)
				}
			}
		}
		return router.HandleContext(ctx, tick)
	}

	var dhanSequence atomic.Uint64
	dhanPoller := &dhan.Poller{
		Client:       dhan.QuoteClient{},
		Registry:     registry,
		AccessToken:  dhanAccessToken,
		ClientID:     dhanClientID,
		Keys:         dhanKeys,
		KeysSnapshot: dhanQuoteKeys.Snapshot,
		Interval:     config.DhanPollInterval(),
		NextSequence: func() uint64 { return dhanSequence.Add(1) },
		OnError: func(err error) {
			metrics.ObserveError(dhan.ProviderName)
			log.Printf("Dhan standby feed error: %v", err)
		},
	}

	// In Q3, authority-switch recovery owns reconciliation. The Upstox
	// supervisor therefore reconnects without independently writing recovery
	// bars, avoiding duplicate provider recoveries for the same outage window.
	upstoxSupervisor := &upstox.Supervisor{
		Runner:          upstoxRunner,
		RequestSnapshot: upstoxRequestSnapshot,
	}

	go runProviderLoop(ctx, upstox.ProviderName, metrics, func() error {
		return upstoxSupervisor.Run(ctx, upstoxAccessToken, upstoxRequest, onTick)
	})
	go runProviderLoop(ctx, dhan.ProviderName, metrics, func() error {
		return dhanPoller.Run(ctx, onTick)
	})

	<-ctx.Done()
	return ctx.Err()
}

func runProviderLoop(ctx context.Context, provider string, metrics *resilience.Metrics, run func() error) {
	for ctx.Err() == nil {
		err := run()
		if ctx.Err() != nil {
			return
		}
		metrics.ObserveError(provider)
		log.Printf("%s provider loop exited: %v", provider, err)

		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
