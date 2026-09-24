package runtime

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type fakeAssembler struct {
	tick domain.Tick
}

func (a fakeAssembler) Apply(domain.Tick) (domain.Tick, bool, error) {
	return a.tick, true, nil
}

type collectingPublisher struct {
	bars []domain.Bar
}

func (p *collectingPublisher) PublishBar(bar domain.Bar) {
	p.bars = append(p.bars, bar)
}

func TestMarketSinkUsesSamePipelineForDirectAndSyntheticTicks(t *testing.T) {
	pipeline := &fakePipeline{}
	publisher := &collectingPublisher{}
	sink := &MarketSink{
		Pipeline: pipeline,
		Synthetic: fakeAssembler{tick: domain.Tick{
			InstrumentID: "QNEXT:NIFTY-SYN",
			Price:        25101,
			EventTime:    time.Now().UTC(),
		}},
		Publisher: publisher,
		DirectInstruments: map[string]bool{
			"NSE:NIFTY50": true,
		},
	}

	if err := sink.Handle(domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Price:        25100,
		EventTime:    time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if len(pipeline.ticks) != 2 {
		t.Fatalf("expected direct + synthetic ticks in same pipeline, got %+v", pipeline.ticks)
	}
	if pipeline.ticks[0].InstrumentID != "NSE:NIFTY50" || pipeline.ticks[1].InstrumentID != "QNEXT:NIFTY-SYN" {
		t.Fatalf("unexpected pipeline order: %+v", pipeline.ticks)
	}
}
