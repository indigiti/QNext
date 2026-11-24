package symbol

import (
	"errors"
	"strings"
)

// RegisterMapped commits an instrument and its first provider mapping in a
// single critical section. A rejected mapping cannot leave a visible orphan.
func (r *Registry) RegisterMapped(instrument Instrument, mapping ProviderInstrument) error {
	if err := instrument.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(mapping.Provider) == "" || strings.TrimSpace(mapping.ProviderKey) == "" ||
		mapping.InstrumentID != instrument.ID {
		return errors.New("invalid instrument/provider mapping")
	}
	key := providerLookupKey(mapping.Provider, mapping.ProviderKey)
	id := providerLookupKey(mapping.Provider, mapping.InstrumentID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.instruments[instrument.ID]; exists {
		return errors.New("instrument already registered")
	}
	if _, exists := r.providerByKey[key]; exists {
		return errors.New("provider key already registered")
	}
	if _, exists := r.providerByID[id]; exists {
		return errors.New("provider mapping already registered")
	}
	instrument.Aliases = append([]string(nil), instrument.Aliases...)
	r.instruments[instrument.ID] = instrument
	r.providerByKey[key] = mapping
	r.providerByID[id] = mapping
	return nil
}
