package pipeline

import (
	"errors"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

const subminuteRollupVersion = "candle-rollup-v3-canonical-5s"

type subminuteState struct {
	open     time.Time
	close    time.Time
	expected int
	children map[int64]domain.Bar
}

type subminuteRollupEngine struct {
	states     map[string]*subminuteState
	incomplete map[string]uint64
}

func newSubminuteRollupEngine() *subminuteRollupEngine {
	return &subminuteRollupEngine{
		states:     make(map[string]*subminuteState),
		incomplete: make(map[string]uint64),
	}
}

func (r *subminuteRollupEngine) Apply(source domain.Bar, timeframe string) ([]domain.Bar, error) {
	if source.Timeframe != "5s" {
		return nil, errors.New("subminute rollup source must be 5s")
	}
	open, closeAt, err := candle.Bucket(source.OpenTime, timeframe)
	if err != nil {
		return nil, err
	}
	seconds := int(closeAt.Sub(open) / time.Second)
	if seconds <= 0 || seconds%5 != 0 {
		return nil, errors.New("subminute target must be divisible by 5 seconds")
	}
	key := source.InstrumentID + "|" + timeframe
	state := r.states[key]
	var updates []domain.Bar

	if state != nil && !open.Equal(state.open) {
		if subminuteComplete(state) {
			final := aggregateSubminute(state, timeframe)
			final.Final = true
			updates = append(updates, final)
		} else if len(state.children) > 0 {
			r.incomplete[timeframe]++
		}
		state = nil
	}
	if state == nil {
		state = &subminuteState{
			open:     open,
			close:    closeAt,
			expected: seconds / 5,
			children: make(map[int64]domain.Bar, seconds/5),
		}
		r.states[key] = state
	}

	childKey := source.OpenTime.UnixMilli()
	previous, exists := state.children[childKey]
	if !exists || newerFiveSecond(previous, source) {
		state.children[childKey] = source
	}
	forming := aggregateSubminute(state, timeframe)
	if forming.InstrumentID != "" {
		forming.Final = false
		updates = append(updates, forming)
	}
	return updates, nil
}

func subminuteComplete(state *subminuteState) bool {
	if state == nil || state.expected <= 0 || len(state.children) != state.expected {
		return false
	}
	for _, bar := range state.children {
		if !bar.Final {
			return false
		}
	}
	return true
}

func aggregateSubminute(state *subminuteState, timeframe string) domain.Bar {
	if state == nil || len(state.children) == 0 {
		return domain.Bar{}
	}
	var first domain.Bar
	var last domain.Bar
	var firstKey int64
	var lastKey int64
	var high float64
	var low float64
	var volume float64
	var quality domain.Quality
	var recovered bool
	var carryForward bool
	initialized := false

	for key, bar := range state.children {
		if !initialized {
			first, last = bar, bar
			firstKey, lastKey = key, key
			high, low = bar.High, bar.Low
			quality = bar.Quality
			initialized = true
		} else {
			if key < firstKey {
				firstKey, first = key, bar
			}
			if key > lastKey {
				lastKey, last = key, bar
			}
			if bar.High > high {
				high = bar.High
			}
			if bar.Low < low {
				low = bar.Low
			}
			quality = worseQuality(quality, bar.Quality)
		}
		volume += bar.Volume
		recovered = recovered || bar.Recovered
		carryForward = carryForward || bar.CarryForward
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
		CarryForward:        carryForward,
		SourceSequence:      last.SourceSequence,
		CandleEngineVersion: subminuteRollupVersion,
		SyntheticVersion:    last.SyntheticVersion,
		CreatedAt:           last.CreatedAt,
	}
}

func newerFiveSecond(previous, next domain.Bar) bool {
	if next.Revision != previous.Revision {
		return next.Revision > previous.Revision
	}
	if previous.Final && !next.Final {
		return false
	}
	if next.SourceSequence != previous.SourceSequence {
		return next.SourceSequence > previous.SourceSequence
	}
	if next.Final != previous.Final {
		return next.Final
	}
	return next.CreatedAt.After(previous.CreatedAt)
}
