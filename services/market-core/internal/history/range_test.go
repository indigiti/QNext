package history

import (
	"testing"
	"time"
)

func TestLoadRangeFiltersCanonicalBars(t *testing.T) {
	store := New(t.TempDir())
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		bar := testBar(base.Add(time.Duration(i)*30*time.Second), 25101+float64(i), 0)
		if err := store.AppendBar(bar); err != nil {
			t.Fatal(err)
		}
	}

	bars, err := store.LoadRange("NSE:NIFTY50", "30s", base.Add(30*time.Second), base.Add(90*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	if bars[0].Close != 25102 || bars[1].Close != 25103 {
		t.Fatalf("unexpected range: %+v", bars)
	}
}
