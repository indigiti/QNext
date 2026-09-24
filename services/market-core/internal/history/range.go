package history

import (
	"errors"
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
		bars, err := s.LoadDay(instrumentID, timeframe, day)
		if err != nil {
			return nil, err
		}
		for _, bar := range bars {
			if !bar.OpenTime.Before(from) && bar.OpenTime.Before(to) {
				result = append(result, bar)
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return result, nil
}
