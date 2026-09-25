package marketconfig

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
)

type Config struct {
	Timeframes []string       `json:"timeframes"`
	Markets    []MarketConfig `json:"markets,omitempty"`

	// Legacy single-market fields are retained for backwards compatibility
	// with already-deployed q1-market.json files.
	Nifty     Instrument      `json:"nifty,omitempty"`
	Synthetic SyntheticConfig `json:"synthetic,omitempty"`
}

type MarketConfig struct {
	Symbol     string          `json:"symbol"`
	Name       string          `json:"name"`
	Exchange   string          `json:"exchange"`
	CalendarID string          `json:"calendar_id"`
	Aliases    []string        `json:"aliases,omitempty"`
	Underlying Instrument      `json:"underlying"`
	Synthetic  SyntheticConfig `json:"synthetic"`
}

type Instrument struct {
	InstrumentID string `json:"instrument_id"`
	ProviderKey  string `json:"provider_key"`
}

type SyntheticConfig struct {
	InstrumentID           string         `json:"instrument_id"`
	Version                string         `json:"version"`
	MinimumValidCandidates int            `json:"minimum_valid_candidates"`
	MaxLegAgeMS            int64          `json:"max_leg_age_ms"`
	MaxLegTimeSkewMS       int64          `json:"max_leg_time_skew_ms"`
	Legs                   []Leg          `json:"legs,omitempty"`
	Auto                   *AutoLegConfig `json:"auto,omitempty"`
}

type AutoLegConfig struct {
	StrikeInterval      float64 `json:"strike_interval"`
	ActiveStrikes       int     `json:"active_strikes"`
	WarmStrikes         int     `json:"warm_strikes"`
	ATMHysteresisPoints float64 `json:"atm_hysteresis_points"`
	ATMConfirmationMS   int64   `json:"atm_confirmation_ms"`
}

