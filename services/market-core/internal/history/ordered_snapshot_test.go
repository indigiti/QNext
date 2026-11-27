package history

import (
	"testing"
	"time"
)

func TestCurrentDayOrderedSnapshotIsolationAndRevisions(t *testing.T) {
	store := New(t.TempDir())
	base := time.Date(2026, 11, 27, 3, 45, 0, 0, time.UTC)
	fixedCurrentDay(store, base)

	// Arrive out of order, then hydrate the current-day cache.
	store.observeBar(testBar(base.Add(time.Minute), 25103, 0))
	store.observeBar(testBar(base, 25101, 0))
	store.observeBar(testBar(base.Add(30*time.Second), 25102, 0))
	all, err := store.LoadDay("NSE:NIFTY50", "30s", base)
	if err != nil || len(all) != 3 {
		t.Fatalf("hydrate: bars=%v err=%v", all, err)
	}
	all[1].Close = -1
	first, err := store.LoadRange("NSE:NIFTY50", "30s", base.Add(30*time.Second), base.Add(time.Minute))
	if err != nil || len(first) != 1 || first[0].Close != 25102 {
		t.Fatalf("range or caller isolation: bars=%v err=%v", first, err)
	}

	// A late correction invalidates the snapshot, while a stale revision
	// cannot overwrite it. A previous caller's slice must remain independent.
	corrected := testBar(base.Add(30*time.Second), 25104, 2)
	corrected.Volume = 123
	corrected.VolumeSource = "FUTURES_PROXY"
	corrected.VolumeStatus = "OBSERVED"
	store.observeBar(corrected)
	store.observeBar(testBar(base.Add(30*time.Second), 25000, 1))
	second, err := store.LoadRange("NSE:NIFTY50", "30s", base.Add(30*time.Second), base.Add(time.Minute))
	if err != nil || len(second) != 1 || second[0] != corrected {
		t.Fatalf("correction lost: bars=%v err=%v", second, err)
	}
	if first[0].Close != 25102 {
		t.Fatal("published result changed after correction")
	}
	second[0].Volume = 0
	third, err := store.LoadRange("NSE:NIFTY50", "30s", base.Add(30*time.Second), base.Add(time.Minute))
	if err != nil || len(third) != 1 || third[0] != corrected {
		t.Fatalf("range result mutated cache: bars=%v err=%v", third, err)
	}
}

func BenchmarkCurrentDayRecentRange(b *testing.B) {
	store := New(b.TempDir())
	base := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	fixedCurrentDay(store, base)
	const count = 4500
	for i := 0; i < count; i++ {
		bar := testBar(base.Add(time.Duration(i)*5*time.Second), 25101, 0)
		bar.Timeframe = "5s"
		bar.CloseTime = bar.OpenTime.Add(5 * time.Second)
		store.observeBar(bar)
	}
	end := base.Add(count * 5 * time.Second)
	if _, err := store.LoadDay("NSE:NIFTY50", "5s", base); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bars, err := store.LoadRange("NSE:NIFTY50", "5s", end.Add(-10*time.Second), end)
		if err != nil || len(bars) != 2 {
			b.Fatalf("range: len=%d err=%v", len(bars), err)
		}
	}
}
