package marketconfig

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
)

type Config struct {
	Timeframes []string        `json:"timeframes"`
	Nifty      Instrument      `json:"nifty"`
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
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if len(c.Timeframes) == 0 {
		return errors.New("at least one timeframe is required")
	}
	if len(c.RecoverableTimeframes()) == 0 {
		return errors.New("Q1 requires at least one recoverable minute timeframe: 1m, 3m, or 5m")
	}
	if c.Nifty.InstrumentID == "" || c.Nifty.ProviderKey == "" {
		return errors.New("NIFTY instrument_id and provider_key are required")
	}
	if c.Synthetic.InstrumentID == "" || c.Synthetic.Version == "" {
		return errors.New("synthetic instrument_id and version are required")
	}
	if c.Synthetic.MinimumValidCandidates <= 0 {
		return errors.New("minimum_valid_candidates must be positive")
	}
	if c.Synthetic.MaxLegAgeMS <= 0 || c.Synthetic.MaxLegTimeSkewMS <= 0 {
		return errors.New("synthetic leg age and time-skew limits must be positive")
	}

	if c.Synthetic.Auto != nil {
		if len(c.Synthetic.Legs) != 0 {
			return errors.New("synthetic auto mode cannot also define fixed legs")
		}
		return validateAuto(c.Synthetic)
	}
	return validateFixedLegs(c.Synthetic)
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
		return errors.New("fixed NIFTY-SYN requires exactly ten option legs")
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
		return errors.New("fixed NIFTY-SYN requires exactly five strikes")
	}
	for _, pair := range strikes {
		if !pair.call || !pair.put {
			return errors.New("every synthetic strike requires call and put legs")
		}
	}
	return nil
}

func (c Config) AutoLegsEnabled() bool {
	return c.Synthetic.Auto != nil
}

func (c Config) ProviderKeys() []string {
	keys := []string{c.Nifty.ProviderKey}
	if c.Synthetic.Auto == nil {
		for _, leg := range c.Synthetic.Legs {
			keys = append(keys, leg.ProviderKey)
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
