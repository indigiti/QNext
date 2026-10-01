package research

import (
	"context"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/history"
)

func TestSynPlusPersistsChartHistoryWithoutMarketCore(t *testing.T) {
	root := t.TempDir()
	store, err := NewJSONLStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	published := 0
	collector, err := NewSynPlusCollectorWithStore(store, SynPlusConfig{
		InstrumentID:    "QNEXT:NIFTY-SYN+",
		Version:         "nifty-syn-plus-v1",
		ChartTimeframes: []string{"15s", "1m"},
		OnSnapshot: func(SynPlusSnapshot) {
			published++
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer collector.Close()

	firstAt := time.Date(2026, 9, 28, 3, 45, 1, 0, time.UTC)
	base := SynPlusSnapshot{
		Schema:          SynPlusSnapshotSchema,
		InstrumentID:    "QNEXT:NIFTY-SYN+",
		Version:         "nifty-syn-plus-v1",
		Value:           25000,
		Quality:         "GOOD",
		ValidCandidates: 7,
	}
	// Feed one valid SYN+ snapshot into each canonical 5s bucket. The fourth
	// bucket transition finalizes the first 15s bar from exactly three complete
	// 5s children, matching the production sub-minute hierarchy.
	for i := 0; i < 4; i++ {
		snapshot := base
		snapshot.SnapshotAtMS = firstAt.Add(time.Duration(i) * 5 * time.Second).UnixMilli()
		snapshot.Value = 25000 + float64(i)
		if err := collector.persist(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	if published != 4 {
		t.Fatalf("expected 4 snapshot callbacks, got %d", published)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := history.FlushAsyncWriters(ctx); err != nil {
		t.Fatalf("flush async chart history: %v", err)
	}

	bars, err := history.New(root).LoadDay("QNEXT:NIFTY-SYN+", "15s", firstAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one finalized 15s bar, got %d", len(bars))
	}
	if bars[0].Open != 25000 || bars[0].Close != 25002 || !bars[0].Final {
		t.Fatalf("unexpected persisted chart bar: %+v", bars[0])
	}
	if bars[0].AuthorityProvider != "qnext-syn-plus-shadow" {
		t.Fatalf("unexpected provider: %s", bars[0].AuthorityProvider)
	}
	if bars[0].CandleEngineVersion != "candle-rollup-v3-canonical-5s" {
		t.Fatalf("unexpected SYN+ subminute lineage: %s", bars[0].CandleEngineVersion)
	}
}
