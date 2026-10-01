package feedstatus

import (
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

const (
	defaultCarryInstrumentFreshness = 30 * time.Second
	defaultCarryProviderFreshness   = 8 * time.Second
)

// HealthyForCarry returns true only when an instrument is still recent and
// its provider is actively delivering data. It is deliberately stricter than
// ordinary status reporting because a false positive here would fabricate a
// flat 5s candle across a genuine feed outage.
func (t *Tracker) HealthyForCarry(instrumentID string, now time.Time) bool {
	if t == nil || instrumentID == "" || now.IsZero() {
		return false
	}
	t.mu.RLock()
	instrument, ok := t.instruments[instrumentID]
	if !ok {
		t.mu.RUnlock()
		return false
	}
	provider, providerOK := t.providers[instrument.Provider]
	t.mu.RUnlock()
	if !providerOK || instrument.Price <= 0 {
		return false
	}
	if instrument.Quality == domain.QualityInvalid ||
		instrument.Quality == domain.QualityDegraded ||
		instrument.Quality == domain.QualityStale {
		return false
	}
	instrumentAge := AgeMS(instrument.LastReceivedTimeMS, now)
	providerAge := AgeMS(provider.LastReceivedTimeMS, now)
	return instrumentAge >= 0 &&
		instrumentAge <= defaultCarryInstrumentFreshness.Milliseconds() &&
		providerAge >= 0 &&
		providerAge <= defaultCarryProviderFreshness.Milliseconds()
}
