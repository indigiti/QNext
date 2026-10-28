package capture

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeRawFixture(t *testing.T, root, provider, day, payload string, modified time.Time) string {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "raw", provider, parsed.Format("2006"), parsed.Format("01"), day+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(payload), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRawRetentionKeepsThirtyUTCDaysAndOnlyRawFiles(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.October, 29, 18, 30, 0, 0, time.UTC)
	oldTime := now.Add(-72 * time.Hour)
	expired := writeRawFixture(t, root, "upstox", "2026-09-29", "old", oldTime)
	atBoundary := writeRawFixture(t, root, "upstox", "2026-09-30", "keep", oldTime)
	today := writeRawFixture(t, root, "upstox-research", "2026-10-29", "live", oldTime)
	anotherProvider := writeRawFixture(t, root, "upstox-research", "2026-08-10", "old-collector", oldTime)
	touchedOld := writeRawFixture(t, root, "upstox", "2026-09-20", "recently edited", now.Add(-time.Hour))

	market := filepath.Join(root, "market", "NSE_NIFTY50", "1m", "2026", "09", "2026-09-29.jsonl")
	ledger := filepath.Join(root, "futures-volume-ledger", "nifty", "2026-09-29.jsonl")
	for _, p := range []string{market, ledger} {
		if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("protected"), 0640); err != nil {
			t.Fatal(err)
		}
	}

	mismatched := filepath.Join(root, "raw", "upstox", "2026", "08", "2026-09-01.jsonl")
	if err := os.MkdirAll(filepath.Dir(mismatched), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mismatched, []byte("wrong folder"), 0640); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(root, "raw", "upstox", "2026", "09", "audit-notes.jsonl")
	if err := os.WriteFile(unknown, []byte("unknown"), 0640); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("do not touch"), 0640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "raw", "upstox", "2026", "09", "2026-09-28.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "raw", "upstox"), filepath.Join(root, "raw", "linked-provider")); err != nil {
		t.Fatal(err)
	}

	stats, err := PruneRawFrames(root, now, 30)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CutoffDate != "2026-09-30" || stats.DeletedFiles != 2 || stats.ReclaimedBytes != int64(len("old")+len("old-collector")) {
		t.Fatalf("unexpected raw sweep: %+v", stats)
	}
	for _, p := range []string{expired, anotherProvider} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("expired raw file was not removed: %s (%v)", p, err)
		}
	}
	for _, p := range []string{atBoundary, today, touchedOld, market, ledger, mismatched, unknown, outside, link} {
		if _, err := os.Lstat(p); err != nil {
			t.Fatalf("protected or unknown file was changed: %s (%v)", p, err)
		}
	}
	if stats.SkippedRecent != 1 || stats.Incomplete {
		t.Fatalf("unexpected scan safeguards: %+v", stats)
	}
}

func TestRawRetentionRejectsUnsafeRootsAndSupportsDisabledService(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	if _, err := PruneRawFrames(root, now, 0); err == nil {
		t.Fatal("invalid retention configuration must not delete files")
	}
	absent, err := PruneRawFrames(root, now, 30)
	if err != nil || absent.DeletedFiles != 0 {
		t.Fatalf("missing raw folder must be a no-op: %+v %v", absent, err)
	}
	outside := t.TempDir()
	linkRoot := filepath.Join(root, "symlink-root")
	if err := os.Symlink(outside, linkRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneRawFrames(linkRoot, now, 30); err == nil {
		t.Fatal("storage-root symlink must be rejected")
	}
	if err := os.Symlink(outside, filepath.Join(root, "raw")); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneRawFrames(root, now, 30); err == nil {
		t.Fatal("raw symlink must be rejected")
	}

	service := NewRawRetention(root, 0)
	done, cancel := context.WithCancel(context.Background())
	cancel()
	service.Run(done, nil)
	status := service.Snapshot()
	if status.Enabled || status.KeepDays != 0 || status.LastRunAt != "" {
		t.Fatalf("disabled retention should not execute: %+v", status)
	}
}

func TestRawRetentionBoundedPassContinuesOnNextRun(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	for i := 1; i <= rawRetentionMaxDeletes+7; i++ {
		day := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i-1).Format("2006-01-02")
		writeRawFixture(t, root, "upstox", day, "old\n", now.Add(-72*time.Hour))
	}
	first, err := PruneRawFrames(root, now, 30)
	if err != nil {
		t.Fatal(err)
	}
	if first.DeletedFiles == 0 || !first.Incomplete {
		t.Fatalf("first sweep should be bounded: %+v", first)
	}
	second, err := PruneRawFrames(root, now, 30)
	if err != nil {
		t.Fatal(err)
	}
	if first.DeletedFiles+second.DeletedFiles != rawRetentionMaxDeletes+7 || second.Incomplete {
		t.Fatalf("successive bounded passes must catch up: first=%+v second=%+v", first, second)
	}
}
