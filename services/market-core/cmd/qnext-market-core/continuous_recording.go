package main

import (
	"log"
	"os"
	"strings"

	"github.com/indigiti/QNext/services/market-core/internal/demand"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

// initContinuousSyntheticRecording reserves one demand reference for every
// enabled auto-leg synthetic. Acquire intentionally happens before managers
// register: the demand registry retains the reference and activates the target
// as soon as autolegs.New registers it. Browser/strategy refs remain additive.
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
	for _, market := range config.EffectiveMarkets() {
		if market.Synthetic.Auto == nil || strings.TrimSpace(market.Synthetic.InstrumentID) == "" {
			continue
		}
		demand.Default().Acquire(market.Synthetic.InstrumentID)
	}
}
