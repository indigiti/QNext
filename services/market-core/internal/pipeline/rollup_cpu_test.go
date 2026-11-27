package pipeline

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestRollupProxyProvenanceAfterRevisions(t *testing.T) {
	base := time.Date(2026, 11, 27, 3, 45, 0, 0, time.UTC)
	r := newRollupEngine("fixture")
	a := rollupMinute(base, 100, 110, 90, 100, 20, 1, true)
	b := rollupMinute(base.Add(time.Minute), 100, 105, 95, 102, 30, 2, false)
	a.VolumeSource, b.VolumeSource = "FUTURES_PROXY", "FUTURES_PROXY"
	check := func(bar domain.Bar, proxy bool, volume float64) {
		t.Helper()
		updates, err := r.Apply(bar, "5m")
		if err != nil {
			t.Fatal(err)
		}
		got := lastRollupUpdate(t, updates)
		source, status := "", "UNKNOWN"
		if proxy {
			source, status = "FUTURES_PROXY", "OBSERVED"
		}
		if got.Volume != volume || got.VolumeSource != source || got.VolumeStatus != status {
			t.Fatalf("unexpected volume or provenance: %+v", got)
		}
	}
	check(a, true, 20)
	check(b, true, 50)
	corrected := a
	corrected.Revision++
	corrected.High = 103 // forces summary rebuild
	corrected.Volume = 10
	corrected.VolumeSource = ""
	check(corrected, true, 40)
	check(a, true, 40) // stale proxy revision must not affect the counter
	b.SourceSequence++
	b.VolumeSource = ""
	check(b, false, 40)
	b.SourceSequence++
	b.VolumeSource = "FUTURES_PROXY"
	check(b, true, 40)
}

func TestSubminuteFinalCountFollowsAcceptedRevisions(t *testing.T) {
	base := time.Date(2026, 11, 27, 3, 45, 0, 0, time.UTC)
	r := newSubminuteRollupEngine()
	bar := rollupMinute(base, 100, 101, 99, 100, 10, 1, false)
	bar.Timeframe = "5s"
	bar.CloseTime = base.Add(5 * time.Second)
	apply := func(b domain.Bar) {
		t.Helper()
		if _, err := r.Apply(b, "15s"); err != nil {
			t.Fatal(err)
		}
	}
	apply(bar)
	bar.Final = true
	apply(bar)
	apply(bar) // duplicate final must not increment
	for i := 1; i < 3; i++ {
		child := bar
		child.OpenTime = base.Add(time.Duration(i) * 5 * time.Second)
		child.CloseTime = child.OpenTime.Add(5 * time.Second)
		apply(child)
	}
	state := r.states[bar.InstrumentID+"|15s"]
	if !subminuteComplete(state) {
		t.Fatal("three accepted finals must complete 15s")
	}
	corrected := bar
	corrected.Revision++
	corrected.Final = false
	apply(corrected)
	apply(bar) // stale final must not restore completion
	if subminuteComplete(state) {
		t.Fatal("higher revision forming child must invalidate completion")
	}
	corrected.Final = true
	apply(corrected)
	if !subminuteComplete(state) {
		t.Fatal("accepted final must restore completion")
	}
}

func BenchmarkDailyRollupNoProxy(b *testing.B) {
	base := time.Date(2026, 11, 27, 3, 45, 0, 0, time.UTC)
	r := newRollupEngine("fixture")
	for i := 0; i < 300; i++ {
		bar := rollupMinute(base.Add(time.Duration(i)*time.Minute), 100, 101, 99, 100, 10, uint64(i+1), true)
		if _, err := r.Apply(bar, "1D"); err != nil {
			b.Fatal(err)
		}
	}
	bar := rollupMinute(base.Add(300*time.Minute), 100, 101, 99, 100, 10, 301, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bar.SourceSequence++
		if _, err := r.Apply(bar, "1D"); err != nil {
			b.Fatal(err)
		}
	}
}
