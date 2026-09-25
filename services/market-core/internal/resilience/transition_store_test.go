package resilience

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTransitionStoreAppendsImmutableEvents(t *testing.T) {
	root := t.TempDir()
	store := NewTransitionStore(root)
	at := time.Date(2026, 9, 25, 9, 15, 3, 0, time.UTC)
	event := SwitchEvent{
		InstrumentID:  "NSE:NIFTY50",
		From:          "upstox",
		To:            "dhan",
		Reason:        "PRIMARY_STALE",
		PolicyVersion: "q3-authority-v1",
		AtMS:          at.UnixMilli(),
		GapMS:         3000,
	}
	if err := store.Append(event); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(event); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "resilience", "authority-transitions", "2026", "09", "2026-09-25.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%d", len(lines))
	}
	if !strings.Contains(lines[0], `"authority_policy_version":"q3-authority-v1"`) ||
		!strings.Contains(lines[0], `"reason":"PRIMARY_STALE"`) {
		t.Fatalf("record=%s", lines[0])
	}
}
