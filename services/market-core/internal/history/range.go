package history

import (
	"errors"
	"sort"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func (s *Store) LoadRange(instrumentID, timeframe string, from, to time.Time) ([]domain.Bar, error) {
	if from.IsZero() || to.IsZero() {
		return nil, errors.New("from and to are required")
	}
	from = from.UTC()
	to = to.UTC()
	if !from.Before(to) {
		return nil, errors.New("from must be before to")
	}

	day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	var result []domain.Bar
	for day.Before(to) {
		bars, err := s.loadDay(instrumentID, timeframe, day, false)
		if err != nil {
			return nil, err
		}
		// Both disk and current-day snapshots are ordered by open time.
		// Copy only the selected bars; callers cannot mutate the cache.
		start := sort.Search(len(bars), func(i int) bool {
			return !bars[i].OpenTime.Before(from)
		})
		end := sort.Search(len(bars), func(i int) bool {
			return !bars[i].OpenTime.Before(to)
		})
		result = append(result, bars[start:end]...)
		day = day.AddDate(0, 0, 1)
	}
	return result, nil
}
