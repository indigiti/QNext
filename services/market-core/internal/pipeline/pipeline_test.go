package pipeline

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
)

func pipelineTick(at string, price float64, sequence uint64) domain.Tick {
	ts, err := time.Parse(time.RFC3339, at)
	if err != nil {
		panic(err)
	}
	return domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "fixture",
		Price:        price,
		EventTime:    ts,
		Sequence:     sequence,
		Quality:      domain.QualityGood,
	}
}

func TestPipelinePersistsOnlyFinalizedBars(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("candle-v1"), store, []string{"30s", "1m"})
	if err != nil {
		t.Fatal(err)
	}

	inputs := []domain.Tick{
		pipelineTick("2026-09-24T03:45:00Z", 25100, 1),
		pipelineTick("2026-09-24T03:45:10Z", 25102, 2),
		pipelineTick("2026-09-24T03:45:20Z", 25099, 3),
		pipelineTick("2026-09-24T03:45:30Z", 25105, 4),
	}
	for _, input := range inputs {
		if _, err := pipe.ApplyTick(input); err != nil {
			t.Fatal(err)
		}
	}

	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	bars30s, err := store.LoadDay("NSE:NIFTY50", "30s", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars30s) != 1 || !bars30s[0].Final {
		t.Fatalf("expected one finalized 30s bar, got %+v", bars30s)
	}

	bars1m, err := store.LoadDay("NSE:NIFTY50", "1m", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars1m) != 0 {
		t.Fatalf("expected no finalized 1m bars yet, got %+v", bars1m)
	}
}

func TestPipelineRejectsUnsupportedTimeframe(t *testing.T) {
	_, err := New(candle.New("candle-v1"), nil, []string{"2m"})
	if err == nil {
		t.Fatal("expected unsupported timeframe to fail")
	}
}
