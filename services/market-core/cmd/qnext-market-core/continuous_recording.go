package main

import (
	"log"
	"os"
	"strings"

	"github.com/indigiti/QNext/services/market-core/internal/demand"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

// initContinuousSyntheticRecording reserves one demand reference for every
// enabled auto-leg synthetic. This is deliberately independent of browser
// demand: every configured INDEX-SYN must keep producing canonical 5s history
// throughout the trading session so 15s/30s can be reconstructed later.
// Acquire intentionally happens before managers register; the demand registry
// retains the reference and activates the target as soon as autolegs.New
// registers it. Browser/strategy refs remain additive.
func init() {
	if strings.TrimSpace(os.Getenv("QNEXT_CONTINUOUS_SYNTHETIC_RECORDING")) == "0" {
		return
	}
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
