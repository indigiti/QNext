package runtime

import (
	"errors"
	"testing"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type fakePipeline struct {
	ticks []domain.Tick
	err   error
}

func (p *fakePipeline) ApplyTick(tick domain.Tick) ([]domain.Bar, error) {
	p.ticks = append(p.ticks, tick)
	return nil, p.err
}

func TestPipelineSinkForwardsCanonicalTick(t *testing.T) {
	pipeline := &fakePipeline{}
	sink, err := PipelineSink(pipeline)
	if err != nil {
		t.Fatal(err)
	}

	tick := domain.Tick{InstrumentID: "NSE:NIFTY50", Price: 25100}
	if err := sink(tick); err != nil {
		t.Fatal(err)
	}
	if len(pipeline.ticks) != 1 || pipeline.ticks[0].InstrumentID != tick.InstrumentID {
		t.Fatalf("unexpected pipeline ticks: %+v", pipeline.ticks)
	}
}

func TestPipelineSinkPropagatesFailure(t *testing.T) {
	pipeline := &fakePipeline{err: errors.New("persist failed")}
	sink, err := PipelineSink(pipeline)
	if err != nil {
		t.Fatal(err)
	}
	if err := sink(domain.Tick{}); err == nil || err.Error() != "persist failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}
