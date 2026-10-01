package main

import (
	"log"
	"os"
	"strings"

	"github.com/indigiti/QNext/services/market-core/internal/demand"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

// initContinuousSyntheticRecording reserves one permanent demand reference for
// every enabled auto-leg synthetic whenever live market config is present.
// This is deliberately independent of browser demand: every configured
// INDEX-SYN must keep producing canonical 5s history throughout the trading
// session so exact 15s/30s history exists before any user opens a chart.
// Acquire intentionally happens before managers register; the demand registry
// retains the reference and activates the target as soon as autolegs.New
// registers it. Browser/strategy refs remain additive, but cannot turn the
// recorder off.
func init() {
	configPath := strings.TrimSpace(os.Getenv("QNEXT_MARKET_CONFIG"))
	if configPath == "" {
		return
	}
	config, err := marketconfig.Load(configPath)
	if err != nil {
		log.Printf("continuous synthetic recorder config unavailable: %v", err)
		return
	}
	for _, instrumentID := range continuousSyntheticInstrumentIDs(config) {
		demand.Default().Acquire(instrumentID)
	}
}

func continuousSyntheticInstrumentIDs(config marketconfig.Config) []string {
	markets := config.EffectiveMarkets()
	result := make([]string, 0, len(markets))
	for _, market := range markets {
		instrumentID := strings.TrimSpace(market.Synthetic.InstrumentID)
		if market.Synthetic.Auto == nil || instrumentID == "" {
			continue
		}
		result = append(result, instrumentID)
	}
	return result
}
