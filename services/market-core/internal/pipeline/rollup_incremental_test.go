package pipeline

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func rollupMinute(at time.Time, open, high, low, close, volume float64, sequence uint64, final bool) domain.Bar {
	return domain.Bar{
		InstrumentID:      "NSE:NIFTY50",
		Timeframe:         "1m",
		OpenTime:          at,
		CloseTime:         at.Add(time.Minute),
		Open:              open,
		High:              high,
		Low:               low,
		Close:             close,
		Volume:            volume,
		Final:             final,
		AuthorityProvider: "fixture",
		Quality:           domain.QualityGood,
		SourceSequence:    sequence,
		CreatedAt:         at.Add(time.Duration(sequence) * time.Millisecond),
	}
}

func lastRollupUpdate(t *testing.T, updates []domain.Bar) domain.Bar {
	t.Helper()
	if len(updates) == 0 {
		t.Fatal("expected rollup update")
	}
	return updates[len(updates)-1]
}

func TestIncrementalRollupReplacesFormingVolumeWithoutDoubleCount(t *testing.T) {
	engine := newRollupEngine("candle-rollup-v2-incremental")
	base := time.Date(2026, 9, 30, 3, 45, 0, 0, time.UTC)

	updates, err := engine.Apply(rollupMinute(base, 100, 101, 99, 100, 10, 1, false), "2m")
	if err != nil {
		t.Fatal(err)
	}
	if got := lastRollupUpdate(t, updates); got.Volume != 10 || got.High != 101 || got.Low != 99 {
		t.Fatalf("unexpected initial rollup: %+v", got)
	}

	updates, err = engine.Apply(rollupMinute(base, 100, 103, 98, 102, 25, 2, false), "2m")
	if err != nil {
		t.Fatal(err)
	}
	if got := lastRollupUpdate(t, updates); got.Volume != 25 || got.High != 103 || got.Low != 98 || got.Close != 102 {
		t.Fatalf("forming minute was double-counted: %+v", got)
	}

	firstFinal := rollupMinute(base, 100, 103, 98, 102, 25, 2, true)
	if _, err := engine.Apply(firstFinal, "2m"); err != nil {
		t.Fatal(err)
	}
	second := rollupMinute(base.Add(time.Minute), 102, 104, 101, 103, 5, 3, false)
	updates, err = engine.Apply(second, "2m")
	if err != nil {
		t.Fatal(err)
	}
	if got := lastRollupUpdate(t, updates); got.Volume != 30 || got.Open != 100 || got.High != 104 || got.Low != 98 || got.Close != 103 {
		t.Fatalf("unexpected two-minute forming aggregate: %+v", got)
	}

	second.Final = true
	if _, err := engine.Apply(second, "2m"); err != nil {
		t.Fatal(err)
	}
	updates, err = engine.Apply(rollupMinute(base.Add(2*time.Minute), 103, 103, 103, 103, 1, 4, false), "2m")
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 {
		t.Fatalf("expected finalized prior rollup plus new forming bar, got %+v", updates)
	}
	final := updates[0]
	if !final.Final || final.Volume != 30 || final.Open != 100 || final.High != 104 || final.Low != 98 || final.Close != 103 {
		t.Fatalf("unexpected finalized incremental rollup: %+v", final)
	}
	if final.CandleEngineVersion != "candle-rollup-v2-incremental" {
		t.Fatalf("unexpected rollup version: %q", final.CandleEngineVersion)
	}
}

func TestIncrementalRollupIgnoresStaleMinuteUpdate(t *testing.T) {
	engine := newRollupEngine("candle-rollup-v2-incremental")
	base := time.Date(2026, 9, 30, 3, 45, 0, 0, time.UTC)
	current := rollupMinute(base, 100, 103, 98, 102, 25, 20, false)
	if _, err := engine.Apply(current, "5m"); err != nil {
		t.Fatal(err)
	}
	stale := rollupMinute(base, 100, 110, 90, 95, 100, 19, false)
	updates, err := engine.Apply(stale, "5m")
	if err != nil {
		t.Fatal(err)
	}
	got := lastRollupUpdate(t, updates)
	if got.Volume != 25 || got.High != 103 || got.Low != 98 || got.Close != 102 || got.SourceSequence != 20 {
		t.Fatalf("stale minute changed incremental state: %+v", got)
	}
}

func TestIncrementalRollupRebuildsRareNonMonotonicCorrectionExactly(t *testing.T) {
	engine := newRollupEngine("candle-rollup-v2-incremental")
	base := time.Date(2026, 9, 30, 3, 45, 0, 0, time.UTC)

	first := rollupMinute(base, 100, 110, 90, 101, 10, 10, true)
	first.Quality = domain.QualityDegraded
	first.Recovered = true
	if _, err := engine.Apply(first, "2m"); err != nil {
		t.Fatal(err)
	}
	second := rollupMinute(base.Add(time.Minute), 101, 105, 95, 104, 5, 20, false)
	if _, err := engine.Apply(second, "2m"); err != nil {
		t.Fatal(err)
	}

	corrected := rollupMinute(base, 100, 102, 98, 101, 8, 10, true)
	corrected.Revision = 1
	corrected.Corrected = true
	corrected.CorrectedAt = base.Add(2 * time.Minute)
	updates, err := engine.Apply(corrected, "2m")
	if err != nil {
		t.Fatal(err)
	}
	got := lastRollupUpdate(t, updates)
	if got.Volume != 13 || got.High != 105 || got.Low != 95 || got.Quality != domain.QualityGood || got.Recovered {
		t.Fatalf("non-monotonic correction was not rebuilt exactly: %+v", got)
	}
}
