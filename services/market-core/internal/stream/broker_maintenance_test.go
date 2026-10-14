package stream

import (
	"testing"
	"time"
)

func TestIdleReplayCompactionPreservesLastBarSequenceAndResync(t *testing.T) {
	b := NewBroker(64, 4)
	at := time.Date(2026, 10, 14, 3, 45, 0, 0, time.UTC)
	for i := 1; i <= 35; i++ {
		b.PublishBar(bar("NSE:NIFTY50", "15s", at, float64(i)))
	}
	key := streamKey("NSE:NIFTY50", "15s")
	original := b.streams[key]
	if original.seq != 35 || len(original.replay.items) != 64 {
		t.Fatalf("unexpected replay setup: seq=%d slots=%d", original.seq, len(original.replay.items))
	}
	// Maintenance must not compact a stream that still has an active chart.
	sub, err := b.Subscribe("NSE:NIFTY50", "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(60 * time.Minute)
	if compacted, _ := b.CompactIdleReplay(future, 45*time.Minute); compacted != 0 {
		t.Fatal("active subscription compacted")
	}
	sub.Cancel()
	if compacted, _ := b.CompactIdleReplay(future, 45*time.Minute); compacted != 1 {
		t.Fatalf("expected compaction after unsubscribe, got %d", compacted)
	}
	state := b.streams[key]
	if state != original || state.seq != 35 || state.replay.Len() != 1 || len(state.replay.items) != 1 {
		t.Fatalf("compaction changed stream identity/sequence: %+v", state)
	}
	latest, ok := b.LatestBar("NSE:NIFTY50", "15s")
	if !ok || latest.Close != 35 {
		t.Fatalf("latest bar lost during compaction: %+v", latest)
	}
	stats := b.Stats()
	if stats.ReclaimedReplaySlots != 63 || stats.CompactedStreams != 1 || stats.ReplaySlots != 1 {
		t.Fatalf("unexpected compaction telemetry: %+v", stats)
	}
	tooOld := uint64(32)
	expired, err := b.Subscribe("NSE:NIFTY50", "15s", &tooOld)
	if err != nil || !expired.ResyncRequired {
		t.Fatalf("old replay must request REST resync: subscription=%+v err=%v", expired, err)
	}
	lastOnly := uint64(34)
	replayed, err := b.Subscribe("NSE:NIFTY50", "15s", &lastOnly)
	if err != nil || replayed.ResyncRequired {
		t.Fatalf("latest replay unexpectedly unavailable: %+v err=%v", replayed, err)
	}
	if len(replayed.Replay) != 1 || replayed.Replay[0].Seq != 35 {
		t.Fatalf("latest sequence not replayable: %+v", replayed.Replay)
	}
	replayed.Cancel()
	b.PublishBar(bar("NSE:NIFTY50", "15s", at, 36))
	next := b.streams[key]
	if next.seq != 36 || next.replay.Len() != 2 || len(next.replay.items) != 2 {
		t.Fatalf("post-compaction append damaged replay growth: %+v", next)
	}
}

func TestIdleReplayCompactionKeepsRecentStreamAndEvictsUnusedEmptyStream(t *testing.T) {
	b := NewBroker(32, 4)
	at := time.Now()
	b.PublishBar(bar("NSE:NIFTY50", "15s", at, 1))
	for i := 0; i < 18; i++ {
		b.PublishBar(bar("NSE:NIFTY50", "15s", at, 2))
	}
	empty, err := b.Subscribe("NSE:EMPTY", "1m", nil)
	if err != nil {
		t.Fatal(err)
	}
	empty.Cancel()
	// Without inactivity, housekeeping must leave published replay intact.
	if compacted, removed := b.CompactIdleReplay(at.Add(time.Minute), 45*time.Minute); compacted != 0 || removed != 0 {
		t.Fatalf("premature maintenance: compacted=%d removed=%d", compacted, removed)
	}
	if compacted, removed := b.CompactIdleReplay(at.Add(time.Hour), 45*time.Minute); compacted != 1 || removed != 1 {
		t.Fatalf("expired housekeeping: compacted=%d removed=%d", compacted, removed)
	}
	if _, ok := b.streams[streamKey("NSE:EMPTY", "1m")]; ok {
		t.Fatal("never-published empty stream was not removed")
	}
	if got := b.Stats().EvictedEmptyStreams; got != 1 {
		t.Fatalf("eviction telemetry=%d", got)
	}
}

func TestSampledLatencyHistogramPercentiles(t *testing.T) {
	var h sampledLatencyHistogram
	for i := 0; i < 160; i++ {
		elapsed := 2 * time.Microsecond
		if i >= 152 {
			elapsed = 2 * time.Millisecond
		}
		h.observe(elapsed)
	}
	stats := h.snapshot()
	if stats.Samples != 10 || stats.P50US != 2 || stats.P95US != 2048 || stats.P99US != 2048 {
		t.Fatalf("incorrect bounded percentile histogram: %+v", stats)
	}
}