type Leg struct {
	InstrumentID string  `json:"instrument_id"`
	ProviderKey  string  `json:"provider_key"`
	Strike       float64 `json:"strike"`
	Side         string  `json:"side"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}
	config.normalizePrimary()
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c *Config) normalizePrimary() {
	if len(c.Markets) == 0 {
		return
	}
	for _, market := range c.Markets {
		if strings.EqualFold(strings.TrimSpace(market.Symbol), "NIFTY") {
			c.Nifty = market.Underlying
			c.Synthetic = market.Synthetic
			return
		}
	}
	c.Nifty = c.Markets[0].Underlying
	c.Synthetic = c.Markets[0].Synthetic
}

func (c Config) Validate() error {
	if len(c.Timeframes) == 0 {
		return errors.New("at least one timeframe is required")
	}
	if len(c.RecoverableTimeframes()) == 0 {
		return errors.New("Q1 requires at least one recoverable minute timeframe: 1m, 3m, or 5m")
	}

	markets := c.EffectiveMarkets()
	if len(markets) == 0 {
		return errors.New("at least one market is required")
	}

	symbols := make(map[string]bool)
	instrumentIDs := make(map[string]bool)
	providerKeys := make(map[string]bool)
	syntheticIDs := make(map[string]bool)
	for _, market := range markets {
		if err := validateMarket(market); err != nil {
			return err
		}
		symbol := strings.ToUpper(strings.TrimSpace(market.Symbol))
		if symbols[symbol] {
			return errors.New("market symbols must be unique")
		}
		symbols[symbol] = true

		if instrumentIDs[market.Underlying.InstrumentID] {
			return errors.New("underlying instrument IDs must be unique")
		}
		instrumentIDs[market.Underlying.InstrumentID] = true

		if providerKeys[market.Underlying.ProviderKey] {
			return errors.New("underlying provider keys must be unique")
		}
		providerKeys[market.Underlying.ProviderKey] = true

		if syntheticIDs[market.Synthetic.InstrumentID] {
			return errors.New("synthetic instrument IDs must be unique")
		}
		syntheticIDs[market.Synthetic.InstrumentID] = true
	}
	return nil
}

func validateMarket(m MarketConfig) error {
	if strings.TrimSpace(m.Symbol) == "" ||
		strings.TrimSpace(m.Name) == "" ||
		strings.TrimSpace(m.Exchange) == "" ||
		strings.TrimSpace(m.CalendarID) == "" {
		return errors.New("market symbol, name, exchange, and calendar_id are required")
	}
	if m.Underlying.InstrumentID == "" || m.Underlying.ProviderKey == "" {
		return errors.New("market underlying instrument_id and provider_key are required")
	}
	return validateSynthetic(m.Synthetic)
}

func validateSynthetic(s SyntheticConfig) error {
	if s.InstrumentID == "" || s.Version == "" {
		return errors.New("synthetic instrument_id and version are required")
	}
	if s.MinimumValidCandidates <= 0 {
		return errors.New("minimum_valid_candidates must be positive")
	}
	if s.MaxLegAgeMS <= 0 || s.MaxLegTimeSkewMS <= 0 {
		return errors.New("synthetic leg age and time-skew limits must be positive")
	}

	if s.Auto != nil {
		if len(s.Legs) != 0 {
			return errors.New("synthetic auto mode cannot also define fixed legs")
		}
		return validateAuto(s)
	}
	return validateFixedLegs(s)
}

func validateAuto(s SyntheticConfig) error {
	auto := s.Auto
	if auto == nil {
		return errors.New("auto leg configuration is required")
	}
	if auto.StrikeInterval <= 0 {
		return errors.New("auto strike_interval must be positive")
	}
	if auto.ActiveStrikes < 3 || auto.ActiveStrikes%2 == 0 {
		return errors.New("auto active_strikes must be odd and at least 3")
	}
	if auto.WarmStrikes < auto.ActiveStrikes || auto.WarmStrikes%2 == 0 {
		return errors.New("auto warm_strikes must be odd and no smaller than active_strikes")
	}
	if auto.ATMHysteresisPoints < 0 || auto.ATMHysteresisPoints >= auto.StrikeInterval/2 {
		return errors.New("auto ATM hysteresis must be non-negative and below half the strike interval")
	}
	if auto.ATMConfirmationMS < 0 {
		return errors.New("auto ATM confirmation cannot be negative")
	}
	if s.MinimumValidCandidates > auto.ActiveStrikes {
		return errors.New("minimum_valid_candidates cannot exceed auto active_strikes")
	}
	return nil
}

func validateFixedLegs(s SyntheticConfig) error {
	if len(s.Legs) != 10 {
		return errors.New("fixed synthetic requires exactly ten option legs")
	}

	type strikeSides struct {
		call bool
		put  bool
	}
	strikes := make(map[float64]*strikeSides)
	instruments := make(map[string]bool)
	providerKeys := make(map[string]bool)
	for _, leg := range s.Legs {
		if leg.InstrumentID == "" || leg.ProviderKey == "" || leg.Strike <= 0 {
			return errors.New("synthetic legs require instrument_id, provider_key, and positive strike")
		}
		if instruments[leg.InstrumentID] || providerKeys[leg.ProviderKey] {
			return errors.New("synthetic leg instruments/provider keys must be unique")
		}
		instruments[leg.InstrumentID] = true
		providerKeys[leg.ProviderKey] = true

		side := strings.ToUpper(strings.TrimSpace(leg.Side))
		pair := strikes[leg.Strike]
		if pair == nil {
			pair = &strikeSides{}
			strikes[leg.Strike] = pair
		}
		switch side {
		case "CALL":
			if pair.call {
				return errors.New("duplicate call leg for strike")
			}
			pair.call = true
		case "PUT":
			if pair.put {
				return errors.New("duplicate put leg for strike")
			}
			pair.put = true
		default:
			return errors.New("synthetic leg side must be CALL or PUT")
		}
	}
	if len(strikes) != 5 {
		return errors.New("fixed synthetic requires exactly five strikes")
	}
	for _, pair := range strikes {
		if !pair.call || !pair.put {
			return errors.New("every synthetic strike requires call and put legs")
		}
	}
	return nil
}

func (c Config) EffectiveMarkets() []MarketConfig {
	if len(c.Markets) > 0 {
		result := make([]MarketConfig, len(c.Markets))
		copy(result, c.Markets)
		return result
	}

	markets := DefaultMarkets()
	if c.Nifty.InstrumentID != "" || c.Nifty.ProviderKey != "" {
		markets[0].Underlying = c.Nifty
	}
	if c.Synthetic.InstrumentID != "" || c.Synthetic.Version != "" {
		markets[0].Synthetic = c.Synthetic
	}
	return markets
}

func DefaultMarkets() []MarketConfig {
	return []MarketConfig{
		defaultMarket(
			"NIFTY",
			"Nifty 50",
			"NSE",
			"NSE_EQ",
			[]string{"NIFTY 50"},
			"NSE:NIFTY50",
			"NSE_INDEX|Nifty 50",
			"QNEXT:NIFTY-SYN",
			"nifty-syn-v2",
			50,
			5,
		),
		defaultMarket(
			"BANKNIFTY",
			"Nifty Bank",
			"NSE",
			"NSE_EQ",
			[]string{"NIFTY BANK", "BANK NIFTY"},
			"NSE:BANKNIFTY",
			"NSE_INDEX|Nifty Bank",
			"QNEXT:BANKNIFTY-SYN",
			"banknifty-syn-v1",
			100,
			10,
		),
		defaultMarket(
			"MIDCPNIFTY",
			"Nifty Midcap Select",
			"NSE",
			"NSE_EQ",
			[]string{"NIFTY MID SELECT", "MIDCAP NIFTY"},
			"NSE:MIDCPNIFTY",
			"NSE_INDEX|NIFTY MID SELECT",
			"QNEXT:MIDCPNIFTY-SYN",
			"midcpnifty-syn-v1",
			25,
			2.5,
		),
		defaultMarket(
			"FINNIFTY",
			"Nifty Financial Services",
			"NSE",
			"NSE_EQ",
			[]string{"NIFTY FIN SERVICE", "FIN NIFTY"},
			"NSE:FINNIFTY",
			"NSE_INDEX|Nifty Fin Service",
			"QNEXT:FINNIFTY-SYN",
			"finnifty-syn-v1",
			50,
			5,
		),
		defaultMarket(
			"SENSEX",
			"S&P BSE Sensex",
			"BSE",
			"BSE_EQ",
			[]string{"BSE SENSEX", "BSX"},
			"BSE:SENSEX",
			"BSE_INDEX|SENSEX",
			"QNEXT:SENSEX-SYN",
			"sensex-syn-v1",
			100,
			10,
		),
		defaultMarket(
			"BANKEX",
			"S&P BSE Bankex",
			"BSE",
			"BSE_EQ",
			[]string{"BSE BANKEX", "BKX"},
			"BSE:BANKEX",
			"BSE_INDEX|BANKEX",
			"QNEXT:BANKEX-SYN",
			"bankex-syn-v1",
			100,
			10,
		),
	}
}

func defaultMarket(
	symbol string,
	name string,
	exchange string,
	calendarID string,
	aliases []string,
	instrumentID string,
	providerKey string,
	syntheticID string,
	version string,
	strikeInterval float64,
	hysteresis float64,
) MarketConfig {
	return MarketConfig{
		Symbol:     symbol,
		Name:       name,
		Exchange:   exchange,
		CalendarID: calendarID,
		Aliases:    aliases,
		Underlying: Instrument{
			InstrumentID: instrumentID,
			ProviderKey:  providerKey,
		},
		Synthetic: SyntheticConfig{
			InstrumentID:           syntheticID,
			Version:                version,
			MinimumValidCandidates: 3,
			MaxLegAgeMS:            2000,
			MaxLegTimeSkewMS:       1000,
			Auto: &AutoLegConfig{
				StrikeInterval:      strikeInterval,
				ActiveStrikes:       5,
				WarmStrikes:         7,
				ATMHysteresisPoints: hysteresis,
				ATMConfirmationMS:   750,
			},
		},
	}
}

func (c Config) AutoLegsEnabled() bool {
	for _, market := range c.EffectiveMarkets() {
		if market.Synthetic.Auto != nil {
			return true
		}
	}
	return false
}

func (c Config) ProviderKeys() []string {
	var keys []string
	for _, market := range c.EffectiveMarkets() {
		keys = append(keys, market.Underlying.ProviderKey)
		if market.Synthetic.Auto == nil {
			for _, leg := range market.Synthetic.Legs {
				keys = append(keys, leg.ProviderKey)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func (c Config) RecoverableTimeframes() []string {
	var result []string
	for _, timeframe := range c.Timeframes {
		switch timeframe {
		case "1m", "3m", "5m":
			result = append(result, timeframe)
		}
	}
	return result
}
