package pipeline

import (
	"errors"
	"fmt"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type HistoryWriter interface {
	AppendBar(domain.Bar) error
}

type Pipeline struct {
	candles    *candle.Engine
	history    HistoryWriter
	timeframes []string
}

func New(candles *candle.Engine, history HistoryWriter, timeframes []string) (*Pipeline, error) {
	if candles == nil {
		return nil, errors.New("candle engine is required")
	}
	if len(timeframes) == 0 {
		return nil, errors.New("at least one timeframe is required")
	}
	for _, timeframe := range timeframes {
		if err := candle.ValidateTimeframe(timeframe); err != nil {
			return nil, err
		}
	}

	copyOfTimeframes := append([]string(nil), timeframes...)
	return &Pipeline{
		candles:    candles,
		history:    history,
		timeframes: copyOfTimeframes,
	}, nil
}

func (p *Pipeline) ApplyTick(tick domain.Tick) ([]domain.Bar, error) {
	var updates []domain.Bar
	for _, timeframe := range p.timeframes {
		bars, err := p.candles.Apply(tick, timeframe)
		if err != nil {
			return nil, fmt.Errorf("apply %s candle: %w", timeframe, err)
		}
		for _, bar := range bars {
			if bar.Final && p.history != nil {
				if err := p.history.AppendBar(bar); err != nil {
					return nil, fmt.Errorf("persist %s bar: %w", timeframe, err)
				}
			}
			updates = append(updates, bar)
		}
	}
	return updates, nil
}
