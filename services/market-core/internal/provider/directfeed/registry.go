// Package directfeed defines the normalized direct-market adapter boundary.
// Synthetic/volume-proxy providers retain their specialized routing.
package directfeed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

const MaxAdditionalFeeds = 8

type Spec struct {
	Adapter      string `json:"adapter"`
	InstrumentID string `json:"instrument_id"`
	ProviderKey  string `json:"provider_key"`
	Enabled      bool   `json:"enabled"`
}

type Binding struct {
	Instrument        symbol.Instrument
	Provider          string
	Key               string
	History           func(context.Context, string, time.Time, time.Time) ([]domain.Bar, error)
	Run               func(context.Context, func(domain.Tick) error, func(context.Context) error) error
	HistoryTimeframes []string // nil preserves the legacy 1m/5s recovery contract
}

type Adapter func(context.Context, Spec) (Binding, error)

// Registry is populated at process startup, before any adapter is started.
type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

func (r *Registry) Register(name string, adapter Adapter) error {
	if !validAdapter.MatchString(name) || adapter == nil {
		return errors.New("invalid direct adapter")
	}
	if _, exists := r.adapters[name]; exists {
		return errors.New("direct adapter already registered")
	}
	r.adapters[name] = adapter
	return nil
}

var validAdapter = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var validID = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}:[A-Z0-9][A-Z0-9_-]{0,95}$`)

func (s Spec) Validate() error {
	if !validAdapter.MatchString(s.Adapter) || !validID.MatchString(s.InstrumentID) ||
		strings.TrimSpace(s.ProviderKey) != s.ProviderKey || len(s.ProviderKey) > 128 {
		return errors.New("invalid direct feed identity")
	}
	return nil
}

func Load(path string) ([]Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Limit input even when an operator accidentally supplies a large file.
	const maxBytes = 64 << 10
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBytes {
		return nil, errors.New("direct feed configuration too large")
	}
	var cfg struct {
		Feeds []Spec `json:"feeds"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("direct feed configuration must contain one JSON object")
	}
	if cfg.Feeds == nil || len(cfg.Feeds) > MaxAdditionalFeeds {
		return nil, errors.New("direct feed configuration missing feeds or exceeds limit")
	}
	seen := make(map[string]bool)
	for _, spec := range cfg.Feeds {
		if err := spec.Validate(); err != nil {
			return nil, err
		}
		if seen[spec.InstrumentID] {
			return nil, fmt.Errorf("duplicate direct instrument %s", spec.InstrumentID)
		}
		seen[spec.InstrumentID] = true
	}
	return cfg.Feeds, nil
}

// Prepare performs discovery before atomically making the symbol visible.
// It never starts ingestion for disabled, unknown or unverified instruments.
func (r *Registry) Prepare(ctx context.Context, spec Spec, symbols *symbol.Registry, calendars *marketcalendar.Registry) (Binding, error) {
	if err := spec.Validate(); err != nil {
		return Binding{}, err
	}
	if !spec.Enabled {
		return Binding{}, errors.New("direct feed disabled")
	}
	adapter, ok := r.adapters[spec.Adapter]
	if !ok {
		return Binding{}, errors.New("unknown direct adapter")
	}
	if symbols == nil || calendars == nil {
		return Binding{}, errors.New("direct feed requires symbol and calendar registries")
	}
	binding, err := adapter(ctx, spec)
	if err != nil {
		return Binding{}, err
	}
	if binding.Instrument.ID != spec.InstrumentID || binding.History == nil || binding.Run == nil ||
		binding.Provider == "" || binding.Key == "" ||
		(spec.ProviderKey != "" && spec.ProviderKey != binding.Key) ||
		binding.Instrument.Synthetic || binding.Instrument.DataMode != "stream" {
		return Binding{}, errors.New("adapter returned incompatible direct binding")
	}
	cal, ok := calendars.Definition(binding.Instrument.CalendarID)
	if !ok || cal.Timezone != binding.Instrument.Timezone {
		return Binding{}, errors.New("unregistered or incompatible direct calendar")
	}
	// This runtime lane supports continuous UTC sessions only. Reject an
	// unsupported calendar before exposing a symbol to the shared engine.
	if !cal.Continuous || cal.Timezone != "UTC" {
		return Binding{}, errors.New("direct calendar requires continuous UTC policy")
	}
	for _, tf := range binding.HistoryTimeframes {
		if tf != "1m" && tf != "5s" {
			return Binding{}, errors.New("unsupported direct history timeframe")
		}
	}
	if err := symbols.RegisterMapped(binding.Instrument, symbol.ProviderInstrument{
		Provider: binding.Provider, InstrumentID: binding.Instrument.ID, ProviderKey: binding.Key,
	}); err != nil {
		return Binding{}, err
	}
	return binding, nil
}
