package synthetic

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestAssemblerProducesFiveStrikeNiftySynthetic(t *testing.T) {
	def := Definition{
		ID:                     "QNEXT:NIFTY-SYN",
		Version:                "nifty-syn-v1",
		MinimumValidCandidates: 3,
		MaxLegAge:              2 * time.Second,
		MaxLegTimeSkew:         time.Second,
	}
	legs := []LegBinding{
		{"CE25000", 25000, LegCall}, {"PE25000", 25000, LegPut},
		{"CE25050", 25050, LegCall}, {"PE25050", 25050, LegPut},
		{"CE25100", 25100, LegCall}, {"PE25100", 25100, LegPut},
		{"CE25150", 25150, LegCall}, {"PE25150", 25150, LegPut},
		{"CE25200", 25200, LegCall}, {"PE25200", 25200, LegPut},
	}
	var sequence uint64
	assembler, err := NewAssembler(def, legs, func() uint64 {
		sequence++
		return sequence
	})
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	values := map[string]float64{
		"CE25000": 160, "PE25000": 58,
		"CE25050": 128, "PE25050": 78,
		"CE25100": 101, "PE25100": 100,
		"CE25150": 74, "PE25150": 125,
		"CE25200": 55, "PE25200": 152,
	}

	var got domain.Tick
	var emitted bool
	for _, leg := range legs {
		tick := domain.Tick{
			InstrumentID: leg.InstrumentID,
			Provider:     "upstox",
			Price:        values[leg.InstrumentID],
			EventTime:    at,
			ReceivedTime: at,
			ProcessedTime: at,
			Quality:      domain.QualityGood,
		}
		got, emitted, err = assembler.Apply(tick)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !emitted {
		t.Fatal("expected synthetic tick after all legs were populated")
	}
	if got.InstrumentID != "QNEXT:NIFTY-SYN" || got.Price != 25101 {
		t.Fatalf("unexpected synthetic tick: %+v", got)
	}
	if got.Provider != SyntheticProvider || got.SyntheticVersion != "nifty-syn-v1" {
		t.Fatalf("missing synthetic lineage: %+v", got)
	}
}

func TestAssemblerRefusesStaleSynthetic(t *testing.T) {
	def := Definition{
		ID:                     "QNEXT:NIFTY-SYN",
		Version:                "nifty-syn-v1",
		MinimumValidCandidates: 1,
		MaxLegAge:              time.Second,
		MaxLegTimeSkew:         time.Second,
	}
	assembler, err := NewAssembler(def, []LegBinding{
		{"CE", 25100, LegCall},
		{"PE", 25100, LegPut},
	}, func() uint64 { return 1 })
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	if _, emitted, err := assembler.Apply(domain.Tick{
		InstrumentID: "CE", Price: 101, EventTime: base, Quality: domain.QualityGood,
	}); err != nil || emitted {
		t.Fatalf("unexpected first-leg result emitted=%v err=%v", emitted, err)
	}
	if _, emitted, err := assembler.Apply(domain.Tick{
		InstrumentID: "PE", Price: 100, EventTime: base.Add(3 * time.Second), Quality: domain.QualityGood,
	}); err != nil {
		t.Fatal(err)
	} else if emitted {
		t.Fatal("stale/skewed legs must not emit a synthetic tick")
	}
}
