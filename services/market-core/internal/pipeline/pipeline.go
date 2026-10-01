package pipeline

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type HistoryWriter interface {
	AppendBar(domain.Bar) error
}

type AsyncHistoryWriter interface {
	AppendBarAsync(domain.Bar) error
}

type CarryHealth = func(instrumentID string, lastTick domain.Tick, at time.Time) bool

type SubminuteSnapshot struct {
	Enabled             bool              `json:"enabled"`
	SourceTimeframe     string            `json:"source_timeframe,omitempty"`
	FiveSecondFinals    uint64            `json:"five_second_finals"`
	CarryForwardFinals  uint64            `json:"carry_forward_finals"`
	ClockAdvanceCalls   uint64            `json:"clock_advance_calls"`
	FinalizedByInterval map[string]uint64 `json:"finalized_by_interval,omitempty"`
	IncompleteBuckets   map[string]uint64 `json:"incomplete_buckets,omitempty"`
}

type Pipeline struct {
	mu sync.Mutex

	candles   *candle.Engine
	rollups   *rollupEngine
	subminute *subminuteRollupEngine
	history   HistoryWriter

	direct           []string
	subminuteDerived []string
	derived          []string
	hasCanonical5s   bool
	lastTicks        map[string]domain.Tick
	stats            SubminuteSnapshot
}

func New(candles *candle.Engine, history HistoryWriter, timeframes []string) (*Pipeline, error) {
	if candles == nil {
		return nil, errors.New("candle engine is required")
	}
	if len(timeframes) == 0 {
		return nil, errors.New("at least one timeframe is required")
	}

	hasOneMinute := false
	hasFiveSeconds := false
	var direct []string
	var subminuteDerived []string
	var derived []string
	for _, timeframe := range timeframes {
		if err := candle.ValidateTimeframe(timeframe); err != nil {
			return nil, err
		}
		switch {
		case timeframe == "5s":
			hasFiveSeconds = true
			direct = append(direct, timeframe)
		case timeframe == "15s" || timeframe == "30s":
			subminuteDerived = append(subminuteDerived, timeframe)
		case timeframe == "1m":
			hasOneMinute = true
			subminuteDerived = append(subminuteDerived, timeframe)
		case strings.HasSuffix(timeframe, "s"):
			direct = append(direct, timeframe)
		case timeframe == "1W" || strings.HasSuffix(timeframe, "M"):
			direct = append(direct, timeframe)
		default:
			derived = append(derived, timeframe)
		}
	}
	if len(derived) > 0 && !hasOneMinute {
		return nil, errors.New("1m is required when derived candle timeframes are enabled")
	}
	if len(subminuteDerived) > 0 && !hasFiveSeconds {
		direct = append([]string{"5s"}, direct...)
		hasFiveSeconds = true
	}

	stats := SubminuteSnapshot{
		Enabled:             hasFiveSeconds && len(subminuteDerived) > 0,
		SourceTimeframe:     "5s",
		FinalizedByInterval: make(map[string]uint64),
		IncompleteBuckets:   make(map[string]uint64),
	}
	return &Pipeline{
		candles:          candles,
		rollups:          newRollupEngine("candle-rollup-v2-incremental"),
		subminute:        newSubminuteRollupEngine(),
		history:          history,
		direct:           append([]string(nil), direct...),
		subminuteDerived: append([]string(nil), subminuteDerived...),
		derived:          append([]string(nil), derived...),
		hasCanonical5s:   hasFiveSeconds,
		lastTicks:        make(map[string]domain.Tick),
		stats:            stats,
	}, nil
}

func (p *Pipeline) ApplyTick(tick domain.Tick) ([]domain.Bar, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastTicks[tick.InstrumentID] = tick
	var updates []domain.Bar
	var fiveSecondUpdates []domain.Bar

	for _, timeframe := range p.direct {
		bars, err := p.candles.Apply(tick, timeframe)
		if err != nil {
			return nil, fmt.Errorf("apply %s candle: %w", timeframe, err)
		}
		if timeframe == "5s" {
			fiveSecondUpdates = append(fiveSecondUpdates, bars...)
		}
		for _, bar := range bars {
			if err := p.persistFinal(bar); err != nil {
				return nil, err
			}
			p.observeFinal(bar)
			updates = append(updates, bar)
		}
	}

	derivedUpdates, err := p.applyFiveSecondUpdates(fiveSecondUpdates)
	if err != nil {
		return nil, err
	}
	updates = append(updates, derivedUpdates...)
	return updates, nil
}

