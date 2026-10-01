package httpapi

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestBuildFiveSecondSymbolHealthShowsExactBreaks(t *testing.T) {
	start := time.Date(2026, 10, 1, 3, 45, 0, 0, time.UTC)
	result := fiveSecondSymbolHealth{
		InstrumentID: "QNEXT:NIFTY-SYN",
		Symbol:       "NIFTY-SYN",
		Expected:     6,
		Status:       "WAITING",
		Timeline:     []fiveSecondHealthSegment{},
	}
	bars := []domain.Bar{
		fiveSecondHealthBar(start, domain.QualityGood, false, false),
		fiveSecondHealthBar(start.Add(5*time.Second), domain.QualityGood, true, false),
		// 10s intentionally absent -> MISSING.
		fiveSecondHealthBar(start.Add(15*time.Second), domain.QualityDegraded, false, false),
		fiveSecondHealthBar(start.Add(20*time.Second), domain.QualityRecovered, false, true),
		fiveSecondHealthBar(start.Add(25*time.Second), domain.QualityGood, false, false),
	}

	got := buildFiveSecondSymbolHealth(result, start, start.Add(30*time.Second), bars)
	if got.Good != 2 || got.CarryForward != 1 || got.Recovered != 1 || got.Degraded != 1 || got.Missing != 1 {
		t.Fatalf("unexpected counts: %+v", got)
	}
	if got.Present != 5 || got.CompletenessPct < 83.3 || got.CompletenessPct > 83.4 {
		t.Fatalf("unexpected completeness: %+v", got)
	}
	if got.HealthyPct < 66.6 || got.HealthyPct > 66.7 {
		t.Fatalf("unexpected healthy percentage: %+v", got)
	}
	if got.Status != "CHECK" {
		t.Fatalf("expected CHECK status, got %s", got.Status)
	}
	if len(got.Timeline) != 6 {
		t.Fatalf("expected six state segments, got %+v", got.Timeline)
	}
	missing := got.Timeline[2]
	if missing.State != "MISSING" || missing.StartMS != start.Add(10*time.Second).UnixMilli() || missing.EndMS != start.Add(15*time.Second).UnixMilli() {
		t.Fatalf("missing segment did not preserve exact break: %+v", missing)
	}
}

func TestAppendFiveSecondHealthSegmentCompressesContiguousState(t *testing.T) {
	start := time.Date(2026, 10, 1, 3, 45, 0, 0, time.UTC)
	segments := []fiveSecondHealthSegment{}
	for i := 0; i < 3; i++ {
		open := start.Add(time.Duration(i) * 5 * time.Second)
		segments = appendFiveSecondHealthSegment(segments, open, open.Add(5*time.Second), "GOOD", "")
	}
	if len(segments) != 1 || segments[0].Buckets != 3 || segments[0].EndMS != start.Add(15*time.Second).UnixMilli() {
		t.Fatalf("expected contiguous GOOD buckets to compress: %+v", segments)
	}
}

func TestIncludeFiveSecondHealthInstrumentKeepsIndexAndSynthetic(t *testing.T) {
	if !includeFiveSecondHealthInstrument(structInstrument("NIFTY", "index", false)) {
		t.Fatal("index should be included")
	}
	if !includeFiveSecondHealthInstrument(structInstrument("NIFTY-SYN", "synthetic", true)) {
		t.Fatal("index synthetic should be included")
	}
	if includeFiveSecondHealthInstrument(structInstrument("RELIANCE", "equity", false)) {
		t.Fatal("ordinary equity should not be included")
	}
}

func fiveSecondHealthBar(open time.Time, quality domain.Quality, carry, recovered bool) domain.Bar {
	return domain.Bar{
		InstrumentID: "QNEXT:NIFTY-SYN",
		Timeframe:    "5s",
		OpenTime:     open,
		CloseTime:    open.Add(5 * time.Second),
		Final:        true,
		Quality:      quality,
		CarryForward: carry,
		Recovered:    recovered,
	}
}
