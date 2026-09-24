package runtime

import (
	"errors"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type SyntheticAssembler interface {
	Apply(domain.Tick) (domain.Tick, bool, error)
}

type BarPublisher interface {
	PublishBar(domain.Bar)
}

type MarketSink struct {
	Pipeline          TickPipeline
	Synthetic         SyntheticAssembler
	Publisher         BarPublisher
	DirectInstruments map[string]bool
}

func (s *MarketSink) Handle(tick domain.Tick) error {
	if s.Pipeline == nil {
		return errors.New("canonical tick pipeline is required")
	}

	if len(s.DirectInstruments) == 0 || s.DirectInstruments[tick.InstrumentID] {
		if err := s.applyCanonical(tick); err != nil {
			return err
		}
	}

	if s.Synthetic != nil {
		syntheticTick, emitted, err := s.Synthetic.Apply(tick)
		if err != nil {
			return err
		}
		if emitted {
			if err := s.applyCanonical(syntheticTick); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *MarketSink) applyCanonical(tick domain.Tick) error {
	bars, err := s.Pipeline.ApplyTick(tick)
	if err != nil {
		return err
	}
	if s.Publisher != nil {
		for _, bar := range bars {
			s.Publisher.PublishBar(bar)
		}
	}
	return nil
}
