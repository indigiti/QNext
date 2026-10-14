package stream

import (
	"context"
	"math/bits"
	"sync/atomic"
	"time"
)

// LatencyStats contains approximate upper-bound percentiles from a bounded,
// allocation-free histogram. Sampling is one observation per 16 operations.
// These are broker processing and socket write durations, not feed-to-screen RTT.
type LatencyStats struct {
	Samples uint64 `json:"samples"`
	P50US   uint64 `json:"p50_us"`
	P95US   uint64 `json:"p95_us"`
	P99US   uint64 `json:"p99_us"`
}

const latencyBucketCount = 26

type sampledLatencyHistogram struct {
	calls   atomic.Uint64
	buckets [latencyBucketCount]atomic.Uint64
}

func (h *sampledLatencyHistogram) observe(elapsed time.Duration) {
	if h.calls.Add(1)&15 != 0 {
		return
	}
	var us uint64 = 1
	if elapsed > 0 {
		us = uint64(elapsed/time.Microsecond) + 1
	}
	index := bits.Len64(us - 1)
	if index >= latencyBucketCount {
		index = latencyBucketCount - 1
	}
	h.buckets[index].Add(1)
}

func (h *sampledLatencyHistogram) snapshot() LatencyStats {
	var buckets [latencyBucketCount]uint64
	var count uint64
	for i := range buckets {
		buckets[i] = h.buckets[i].Load()
		count += buckets[i]
	}
	if count == 0 {
		return LatencyStats{}
	}
	percentile := func(percent uint64) uint64 {
		target := (count*percent + 99) / 100
		var seen uint64
		for i, n := range buckets {
			seen += n
			if seen >= target {
				return uint64(1) << i
			}
		}
		return uint64(1) << (latencyBucketCount - 1)
	}
	return LatencyStats{
		Samples: count,
		P50US: percentile(50),
		P95US: percentile(95),
		P99US: percentile(99),
	}
}

// CompactIdleReplay preserves the canonical stream ID, sequence watermark,
// last candle, and its replay semantics. It only frees surplus replay slots
// when a stream has no subscribers and no publishes for idleFor.
// An out-of-window resume still requests a REST resync via the normal protocol.
func (b *Broker) CompactIdleReplay(at time.Time, idleFor time.Duration) (compacted, removedEmpty int) {
	if b == nil || at.IsZero() || idleFor <= 0 {
		return 0, 0
	}
	nowMS := at.UnixMilli()
	threshold := idleFor.Milliseconds()
	if threshold <= 0 {
		threshold = 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, state := range b.streams {
		if len(state.subs) != 0 || state.idleSinceMS == 0 ||
			nowMS-state.idleSinceMS < threshold ||
			(state.lastPublishedAtMS != 0 && nowMS-state.lastPublishedAtMS < threshold) {
			continue
		}
		if state.seq == 0 {
			// Never published: no replay identity/sequence can be lost.
			delete(b.streams, key)
			removedEmpty++
			b.evictedEmptyStreams++
			continue
		}
		if freed := state.replay.CompactToLatest(); freed > 0 {
			compacted++
			b.compactedStreams++
			b.reclaimedReplaySlots += uint64(freed)
		}
	}
	return compacted, removedEmpty
}

// CompactToLatest retains one event and its original sequence, releasing the
// previous allocation. The next append grows the ring normally.
func (r *replayRing) CompactToLatest() int {
	if r == nil || len(r.items) <= 1 {
		return 0
	}
	last, ok := r.Last()
	if !ok {
		return 0
	}
	slotsFreed := len(r.items) - 1
	r.items = []BarEvent{last}
	r.start = 0
	r.size = 1
	return slotsFreed
}

// RunMaintenance is context-bound to the market-core process: there is no
// per-stream ticker or expiry work in the candle/publish hot path.
func (b *Broker) RunMaintenance(ctx context.Context, interval, idleFor time.Duration) {
	if b == nil || ctx == nil || interval <= 0 || idleFor <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			b.CompactIdleReplay(now, idleFor)
		}
	}
}
