package runtime

import (
	"errors"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/feedstatus"
	"github.com/indigiti/QNext/services/market-core/internal/observability"
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

type SubminuteTelemetrySource interface {
	SubminuteTelemetry() any
}

var subminuteTelemetryOnce sync.Once

type MarketSink struct {
	Pipeline          TickPipeline
	Synthetic         SyntheticAssembler
	Synthetics        []SyntheticAssembler
	Publisher         BarPublisher
	DirectInstruments map[string]bool
	Observer          func(domain.Tick)

	// CarryHealthy is the authoritative gate for zero-volume 5s carry bars.
	// Production defaults to feedstatus.HealthyForCarryDefault so a wall-clock
	// finalizer cannot fabricate continuity when provider or instrument data is
	// stale/degraded.
	CarryHealthy func(string, domain.Tick, time.Time) bool

	// ClockInterval controls how frequently the process clock checks for due 5s
	// boundaries. The default 100ms cadence bounds normal finalization lag while
	// candle alignment itself remains anchored to the exchange/session bucket.
	ClockInterval time.Duration
	ClockDone     <-chan struct{}
	ClockError    func(error)

	clockOnce sync.Once
}

func (s *MarketSink) Handle(tick domain.Tick) error {
	if s.Pipeline == nil {
		return errors.New("canonical tick pipeline is required")
	}

	if source, ok := s.Pipeline.(SubminuteTelemetrySource); ok {
		subminuteTelemetryOnce.Do(func() {
			observability.SetSubminuteSource(source.SubminuteTelemetry)
		})
	}

	// Start the independent finalization clock on first live activity. From this
	// point candle closure no longer depends on another provider tick arriving.
	s.startClockFinalizer()

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
	return nil
}

func (s *MarketSink) startClockFinalizer() {
	clocked, ok := s.Pipeline.(ClockPipeline)
	if !ok {
		return
	}

	s.clockOnce.Do(func() {
		interval := s.ClockInterval
		if interval <= 0 {
			interval = 100 * time.Millisecond
		}
		healthy := s.CarryHealthy
		if healthy == nil {
			healthy = feedstatus.HealthyForCarryDefault
		}
		done := s.ClockDone
		onError := s.ClockError

		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case now := <-ticker.C:
					bars, err := clocked.AdvanceClock(now.UTC(), healthy)
					if err != nil {
						if onError != nil {
							onError(err)
						}
						continue
					}
					s.publishBars(bars)
				}
			}
		}()
	})
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
