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

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/httpapi"
	"github.com/indigiti/QNext/services/market-core/internal/integrity"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
	"github.com/indigiti/QNext/services/market-core/internal/pipeline"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
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

	addr := env("QNEXT_HTTP_ADDR", ":8080")
	storageRoot := env("QNEXT_STORAGE_ROOT", "./storage")
	started := time.Now().UTC()

	store := history.New(storageRoot)
	broker := stream.NewBroker(1024, 128)
	handler := httpapi.New(store, httpapi.Options{
		Version:       version,
		Commit:        commit,
		StartedAt:     started,
		StreamHandler: stream.NewWebSocketHandler(broker),
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

	if configPath := strings.TrimSpace(os.Getenv("QNEXT_MARKET_CONFIG")); configPath != "" {
		accessToken := strings.TrimSpace(os.Getenv("UPSTOX_ACCESS_TOKEN"))
		if accessToken == "" {
			log.Fatal("UPSTOX_ACCESS_TOKEN is required when QNEXT_MARKET_CONFIG is set")
		}
		config, err := marketconfig.Load(configPath)
		if err != nil {
			log.Fatalf("load QNext market config: %v", err)
		}

		go func() {
			if err := runMarket(ctx, config, accessToken, store, broker); err != nil && ctx.Err() == nil {
				errCh <- err
			}
		}()
	} else {
		log.Printf("QNEXT_MARKET_CONFIG is not set; HTTP/history/stream services are running without live provider ingestion")
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

func runMarket(
	ctx context.Context,
	config marketconfig.Config,
	accessToken string,
	store *history.Store,
	broker *stream.Broker,
) error {
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID:         config.Nifty.InstrumentID,
		Symbol:     "NIFTY",
		Name:       "Nifty 50",
		AssetClass: "INDEX",
		Exchange:   "NSE",
		Currency:   "INR",
		Timezone:   "Asia/Kolkata",
	}); err != nil {
		return err
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider:     upstox.ProviderName,
		InstrumentID: config.Nifty.InstrumentID,
		ProviderKey:  config.Nifty.ProviderKey,
	}); err != nil {
		return err
	}

	legs := make([]synthetic.LegBinding, 0, len(config.Synthetic.Legs))
	for _, leg := range config.Synthetic.Legs {
		if err := registry.Register(symbol.Instrument{
			ID:         leg.InstrumentID,
			Symbol:     leg.InstrumentID,
			Name:       leg.InstrumentID,
			AssetClass: "OPTION",
			Exchange:   "NSE",
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
		}); err != nil {
			return err
		}
		if err := registry.RegisterProvider(symbol.ProviderInstrument{
			Provider:     upstox.ProviderName,
			InstrumentID: leg.InstrumentID,
			ProviderKey:  leg.ProviderKey,
		}); err != nil {
			return err
		}

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

	canonicalPipeline, err := pipeline.New(candle.New("candle-v1"), store, config.Timeframes)
	if err != nil {
		return err
	}

	var syntheticSequence atomic.Uint64
	assembler, err := synthetic.NewAssembler(synthetic.Definition{
		ID:                     config.Synthetic.InstrumentID,
		Version:                config.Synthetic.Version,
		MinimumValidCandidates: config.Synthetic.MinimumValidCandidates,
		MaxLegAge:              time.Duration(config.Synthetic.MaxLegAgeMS) * time.Millisecond,
		MaxLegTimeSkew:         time.Duration(config.Synthetic.MaxLegTimeSkewMS) * time.Millisecond,
	}, legs, func() uint64 {
		return syntheticSequence.Add(1)
	})
	if err != nil {
		return err
	}

	marketSink := &qruntime.MarketSink{
		Pipeline:  canonicalPipeline,
		Synthetic: assembler,
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
		Authorizer: upstox.Authorizer{},
		Dialer:     upstox.GorillaDialer{},
		Decoder:    upstox.ProtobufDecoder{},
		Normalizer: normalizer,
		NextSequence: func() uint64 {
			return providerSequence.Add(1)
		},
	}

	recovery := &upstox.IntradayRecovery{
		Client:        upstox.IntradayClient{},
		AccessToken:   accessToken,
		Registry:      registry,
		History:       store,
		Timeframes:    config.RecoverableTimeframes(),
		InstrumentIDs: map[string]bool{config.Nifty.InstrumentID: true},
	}

	supervisor := &upstox.Supervisor{
		Runner:   wire,
		Recovery: recovery,
	}

	return supervisor.Run(ctx, accessToken, upstox.SubscriptionRequest{
		GUID:   "qnext-market-core",
		Method: upstox.MethodSubscribe,
		Data: upstox.SubscriptionData{
			Mode:           upstox.ModeLTPC,
			InstrumentKeys: config.ProviderKeys(),
		},
	}, dedupe.Handle)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
