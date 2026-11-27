package research

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
)

func TestSynPlusMedianScratchParity(t *testing.T) {
	for _, input := range [][]float64{nil, {5}, {7, 1}, {9, 1, 4, 4, 2}, {8, 2, 5, 1}} {
		original := append([]float64(nil), input...)
		want := median(input)
		if !reflect.DeepEqual(input, original) {
			t.Fatal("median mutated caller values")
		}
		if got := medianInPlace(append([]float64(nil), input...)); got != want {
			t.Fatalf("median mismatch: %v != %v", got, want)
		}
	}
}

func TestOptionSnapshotSharedSpotAndSortedKeys(t *testing.T) {
	root := t.TempDir()
	c, err := NewOptionsCollector(root)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	plan := testPlan()
	addContractPair(&plan, "z", 25000, plan.CurrentExpiry)
	addContractPair(&plan, "a", 25050, plan.CurrentExpiry)
	at := time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC)
	update := func(when time.Time, spot float64, entries map[string]ResearchStateEntry) {
		t.Helper()
		if err := c.ObserveState(ResearchStateUpdate{Plan: plan, SourceTime: when, SourceMS: when.UnixMilli(), Spot: spot, SpotUpdated: true, Updates: entries}); err != nil {
			t.Fatal(err)
		}
	}
	entries := map[string]ResearchStateEntry{
		"z-CE": {State: microState(110), UpdatedAt: at},
		"a-PE": {State: microState(95), UpdatedAt: at},
	}
	update(at, 25000, entries)
	update(at.Add(500*time.Millisecond), 25010, nil)
	update(at.Add(time.Second), 25020, nil)
	if !reflect.DeepEqual(c.sortedKeys, []string{"a-PE", "z-CE"}) {
		t.Fatalf("sorted keys: %v", c.sortedKeys)
	}
	delete(plan.Contracts, "a-PE")
	update(at.Add(1500*time.Millisecond), 25030, nil)
	if c.sortedKeys != nil {
		t.Fatal("removed contract did not invalidate sorted keys")
	}
	update(at.Add(2*time.Second), 25040, nil)
	if !reflect.DeepEqual(c.sortedKeys, []string{"z-CE"}) {
		t.Fatalf("replanned keys: %v", c.sortedKeys)
	}
	file, err := os.Open(filepath.Join(root, "research", "options", "1s", "2026", "09", "2026-09-28.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	wantKeys := []string{"a-PE", "z-CE", "z-CE"}
	wantSpots := []float64{25010, 25010, 25030}
	for i := range wantKeys {
		if !scanner.Scan() {
			t.Fatal("missing snapshot")
		}
		var got OptionSnapshot
		if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.ProviderKey != wantKeys[i] || got.UnderlyingPrice != wantSpots[i] || got.SourceCurrentTSMS != at.UnixMilli() {
			t.Fatalf("snapshot %d: %+v", i, got)
		}
	}
	if scanner.Scan() || scanner.Err() != nil {
		t.Fatal("unexpected snapshot tail")
	}
}

func TestResearchHubIgnoresUnselectedFeed(t *testing.T) {
	consumer := &recordingStateConsumer{}
	hub := NewResearchStateHub(consumer)
	plan := testPlan()
	addContractPair(&plan, "p", 25000, plan.CurrentExpiry)
	at := time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC)
	envelope := testEnvelope(at, 25010, 110, 100)
	envelope.Feeds["unselected"] = upstox.Feed{FirstLevelWithGreeks: statePtr(microState(99))}
	if err := hub.Observe(envelope, plan); err != nil {
		t.Fatal(err)
	}
	if len(consumer.updates[0].Updates) != 2 {
		t.Fatalf("unexpected updates: %v", consumer.updates[0].Updates)
	}
}

func BenchmarkSynPlusEvaluateChain(b *testing.B) {
	at := time.Date(2026, 9, 28, 4, 0, 1, 0, time.UTC)
	c := &SynPlusCollector{cfg: SynPlusConfig{MinimumValidCandidates: 3, MaxLegAge: 2 * time.Second, MaxLegTimeSkew: time.Second}, legs: make(map[string]synPlusLeg)}
	plan := testPlan()
	for i := 0; i < 41; i++ {
		addPair(c, &plan, at, fmt.Sprintf("p%d", i), float64(24000+i*50), 110, 100, plan.CurrentExpiry)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := c.evaluate(plan, at, at.UnixMilli()); got.ValidCandidates != 41 {
			b.Fatal(got.ValidCandidates)
		}
	}
}
