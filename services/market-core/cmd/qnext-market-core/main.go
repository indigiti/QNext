package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/autolegs"
	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/capture"
	"github.com/indigiti/QNext/services/market-core/internal/feedstatus"
	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/httpapi"
	"github.com/indigiti/QNext/services/market-core/internal/integrity"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
	"github.com/indigiti/QNext/services/market-core/internal/pipeline"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/resilience"
	qruntime "github.com/indigiti/QNext/services/market-core/internal/runtime"
	"github.com/indigiti/QNext/services/market-core/internal/stream"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
	"github.com/indigiti/QNext/services/market-core/internal/synthetic"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := env("QNEXT_HTTP_ADDR", "127.0.0.1:18080")
	storageRoot := env("QNEXT_STORAGE_ROOT", "./storage")
	started := time.Now().UTC()

	var config *marketconfig.Config
	var accessToken string
	if configPath := strings.TrimSpace(os.Getenv("QNEXT_MARKET_CONFIG")); configPath != "" {
		loaded, err := marketconfig.Load(configPath)
		if err != nil {
			log.Fatalf("load QNext market config: %v", err)
		}
		config = &loaded
		accessToken = strings.TrimSpace(os.Getenv("UPSTOX_ACCESS_TOKEN"))
		if accessToken == "" {
			log.Fatal("UPSTOX_ACCESS_TOKEN is required when QNEXT_MARKET_CONFIG is set")
		}
	}

	var gapRecoveryTracker *upstox.GapRecoveryTracker
	if config != nil {
		gapRecoveryTracker = upstox.NewGapRecoveryTracker(
			config.RecoverableTimeframes(),
			nonExactRecoveryTimeframes(config.Timeframes),
		)
	} else {
		gapRecoveryTracker = upstox.NewGapRecoveryTracker(nil, nil)
	}

	registry, err := buildRegistry(config)
	if err != nil {
		log.Fatalf("build symbol registry: %v", err)
	}
	calendars := marketcalendar.DefaultRegistry()

	resilienceMetrics := resilience.NewMetrics()
	var resilienceConfig *resilience.Config
	var dhanClientID string
	var dhanAccessToken string
	if resiliencePath := strings.TrimSpace(os.Getenv("QNEXT_RESILIENCE_CONFIG")); resiliencePath != "" {
		if config == nil {
			log.Fatal("QNEXT_MARKET_CONFIG is required when QNEXT_RESILIENCE_CONFIG is set")
		}
		loaded, err := resilience.Load(resiliencePath)
		if err != nil {
			log.Fatalf("load QNext resilience config: %v", err)
		}
		if err := registerDhanMappings(*config, loaded, registry); err != nil {
			log.Fatalf("register Dhan resilience mappings: %v", err)
		}
		dhanClientID = strings.TrimSpace(os.Getenv("DHAN_CLIENT_ID"))
		dhanAccessToken = strings.TrimSpace(os.Getenv("DHAN_ACCESS_TOKEN"))
		if dhanClientID == "" || dhanAccessToken == "" {
			log.Fatal("DHAN_CLIENT_ID and DHAN_ACCESS_TOKEN are required when QNEXT_RESILIENCE_CONFIG is set")
		}
		resilienceConfig = &loaded
	}

	store := history.New(storageRoot)
	broker := stream.NewBroker(1024, 128)
	feedTracker := feedstatus.New()

	var historicalRepairer *upstox.HistoricalRepairer
	if config != nil {
		historicalRepairer = &upstox.HistoricalRepairer{
			Client: upstox.HistoricalRangeClient{
				Intraday:       upstox.IntradayClient{},
				MarketTimezone: "Asia/Kolkata",
			},
			AccessToken: accessToken,
			History:     store,
			Calendars:   calendars,
			Markets:     config.EffectiveMarkets(),
			Timeframes:  append([]string(nil), config.Timeframes...),
			Resync:      broker,
		}
	}

	enabledTimeframes := marketconfig.DefaultChartTimeframes()
	if config != nil {
		enabledTimeframes = config.EffectiveChartTimeframes()
	}

	handler := httpapi.New(store, httpapi.Options{
		Version:           version,
		Commit:            commit,
		StartedAt:         started,
		StreamHandler:     stream.NewWebSocketHandler(broker),
		EnabledTimeframes: enabledTimeframes,
		LiveBars:          broker,
		Symbols:           registry,
		Calendars:         calendars,
		ResilienceStatus: func() any {
			return resilienceMetrics.Snapshot()
		},
		HistoricalRepair: func(
			ctx context.Context,
			days int,
			markets []string,
			reason string,
		) (any, error) {
			if historicalRepairer == nil {
				return nil, errors.New("historical repair is unavailable without live market configuration")
			}
			return historicalRepairer.Repair(ctx, upstox.HistoricalRepairRequest{
				Days:    days,
				Markets: markets,
				Reason:  reason,
			})
		},
		HistoricalRepairStatus: func() any {
			if historicalRepairer == nil {
				return upstox.HistoricalRepairStatus{}
			}
			return historicalRepairer.Status()
		},
		FeedStatus: func() any {
			snapshot := feedTracker.Snapshot()
			markets := marketconfig.DefaultMarkets()
			if config != nil {
				markets = config.EffectiveMarkets()
			}

			niftyID := "NSE:NIFTY50"
			syntheticID := "QNEXT:NIFTY-SYN"
			marketStatus := make([]map[string]any, 0, len(markets))
			for _, market := range markets {
				marketStatus = append(marketStatus, map[string]any{
					"symbol":                   market.Symbol,
					"underlying_instrument_id": market.Underlying.InstrumentID,
					"synthetic_instrument_id":  market.Synthetic.InstrumentID,
					"exchange":                 market.Exchange,
				})
				if strings.EqualFold(market.Symbol, "NIFTY") {
					niftyID = market.Underlying.InstrumentID
					syntheticID = market.Synthetic.InstrumentID
				}
			}

			return map[string]any{
				"live_configured":         config != nil,
				"resilience_configured":   resilienceConfig != nil,
				"nifty_instrument_id":     niftyID,
				"synthetic_instrument_id": syntheticID,
				"markets":                 marketStatus,
				"telemetry":               snapshot,
				"resilience":              resilienceMetrics.Snapshot(),
				"gap_recovery":            gapRecoveryTracker.Snapshot(),
			}
		},
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("qnext-market-core listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if historicalRepairer != nil {
		go func() {
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			_, err := historicalRepairer.Repair(ctx, upstox.HistoricalRepairRequest{
				Days:   3,
				Reason: "startup_reconciliation",
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("startup historical repair: %v", err)
			}
		}()
	}

	if config != nil {
		go func() {
			if err := runMarket(
				ctx,
				*config,
				accessToken,
				registry,
				store,
				broker,
				resilienceConfig,
				dhanClientID,
				dhanAccessToken,
				resilienceMetrics,
				feedTracker,
				gapRecoveryTracker,
			); err != nil && ctx.Err() == nil {
				errCh <- err
			}
		}()
	} else {
		log.Printf("QNEXT_MARKET_CONFIG is not set; workspace catalog and HTTP/history/stream services are running without live provider ingestion")
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		log.Printf("qnext-market-core fatal runtime error: %v", err)
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
}

func buildRegistry(config *marketconfig.Config) (*symbol.Registry, error) {
	registry := symbol.NewRegistry()
	markets := marketconfig.DefaultMarkets()
	if config != nil {
		markets = config.EffectiveMarkets()
	}

	for _, market := range markets {
		if err := registry.Register(symbol.Instrument{
			ID:         market.Underlying.InstrumentID,
			Symbol:     market.Symbol,
			Name:       market.Name,
			AssetClass: "INDEX",
			Exchange:   market.Exchange,
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
			CalendarID: market.CalendarID,
			Aliases:    market.Aliases,
			Visible:    true,
		}); err != nil {
			return nil, err
		}

		if err := registry.Register(symbol.Instrument{
			ID:         market.Synthetic.InstrumentID,
			Symbol:     market.Symbol + "-SYN",
			Name:       "QNext " + market.Name + " Synthetic",
			AssetClass: "INDEX",
			Exchange:   "QNEXT",
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
			CalendarID: market.CalendarID,
			Aliases:    []string{market.Symbol + " SYN", "SYNTHETIC " + market.Symbol},
			Synthetic:  true,
			Visible:    true,
		}); err != nil {
			return nil, err
		}

		if err := registry.RegisterProvider(symbol.ProviderInstrument{
			Provider:     upstox.ProviderName,
			InstrumentID: market.Underlying.InstrumentID,
			ProviderKey:  market.Underlying.ProviderKey,
		}); err != nil {
			return nil, err
		}

		for _, leg := range market.Synthetic.Legs {
			if err := registry.Register(symbol.Instrument{
				ID:         leg.InstrumentID,
				Symbol:     leg.InstrumentID,
				Name:       leg.InstrumentID,
				AssetClass: "OPTION",
				Exchange:   market.Exchange,
				Currency:   "INR",
				Timezone:   "Asia/Kolkata",
				CalendarID: market.CalendarID,
				Visible:    false,
			}); err != nil {
				return nil, err
			}
			if err := registry.RegisterProvider(symbol.ProviderInstrument{
				Provider:     upstox.ProviderName,
				InstrumentID: leg.InstrumentID,
				ProviderKey:  leg.ProviderKey,
			}); err != nil {
				return nil, err
			}
		}
	}

	return registry, nil
}

func runMarket(
	ctx context.Context,
	config marketconfig.Config,
	accessToken string,
	registry *symbol.Registry,
	store *history.Store,
	broker *stream.Broker,
	resilienceConfig *resilience.Config,
	dhanClientID string,
	dhanAccessToken string,
	resilienceMetrics *resilience.Metrics,
	feedTracker *feedstatus.Tracker,
	gapRecoveryTracker *upstox.GapRecoveryTracker,
) error {
	canonicalPipeline, err := pipeline.New(
		candle.New("candle-v2-session-aligned"),
		store,
		config.Timeframes,
	)
	if err != nil {
		return err
	}

	subscriptions, err := upstox.NewSubscriptionState(
		"qnext-market-core",
		upstox.ModeLTPC,
		config.ProviderKeys(),
	)
	if err != nil {
		return err
	}

	var syntheticSequence atomic.Uint64
	nextSyntheticSequence := func() uint64 {
		return syntheticSequence.Add(1)
	}

	markets := config.EffectiveMarkets()
	syntheticEngines := make([]qruntime.SyntheticAssembler, 0, len(markets))
	directInstruments := make(map[string]bool, len(markets))

	for index, market := range markets {
		directInstruments[market.Underlying.InstrumentID] = true
		syntheticConfig := market.Synthetic

		if syntheticConfig.Auto != nil {
			auto := syntheticConfig.Auto
			manager, managerErr := autolegs.New(autolegs.Config{
				UnderlyingInstrumentID: market.Underlying.InstrumentID,
				SyntheticInstrumentID:  syntheticConfig.InstrumentID,
				Version:                syntheticConfig.Version,
				StrikeInterval:         auto.StrikeInterval,
				ActiveStrikes:          auto.ActiveStrikes,
				WarmStrikes:            auto.WarmStrikes,
				HysteresisPoints:       auto.ATMHysteresisPoints,
				Confirmation:           time.Duration(auto.ATMConfirmationMS) * time.Millisecond,
				MinimumValidCandidates: syntheticConfig.MinimumValidCandidates,
				MaxLegAge:              time.Duration(syntheticConfig.MaxLegAgeMS) * time.Millisecond,
				MaxLegTimeSkew:         time.Duration(syntheticConfig.MaxLegTimeSkewMS) * time.Millisecond,
			}, upstox.OptionLegResolver{
				Client:        upstox.OptionContractsClient{},
				AccessToken:   accessToken,
				UnderlyingKey: market.Underlying.ProviderKey,
				Registry:      registry,
			}, subscriptions, nextSyntheticSequence, func(err error) {
				log.Printf("%s auto-leg manager: %v", syntheticConfig.InstrumentID, err)
			})
			if managerErr != nil {
				return managerErr
			}
			syntheticEngines = append(syntheticEngines, manager)

			if feedTracker != nil {
				managerRef := manager
				feedTracker.SetSyntheticStatusFor(
					syntheticConfig.InstrumentID,
					func() any { return managerRef.Status() },
				)
				if index == 0 || strings.EqualFold(market.Symbol, "NIFTY") {
					feedTracker.SetSyntheticStatus(func() any { return managerRef.Status() })
				}
			}

			managerRef := manager
			syntheticID := syntheticConfig.InstrumentID
			go func() {
				if runErr := managerRef.Run(ctx); runErr != nil && ctx.Err() == nil {
					log.Printf("%s auto-leg manager stopped: %v", syntheticID, runErr)
				}
			}()
			continue
		}

		legs := make([]synthetic.LegBinding, 0, len(syntheticConfig.Legs))
		for _, leg := range syntheticConfig.Legs {
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
		assembler, assemblerErr := synthetic.NewAssembler(synthetic.Definition{
			ID:                     syntheticConfig.InstrumentID,
			Version:                syntheticConfig.Version,
			MinimumValidCandidates: syntheticConfig.MinimumValidCandidates,
			MaxLegAge:              time.Duration(syntheticConfig.MaxLegAgeMS) * time.Millisecond,
			MaxLegTimeSkew:         time.Duration(syntheticConfig.MaxLegTimeSkewMS) * time.Millisecond,
		}, legs, nextSyntheticSequence)
		if assemblerErr != nil {
			return assemblerErr
		}
		syntheticEngines = append(syntheticEngines, assembler)
	}

	marketSink := &qruntime.MarketSink{
		Pipeline:          canonicalPipeline,
		Synthetics:        syntheticEngines,
		Publisher:         broker,
		DirectInstruments: directInstruments,
		Observer:          feedTracker.Observe,
	}
	dedupe := integrity.NewDedupeSink(8192, marketSink.Handle)

	normalizer, err := upstox.NewNormalizer(registry)
	if err != nil {
		return err
	}
	var providerSequence atomic.Uint64
	wire := &upstox.WireClient{
		Authorizer:          upstox.Authorizer{},
		Dialer:              upstox.GorillaDialer{},
		Decoder:             upstox.ProtobufDecoder{},
		Normalizer:          normalizer,
		SubscriptionUpdates: subscriptions.Updates(),
		NextSequence: func() uint64 {
			return providerSequence.Add(1)
		},
		InactivityTimeout: 20 * time.Second,
		WatchdogInterval:  5 * time.Second,
		WatchdogActive:    regularMarketSessionActive,
	}
	if strings.TrimSpace(os.Getenv("QNEXT_RAW_CAPTURE")) == "1" {
		wire.CaptureFrame = capture.NewFrameStore(env("QNEXT_STORAGE_ROOT", "./storage"), upstox.ProviderName).Append
	}

	recovery := &upstox.IntradayRecovery{
		Client:        upstox.IntradayClient{},
		AccessToken:   accessToken,
		Registry:      registry,
		History:       store,
		Timeframes:    config.RecoverableTimeframes(),
		InstrumentIDs: directInstruments,
		Resync:        broker,
		Status:        gapRecoveryTracker,
	}

	request := subscriptions.Snapshot()
	if resilienceConfig == nil {
		supervisor := &upstox.Supervisor{
			Runner:          wire,
			Recovery:        recovery,
			RequestSnapshot: subscriptions.Snapshot,
		}
		return supervisor.Run(ctx, accessToken, request, dedupe.Handle)
	}

	return runResilientMarket(
		ctx,
		config,
		*resilienceConfig,
		accessToken,
		dhanClientID,
		dhanAccessToken,
		registry,
		store,
		wire,
		recovery,
		request,
		subscriptions.Snapshot,
		dedupe.Handle,
		resilienceMetrics,
	)
}

func nonExactRecoveryTimeframes(timeframes []string) []string {
	recoverable := map[string]bool{"1m": true}
	result := make([]string, 0, len(timeframes))
	for _, timeframe := range timeframes {
		if !recoverable[timeframe] {
			result = append(result, timeframe)
		}
	}
	return result
}

func regularMarketSessionActive(at time.Time) bool {
	definition := marketcalendar.NSEEquities2026()
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return false
	}
	local := at.In(location)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	if _, closed := definition.ClosedDates[local.Format("2006-01-02")]; closed {
		return false
	}

	open := time.Date(
		local.Year(), local.Month(), local.Day(),
		definition.RegularOpen.Hour, definition.RegularOpen.Minute,
		0, 0, location,
	)
	closeAt := time.Date(
		local.Year(), local.Month(), local.Day(),
		definition.RegularClose.Hour, definition.RegularClose.Minute,
		0, 0, location,
	)
	return !local.Before(open) && local.Before(closeAt)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
