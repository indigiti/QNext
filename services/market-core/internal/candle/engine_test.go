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

func TestIntradayBucketRejectsTicksOutsideRegularSession(t *testing.T) {
	for _, input := range []string{
		"2026-10-13T09:14:59+05:30",
		"2026-10-13T15:30:00+05:30",
		"2026-10-13T15:30:05+05:30",
		"2026-10-13T23:00:00+05:30",
	} {
		at, err := time.Parse(time.RFC3339, input)
		if err != nil {
			t.Fatal(err)
		}
		for _, timeframe := range []string{"5s", "15s", "30s", "1m", "3m"} {
			_, _, err := Bucket(at, timeframe)
			if !errors.Is(err, ErrOutsideSession) {
				t.Fatalf("%s at %s: want ErrOutsideSession, got %v", timeframe, input, err)
			}
		}
	}
}

func TestRegularSessionLastBucketAndNextSessionOpen(t *testing.T) {
	for _, input := range []struct {
		at        string
		timeframe string
		open      string
		closeAt   string
	}{
		{"2026-10-13T09:15:00+05:30", "5s", "2026-10-13T09:15:00+05:30", "2026-10-13T09:15:05+05:30"},
		{"2026-10-13T15:29:55+05:30", "5s", "2026-10-13T15:29:55+05:30", "2026-10-13T15:30:00+05:30"},
		{"2026-10-13T15:29:55+05:30", "15s", "2026-10-13T15:29:45+05:30", "2026-10-13T15:30:00+05:30"},
		{"2026-10-13T15:29:55+05:30", "30s", "2026-10-13T15:29:30+05:30", "2026-10-13T15:30:00+05:30"},
		{"2026-10-13T15:29:55+05:30", "1m", "2026-10-13T15:29:00+05:30", "2026-10-13T15:30:00+05:30"},
		{"2026-10-14T09:15:00+05:30", "5s", "2026-10-14T09:15:00+05:30", "2026-10-14T09:15:05+05:30"},
	} {
		at, _ := time.Parse(time.RFC3339, input.at)
		wantOpen, _ := time.Parse(time.RFC3339, input.open)
		wantClose, _ := time.Parse(time.RFC3339, input.closeAt)
		open, closeAt, err := Bucket(at, input.timeframe)
		if err != nil || !open.Equal(wantOpen) || !closeAt.Equal(wantClose) || !closeAt.After(open) {
			t.Fatalf("%s at %s: %s..%s err=%v, want %s..%s", input.timeframe, input.at, open, closeAt, err, wantOpen, wantClose)
		}
	}
}

func TestCarryFinalizerStopsCleanlyAtSessionClose(t *testing.T) {
	engine := New("session-close-test")
	at, _ := time.Parse(time.RFC3339, "2026-10-13T15:29:55+05:30")
	input := domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "upstox",
		Price:        25000,
		EventTime:    at,
		Quality:      domain.QualityGood,
	}
	if _, err := engine.Apply(input, "5s"); err != nil {
		t.Fatal(err)
	}
	closeAt, _ := time.Parse(time.RFC3339, "2026-10-13T15:30:00+05:30")
	bars, err := engine.Advance("NSE:NIFTY50", "5s", closeAt, true)
	if err != nil {
		t.Fatalf("session close is not a candle error: %v", err)
	}
	if len(bars) != 1 || !bars[0].Final || !bars[0].CloseTime.Equal(closeAt) {
		t.Fatalf("expected one terminal final and no phantom carry: %+v", bars)
	}
	repeated, err := engine.Advance("NSE:NIFTY50", "5s", closeAt.Add(8*time.Hour), true)
	if err != nil || len(repeated) != 0 {
		t.Fatalf("clock generated bars after session close: %+v err=%v", repeated, err)
	}
}
