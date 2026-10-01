package dhan

import (
	"sort"
	"sync"
)

type KeyRegistry struct {
	mu   sync.RWMutex
	keys map[string]InstrumentKey
}

func NewKeyRegistry(initial []InstrumentKey) *KeyRegistry {
	registry := &KeyRegistry{keys: make(map[string]InstrumentKey, len(initial))}
	registry.Ensure(initial...)
	return registry
}

func (r *KeyRegistry) Ensure(keys ...InstrumentKey) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, key := range keys {
		if key.SecurityID <= 0 || key.ExchangeSegment == "" || key.Instrument == "" {
			continue
		}
		r.keys[key.String()] = key
	}
}

func (r *KeyRegistry) Snapshot() []InstrumentKey {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	result := make([]InstrumentKey, 0, len(r.keys))
	for _, key := range r.keys {
		result = append(result, key)
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		return result[i].String() < result[j].String()
	})
	return result
}
