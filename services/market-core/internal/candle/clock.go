package candle

import (
	"errors"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

// Advance finalizes the current candle when the trusted market clock has
// crossed its close. When carryForward is true, zero-volume candles are
// created from the previous close until the active bucket is reached. This is
// intended only for a caller that has independently verified feed health.
func (e *Engine) Advance(instrumentID, timeframe string, at time.Time, carryForward bool) ([]domain.Bar, error) {
	if instrumentID == "" || at.IsZero() {
		return nil, errors.New("advance requires instrument and market time")
	}
	if _, err := parseTimeframe(timeframe); err != nil {
		return nil, err
	}

	key := instrumentID + "|" + timeframe
	e.mu.Lock()
	defer e.mu.Unlock()

	current, exists := e.bars[key]
	if !exists || current.CloseTime.After(at.UTC()) {
		return nil, nil
	}

	var updates []domain.Bar
	for {
		current.Final = true
		e.bars[key] = current
		updates = append(updates, current)

		if !carryForward {
			delete(e.bars, key)
			break
		}

		nextOpen, nextClose, err := Bucket(current.CloseTime, timeframe)
		if err != nil {
			return nil, err
		}
		// At or after the regular-session close Bucket can collapse to a
		// zero-length interval. Never carry into that region or overnight.
		if !nextClose.After(nextOpen) || nextOpen.Before(current.CloseTime) {
			delete(e.bars, key)
			break
		}

		next := domain.Bar{
			InstrumentID:        current.InstrumentID,
			Timeframe:           timeframe,
			OpenTime:            nextOpen,
			CloseTime:           nextClose,
			Open:                current.Close,
			High:                current.Close,
			Low:                 current.Close,
			Close:               current.Close,
			Volume:              0,
			Final:               false,
			Revision:            0,
			AuthorityProvider:   current.AuthorityProvider,
			Quality:             current.Quality,
			Recovered:           current.Recovered,
			SourceSequence:      current.SourceSequence,
			CandleEngineVersion: current.CandleEngineVersion,
			SyntheticVersion:    current.SyntheticVersion,
			CreatedAt:           at.UTC(),
		}
		e.bars[key] = next
		updates = append(updates, next)
		if next.CloseTime.After(at.UTC()) {
			break
		}
		current = next
	}
	return updates, nil
}
