package domain

import "testing"

func TestQualityRankPreservesLegacyOrdering(t *testing.T) {
	for rank, value := range []Quality{QualityGood, QualityRecovered, QualityPartial, QualityStale, QualityDegraded, QualityInvalid} {
		if got := QualityRank(value); got != rank {
			t.Fatalf("quality %s rank: got %d want %d", value, got, rank)
		}
	}
	for _, value := range []Quality{"", "UNKNOWN"} {
		if QualityRank(value) != 0 {
			t.Fatalf("unknown quality %q must retain zero rank", value)
		}
	}
}
