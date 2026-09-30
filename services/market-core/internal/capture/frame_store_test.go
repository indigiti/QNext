package capture

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFrameStoreAppendIsBestEffortAndPersistsAsynchronously(t *testing.T) {
	root := t.TempDir()
	store := NewFrameStore(root, "UPSTOX")
	fixed := time.Date(2026, 9, 30, 7, 45, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }

	if err := store.Append([]byte("frame-1")); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "raw", "upstox", "2026", "09", "2026-09-30.jsonl")
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("raw frame was not persisted asynchronously: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	stats := store.Stats()
	if stats.Enqueued != 1 || stats.Dropped != 0 || stats.Errors != 0 {
		t.Fatalf("unexpected capture stats: %+v", stats)
	}
}

func TestFrameStoreDoesNotSurfaceStorageFailureToLiveCaller(t *testing.T) {
	store := NewFrameStore(filepath.Join(t.TempDir(), "blocked"), "upstox")
	if err := os.WriteFile(store.root, []byte("not-a-directory"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := store.Append([]byte("frame-2")); err != nil {
		t.Fatalf("capture storage failure must not block live ingestion: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for store.Stats().Errors == 0 {
		if time.Now().After(deadline) {
			t.Fatal("expected asynchronous capture error to be counted")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
