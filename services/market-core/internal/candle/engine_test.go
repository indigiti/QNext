package candle

import (
	"errors"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func tick(at string, price float64, sequence uint64) domain.Tick {
	ts, err := time.Parse(time.RFC3339, at)
	if err != nil {
		panic(err)
	}
	return domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "fixture",
		Price:        price,
		Quantity:     float64(sequence * 10),
		EventTime:    ts,
		Sequence:     sequence,
		Quality:      domain.QualityGood,
	}
}

func TestThirtySecondCandleFinality(t *testing.T) {
	engine := New("candle-v1")

	for _, input := range []domain.Tick{
		tick("2026-09-24T03:45:00Z", 25100, 1),
		tick("2026-09-24T03:45:10Z", 25102, 2),
		tick("2026-09-24T03:45:20Z", 25099, 3),
	} {
		if _, err := engine.Apply(input, "30s"); err != nil {
			t.Fatal(err)
		}
	}

	out, err := engine.Apply(tick("2026-09-24T03:45:30Z", 25105, 4), "30s")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected finalized+forming bars, got %d", len(out))
	}

	final := out[0]
	if !final.Final {
		t.Fatal("previous bar must be final")
	}
	if final.Open != 25100 || final.High != 25102 || final.Low != 25099 || final.Close != 25099 {
		t.Fatalf("unexpected OHLC: %+v", final)
	}
	if final.Volume != 60 {
		t.Fatalf("unexpected volume: got %.0f want 60", final.Volume)
	}

	forming := out[1]
	if forming.Final || forming.Open != 25105 || forming.Close != 25105 {
		t.Fatalf("unexpected forming bar: %+v", forming)
	}
}

func TestLateTickRejected(t *testing.T) {
	engine := New("candle-v1")
	if _, err := engine.Apply(tick("2026-09-24T03:45:30Z", 25105, 2), "30s"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Apply(tick("2026-09-24T03:45:00Z", 25100, 1), "30s"); !errors.Is(err, ErrLateTick) {
		t.Fatalf("expected ErrLateTick, got %v", err)
	}
}
