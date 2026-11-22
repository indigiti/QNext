package liveaudit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/stream"
)

func testBar(id, tf string, at time.Time, volume float64, revision uint32, final bool) domain.Bar {
	duration := auditedFrames[tf]
	return domain.Bar{InstrumentID: id, Timeframe: tf, OpenTime: at,
		CloseTime: at.Add(duration), Open: 25100, High: 25100,
		Low: 25100, Close: 25100, Volume: volume,
		Final: final, Revision: revision,
		VolumeSource: "FUTURES_PROXY", VolumeStatus: "OBSERVED"}
}

func TestLiveIntegritySourceLagWaitsForCorrection(t *testing.T) {
	const source = "NSE:NIFTY50"
	const syn = "QNEXT:NIFTY-SYN+"
	m := New(t.TempDir(), map[string]string{syn: source})
	at := time.Date(2026, 10, 28, 3, 45, 0, 0, time.UTC)
	published := time.Date(2026, 10, 28, 3, 45, 7, 0, time.UTC)
	send := func(id string, seq uint64, bar domain.Bar, at time.Time) {
		t.Helper()
		m.observe(stream.BarEvent{Seq: seq, Bar: bar, PublishedAt: at})
	}
	empty := testBar(syn, "15s", at, 0, 0, false)
	empty.VolumeSource, empty.VolumeStatus = "", "UNKNOWN"
	send(syn, 1, empty, published)
	send(source, 1, testBar(source, "15s", at, 12, 1, false), published)
	m.inspect(published.Add(checkDelay-time.Millisecond))
	if got := m.Snapshot().Counts["SYN_FUTURES_VOLUME_LAG"]; got != 0 { t.Fatalf("premature lag alert: %d", got) }
	send(syn, 2, testBar(syn, "15s", at, 12, 1, false), published.Add(time.Millisecond))
	m.inspect(published.Add(5*time.Second))
	if got := m.Snapshot().Counts["SYN_FUTURES_VOLUME_LAG"]; got != 0 { t.Fatalf("corrected volume flagged: %d", got) }

	next := at.Add(15*time.Second)
	send(syn, 3, testBar(syn, "15s", next, 0, 0, false), published.Add(time.Second))
	send(source, 2, testBar(source, "15s", next, 30, 1, false), published.Add(time.Second))
	m.inspect(published.Add(6*time.Second))
	got := m.Snapshot()
	if got.Counts["SYN_FUTURES_VOLUME_LAG"] != 1 || got.Recent[len(got.Recent)-1].ExpectedVolume != 30 {
		t.Fatalf("expected one persistent source-lag alert: %+v", got)
	}
	m.inspect(published.Add(7*time.Second))
	if m.Snapshot().Counts["SYN_FUTURES_VOLUME_LAG"] != 1 { t.Fatal("lag alert repeated for same candle") }
	path := filepath.Join(m.root, published.In(india).Format("2006-01"), published.In(india).Format("2006-01-02")+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "SYN_FUTURES_VOLUME_LAG") {
		t.Fatalf("missing local JSONL evidence: %s %v", data, err)
	}
}

func TestLiveIntegrityDetectsUnrevisionedFinalAndGaps(t *testing.T) {
	m := New(t.TempDir(), map[string]string{"QNEXT:NIFTY-SYN": "NSE:NIFTY50"})
	at := time.Date(2026, 10, 28, 3, 45, 0, 0, time.UTC)
	first := testBar("QNEXT:NIFTY-SYN", "15s", at, 12, 2, true)
	m.observe(stream.BarEvent{Seq: 1, Bar: first, PublishedAt: at})
	repeated := first
	repeated.Volume = 11
	m.observe(stream.BarEvent{Seq: 2, Bar: repeated, PublishedAt: at.Add(time.Second)})
	next := testBar(first.InstrumentID, "15s", at.Add(45*time.Second), 15, 0, true)
	m.observe(stream.BarEvent{Seq: 3, Bar: next, PublishedAt: at.Add(45*time.Second)})
	snap := m.Snapshot()
	if snap.Counts["VOLUME_REGRESSION"] != 1 || snap.Counts["PUBLISHED_CANDLE_GAP"] != 1 {
		t.Fatalf("unexpected regression/gap counts: %+v", snap.Counts)
	}
	// An old message must not rewind the current bar or report a false gap.
	m.observe(stream.BarEvent{Seq: 2, Bar: first, PublishedAt: at.Add(time.Minute)})
	if m.Snapshot().Counts["PUBLISHED_CANDLE_GAP"] != 1 {
		t.Fatal("stale stream sequence caused another gap")
	}
}

func TestLiveIntegrityCaptureAndMonitorLossEvidence(t *testing.T) {
	m := New(t.TempDir(), map[string]string{"QNEXT:NIFTY-SYN+": "NSE:NIFTY50"})
	m.SetDiagnostics(func() (bool, uint64, uint64, uint64) { return true, 3, 2, 4 })
	at := time.Date(2026, 10, 28, 3, 45, 0, 0, time.UTC)
	m.inspect(at)
	snap := m.Snapshot()
	if !snap.RawCaptureEnabled || snap.RawCaptureDropped != 3 || snap.RawCaptureErrors != 2 ||
		snap.AuditDropped != 4 || snap.Counts["RAW_CAPTURE_LOSS"] != 1 ||
		snap.Counts["AUDIT_QUEUE_OVERFLOW"] != 1 || snap.Counts["RAW_CAPTURE_WRITE_ERROR"] != 1 {
		t.Fatalf("capture loss not surfaced: %+v", snap)
	}
	m.inspect(at.Add(time.Second))
	if m.Snapshot().Counts["RAW_CAPTURE_LOSS"] != 1 { t.Fatal("repeated capture warning") }
}

func TestLiveIntegrityStopsOnContextCancellation(t *testing.T) {
	m := New(t.TempDir(), map[string]string{"QNEXT:NIFTY-SYN+": "NSE:NIFTY50"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second): t.Fatal("monitor did not stop")
	}
}
