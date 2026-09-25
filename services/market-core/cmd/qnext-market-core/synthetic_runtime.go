package main

import (
	"context"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/autolegs"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	qruntime "github.com/indigiti/QNext/services/market-core/internal/runtime"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
	"github.com/indigiti/QNext/services/market-core/internal/synthetic"
)

type syntheticRuntime struct {
	Assembler     qruntime.SyntheticAssembler
	Subscriptions *upstox.SubscriptionState
}

func buildSyntheticRuntime(
	ctx context.Context,
	config marketconfig.Config,
	accessToken string,
	registry *symbol.Registry,
) (*syntheticRuntime, error) {
	subscriptions, err := upstox.NewSubscriptionState(
		"qnext-market-core",
		upstox.ModeLTPC,
		config.ProviderKeys(),
	)
	if err != nil {
		return nil, err
	}

	var syntheticSequence atomic.Uint64
	nextSyntheticSequence := func() uint64 {
		return syntheticSequence.Add(1)
	}

	if config.AutoLegsEnabled() {
		auto := config.Synthetic.Auto
		manager, err := autolegs.New(autolegs.Config{
			UnderlyingInstrumentID: config.Nifty.InstrumentID,
			SyntheticInstrumentID:  config.Synthetic.InstrumentID,
			Version:                config.Synthetic.Version,
			StrikeInterval:         auto.StrikeInterval,
			ActiveStrikes:          auto.ActiveStrikes,
			WarmStrikes:            auto.WarmStrikes,
			HysteresisPoints:       auto.ATMHysteresisPoints,
			Confirmation:           time.Duration(auto.ATMConfirmationMS) * time.Millisecond,
			MinimumValidCandidates: config.Synthetic.MinimumValidCandidates,
			MaxLegAge:              time.Duration(config.Synthetic.MaxLegAgeMS) * time.Millisecond,
			MaxLegTimeSkew:         time.Duration(config.Synthetic.MaxLegTimeSkewMS) * time.Millisecond,
		}, upstox.OptionLegResolver{
			Client:        upstox.OptionContractsClient{},
			AccessToken:   accessToken,
			UnderlyingKey: config.Nifty.ProviderKey,
			Registry:      registry,
		}, subscriptions, nextSyntheticSequence, func(err error) {
			log.Printf("NIFTY-SYN auto leg manager: %v", err)
		})
		if err != nil {
			return nil, err
		}

		go func() {
			if err := manager.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("NIFTY-SYN auto leg manager stopped: %v", err)
			}
		}()

		return &syntheticRuntime{
			Assembler:     manager,
			Subscriptions: subscriptions,
		}, nil
	}

	legs := make([]synthetic.LegBinding, 0, len(config.Synthetic.Legs))
	for _, leg := range config.Synthetic.Legs {
		side := synthetic.LegCall
		if strings.EqualFold(leg.Side, "PUT") {
			side = synthetic.LegPut
		}
		legs = append(legs, synthetic.LegBinding{
			InstrumentID: leg.InstrumentID,
			Strike:       leg.Strike,
			Side:         side,
		})
	}

	assembler, err := synthetic.NewAssembler(synthetic.Definition{
		ID:                     config.Synthetic.InstrumentID,
		Version:                config.Synthetic.Version,
		MinimumValidCandidates: config.Synthetic.MinimumValidCandidates,
		MaxLegAge:              time.Duration(config.Synthetic.MaxLegAgeMS) * time.Millisecond,
		MaxLegTimeSkew:         time.Duration(config.Synthetic.MaxLegTimeSkewMS) * time.Millisecond,
	}, legs, nextSyntheticSequence)
	if err != nil {
		return nil, err
	}

	return &syntheticRuntime{
		Assembler:     assembler,
		Subscriptions: subscriptions,
	}, nil
}
