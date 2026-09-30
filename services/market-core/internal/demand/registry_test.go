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

type blockingTarget struct {
	mu                sync.Mutex
	activations       int
	deactivations     int
	deactivateStarted chan struct{}
	allowDeactivate   chan struct{}
	startOnce          sync.Once
}

func (b *blockingTarget) Activate(context.Context) error {
	b.mu.Lock()
	b.activations++
	b.mu.Unlock()
	return nil
}

func (b *blockingTarget) Deactivate(ctx context.Context) error {
	b.mu.Lock()
	b.deactivations++
	b.mu.Unlock()
	b.startOnce.Do(func() { close(b.deactivateStarted) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.allowDeactivate:
		return nil
	}
}

func (b *blockingTarget) counts() (int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.activations, b.deactivations
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

func TestRegistrySerializesReactivationBehindIdleCleanup(t *testing.T) {
	registry := New(5 * time.Millisecond)
	target := &blockingTarget{
		deactivateStarted: make(chan struct{}),
		allowDeactivate:   make(chan struct{}),
	}
	instrumentID := "QNEXT:FINNIFTY-SYN"
	registry.Register(instrumentID, target)
	registry.Acquire(instrumentID)
	waitForBlockingCounts(t, target, 1, 0)

	registry.Release(instrumentID)
	select {
	case <-target.deactivateStarted:
	case <-time.After(time.Second):
		t.Fatal("idle cleanup did not start")
	}

	registry.Acquire(instrumentID)
	time.Sleep(15 * time.Millisecond)
	if activations, _ := target.counts(); activations != 1 {
		t.Fatalf("reactivation overlapped cleanup: activations=%d", activations)
	}

	close(target.allowDeactivate)
	waitForBlockingCounts(t, target, 2, 1)
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

func waitForBlockingCounts(t *testing.T, target *blockingTarget, activations, deactivations int) {
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
