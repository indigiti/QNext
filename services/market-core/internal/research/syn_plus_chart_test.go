package research

import (
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
	first := SynPlusSnapshot{
		Schema:          SynPlusSnapshotSchema,
		InstrumentID:    "QNEXT:NIFTY-SYN+",
		Version:         "nifty-syn-plus-v1",
		SnapshotAtMS:    firstAt.UnixMilli(),
		Value:           25000,
		Quality:         "GOOD",
		ValidCandidates: 7,
	}
	second := first
	second.SnapshotAtMS = firstAt.Add(16 * time.Second).UnixMilli()
	second.Value = 25004

	if err := collector.persist(first); err != nil {
		t.Fatal(err)
	}
	if err := collector.persist(second); err != nil {
		t.Fatal(err)
	}
	if published != 2 {
		t.Fatalf("expected 2 snapshot callbacks, got %d", published)
	}

	bars, err := history.New(root).LoadDay("QNEXT:NIFTY-SYN+", "15s", firstAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one finalized 15s bar, got %d", len(bars))
	}
	if bars[0].Close != 25000 || !bars[0].Final {
		t.Fatalf("unexpected persisted chart bar: %+v", bars[0])
	}
	if bars[0].AuthorityProvider != "qnext-syn-plus-shadow" {
		t.Fatalf("unexpected provider: %s", bars[0].AuthorityProvider)
	}
}
