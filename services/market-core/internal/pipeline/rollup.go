package pipeline

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type rollupState struct {
	open    time.Time
	close   time.Time
	minutes map[int64]domain.Bar
}

type bucketResolver func(instrumentID string, at time.Time, timeframe string) (time.Time, time.Time, error)

type rollupEngine struct {
	version string
	states  map[string]*rollupState
	bucket  bucketResolver
}

func newRollupEngine(version string, resolver bucketResolver) *rollupEngine {
	if resolver == nil {
		resolver = func(_ string, at time.Time, timeframe string) (time.Time, time.Time, error) {
			return candle.Bucket(at, timeframe)
		}
	}
	return &rollupEngine{
		version: version,
		states:  make(map[string]*rollupState),
		bucket:  resolver,
	}
}

func (r *rollupEngine) Apply(oneMinute domain.Bar, timeframe string) ([]domain.Bar, error) {
	if oneMinute.Timeframe != "1m" {
		return nil, errors.New("rollup source must be 1m")
	}
	open, closeAt, err := r.bucket(oneMinute.InstrumentID, oneMinute.OpenTime, timeframe)
	if err != nil {
		return nil, err
	}
	key := oneMinute.InstrumentID + "|" + timeframe
	state := r.states[key]

	var updates []domain.Bar
	if state != nil && !open.Equal(state.open) {
		if completeRollup(state, timeframe) {
			final := aggregateRollup(state, timeframe, r.version)
			final.Final = true
			updates = append(updates, final)
		}
		state = nil
	}

	if state == nil {
		state = &rollupState{
			open:    open,
			close:   closeAt,
			minutes: make(map[int64]domain.Bar),
		}
		r.states[key] = state
	}

	state.minutes[oneMinute.OpenTime.UnixMilli()] = oneMinute
	if completeRollup(state, timeframe) {
		final := aggregateRollup(state, timeframe, r.version)
		final.Final = true
		updates = append(updates, final)
		delete(r.states, key)
		return updates, nil
	}

	forming := aggregateRollup(state, timeframe, r.version)
	forming.Final = false
	updates = append(updates, forming)
	return updates, nil
}

func completeRollup(state *rollupState, timeframe string) bool {
	if state == nil || len(state.minutes) == 0 {
		return false
	}
	for _, bar := range state.minutes {
		if !bar.Final {
			return false
		}
	}
	expected := expectedMinuteCount(state.open, state.close, timeframe)
	return expected > 0 && len(state.minutes) == expected
}

func expectedMinuteCount(open, closeAt time.Time, timeframe string) int {
	if timeframe == "1D" {
		return 375
	}
	if strings.HasSuffix(timeframe, "m") || strings.HasSuffix(timeframe, "h") {
		return int(closeAt.Sub(open) / time.Minute)
	}
	return 0
}

func aggregateRollup(state *rollupState, timeframe, version string) domain.Bar {
	keys := make([]int64, 0, len(state.minutes))
	for key := range state.minutes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	first := state.minutes[keys[0]]
	last := state.minutes[keys[len(keys)-1]]
	high := first.High
	low := first.Low
	volume := float64(0)
	recovered := false
	quality := first.Quality
	for _, key := range keys {
		bar := state.minutes[key]
		if bar.High > high {
			high = bar.High
		}
		if bar.Low < low {
			low = bar.Low
		}
		volume += bar.Volume
		recovered = recovered || bar.Recovered
		quality = worseQuality(quality, bar.Quality)
	}

	return domain.Bar{
		InstrumentID:        first.InstrumentID,
		Timeframe:           timeframe,
		OpenTime:            state.open,
		CloseTime:           state.close,
		Open:                first.Open,
		High:                high,
		Low:                 low,
		Close:               last.Close,
		Volume:              volume,
		Revision:            0,
		AuthorityProvider:   last.AuthorityProvider,
		Quality:             quality,
		Recovered:           recovered,
		SourceSequence:      last.SourceSequence,
		CandleEngineVersion: version,
		SyntheticVersion:    last.SyntheticVersion,
		CreatedAt:           last.CreatedAt,
	}
}

func worseQuality(a, b domain.Quality) domain.Quality {
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
