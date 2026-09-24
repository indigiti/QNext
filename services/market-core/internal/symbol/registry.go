package symbol

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

type Instrument struct {
	ID         string   `json:"instrument_id"`
	Symbol     string   `json:"symbol"`
	Name       string   `json:"name"`
	AssetClass string   `json:"asset_class"`
	Exchange   string   `json:"exchange"`
	Currency   string   `json:"currency"`
	Timezone   string   `json:"timezone"`
	Aliases    []string `json:"aliases,omitempty"`
	Synthetic  bool     `json:"synthetic"`
}

func (i Instrument) Validate() error {
	if i.ID == "" || i.Symbol == "" || i.Name == "" {
		return errors.New("instrument_id, symbol and name are required")
	}
	if i.AssetClass == "" || i.Exchange == "" || i.Currency == "" || i.Timezone == "" {
		return errors.New("asset_class, exchange, currency and timezone are required")
	}
	return nil
}

type ProviderInstrument struct {
	Provider     string `json:"provider"`
	InstrumentID string `json:"instrument_id"`
	ProviderKey  string `json:"provider_key"`
}

type Registry struct {
	mu            sync.RWMutex
	instruments   map[string]Instrument
	providerByKey map[string]ProviderInstrument
	providerByID  map[string]ProviderInstrument
}

func NewRegistry() *Registry {
	return &Registry{
		instruments:   make(map[string]Instrument),
		providerByKey: make(map[string]ProviderInstrument),
		providerByID:  make(map[string]ProviderInstrument),
	}
}

func (r *Registry) Register(instrument Instrument) error {
	if err := instrument.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.instruments[instrument.ID]; exists {
		return errors.New("instrument already registered")
	}
	r.instruments[instrument.ID] = instrument
	return nil
}

func (r *Registry) RegisterProvider(mapping ProviderInstrument) error {
	if mapping.Provider == "" || mapping.InstrumentID == "" || mapping.ProviderKey == "" {
		return errors.New("provider, instrument_id and provider_key are required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.instruments[mapping.InstrumentID]; !exists {
		return errors.New("canonical instrument is not registered")
	}

	key := providerLookupKey(mapping.Provider, mapping.ProviderKey)
	id := providerLookupKey(mapping.Provider, mapping.InstrumentID)
	if _, exists := r.providerByKey[key]; exists {
		return errors.New("provider key already registered")
	}
	if _, exists := r.providerByID[id]; exists {
		return errors.New("provider mapping already registered for instrument")
	}

	r.providerByKey[key] = mapping
	r.providerByID[id] = mapping
	return nil
}

func (r *Registry) Instrument(id string) (Instrument, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	instrument, ok := r.instruments[id]
	return instrument, ok
}

func (r *Registry) ResolveProviderKey(provider, providerKey string) (Instrument, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mapping, ok := r.providerByKey[providerLookupKey(provider, providerKey)]
	if !ok {
		return Instrument{}, false
	}
	instrument, ok := r.instruments[mapping.InstrumentID]
	return instrument, ok
}

func (r *Registry) ProviderMapping(provider, instrumentID string) (ProviderInstrument, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	mapping, ok := r.providerByID[providerLookupKey(provider, instrumentID)]
	return mapping, ok
}

func (r *Registry) Search(query string) []Instrument {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Instrument
	for _, instrument := range r.instruments {
		if matches(instrument, q) {
			result = append(result, instrument)
		}
	}
	sort.Slice(result, func(a, b int) bool {
		if result[a].Symbol == result[b].Symbol {
			return result[a].ID < result[b].ID
		}
		return result[a].Symbol < result[b].Symbol
	})
	return result
}

func matches(instrument Instrument, query string) bool {
	fields := []string{instrument.ID, instrument.Symbol, instrument.Name}
	fields = append(fields, instrument.Aliases...)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func providerLookupKey(provider, key string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "\x00" + strings.TrimSpace(key)
}
