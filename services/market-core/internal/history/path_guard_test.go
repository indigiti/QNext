package history

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryPathsRemainUnderMarketRootForDotSegments(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	at := time.Date(2026, 11, 23, 3, 45, 0, 0, time.UTC)
	marketRoot := filepath.Join(root, "market")

	for _, item := range []struct {
		instrument string
		timeframe  string
	}{
		{"..", "1m"},
		{"NSE:NIFTY50", ".."},
		{"..", ".."},
		{".", "."},
		{"QNEXT:NIFTY-SYN+", "5s"},
		{"NSE:NIFTY50", "1m"},
	} {
		path := store.dayPath(item.instrument, item.timeframe, at)
		relative, err := filepath.Rel(marketRoot, path)
		if err != nil {
			t.Fatal(err)
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("history path escaped market root: %q/%q -> %q", item.instrument, item.timeframe, path)
		}
	}

	if got := safeComponent("NSE:NIFTY50"); got != "NSE_NIFTY50" {
		t.Fatalf("existing canonical folder layout changed: %q", got)
	}
	if safeComponent(".") == "." || safeComponent("..") == ".." {
		t.Fatal("directory traversal segments remained unchanged")
	}
}
