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
	handler := httpapi.New(store, httpapi.Options{
		Version:       version,
		Commit:        commit,
		StartedAt:     started,
		StreamHandler: stream.NewWebSocketHandler(broker),
		Symbols:       registry,
		Calendars:     calendars,
		ResilienceStatus: func() any {
			return resilienceMetrics.Snapshot()
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
	niftyID := "NSE:NIFTY50"
	syntheticID := "QNEXT:NIFTY-SYN"
	if config != nil {
		niftyID = config.Nifty.InstrumentID
		syntheticID = config.Synthetic.InstrumentID
	}

	if err := registry.Register(symbol.Instrument{
		ID:         niftyID,
		Symbol:     "NIFTY",
		Name:       "Nifty 50",
		AssetClass: "INDEX",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
		CalendarID: "NSE_EQ",
		Aliases:    []string{"NIFTY 50"},
		Visible:    true,
	}); err != nil {
		return nil, err
	}
	if err := registry.Register(symbol.Instrument{
		ID:         syntheticID,
		Symbol:     "NIFTY-SYN",
		Name:       "QNext Nifty Synthetic",
		AssetClass: "INDEX",
		Exchange:   "QNEXT",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
		CalendarID: "NSE_EQ",
		Aliases:    []string{"NIFTY SYN", "SYNTHETIC NIFTY"},
		Synthetic:  true,
		Visible:    true,
	}); err != nil {
		return nil, err
	}

	if config == nil {
		return registry, nil
	}

	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider:     upstox.ProviderName,
		InstrumentID: config.Nifty.InstrumentID,
		ProviderKey:  config.Nifty.ProviderKey,
	}); err != nil {
		return nil, err
	}

	for _, leg := range config.Synthetic.Legs {
		if err := registry.Register(symbol.Instrument{
			ID:         leg.InstrumentID,
			Symbol:     leg.InstrumentID,
			Name:       leg.InstrumentID,
			AssetClass: "OPTION",
			Exchange:   "NSE",
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
			CalendarID: "NSE_EQ",
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
) error {
	canonicalPipeline, err := pipeline.New(candle.New("candle-v1"), store, config.Timeframes)
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

	var syntheticEngine qruntime.SyntheticAssembler
	if config.AutoLegsEnabled() {
		auto := config.Synthetic.Auto
		manager, managerErr := autolegs.New(autolegs.Config{
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
			log.Printf("NIFTY-SYN auto-leg manager: %v", err)
		})
		if managerErr != nil {
			return managerErr
		}
		syntheticEngine = manager
		go func() {
			if runErr := manager.Run(ctx); runErr != nil && ctx.Err() == nil {
				log.Printf("NIFTY-SYN auto-leg manager stopped: %v", runErr)
			}
		}()
	} else {
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
		assembler, assemblerErr := synthetic.NewAssembler(synthetic.Definition{
			ID:                     config.Synthetic.InstrumentID,
			Version:                config.Synthetic.Version,
			MinimumValidCandidates: config.Synthetic.MinimumValidCandidates,
			MaxLegAge:              time.Duration(config.Synthetic.MaxLegAgeMS) * time.Millisecond,
			MaxLegTimeSkew:         time.Duration(config.Synthetic.MaxLegTimeSkewMS) * time.Millisecond,
		}, legs, nextSyntheticSequence)
		if assemblerErr != nil {
			return assemblerErr
		}
		syntheticEngine = assembler
	}

	marketSink := &qruntime.MarketSink{
		Pipeline:  canonicalPipeline,
		Synthetic: syntheticEngine,
		Publisher: broker,
		DirectInstruments: map[string]bool{
			config.Nifty.InstrumentID: true,
		},
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
		InstrumentIDs: map[string]bool{config.Nifty.InstrumentID: true},
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

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
