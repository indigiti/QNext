package resilience

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"time"
)

const ConfigVersion = "q3.resilience.v1"

type Config struct {
	Version            string              `json:"version"`
	MaxStalenessMS     int64               `json:"max_staleness_ms"`
	GapRecoveryMS      int64               `json:"gap_recovery_ms"`
	DhanPollIntervalMS int64               `json:"dhan_poll_interval_ms"`
	Instruments        []InstrumentMapping `json:"instruments"`
}

type InstrumentMapping struct {
	InstrumentID    string `json:"instrument_id"`
	DhanProviderKey string `json:"dhan_provider_key"`
	Recoverable     bool   `json:"recoverable"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}
func (c Config) Validate() error {
	if c.Version != ConfigVersion {
		return errors.New("unsupported resilience config version")
	}
	if c.MaxStalenessMS <= 0 {
		return errors.New("max_staleness_ms must be positive")
	}
	if c.GapRecoveryMS <= 0 {
		return errors.New("gap_recovery_ms must be positive")
	}
	if c.DhanPollIntervalMS < 1000 {
		return errors.New("dhan_poll_interval_ms must be at least 1000")
	}
	if len(c.Instruments) == 0 {
		return errors.New("at least one Dhan mapping is required")
	}
	ids := map[string]bool{}
	keys := map[string]bool{}
	for _, m := range c.Instruments {
		if m.InstrumentID == "" || m.DhanProviderKey == "" {
			return errors.New("instrument_id and dhan_provider_key are required")
		}
		if ids[m.InstrumentID] {
			return errors.New("duplicate resilience instrument")
		}
		if keys[m.DhanProviderKey] {
			return errors.New("duplicate Dhan provider key")
		}
		ids[m.InstrumentID] = true
		keys[m.DhanProviderKey] = true
	}
	return nil
}
func (c Config) MaxStaleness() time.Duration {
	return time.Duration(c.MaxStalenessMS) * time.Millisecond
}
func (c Config) GapRecovery() time.Duration { return time.Duration(c.GapRecoveryMS) * time.Millisecond }
func (c Config) DhanPollInterval() time.Duration {
	return time.Duration(c.DhanPollIntervalMS) * time.Millisecond
}
func (c Config) Mapping(instrumentID string) (InstrumentMapping, bool) {
	for _, m := range c.Instruments {
		if m.InstrumentID == instrumentID {
			return m, true
		}
	}
	return InstrumentMapping{}, false
}
func (c Config) InstrumentIDs() []string {
	ids := make([]string, 0, len(c.Instruments))
	for _, m := range c.Instruments {
		ids = append(ids, m.InstrumentID)
	}
	sort.Strings(ids)
	return ids
}
func (c Config) RequireCoverage(required []string) error {
	for _, id := range required {
		if _, ok := c.Mapping(id); !ok {
			return errors.New("Dhan mapping missing for canonical instrument: " + id)
		}
	}
	return nil
}
