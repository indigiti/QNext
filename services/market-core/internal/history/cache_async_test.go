package history

import (
	"context"
	"sync"
	"testing"
	"time"
)

func fixedCurrentDay(store *Store, now time.Time) {
	store.cache.now = func() time.Time { return now.UTC() }
}

func TestCurrentDayCacheHydratesOnceThenServesRAM(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	writerStore := New(root)
	fixedCurrentDay(writerStore, now)
	at := time.Date(2026, 9, 30, 3, 45, 0, 0, time.UTC)
	if err := writerStore.AppendBar(testBar(at, 25101, 0)); err != nil {
		t.Fatal(err)
	}

	store := New(root)
	fixedCurrentDay(store, now)
	first, err := store.LoadDay("NSE:NIFTY50", "30s", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.LoadDay("NSE:NIFTY50", "30s", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Close != 25101 || second[0].Close != 25101 {
		t.Fatalf("unexpected cached bars: first=%+v second=%+v", first, second)
	}
	stats := store.CacheStats()
	if stats.DiskLoads != 1 || stats.Misses != 1 || stats.Hits != 1 {
		t.Fatalf("unexpected cache stats: %+v", stats)
	}
}

func TestCurrentDayHydrationPreservesNewerLiveRevision(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	at := time.Date(2026, 9, 30, 3, 45, 0, 0, time.UTC)

	diskStore := New(root)
	fixedCurrentDay(diskStore, now)
	if err := diskStore.AppendBar(testBar(at, 25101, 0)); err != nil {
		t.Fatal(err)
	}

	store := New(root)
	fixedCurrentDay(store, now)
	corrected := testBar(at, 25107, 2)
	store.observeBar(corrected)

	bars, err := store.LoadDay("NSE:NIFTY50", "30s", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].Revision != 2 || bars[0].Close != 25107 {
		t.Fatalf("disk hydration replaced newer live correction: %+v", bars)
	}
}

func TestCurrentDayCacheConcurrentReadersAndAsyncFinals(t *testing.T) {
	store := New(t.TempDir())
	now := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	fixedCurrentDay(store, now)
	writer := NewAsyncWriter(store, 64)

	base := time.Date(2026, 9, 30, 3, 45, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		if err := writer.AppendBar(testBar(base.Add(time.Duration(i)*30*time.Second), 25100+float64(i), 0)); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bars, err := store.LoadRange("NSE:NIFTY50", "30s", base, base.Add(20*30*time.Second))
			if err != nil {
				t.Errorf("load range: %v", err)
				return
			}
			if len(bars) != 20 {
				t.Errorf("expected 20 bars, got %d", len(bars))
			}
		}()
	}
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := writer.Close(ctx); err != nil {
		t.Fatal(err)
	}
	stats := writer.Stats()
	if stats.Writes != 20 || stats.Pending != 0 || stats.Errors != 0 {
		t.Fatalf("unexpected persistence stats after drain: %+v", stats)
	}

	restarted := New(store.root)
	fixedCurrentDay(restarted, now)
	bars, err := restarted.LoadRange("NSE:NIFTY50", "30s", base, base.Add(20*30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 20 {
		t.Fatalf("restart recovery expected 20 bars, got %d", len(bars))
	}
}

func TestAsyncWriterRejectsFormingBar(t *testing.T) {
	store := New(t.TempDir())
	writer := NewAsyncWriter(store, 4)
	bar := testBar(time.Date(2026, 9, 30, 3, 45, 0, 0, time.UTC), 25101, 0)
	bar.Final = false
	if err := writer.AppendBar(bar); err == nil {
		t.Fatal("expected forming bar to be rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := writer.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
