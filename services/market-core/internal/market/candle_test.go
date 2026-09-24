package market

import (
	"testing"
	"time"
)

func TestCandleBuilderBuildsAndFinalizes(t *testing.T) {
	builder, err := NewCandleBuilder(time.Minute, "candle-v1")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 24, 9, 15, 10, 0, time.FixedZone("IST", 5*60*60+30*60))
	tick := func(seq uint64, at time.Time, price, volumeDelta float64) CanonicalTick {
		return CanonicalTick{
			Schema:        SchemaCanonicalTickV1,
			InstrumentID:  "NSE:NIFTY50",
			Provider:      "upstox",
			ProviderKey:   "NSE_INDEX|Nifty 50",
			Price:         price,
			VolumeDelta:   volumeDelta,
			EventTime:     at,
			ReceivedTime:  at.Add(5 * time.Millisecond),
			ProcessedTime: at.Add(8 * time.Millisecond),
			Sequence:      seq,
			Quality:       QualityGood,
		}
	}

	bar, final, err := builder.Apply(tick(1, base, 25180, 10))
	if err != nil || final != nil {
		t.Fatalf("unexpected first apply: final=%v err=%v", final, err)
	}
	if bar.Open != 25180 || bar.High != 25180 || bar.Low != 25180 || bar.Close != 25180 {
		t.Fatalf("unexpected first bar: %+v", bar)
	}

	bar, final, err = builder.Apply(tick(2, base.Add(20*time.Second), 25190, 12))
	if err != nil || final != nil {
		t.Fatalf("unexpected second apply: final=%v err=%v", final, err)
	}
	if bar.High != 25190 || bar.Close != 25190 || bar.Volume != 22 {
		t.Fatalf("unexpected updated bar: %+v", bar)
	}

	next, final, err := builder.Apply(tick(3, base.Add(60*time.Second), 25200, 7))
	if err != nil {
		t.Fatal(err)
	}
	if final == nil || !final.Final {
		t.Fatal("expected finalized previous candle")
	}
	if final.Open != 25180 || final.High != 25190 || final.Close != 25190 {
		t.Fatalf("unexpected finalized bar: %+v", final)
	}
	if next.Open != 25200 || next.Close != 25200 || next.Final {
		t.Fatalf("unexpected next bar: %+v", next)
	}
}

func TestCanonicalTickRejectsInvalidOrdering(t *testing.T) {
	event := time.Now()
	tick := CanonicalTick{
		Schema:        SchemaCanonicalTickV1,
		InstrumentID:  "NSE:NIFTY50",
		Provider:      "upstox",
		ProviderKey:   "NSE_INDEX|Nifty 50",
		Price:         25180,
		EventTime:     event,
		ReceivedTime:  event.Add(-time.Millisecond),
		ProcessedTime: event,
		Sequence:      1,
		Quality:       QualityGood,
	}
	if tick.Validate() == nil {
		t.Fatal("expected timestamp ordering validation error")
	}
}
