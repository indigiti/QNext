package candle

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

var ErrLateTick = errors.New("tick belongs to an already closed candle")

var indiaLocation = time.FixedZone("IST", 5*60*60+30*60)

type timeframeSpec struct {
	value int
	unit  byte
}

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

func SupportedTimeframes() []string {
	return []string{
		"1s", "5s", "10s", "15s", "30s", "45s",
		"1m", "2m", "3m", "5m", "10m", "15m", "30m", "45m",
		"1h", "2h", "3h", "4h",
		"1D", "1W",
		"1M", "3M", "6M", "12M",
	}
}

func ValidateTimeframe(value string) error {
	_, err := parseTimeframe(value)
	return err
}

// ParseTimeframe is retained for callers that need a fixed duration.
// Calendar-based week/month intervals should use Bucket instead.
func ParseTimeframe(value string) (time.Duration, error) {
	spec, err := parseTimeframe(value)
	if err != nil {
		return 0, err
	}
	switch spec.unit {
	case 's':
		return time.Duration(spec.value) * time.Second, nil
	case 'm':
		return time.Duration(spec.value) * time.Minute, nil
	case 'h':
		return time.Duration(spec.value) * time.Hour, nil
	case 'D':
		return 24 * time.Hour, nil
	case 'W':
		return 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("timeframe %q is calendar-based", value)
	}
}

func parseTimeframe(value string) (timeframeSpec, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return timeframeSpec{}, fmt.Errorf("unsupported timeframe %q", value)
	}
	unit := value[len(value)-1]
	number := value[:len(value)-1]
	n, err := strconv.Atoi(number)
	if err != nil || n <= 0 {
		return timeframeSpec{}, fmt.Errorf("unsupported timeframe %q", value)
	}

	valid := false
	switch unit {
	case 's':
		valid = n == 1 || n == 5 || n == 10 || n == 15 || n == 30 || n == 45
	case 'm':
		valid = n == 1 || n == 2 || n == 3 || n == 5 || n == 10 || n == 15 || n == 30 || n == 45
	case 'h':
		valid = n >= 1 && n <= 4
	case 'D':
		valid = n == 1
	case 'W':
		valid = n == 1
	case 'M':
		valid = n == 1 || n == 3 || n == 6 || n == 12
	}
	if !valid {
		return timeframeSpec{}, fmt.Errorf("unsupported timeframe %q", value)
	}
	return timeframeSpec{value: n, unit: unit}, nil
}

func Bucket(at time.Time, timeframe string) (time.Time, time.Time, error) {
	spec, err := parseTimeframe(timeframe)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	local := at.In(indiaLocation)
	switch spec.unit {
	case 's', 'm', 'h':
		var duration time.Duration
		switch spec.unit {
		case 's':
			duration = time.Duration(spec.value) * time.Second
		case 'm':
			duration = time.Duration(spec.value) * time.Minute
		case 'h':
			duration = time.Duration(spec.value) * time.Hour
		}

		sessionOpen := time.Date(
			local.Year(), local.Month(), local.Day(),
			9, 15, 0, 0, indiaLocation,
		)
		if local.Before(sessionOpen) {
			sessionOpen = time.Date(
				local.Year(), local.Month(), local.Day(),
				0, 0, 0, 0, indiaLocation,
			)
		}
		elapsed := local.Sub(sessionOpen)
		open := sessionOpen.Add((elapsed / duration) * duration)
		closeAt := open.Add(duration)
		sessionClose := time.Date(
			local.Year(), local.Month(), local.Day(),
			15, 30, 0, 0, indiaLocation,
		)
		if closeAt.After(sessionClose) {
			closeAt = sessionClose
		}
		return open.UTC(), closeAt.UTC(), nil

	case 'D':
		open := time.Date(
			local.Year(), local.Month(), local.Day(),
			0, 0, 0, 0, indiaLocation,
		)
		return open.UTC(), open.AddDate(0, 0, 1).UTC(), nil

	case 'W':
		dayStart := time.Date(
			local.Year(), local.Month(), local.Day(),
			0, 0, 0, 0, indiaLocation,
		)
		weekday := (int(dayStart.Weekday()) + 6) % 7
		open := dayStart.AddDate(0, 0, -weekday)
		return open.UTC(), open.AddDate(0, 0, 7).UTC(), nil

	case 'M':
		startMonth := ((int(local.Month())-1)/spec.value)*spec.value + 1
		open := time.Date(
			local.Year(), time.Month(startMonth), 1,
			0, 0, 0, 0, indiaLocation,
		)
		return open.UTC(), open.AddDate(0, spec.value, 0).UTC(), nil
	}

	return time.Time{}, time.Time{}, fmt.Errorf("unsupported timeframe %q", timeframe)
}

// Apply returns one forming-bar update for a tick in the current bucket.
// When a tick starts a new bucket, Apply returns the finalized previous bar
// followed by the new forming bar.
func (e *Engine) Apply(tick domain.Tick, timeframe string) ([]domain.Bar, error) {
	if tick.InstrumentID == "" || tick.EventTime.IsZero() {
		return nil, errors.New("tick requires instrument and event time")
	}

	openTime, closeTime, err := Bucket(tick.EventTime, timeframe)
	if err != nil {
		return nil, err
	}
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
