package symbol

import "errors"

// Ensure registers an instrument if it is new and otherwise verifies that the
// existing canonical identity is compatible with the requested definition.
func (r *Registry) Ensure(instrument Instrument) error {
	if err := instrument.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.instruments[instrument.ID]
	if !ok {
		r.instruments[instrument.ID] = instrument
		return nil
	}
	if existing.ID != instrument.ID ||
		existing.AssetClass != instrument.AssetClass ||
		existing.Exchange != instrument.Exchange ||
		existing.Currency != instrument.Currency ||
		existing.Timezone != instrument.Timezone {
		return errors.New("instrument identity conflicts with existing registry entry")
	}
	return nil
}

// EnsureProvider registers a provider mapping if it is new. Repeating the
// exact same mapping is idempotent; changing either side fails closed.
func (r *Registry) EnsureProvider(mapping ProviderInstrument) error {
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

	if existing, ok := r.providerByKey[key]; ok {
		if existing.InstrumentID != mapping.InstrumentID {
			return errors.New("provider key conflicts with existing canonical instrument")
		}
		return nil
	}
	if existing, ok := r.providerByID[id]; ok {
		if existing.ProviderKey != mapping.ProviderKey {
			return errors.New("canonical instrument conflicts with existing provider key")
		}
		return nil
	}

	r.providerByKey[key] = mapping
	r.providerByID[id] = mapping
	return nil
}
