package runtime

import (
	"errors"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type TickPipeline interface {
	ApplyTick(domain.Tick) ([]domain.Bar, error)
}

func PipelineSink(pipeline TickPipeline) (func(domain.Tick) error, error) {
	if pipeline == nil {
		return nil, errors.New("canonical tick pipeline is required")
	}

	return func(tick domain.Tick) error {
		_, err := pipeline.ApplyTick(tick)
		return err
	}, nil
}
