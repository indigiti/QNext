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

func TestExpandedTimeframesUseIndianSessionAnchors(t *testing.T) {
	tests := []struct {
		timeframe string
		at        string
		wantOpen  string
		wantClose string
	}{
		{"10m", "2026-09-25T03:47:00Z", "2026-09-25T03:45:00Z", "2026-09-25T03:55:00Z"},
		{"1h", "2026-09-25T05:20:00Z", "2026-09-25T04:45:00Z", "2026-09-25T05:45:00Z"},
		{"4h", "2026-09-25T08:00:00Z", "2026-09-25T07:45:00Z", "2026-09-25T10:00:00Z"},
		{"1D", "2026-09-25T08:00:00Z", "2026-09-24T18:30:00Z", "2026-09-25T18:30:00Z"},
		{"1W", "2026-09-25T08:00:00Z", "2026-09-20T18:30:00Z", "2026-09-27T18:30:00Z"},
		{"3M", "2026-09-25T08:00:00Z", "2026-06-30T18:30:00Z", "2026-09-30T18:30:00Z"},
	}

	for _, test := range tests {
		at, _ := time.Parse(time.RFC3339, test.at)
		wantOpen, _ := time.Parse(time.RFC3339, test.wantOpen)
		wantClose, _ := time.Parse(time.RFC3339, test.wantClose)
		open, closeAt, err := Bucket(at, test.timeframe)
		if err != nil {
			t.Fatalf("%s: %v", test.timeframe, err)
		}
		if !open.Equal(wantOpen) || !closeAt.Equal(wantClose) {
			t.Fatalf("%s bucket=%s..%s want %s..%s", test.timeframe, open, closeAt, wantOpen, wantClose)
		}
	}
}

func TestSupportedTimeframesValidate(t *testing.T) {
	for _, timeframe := range SupportedTimeframes() {
		if err := ValidateTimeframe(timeframe); err != nil {
			t.Fatalf("%s should validate: %v", timeframe, err)
		}
	}
	for _, timeframe := range []string{"0m", "7h", "2D", "2W", "2M"} {
		if err := ValidateTimeframe(timeframe); err == nil {
			t.Fatalf("%s should be rejected", timeframe)
		}
	}
}


func TestSessionResolverOwnsIntradayBucketAlignment(t *testing.T) {
	resolver := func(_ string, at time.Time) (SessionWindow, bool, error) {
		open, _ := time.Parse(time.RFC3339, "2026-09-25T04:00:00Z")
		closeAt, _ := time.Parse(time.RFC3339, "2026-09-25T09:30:00Z")
		return SessionWindow{Open: open, Close: closeAt}, !at.Before(open) && at.Before(closeAt), nil
	}
	engine := NewWithSessionResolver("candle-v3-calendar", resolver)

	at, _ := time.Parse(time.RFC3339, "2026-09-25T04:07:00Z")
	open, closeAt, err := engine.Bucket("NSE:NIFTY50", at, "10m")
	if err != nil {
		t.Fatal(err)
	}
	wantOpen, _ := time.Parse(time.RFC3339, "2026-09-25T04:00:00Z")
	wantClose, _ := time.Parse(time.RFC3339, "2026-09-25T04:10:00Z")
	if !open.Equal(wantOpen) || !closeAt.Equal(wantClose) {
		t.Fatalf("calendar-aligned bucket=%s..%s want %s..%s", open, closeAt, wantOpen, wantClose)
	}

	outside, _ := time.Parse(time.RFC3339, "2026-09-25T03:59:59Z")
	if _, err := engine.Apply(domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "fixture",
		Price:        25100,
		EventTime:    outside,
		Quality:      domain.QualityGood,
	}, "1m"); !errors.Is(err, ErrOutsideSession) {
		t.Fatalf("expected ErrOutsideSession, got %v", err)
	}
}

func TestFinalizeDueClosesQuietCandleWithoutNextTick(t *testing.T) {
	engine := New("candle-v3-clock-final")
	first := tick("2026-09-24T03:45:00Z", 25100, 1)
	if _, err := engine.Apply(first, "30s"); err != nil {
		t.Fatal(err)
	}

	before, _ := time.Parse(time.RFC3339, "2026-09-24T03:45:29.999Z")
	if got := engine.FinalizeDue(before); len(got) != 0 {
		t.Fatalf("candle finalized early: %+v", got)
	}

	atClose, _ := time.Parse(time.RFC3339, "2026-09-24T03:45:30Z")
	finalized := engine.FinalizeDue(atClose)
	if len(finalized) != 1 || !finalized[0].Final {
		t.Fatalf("expected one clock-finalized candle, got %+v", finalized)
	}
	if got := engine.FinalizeDue(atClose.Add(time.Second)); len(got) != 0 {
		t.Fatalf("clock finalization must be idempotent, got %+v", got)
	}

	if _, err := engine.Apply(tick("2026-09-24T03:45:20Z", 25099, 2), "30s"); !errors.Is(err, ErrLateTick) {
		t.Fatalf("tick for clock-finalized candle must be late, got %v", err)
	}

	next, err := engine.Apply(tick("2026-09-24T03:45:30Z", 25105, 3), "30s")
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || next[0].Final {
		t.Fatalf("next bucket should emit only its forming candle, got %+v", next)
	}
}
