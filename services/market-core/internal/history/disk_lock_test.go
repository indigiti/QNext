package history

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestHistoryDiskLockUsesSanitizedPath(t *testing.T) {
	store := New(t.TempDir())
	day := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	a := store.dayPath("NSE:NIFTY50", "30s", day)
	b := store.dayPath("NSE_NIFTY50", "30s", day)
	if a != b || store.diskLock(a) != store.diskLock(b) {
		t.Fatal("same on-disk file must use the same lock")
	}
}

func TestHistoryUnrelatedFileProgressWhileScanLockHeld(t *testing.T) {
	store := New(t.TempDir())
	day := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	blocked := store.dayPath("NSE:NIFTY50", "30s", day)
	lock := store.diskLock(blocked)
	var other string
	for i := 0; i < historyLockStripes*2; i++ {
		id := fmt.Sprintf("NSE:FIXTURE%d", i)
		if store.diskLock(store.dayPath(id, "30s", day)) != lock {
			other = id
			break
		}
	}
	if other == "" {
		t.Fatal("no independent lock stripe")
	}
	lock.Lock()
	defer lock.Unlock()

	bar := testBar(day, 25101, 0)
	bar.InstrumentID = other
	done := make(chan error, 1)
	go func() {
		if err := store.appendBarDisk(bar); err != nil {
			done <- err
			return
		}
		bars, err := store.loadDayDisk(other, "30s", day)
		if err == nil && (len(bars) != 1 || bars[0].Close != bar.Close) {
			err = fmt.Errorf("unexpected history: %v", bars)
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unrelated history file blocked behind scan lock")
	}
}

func TestHistoryConcurrentSameFileReadsAndWrites(t *testing.T) {
	store := New(t.TempDir())
	day := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	const count = 32
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			bar := testBar(day.Add(time.Duration(i)*30*time.Second), 25101, 0)
			if err := store.appendBarDisk(bar); err != nil {
				t.Errorf("append: %v", err)
			}
		}(i)
		go func() {
			defer wg.Done()
			if _, err := store.loadDayDisk("NSE:NIFTY50", "30s", day); err != nil {
				t.Errorf("read: %v", err)
			}
		}()
	}
	wg.Wait()
	bars, err := store.loadDayDisk("NSE:NIFTY50", "30s", day)
	if err != nil || len(bars) != count {
		t.Fatalf("durable read: len=%d err=%v", len(bars), err)
	}
	for i, bar := range bars {
		if !bar.OpenTime.Equal(day.Add(time.Duration(i) * 30 * time.Second)) {
			t.Fatalf("unexpected bar order at %d: %v", i, bar.OpenTime)
		}
	}
}

func BenchmarkHistoryParallelDiskReads(b *testing.B) {
	store := New(b.TempDir())
	day := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	const files = 8
	ids := make([]string, files)
	for i := range ids {
		ids[i] = fmt.Sprintf("NSE:FIXTURE%d", i)
		for j := 0; j < 120; j++ {
			bar := testBar(day.Add(time.Duration(j)*30*time.Second), 25101, 0)
			bar.InstrumentID = ids[i]
			if err := store.appendBarDisk(bar); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bars, err := store.loadDayDisk(ids[i%files], "30s", day)
			if err != nil || len(bars) != 120 {
				b.Errorf("read: len=%d err=%v", len(bars), err)
				return
			}
			i++
		}
	})
}
