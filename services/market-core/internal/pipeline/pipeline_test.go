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
	_, err := New(candle.New("candle-v1"), nil, []string{"7h"})
	if err == nil {
		t.Fatal("expected unsupported timeframe to fail")
	}
}

func TestPipelineRollsUpTwoMinuteBarsFromCanonicalOneMinute(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("candle-v2"), store, []string{"1m", "2m"})
	if err != nil {
		t.Fatal(err)
	}

	inputs := []domain.Tick{
		pipelineTick("2026-09-24T03:45:00Z", 25100, 1),
		pipelineTick("2026-09-24T03:45:30Z", 25105, 2),
		pipelineTick("2026-09-24T03:46:00Z", 25103, 3),
		pipelineTick("2026-09-24T03:46:30Z", 25108, 4),
		pipelineTick("2026-09-24T03:47:00Z", 25106, 5),
	}
	for _, input := range inputs {
		if _, err := pipe.ApplyTick(input); err != nil {
			t.Fatal(err)
		}
	}

	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	bars, err := store.LoadDay("NSE:NIFTY50", "2m", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one finalized 2m rollup, got %+v", bars)
	}
	got := bars[0]
	if !got.Final || got.Open != 25100 || got.High != 25108 || got.Close != 25108 {
		t.Fatalf("unexpected 2m rollup: %+v", got)
	}
	if got.CandleEngineVersion != "candle-rollup-v1" {
		t.Fatalf("expected rollup engine version, got %q", got.CandleEngineVersion)
	}
}

func TestPipelineRequiresCanonicalMinuteForDerivedIntervals(t *testing.T) {
	_, err := New(candle.New("candle-v2"), nil, []string{"15s", "2m"})
	if err == nil {
		t.Fatal("expected 1m requirement for derived intervals")
	}
}


func TestPipelineClockFinalizesAndPersistsWithoutNextTick(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("candle-v3-clock"), store, []string{"30s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipe.ApplyTick(pipelineTick("2026-09-24T03:45:00Z", 25100, 1)); err != nil {
		t.Fatal(err)
	}

	atClose, _ := time.Parse(time.RFC3339, "2026-09-24T03:45:30Z")
	updates, err := pipe.FinalizeDue(atClose)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || updates[0].Timeframe != "30s" || !updates[0].Final {
		t.Fatalf("expected clock-finalized 30s candle, got %+v", updates)
	}

	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	bars, err := store.LoadDay("NSE:NIFTY50", "30s", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || !bars[0].Final {
		t.Fatalf("expected persisted clock-finalized candle, got %+v", bars)
	}
}

func TestPipelineFinalizesCompleteDerivedBarAtBoundaryWithoutNextTick(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("candle-v3-clock"), store, []string{"1m", "2m"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pipe.ApplyTick(pipelineTick("2026-09-24T03:45:00Z", 25100, 1)); err != nil {
		t.Fatal(err)
	}
	firstClose, _ := time.Parse(time.RFC3339, "2026-09-24T03:46:00Z")
	if _, err := pipe.FinalizeDue(firstClose); err != nil {
		t.Fatal(err)
	}

	if _, err := pipe.ApplyTick(pipelineTick("2026-09-24T03:46:00Z", 25105, 2)); err != nil {
		t.Fatal(err)
	}
	secondClose, _ := time.Parse(time.RFC3339, "2026-09-24T03:47:00Z")
	updates, err := pipe.FinalizeDue(secondClose)
	if err != nil {
		t.Fatal(err)
	}

	foundFinalTwoMinute := false
	for _, bar := range updates {
		if bar.Timeframe == "2m" && bar.Final {
			foundFinalTwoMinute = true
		}
	}
	if !foundFinalTwoMinute {
		t.Fatalf("expected finalized 2m rollup at its boundary, got %+v", updates)
	}

	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	bars, err := store.LoadDay("NSE:NIFTY50", "2m", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || !bars[0].Final {
		t.Fatalf("expected persisted finalized 2m rollup, got %+v", bars)
	}
}

func TestPipelineIgnoresOutOfSessionTick(t *testing.T) {
	resolver := func(_ string, at time.Time) (candle.SessionWindow, bool, error) {
		open, _ := time.Parse(time.RFC3339, "2026-09-25T03:45:00Z")
		closeAt, _ := time.Parse(time.RFC3339, "2026-09-25T10:00:00Z")
		return candle.SessionWindow{Open: open, Close: closeAt}, !at.Before(open) && at.Before(closeAt), nil
	}
	pipe, err := New(candle.NewWithSessionResolver("candle-v3-calendar", resolver), nil, []string{"1m"})
	if err != nil {
		t.Fatal(err)
	}
	updates, err := pipe.ApplyTick(pipelineTick("2026-09-25T03:44:59Z", 25100, 1))
	if err != nil {
		t.Fatalf("out-of-session tick should be rejected without stopping ingestion: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("out-of-session tick must not create candles: %+v", updates)
	}
}


func TestLateTickSkipsClosedShortFrameButUpdatesOpenLongFrame(t *testing.T) {
	pipe, err := New(candle.New("candle-v3-clock"), nil, []string{"30s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipe.ApplyTick(pipelineTick("2026-09-24T03:45:00Z", 25100, 1)); err != nil {
		t.Fatal(err)
	}
	shortClose, _ := time.Parse(time.RFC3339, "2026-09-24T03:45:30Z")
	if _, err := pipe.FinalizeDue(shortClose); err != nil {
		t.Fatal(err)
	}

	updates, err := pipe.ApplyTick(pipelineTick("2026-09-24T03:45:20Z", 25110, 2))
	if err != nil {
		t.Fatalf("late short-frame tick must not stop ingestion: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected only the still-open 1m update, got %+v", updates)
	}
	if updates[0].Timeframe != "1m" || updates[0].Close != 25110 || updates[0].Final {
		t.Fatalf("unexpected longer-frame update: %+v", updates[0])
	}
}
