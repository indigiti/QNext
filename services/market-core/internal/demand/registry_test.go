package demand

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeTarget struct {
	mu          sync.Mutex
	activations int
	deactivates int
}

func (f *fakeTarget) Activate(context.Context) error {
	f.mu.Lock()
	f.activations++
	f.mu.Unlock()
	return nil
}

func (f *fakeTarget) Deactivate(context.Context) error {
	f.mu.Lock()
	f.deactivates++
	f.mu.Unlock()
	return nil
}

func (f *fakeTarget) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activations, f.deactivates
}

func TestRegistryReferenceCountsAndIdleTTL(t *testing.T) {
	registry := New(25 * time.Millisecond)
	target := &fakeTarget{}
	registry.Register("QNEXT:NIFTY-SYN", target)

	registry.Acquire("QNEXT:NIFTY-SYN")
	registry.Acquire("QNEXT:NIFTY-SYN")
	waitForCounts(t, target, 1, 0)

	snapshot := registry.Snapshot()
	if snapshot.TotalRefs != 2 || snapshot.ActiveTargets != 1 {
		t.Fatalf("unexpected active snapshot: %+v", snapshot)
	}

	registry.Release("QNEXT:NIFTY-SYN")
	time.Sleep(35 * time.Millisecond)
	if _, deactivates := target.counts(); deactivates != 0 {
		t.Fatalf("target deactivated while one reference remained: %d", deactivates)
	}

	registry.Release("QNEXT:NIFTY-SYN")
	time.Sleep(10 * time.Millisecond)
	registry.Acquire("QNEXT:NIFTY-SYN")
	time.Sleep(30 * time.Millisecond)
	if _, deactivates := target.counts(); deactivates != 0 {
		t.Fatalf("idle release was not cancelled by renewed demand: %d", deactivates)
	}

	registry.Release("QNEXT:NIFTY-SYN")
	waitForCounts(t, target, 1, 1)
}

func TestRegistryPreservesDemandBeforeTargetRegistration(t *testing.T) {
	registry := New(10 * time.Millisecond)
	registry.Acquire("QNEXT:BANKNIFTY-SYN")

	target := &fakeTarget{}
	registry.Register("QNEXT:BANKNIFTY-SYN", target)
	waitForCounts(t, target, 1, 0)

	snapshot := registry.Snapshot()
	if snapshot.TotalRefs != 1 || len(snapshot.Details) != 1 || !snapshot.Details[0].Active {
		t.Fatalf("unexpected registered snapshot: %+v", snapshot)
	}
}

func waitForCounts(t *testing.T, target *fakeTarget, activations, deactivations int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		gotActivations, gotDeactivations := target.counts()
		if gotActivations >= activations && gotDeactivations >= deactivations {
			return
		}
		time.Sleep(time.Millisecond)
	}
	gotActivations, gotDeactivations := target.counts()
	t.Fatalf("counts did not reach %d/%d; got %d/%d", activations, deactivations, gotActivations, gotDeactivations)
}
