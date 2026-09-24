package certification

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/integrity"
	"github.com/indigiti/QNext/services/market-core/internal/pipeline"
	qruntime "github.com/indigiti/QNext/services/market-core/internal/runtime"
	"github.com/indigiti/QNext/services/market-core/internal/stream"
	"github.com/indigiti/QNext/services/market-core/internal/synthetic"
)

func TestQ1HistoryLiveContinuityAndResume(t *testing.T) {
	store := history.New(t.TempDir())
	broker := stream.NewBroker(32, 32)
	canonicalPipeline, err := pipeline.New(candle.New("candle-v1"), store, []string{"1m"})
	if err != nil {
		t.Fatal(err)
	}
	sink := &qruntime.MarketSink{
		Pipeline:  canonicalPipeline,
		Publisher: broker,
		DirectInstruments: map[string]bool{
			"NSE:NIFTY50": true,
		},
	}

	subscription, err := broker.Subscribe("NSE:NIFTY50", "1m", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Cancel()

	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	for sequence, tick := range []domain.Tick{
		{InstrumentID: "NSE:NIFTY50", Provider: "fixture", Price: 25100, EventTime: base, Quality: domain.QualityGood},
		{InstrumentID: "NSE:NIFTY50", Provider: "fixture", Price: 25102, EventTime: base.Add(30 * time.Second), Quality: domain.QualityGood},
		{InstrumentID: "NSE:NIFTY50", Provider: "fixture", Price: 25101, EventTime: base.Add(time.Minute), Quality: domain.QualityGood},
	} {
		tick.Sequence = uint64(sequence + 1)
		if err := sink.Handle(tick); err != nil {
			t.Fatal(err)
		}
	}

	events := make([]stream.BarEvent, 0, 4)
	for i := 0; i < 4; i++ {
		events = append(events, <-subscription.Events)
	}

	bars, err := store.LoadRange("NSE:NIFTY50", "1m", base, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one finalized history bar, got %+v", bars)
	}
	if err := integrity.CheckBarContinuity(bars, "1m"); err != nil {
		t.Fatal(err)
	}

	var finalEvent, nextLive stream.BarEvent
	for _, event := range events {
		if event.Bar.Final {
			finalEvent = event
		}
		if event.Bar.OpenTime.Equal(base.Add(time.Minute)) && !event.Bar.Final {
			nextLive = event
		}
	}
	if finalEvent.Seq == 0 || nextLive.Seq == 0 {
		t.Fatalf("missing boundary events: %+v", events)
	}
	if finalEvent.Bar.Key() != bars[0].Key() {
		t.Fatalf("history and stream disagree on finalized bar: history=%s live=%s", bars[0].Key(), finalEvent.Bar.Key())
	}
	if err := integrity.CheckHistoryLiveBoundary(bars[0], nextLive.Bar); err != nil {
		t.Fatal(err)
	}

	resumed, err := broker.SubscribeByID(finalEvent.StreamID, finalEvent.Seq)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Cancel()
	if resumed.ResyncRequired || len(resumed.Replay) != 1 {
		t.Fatalf("expected one replay event after final history bar, got %+v", resumed)
	}
	if !resumed.Replay[0].Bar.OpenTime.Equal(nextLive.Bar.OpenTime) {
		t.Fatalf("resume did not preserve history/live boundary: %+v", resumed.Replay)
	}
}

func TestQ1NiftySyntheticUsesCanonicalCandleHistory(t *testing.T) {
	store := history.New(t.TempDir())
	broker := stream.NewBroker(64, 64)
	canonicalPipeline, err := pipeline.New(candle.New("candle-v1"), store, []string{"1m"})
	if err != nil {
		t.Fatal(err)
	}

	legs := []synthetic.LegBinding{
		{InstrumentID: "CE25000", Strike: 25000, Side: synthetic.LegCall},
		{InstrumentID: "PE25000", Strike: 25000, Side: synthetic.LegPut},
		{InstrumentID: "CE25050", Strike: 25050, Side: synthetic.LegCall},
		{InstrumentID: "PE25050", Strike: 25050, Side: synthetic.LegPut},
		{InstrumentID: "CE25100", Strike: 25100, Side: synthetic.LegCall},
		{InstrumentID: "PE25100", Strike: 25100, Side: synthetic.LegPut},
		{InstrumentID: "CE25150", Strike: 25150, Side: synthetic.LegCall},
		{InstrumentID: "PE25150", Strike: 25150, Side: synthetic.LegPut},
		{InstrumentID: "CE25200", Strike: 25200, Side: synthetic.LegCall},
		{InstrumentID: "PE25200", Strike: 25200, Side: synthetic.LegPut},
	}
	var syntheticSequence uint64
	assembler, err := synthetic.NewAssembler(synthetic.Definition{
		ID:                     "QNEXT:NIFTY-SYN",
		Version:                "nifty-syn-v1",
		MinimumValidCandidates: 3,
		MaxLegAge:              2 * time.Second,
		MaxLegTimeSkew:         time.Second,
	}, legs, func() uint64 {
		syntheticSequence++
		return syntheticSequence
	})
	if err != nil {
		t.Fatal(err)
	}

	sink := &qruntime.MarketSink{
		Pipeline:  canonicalPipeline,
		Synthetic: assembler,
		Publisher: broker,
		DirectInstruments: map[string]bool{
			"NSE:NIFTY50": true,
		},
	}

	values := map[string]float64{
		"CE25000": 160, "PE25000": 58,
		"CE25050": 128, "PE25050": 78,
		"CE25100": 101, "PE25100": 100,
		"CE25150": 74, "PE25150": 125,
		"CE25200": 55, "PE25200": 152,
	}
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)

	feedRound := func(at time.Time) {
		t.Helper()
		for i, leg := range legs {
			if err := sink.Handle(domain.Tick{
				InstrumentID:  leg.InstrumentID,
				Provider:      "upstox",
				Price:         values[leg.InstrumentID],
				EventTime:     at,
				ReceivedTime:  at,
				ProcessedTime: at,
				Sequence:      uint64(i + 1),
				Quality:       domain.QualityGood,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	feedRound(base)
	feedRound(base.Add(time.Minute))

	bars, err := store.LoadRange("QNEXT:NIFTY-SYN", "1m", base, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one finalized synthetic bar, got %+v", bars)
	}
	bar := bars[0]
	if bar.Close != 25101 || bar.SyntheticVersion != "nifty-syn-v1" {
		t.Fatalf("unexpected synthetic candle lineage/value: %+v", bar)
	}
	if bar.AuthorityProvider != synthetic.SyntheticProvider {
		t.Fatalf("unexpected synthetic authority: %+v", bar)
	}
}