// AdvanceClock closes canonical 5s buckets from a trusted live market clock.
// With a nil predicate the built-in guard only carries when the last good tick
// is at most eight seconds old. If the upstream stops, no market-clock calls
// occur; after a longer reconnect gap the 5s holes remain explicit and 1m can
// be repaired from upstream history instead of fabricating sub-minute OHLC.
func (p *Pipeline) AdvanceClock(at time.Time, healthy CarryHealth) ([]domain.Bar, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.hasCanonical5s || at.IsZero() {
		return nil, nil
	}
	p.stats.ClockAdvanceCalls++
	var updates []domain.Bar
	var fiveSecondUpdates []domain.Bar
	for instrumentID, lastTick := range p.lastTicks {
		carry := defaultCarryHealthy(lastTick, at.UTC())
		if healthy != nil {
			carry = healthy(instrumentID, lastTick, at.UTC())
		}
		bars, err := p.candles.Advance(instrumentID, "5s", at.UTC(), carry)
		if err != nil {
			return nil, fmt.Errorf("advance 5s candle for %s: %w", instrumentID, err)
		}
		for _, bar := range bars {
			if err := p.persistFinal(bar); err != nil {
				return nil, err
			}
			p.observeFinal(bar)
			fiveSecondUpdates = append(fiveSecondUpdates, bar)
			updates = append(updates, bar)
		}
	}
	derivedUpdates, err := p.applyFiveSecondUpdates(fiveSecondUpdates)
	if err != nil {
		return nil, err
	}
	updates = append(updates, derivedUpdates...)
	return updates, nil
}

func defaultCarryHealthy(lastTick domain.Tick, at time.Time) bool {
	if lastTick.EventTime.IsZero() || lastTick.Price <= 0 || at.Before(lastTick.EventTime) {
		return false
	}
	if lastTick.Quality == domain.QualityInvalid ||
		lastTick.Quality == domain.QualityDegraded ||
		lastTick.Quality == domain.QualityStale {
		return false
	}
	return at.Sub(lastTick.EventTime.UTC()) <= 8*time.Second
}

func (p *Pipeline) applyFiveSecondUpdates(fiveSecondUpdates []domain.Bar) ([]domain.Bar, error) {
	var updates []domain.Bar
	var oneMinuteUpdates []domain.Bar
	for _, source := range fiveSecondUpdates {
		for _, timeframe := range p.subminuteDerived {
			bars, err := p.subminute.Apply(source, timeframe)
			if err != nil {
				return nil, fmt.Errorf("roll up canonical 5s to %s: %w", timeframe, err)
			}
			for _, bar := range bars {
				if err := p.persistFinal(bar); err != nil {
					return nil, err
				}
				p.observeFinal(bar)
				if timeframe == "1m" {
					oneMinuteUpdates = append(oneMinuteUpdates, bar)
				}
				updates = append(updates, bar)
			}
		}
	}

	for _, minuteBar := range oneMinuteUpdates {
		for _, timeframe := range p.derived {
			bars, err := p.rollups.Apply(minuteBar, timeframe)
			if err != nil {
				return nil, fmt.Errorf("roll up %s candle: %w", timeframe, err)
			}
			for _, bar := range bars {
				if err := p.persistFinal(bar); err != nil {
					return nil, err
				}
				updates = append(updates, bar)
			}
		}
	}
	return updates, nil
}

func (p *Pipeline) observeFinal(bar domain.Bar) {
	if !bar.Final {
		return
	}
	if bar.Timeframe == "5s" {
		p.stats.FiveSecondFinals++
		if bar.CarryForward {
			p.stats.CarryForwardFinals++
		}
		return
	}
	if bar.Timeframe == "15s" || bar.Timeframe == "30s" || bar.Timeframe == "1m" {
		p.stats.FinalizedByInterval[bar.Timeframe]++
	}
}

func (p *Pipeline) SubminuteSnapshot() SubminuteSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := p.stats
	out.FinalizedByInterval = make(map[string]uint64, len(p.stats.FinalizedByInterval))
	for key, value := range p.stats.FinalizedByInterval {
		out.FinalizedByInterval[key] = value
	}
	out.IncompleteBuckets = make(map[string]uint64, len(p.subminute.incomplete))
	for key, value := range p.subminute.incomplete {
		out.IncompleteBuckets[key] = value
	}
	return out
}

func (p *Pipeline) persistFinal(bar domain.Bar) error {
	if !bar.Final || p.history == nil {
		return nil
	}
	if async, ok := p.history.(AsyncHistoryWriter); ok {
		if err := async.AppendBarAsync(bar); err != nil {
			return fmt.Errorf("queue %s bar persistence: %w", bar.Timeframe, err)
		}
		return nil
	}
	if err := p.history.AppendBar(bar); err != nil {
		return fmt.Errorf("persist %s bar: %w", bar.Timeframe, err)
	}
	return nil
}
