package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/capture"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/research"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	accessToken := strings.TrimSpace(os.Getenv("UPSTOX_ACCESS_TOKEN"))
	if accessToken == "" {
		log.Fatal("UPSTOX_ACCESS_TOKEN is required")
	}
	market, err := configuredMarket()
	if err != nil {
		log.Fatal(err)
	}
	storageRoot := env("QNEXT_STORAGE_ROOT", "./storage")
	collector, err := research.NewOptionsCollector(storageRoot)
	if err != nil {
		log.Fatalf("create research collector: %v", err)
	}
	defer func() {
		if err := collector.Close(); err != nil {
			log.Printf("close research collector: %v", err)
		}
	}()

	synPlus, err := research.NewSynPlusCollector(storageRoot, research.SynPlusConfig{
		InstrumentID:           env("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:"+market.Symbol+"-SYN+"),
		Version:                env("QNEXT_SYN_PLUS_VERSION", strings.ToLower(market.Symbol)+"-syn-plus-v1"),
		MinimumValidCandidates: market.Synthetic.MinimumValidCandidates,
		MaxLegAge:              time.Duration(market.Synthetic.MaxLegAgeMS) * time.Millisecond,
		MaxLegTimeSkew:         time.Duration(market.Synthetic.MaxLegTimeSkewMS) * time.Millisecond,
	})
	if err != nil {
		log.Fatalf("create SYN+ collector: %v", err)
	}
	defer func() {
		if err := synPlus.Close(); err != nil {
			log.Printf("close SYN+ collector: %v", err)
		}
	}()

	runner := &upstox.ResearchRunner{
		Authorizer:      upstox.Authorizer{},
		Dialer:          upstox.GorillaDialer{},
		Decoder:         upstox.ProtobufDecoder{},
		Contracts:       upstox.OptionContractsClient{},
		GUID:            "qnext-data-collector",
		WingStrikes:     envInt("QNEXT_DATA_COLLECTOR_WING_STRIKES", 20),
		FullWingStrikes: envInt("QNEXT_DATA_COLLECTOR_FULL_WING_STRIKES", 5),
		OnError: func(err error) {
			log.Printf("qnext-data-collector reconnecting after error: %v", err)
		},
	}
	if strings.TrimSpace(os.Getenv("QNEXT_DATA_COLLECTOR_RAW_CAPTURE")) == "1" {
		runner.CaptureFrame = capture.NewFrameStore(storageRoot, "upstox-research").Append
	}

	log.Printf(
		"qnext-data-collector starting market=%s underlying=%s wing=%d full_wing=%d syn_plus=%s",
		market.Symbol,
		market.Underlying.ProviderKey,
		runner.WingStrikes,
		runner.FullWingStrikes,
		env("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:"+market.Symbol+"-SYN+"),
	)
	sinks := research.MultiSink{collector, synPlus}
	if err := runner.Run(ctx, accessToken, market.Underlying.ProviderKey, sinks); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("qnext-data-collector stopped: %v", err)
	}
}

func configuredMarket() (marketconfig.MarketConfig, error) {
	markets := marketconfig.DefaultMarkets()
	if path := strings.TrimSpace(os.Getenv("QNEXT_MARKET_CONFIG")); path != "" {
		config, err := marketconfig.Load(path)
		if err != nil {
			return marketconfig.MarketConfig{}, err
		}
		markets = config.EffectiveMarkets()
	}
	target := strings.ToUpper(strings.TrimSpace(env("QNEXT_DATA_COLLECTOR_MARKET", "NIFTY")))
	for _, market := range markets {
		if strings.ToUpper(strings.TrimSpace(market.Symbol)) == target {
			return market, nil
		}
	}
	return marketconfig.MarketConfig{}, errors.New("QNEXT_DATA_COLLECTOR_MARKET is not configured")
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}
