package pipeline

import (
	"errors"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type rollupState struct {
	open      time.Time
	close     time.Time
	expected  int
	minutes   map[int64]domain.Bar
	count     int
	finals    int
	firstKey  int64
	lastKey   int64
	first     domain.Bar
	last      domain.Bar
	high      float64
	low       float64
	volume    float64
	recovered bool
	quality   domain.Quality
}

type rollupEngine struct {
	version string
	states  map[string]*rollupState
}

func newRollupEngine(version string) *rollupEngine {
	return &rollupEngine{
		version: version,
		states:  make(map[string]*rollupState),
	}
}

func (r *rollupEngine) Apply(oneMinute domain.Bar, timeframe string) ([]domain.Bar, error) {
	if oneMinute.Timeframe != "1m" {
		return nil, errors.New("rollup source must be 1m")
	}
	open, closeAt, err := candle.Bucket(oneMinute.OpenTime, timeframe)
	if err != nil {
		return nil, err
	}
	key := oneMinute.InstrumentID + "|" + timeframe
	state := r.states[key]

	var updates []domain.Bar
	if state != nil && !open.Equal(state.open) {
		if completeRollup(state) {
			final := aggregateRollup(state, timeframe, r.version)
			final.Final = true
			updates = append(updates, final)
		}
		state = nil
	}

	if state == nil {
		state = newRollupState(open, closeAt, timeframe)
		r.states[key] = state
	}

	state.applyMinute(oneMinute)
	forming := aggregateRollup(state, timeframe, r.version)
	forming.Final = false
	updates = append(updates, forming)
	return updates, nil
}

func newRollupState(open, closeAt time.Time, timeframe string) *rollupState {
	return &rollupState{
		open:     open,
		close:    closeAt,
		expected: expectedMinuteCount(open, closeAt, timeframe),
		minutes:  make(map[int64]domain.Bar),
	}
}

func (s *rollupState) applyMinute(bar domain.Bar) {
	key := bar.OpenTime.UnixMilli()
	previous, exists := s.minutes[key]
	if exists && !newerMinute(previous, bar) {
		return
	}

	if !exists {
		s.minutes[key] = bar
		s.count++
		if bar.Final {
			s.finals++
		}
		s.volume += bar.Volume
		if s.count == 1 {
			s.firstKey = key
			s.lastKey = key
			s.first = bar
			s.last = bar
			s.high = bar.High
			s.low = bar.Low
			s.recovered = bar.Recovered
			s.quality = bar.Quality
			return
		}
		if key < s.firstKey {
			s.firstKey = key
			s.first = bar
		}
		if key > s.lastKey {
			s.lastKey = key
			s.last = bar
		}
		if bar.High > s.high {
			s.high = bar.High
		}
		if bar.Low < s.low {
			s.low = bar.Low
		}
		s.recovered = s.recovered || bar.Recovered
		s.quality = worseQuality(s.quality, bar.Quality)
		return
	}

	// The canonical live 1m source is monotonic while a minute is forming:
	// high can only rise, low can only fall, quality can only worsen and
	// recovered can only become true. That makes the normal replacement path
	// O(1), including volume replacement via a delta instead of re-summing.
	// A non-monotonic repaired/corrected minute is still supported exactly by
	// rebuilding this one bounded active bucket.
	needsRebuild :=
		(previous.High == s.high && bar.High < previous.High) ||
		(previous.Low == s.low && bar.Low > previous.Low) ||
		(previous.Recovered && !bar.Recovered) ||
		(qualityRank(bar.Quality) < qualityRank(previous.Quality) && previous.Quality == s.quality)

	s.minutes[key] = bar
	if previous.Final != bar.Final {
		if bar.Final {
			s.finals++
		} else {
			s.finals--
		}
	}

	if needsRebuild {
		s.rebuildSummary()
		return
	}

	s.volume += bar.Volume - previous.Volume
	if bar.High > s.high {
		s.high = bar.High
	}
	if bar.Low < s.low {
		s.low = bar.Low
	}
	if key == s.firstKey {
		s.first = bar
	}
	if key == s.lastKey {
		s.last = bar
	}
	s.recovered = s.recovered || bar.Recovered
	s.quality = worseQuality(s.quality, bar.Quality)
}

func (s *rollupState) rebuildSummary() {
	first := true
	s.volume = 0
	s.recovered = false
	s.quality = ""
	for key, bar := range s.minutes {
		if first {
			s.firstKey = key
			s.lastKey = key
			s.first = bar
			s.last = bar
			s.high = bar.High
			s.low = bar.Low
			s.quality = bar.Quality
			first = false
		} else {
			if key < s.firstKey {
				s.firstKey = key
				s.first = bar
			}
			if key > s.lastKey {
				s.lastKey = key
				s.last = bar
			}
			if bar.High > s.high {
				s.high = bar.High
			}
			if bar.Low < s.low {
				s.low = bar.Low
			}
			s.quality = worseQuality(s.quality, bar.Quality)
		}
		s.volume += bar.Volume
		s.recovered = s.recovered || bar.Recovered
	}
}

func newerMinute(previous, next domain.Bar) bool {
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
	if !next.CorrectedAt.Equal(previous.CorrectedAt) {
		return next.CorrectedAt.After(previous.CorrectedAt)
	}
	if !next.CreatedAt.Equal(previous.CreatedAt) {
		return next.CreatedAt.After(previous.CreatedAt)
	}
	return false
}

func completeRollup(state *rollupState) bool {
	return state != nil && state.expected > 0 && state.count == state.expected && state.finals == state.expected
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
	if state == nil || state.count == 0 {
		return domain.Bar{}
	}
	return domain.Bar{
		InstrumentID:        state.first.InstrumentID,
		Timeframe:           timeframe,
		OpenTime:            state.open,
		CloseTime:           state.close,
		Open:                state.first.Open,
		High:                state.high,
		Low:                 state.low,
		Close:               state.last.Close,
		Volume:              state.volume,
		Revision:            0,
		AuthorityProvider:   state.last.AuthorityProvider,
		Quality:             state.quality,
		Recovered:           state.recovered,
		SourceSequence:      state.last.SourceSequence,
		CandleEngineVersion: version,
		SyntheticVersion:    state.last.SyntheticVersion,
		CreatedAt:           state.last.CreatedAt,
	}
}

func qualityRank(value domain.Quality) int {
	rank := map[domain.Quality]int{
		domain.QualityGood:      0,
		domain.QualityRecovered: 1,
		domain.QualityPartial:   2,
		domain.QualityStale:     3,
		domain.QualityDegraded:  4,
		domain.QualityInvalid:   5,
	}
	return rank[value]
}

func worseQuality(a, b domain.Quality) domain.Quality {
	if qualityRank(b) > qualityRank(a) {
		return b
	}
	return a
}
