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

type Pipeline struct {
	mu      sync.Mutex
	candles *candle.Engine
	rollups *rollupEngine
	history HistoryWriter
	direct  []string
	derived []string
}

func New(candles *candle.Engine, history HistoryWriter, timeframes []string) (*Pipeline, error) {
	if candles == nil {
		return nil, errors.New("candle engine is required")
	}
	if len(timeframes) == 0 {
		return nil, errors.New("at least one timeframe is required")
	}

	hasOneMinute := false
	var direct []string
	var derived []string
	for _, timeframe := range timeframes {
		if err := candle.ValidateTimeframe(timeframe); err != nil {
			return nil, err
		}
		switch {
		case timeframe == "1m":
			hasOneMinute = true
			direct = append(direct, timeframe)
		case strings.HasSuffix(timeframe, "s"):
			direct = append(direct, timeframe)
		case timeframe == "1W" || strings.HasSuffix(timeframe, "M"):
			// Calendar week/month intervals are low-cardinality and stay tick-built.
			direct = append(direct, timeframe)
		default:
			derived = append(derived, timeframe)
		}
	}
	if len(derived) > 0 && !hasOneMinute {
		return nil, errors.New("1m is required when derived candle timeframes are enabled")
	}

	return &Pipeline{
		candles: candles,
		rollups: newRollupEngine("candle-rollup-v1", candles.Bucket),
		history: history,
		direct:  append([]string(nil), direct...),
		derived: append([]string(nil), derived...),
	}, nil
}

func (p *Pipeline) ApplyTick(tick domain.Tick) ([]domain.Bar, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var updates []domain.Bar
	var oneMinuteUpdates []domain.Bar

	for _, timeframe := range p.direct {
		bars, err := p.candles.Apply(tick, timeframe)
		if err != nil {
			if errors.Is(err, candle.ErrOutsideSession) {
				return nil, nil
			}
			if errors.Is(err, candle.ErrLateTick) {
				continue
			}
			return nil, fmt.Errorf("apply %s candle: %w", timeframe, err)
		}
		if timeframe == "1m" {
			oneMinuteUpdates = append(oneMinuteUpdates, bars...)
		}
		for _, bar := range bars {
			if err := p.persistFinal(bar); err != nil {
				return nil, err
			}
			updates = append(updates, bar)
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

func (p *Pipeline) FinalizeDue(now time.Time) ([]domain.Bar, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	directFinals := p.candles.FinalizeDue(now)
	updates := make([]domain.Bar, 0, len(directFinals))
	for _, bar := range directFinals {
		if err := p.persistFinal(bar); err != nil {
			return nil, err
		}
		updates = append(updates, bar)

		if bar.Timeframe != "1m" {
			continue
		}
		for _, timeframe := range p.derived {
			bars, err := p.rollups.Apply(bar, timeframe)
			if err != nil {
				return nil, fmt.Errorf("roll up %s candle: %w", timeframe, err)
			}
			for _, derived := range bars {
				if err := p.persistFinal(derived); err != nil {
					return nil, err
				}
				updates = append(updates, derived)
			}
		}
	}
	return updates, nil
}

func (p *Pipeline) persistFinal(bar domain.Bar) error {
	if !bar.Final || p.history == nil {
		return nil
	}
	if err := p.history.AppendBar(bar); err != nil {
		return fmt.Errorf("persist %s bar: %w", bar.Timeframe, err)
	}
	return nil
}
