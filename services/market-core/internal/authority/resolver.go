package authority

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type Preference struct {
	Provider     string
	Priority     int
	MaxStaleness time.Duration
}

type ProviderState struct {
	Healthy       bool
	Entitled      bool
	LastEventTime time.Time
}

type Selection struct {
	InstrumentID string
	Provider     string
	Priority     int
}

type Resolver struct {
	mu       sync.RWMutex
	registry *symbol.Registry
	policy   map[string][]Preference
	state    map[string]ProviderState
}

func NewResolver(registry *symbol.Registry, policy map[string][]Preference) (*Resolver, error) {
	if registry == nil {
		return nil, errors.New("symbol registry is required")
	}

	normalized := make(map[string][]Preference, len(policy))
	for instrumentID, preferences := range policy {
		if _, ok := registry.Instrument(instrumentID); !ok {
			return nil, errors.New("authority policy references unknown instrument: " + instrumentID)
		}
		if len(preferences) == 0 {
			return nil, errors.New("authority policy requires at least one provider for " + instrumentID)
		}

		seen := make(map[string]bool, len(preferences))
		copyOfPreferences := make([]Preference, 0, len(preferences))
		for _, preference := range preferences {
			provider := strings.ToLower(strings.TrimSpace(preference.Provider))
			if provider == "" {
				return nil, errors.New("authority provider is required")
			}
			if preference.MaxStaleness <= 0 {
				return nil, errors.New("authority max staleness must be positive")
			}
			if seen[provider] {
				return nil, errors.New("duplicate authority provider for " + instrumentID)
			}
			if _, ok := registry.ProviderMapping(provider, instrumentID); !ok {
				return nil, errors.New("authority provider is not registered for " + instrumentID + ": " + provider)
			}
			seen[provider] = true
			preference.Provider = provider
			copyOfPreferences = append(copyOfPreferences, preference)
		}

		sort.Slice(copyOfPreferences, func(i, j int) bool {
			if copyOfPreferences[i].Priority == copyOfPreferences[j].Priority {
				return copyOfPreferences[i].Provider < copyOfPreferences[j].Provider
			}
			return copyOfPreferences[i].Priority < copyOfPreferences[j].Priority
		})
		normalized[instrumentID] = copyOfPreferences
	}

	return &Resolver{
		registry: registry,
		policy:   normalized,
		state:    make(map[string]ProviderState),
	}, nil
}

func (r *Resolver) SetPolicy(instrumentID string, preferences []Preference) error {
	if strings.TrimSpace(instrumentID) == "" {
		return errors.New("authority policy instrument is required")
	}
	if _, ok := r.registry.Instrument(instrumentID); !ok {
		return errors.New("authority policy references unknown instrument: " + instrumentID)
	}
	normalized, err := normalizePreferences(r.registry, instrumentID, preferences)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.policy[instrumentID] = normalized
	r.mu.Unlock()
	return nil
}

func normalizePreferences(registry *symbol.Registry, instrumentID string, preferences []Preference) ([]Preference, error) {
	if len(preferences) == 0 {
		return nil, errors.New("authority policy requires at least one provider for " + instrumentID)
	}
	seen := make(map[string]bool, len(preferences))
	copyOfPreferences := make([]Preference, 0, len(preferences))
	for _, preference := range preferences {
		provider := strings.ToLower(strings.TrimSpace(preference.Provider))
		if provider == "" {
			return nil, errors.New("authority provider is required")
		}
		if preference.MaxStaleness <= 0 {
			return nil, errors.New("authority max staleness must be positive")
		}
		if seen[provider] {
			return nil, errors.New("duplicate authority provider for " + instrumentID)
		}
		if _, ok := registry.ProviderMapping(provider, instrumentID); !ok {
			return nil, errors.New("authority provider is not registered for " + instrumentID + ": " + provider)
		}
		seen[provider] = true
		preference.Provider = provider
		copyOfPreferences = append(copyOfPreferences, preference)
	}
	sort.Slice(copyOfPreferences, func(i, j int) bool {
		if copyOfPreferences[i].Priority == copyOfPreferences[j].Priority {
			return copyOfPreferences[i].Provider < copyOfPreferences[j].Provider
		}
		return copyOfPreferences[i].Priority < copyOfPreferences[j].Priority
	})
	return copyOfPreferences, nil
}

func (r *Resolver) UpdateState(instrumentID, provider string, state ProviderState) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if instrumentID == "" || provider == "" {
		return errors.New("instrument_id and provider are required")
	}
	if _, ok := r.registry.ProviderMapping(provider, instrumentID); !ok {
		return errors.New("provider mapping is not registered")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.state[stateKey(instrumentID, provider)] = state
	return nil
}

func (r *Resolver) Resolve(instrumentID string, now time.Time) (Selection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	preferences := r.policy[instrumentID]
	for _, preference := range preferences {
		state, ok := r.state[stateKey(instrumentID, preference.Provider)]
		if !ok || !state.Healthy || !state.Entitled || state.LastEventTime.IsZero() {
			continue
		}
		age := now.Sub(state.LastEventTime)
		if age < 0 || age > preference.MaxStaleness {
			continue
		}
		return Selection{
			InstrumentID: instrumentID,
			Provider:     preference.Provider,
			Priority:     preference.Priority,
		}, true
	}
	return Selection{}, false
}

func (r *Resolver) Accept(instrumentID, provider string, now time.Time) bool {
	selection, ok := r.Resolve(instrumentID, now)
	if !ok {
		return false
	}
	return selection.Provider == strings.ToLower(strings.TrimSpace(provider))
}

func stateKey(instrumentID, provider string) string {
	return instrumentID + "\x00" + strings.ToLower(strings.TrimSpace(provider))
}
