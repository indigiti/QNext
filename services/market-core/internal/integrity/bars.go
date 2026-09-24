package integrity

import (
	"errors"
	"fmt"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func CheckBarContinuity(bars []domain.Bar, timeframe string) error {
	duration, err := candle.ParseTimeframe(timeframe)
	if err != nil {
		return err
	}
	if len(bars) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(bars))
	var previous time.Time
	for i, bar := range bars {
		if !bar.Final {
			return fmt.Errorf("bar %d is not final", i)
		}
		if bar.Timeframe != timeframe {
			return fmt.Errorf("bar %d has timeframe %q", i, bar.Timeframe)
		}
		if _, exists := seen[bar.Key()]; exists {
			return fmt.Errorf("duplicate canonical bar %s", bar.Key())
		}
		seen[bar.Key()] = struct{}{}

		if i > 0 {
			expected := previous.Add(duration)
			if !bar.OpenTime.Equal(expected) {
				return fmt.Errorf("canonical bar gap: expected %s, got %s", expected.UTC(), bar.OpenTime.UTC())
			}
		}
		previous = bar.OpenTime
	}
	return nil
}

var ErrHistoryLiveDiscontinuity = errors.New("history/live boundary is discontinuous")

func CheckHistoryLiveBoundary(historyBar, liveBar domain.Bar) error {
	if !historyBar.Final {
		return errors.New("history boundary bar must be final")
	}
	if historyBar.InstrumentID != liveBar.InstrumentID || historyBar.Timeframe != liveBar.Timeframe {
		return ErrHistoryLiveDiscontinuity
	}
	if !liveBar.OpenTime.Equal(historyBar.CloseTime) {
		return ErrHistoryLiveDiscontinuity
	}
	return nil
}
