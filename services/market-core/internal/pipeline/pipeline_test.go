package pipeline

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
)

type syncHistoryWriter struct {
	store *history.Store
}

func (w syncHistoryWriter) AppendBar(bar domain.Bar) error {
	return w.store.AppendBar(bar)
}

type dualHistoryWriter struct {
	syncCalls  int
	asyncCalls int
}

func (w *dualHistoryWriter) AppendBar(domain.Bar) error {
	w.syncCalls++
	return nil
}

func (w *dualHistoryWriter) AppendBarAsync(domain.Bar) error {
	w.asyncCalls++
	return nil
}

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
		ReceivedTime: ts,
		Sequence:     sequence,
		Quality:      domain.QualityGood,
	}
}

func applyFiveSecondSeries(t *testing.T, pipe *Pipeline, start time.Time, count int, startPrice float64) {
	t.Helper()
	for i := 0; i < count; i++ {
		tick := domain.Tick{
			InstrumentID: "NSE:NIFTY50",
			Provider:     "fixture",
			Price:        startPrice + float64(i),
			EventTime:    start.Add(time.Duration(i) * 5 * time.Second),
			ReceivedTime: start.Add(time.Duration(i) * 5 * time.Second),
			Sequence:     uint64(i + 1),
			Quality:      domain.QualityGood,
		}
		if _, err := pipe.ApplyTick(tick); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPipelinePersistsOnlyFinalizedBars(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("candle-v3"), syncHistoryWriter{store: store}, []string{"30s", "1m"})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	applyFiveSecondSeries(t, pipe, start, 7, 25100)

	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	bars5s, err := store.LoadDay("NSE:NIFTY50", "5s", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars5s) != 6 {
		t.Fatalf("expected six finalized canonical 5s bars, got %d", len(bars5s))
	}
	bars30s, err := store.LoadDay("NSE:NIFTY50", "30s", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars30s) != 1 || !bars30s[0].Final {
		t.Fatalf("expected one finalized 30s bar, got %+v", bars30s)
	}
	if bars30s[0].Open != 25100 || bars30s[0].High != 25105 || bars30s[0].Close != 25105 {
		t.Fatalf("unexpected canonical 30s rollup: %+v", bars30s[0])
	}
	if bars30s[0].CandleEngineVersion != subminuteRollupVersion {
		t.Fatalf("unexpected subminute lineage %q", bars30s[0].CandleEngineVersion)
	}

	bars1m, err := store.LoadDay("NSE:NIFTY50", "1m", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars1m) != 0 {
		t.Fatalf("expected no finalized 1m bars yet, got %+v", bars1m)
	}
}

func TestPipelineDoesNotFinalizeSubminuteWithMissingFiveSecondChild(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("candle-v3"), syncHistoryWriter{store: store}, []string{"15s", "30s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tick := range []domain.Tick{
		pipelineTick("2026-09-24T03:45:00Z", 100, 1),
		pipelineTick("2026-09-24T03:45:05Z", 101, 2),
		// 03:45:10 is deliberately absent.
		pipelineTick("2026-09-24T03:45:15Z", 103, 3),
	} {
		if _, err := pipe.ApplyTick(tick); err != nil {
			t.Fatal(err)
		}
	}
	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	bars, err := store.LoadDay("NSE:NIFTY50", "15s", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 0 {
		t.Fatalf("missing 5s child must not fabricate a 15s final: %+v", bars)
	}
	if got := pipe.SubminuteSnapshot().IncompleteBuckets["15s"]; got != 1 {
		t.Fatalf("expected one incomplete 15s bucket, got %d", got)
	}
}

func TestPipelineClockCarryBuildsExactDerivedBar(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("candle-v3"), syncHistoryWriter{store: store}, []string{"15s", "30s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipe.ApplyTick(pipelineTick("2026-09-24T03:45:00Z", 100, 1)); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 24, 3, 45, 16, 0, time.UTC)
	if _, err := pipe.AdvanceClock(at, func(string, domain.Tick, time.Time) bool { return true }); err != nil {
		t.Fatal(err)
	}

	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	bars, err := store.LoadDay("NSE:NIFTY50", "15s", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || !bars[0].Final || bars[0].Open != 100 || bars[0].Close != 100 {
		t.Fatalf("unexpected carry-built 15s candle: %+v", bars)
	}
	if !bars[0].CarryForward {
		t.Fatal("derived candle should preserve carry-forward lineage")
	}
	stats := pipe.SubminuteSnapshot()
	if stats.CarryForwardFinals < 2 {
		t.Fatalf("expected finalized carry-forward 5s bars, got %+v", stats)
	}
}

func TestPipelinePrefersAsyncHistoryForFinalBars(t *testing.T) {
	writer := &dualHistoryWriter{}
	pipe := &Pipeline{history: writer}
	bar := domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "30s",
		OpenTime:     time.Date(2026, 9, 30, 3, 45, 0, 0, time.UTC),
		Final:        true,
	}
	if err := pipe.persistFinal(bar); err != nil {
		t.Fatal(err)
	}
	if writer.asyncCalls != 1 || writer.syncCalls != 0 {
		t.Fatalf("expected async persistence only, got async=%d sync=%d", writer.asyncCalls, writer.syncCalls)
	}
}

func TestPipelineRejectsUnsupportedTimeframe(t *testing.T) {
	_, err := New(candle.New("candle-v1"), nil, []string{"7h"})
	if err == nil {
		t.Fatal("expected unsupported timeframe to fail")
	}
}

func TestPipelineRollsUpTwoMinuteBarsFromCanonicalFiveSecondMinute(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("candle-v3"), syncHistoryWriter{store: store}, []string{"1m", "2m"})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	// 25 ticks include the first 5s bucket of the next 2m interval, which
	// causes both complete 1m children and the previous 2m bucket to finalize.
	applyFiveSecondSeries(t, pipe, start, 25, 25100)

	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	bars, err := store.LoadDay("NSE:NIFTY50", "2m", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one finalized 2m rollup, got %+v", bars)
	}
	got := bars[0]
	if !got.Final || got.Open != 25100 || got.High != 25123 || got.Close != 25123 {
		t.Fatalf("unexpected 2m rollup: %+v", got)
	}
	if got.CandleEngineVersion != "candle-rollup-v2-incremental" {
		t.Fatalf("expected minute rollup engine version, got %q", got.CandleEngineVersion)
	}
}

func TestPipelineRequiresCanonicalMinuteForDerivedIntervals(t *testing.T) {
	_, err := New(candle.New("candle-v3"), nil, []string{"15s", "2m"})
	if err == nil {
		t.Fatal("expected 1m requirement for derived intervals")
	}
}
