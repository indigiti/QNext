package synthetic

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestNiftySyntheticMedian(t *testing.T) {
	at := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	def := Definition{
		ID:                     "QNEXT:NIFTY-SYN",
		Version:                "fixture-v1",
		MinimumValidCandidates: 3,
		MaxLegAge:              time.Second,
		MaxLegTimeSkew:         time.Second,
	}
	candidates := []Candidate{
		{25000, 160, 58, at, at},
		{25050, 128, 78, at, at},
		{25100, 101, 100, at, at},
		{25150, 74, 125, at, at},
		{25200, 55, 152, at, at},
	}

	got, err := Compute(def, at, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if got.Quality != domain.QualityGood {
		t.Fatalf("expected GOOD quality, got %s", got.Quality)
	}
	if got.Value != 25101 {
		t.Fatalf("expected 25101, got %v", got.Value)
	}
}

func TestSyntheticRejectsStaleLegs(t *testing.T) {
	at := time.Date(2026, 9, 24, 3, 45, 10, 0, time.UTC)
	stale := at.Add(-10 * time.Second)
	def := Definition{
		ID:                     "QNEXT:NIFTY-SYN",
		Version:                "fixture-v1",
		MinimumValidCandidates: 3,
		MaxLegAge:              time.Second,
		MaxLegTimeSkew:         time.Second,
	}
	got, err := Compute(def, at, []Candidate{
		{25000, 160, 58, stale, stale},
		{25050, 128, 78, stale, stale},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Quality != domain.QualityDegraded {
		t.Fatalf("expected DEGRADED, got %s", got.Quality)
	}
}
