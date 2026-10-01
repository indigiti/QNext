package runtime

import (
	"errors"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type SyntheticAssembler interface {
	Apply(domain.Tick) (domain.Tick, bool, error)
}

type BarPublisher interface {
	PublishBar(domain.Bar)
}

type ClockPipeline interface {
	AdvanceClock(time.Time, func(string, domain.Tick, time.Time) bool) ([]domain.Bar, error)
}

type MarketSink struct {
	Pipeline          TickPipeline
	Synthetic         SyntheticAssembler
	Synthetics        []SyntheticAssembler
	Publisher         BarPublisher
	DirectInstruments map[string]bool
	Observer          func(domain.Tick)
}

func (s *MarketSink) Handle(tick domain.Tick) error {
	if s.Pipeline == nil {
		return errors.New("canonical tick pipeline is required")
	}

	if s.Observer != nil {
		s.Observer(tick)
	}

	if len(s.DirectInstruments) == 0 || s.DirectInstruments[tick.InstrumentID] {
		if err := s.applyCanonical(tick); err != nil {
			return err
		}
	}

	if s.Synthetic != nil {
		if err := s.applySynthetic(s.Synthetic, tick); err != nil {
			return err
		}
	}
	for _, synthetic := range s.Synthetics {
		if synthetic == nil {
			continue
		}
		if err := s.applySynthetic(synthetic, tick); err != nil {
			return err
		}
	}

	// Every fresh provider event advances the trusted market clock for all
	// canonical 5s streams. The pipeline's built-in freshness guard decides
	// whether an empty bucket may be carried; a dead feed produces no clock
	// events and therefore cannot fabricate flat candles.
	if clocked, ok := s.Pipeline.(ClockPipeline); ok && !tick.EventTime.IsZero() {
		bars, err := clocked.AdvanceClock(tick.EventTime.UTC(), nil)
		if err != nil {
			return err
		}
		s.publishBars(bars)
	}
	return nil
}

func (s *MarketSink) applySynthetic(assembler SyntheticAssembler, tick domain.Tick) error {
	syntheticTick, emitted, err := assembler.Apply(tick)
	if err != nil {
		return err
	}
	if !emitted {
		return nil
	}
	if s.Observer != nil {
		s.Observer(syntheticTick)
	}
	return s.applyCanonical(syntheticTick)
}

func (s *MarketSink) applyCanonical(tick domain.Tick) error {
	bars, err := s.Pipeline.ApplyTick(tick)
	if err != nil {
		return err
	}
	s.publishBars(bars)
	return nil
}

func (s *MarketSink) publishBars(bars []domain.Bar) {
	if s.Publisher == nil {
		return
	}
	for _, bar := range bars {
		s.Publisher.PublishBar(bar)
	}
}
