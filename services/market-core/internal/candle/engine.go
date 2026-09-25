package candle

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

var ErrLateTick = errors.New("tick belongs to an already closed candle")

type Engine struct {
	mu      sync.Mutex
	version string
	bars    map[string]domain.Bar
}

func New(version string) *Engine {
	return &Engine{
		version: version,
		bars:    make(map[string]domain.Bar),
	}
}

func ParseTimeframe(value string) (time.Duration, error) {
	switch value {
	case "15s":
		return 15 * time.Second, nil
	case "30s":
		return 30 * time.Second, nil
	case "1m":
		return time.Minute, nil
	case "3m":
		return 3 * time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	default:
		return 0, fmt.Errorf("unsupported timeframe %q", value)
	}
}

// Apply returns one forming-bar update for a tick in the current bucket.
// When a tick starts a new bucket, Apply returns the finalized previous bar
// followed by the new forming bar.
func (e *Engine) Apply(tick domain.Tick, timeframe string) ([]domain.Bar, error) {
	duration, err := ParseTimeframe(timeframe)
	if err != nil {
		return nil, err
	}
	if tick.InstrumentID == "" || tick.EventTime.IsZero() {
		return nil, errors.New("tick requires instrument and event time")
	}

	openTime := tick.EventTime.UTC().Truncate(duration)
	closeTime := openTime.Add(duration)
	key := tick.InstrumentID + "|" + timeframe

	e.mu.Lock()
	defer e.mu.Unlock()

	current, exists := e.bars[key]
	if !exists {
		bar := newBar(tick, timeframe, openTime, closeTime, e.version)
		e.bars[key] = bar
		return []domain.Bar{bar}, nil
	}

	if openTime.Before(current.OpenTime) {
		return nil, ErrLateTick
	}

	if openTime.Equal(current.OpenTime) {
		current.High = max(current.High, tick.Price)
		current.Low = min(current.Low, tick.Price)
		current.Close = tick.Price
		current.Volume += tick.Quantity
		current.Quality = worstQuality(current.Quality, tick.Quality)
		current.AuthorityProvider = tick.Provider
		current.SourceSequence = tick.Sequence
		current.SyntheticVersion = tick.SyntheticVersion
		e.bars[key] = current
		return []domain.Bar{current}, nil
	}

	current.Final = true
	next := newBar(tick, timeframe, openTime, closeTime, e.version)
	e.bars[key] = next
	return []domain.Bar{current, next}, nil
}

func newBar(tick domain.Tick, timeframe string, openTime, closeTime time.Time, version string) domain.Bar {
	return domain.Bar{
		InstrumentID:        tick.InstrumentID,
		Timeframe:           timeframe,
		OpenTime:            openTime,
		CloseTime:           closeTime,
		Open:                tick.Price,
		High:                tick.Price,
		Low:                 tick.Price,
		Close:               tick.Price,
		Volume:              tick.Quantity,
		Final:               false,
		Revision:            0,
		AuthorityProvider:   tick.Provider,
		Quality:             tick.Quality,
		SourceSequence:      tick.Sequence,
		CandleEngineVersion: version,
		SyntheticVersion:    tick.SyntheticVersion,
		CreatedAt:           tickCreationTime(tick),
	}
}

func tickCreationTime(tick domain.Tick) time.Time {
	if !tick.ProcessedTime.IsZero() {
		return tick.ProcessedTime.UTC()
	}
	if !tick.ReceivedTime.IsZero() {
		return tick.ReceivedTime.UTC()
	}
	return tick.EventTime.UTC()
}

func worstQuality(a, b domain.Quality) domain.Quality {
	rank := map[domain.Quality]int{
		domain.QualityGood:      0,
		domain.QualityRecovered: 1,
		domain.QualityPartial:   2,
		domain.QualityStale:     3,
		domain.QualityDegraded:  4,
		domain.QualityInvalid:   5,
	}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
