package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

// Reproducible allocation/throughput benchmarks. These do not depend on a
// broker, market hours, external provider, or production API credentials.
func BenchmarkBrokerPublishNoSubscribers(b *testing.B) {
	broker := NewBroker(1024, 128)
	bar := domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "15s",
		OpenTime:     time.Date(2026, 10, 13, 9, 15, 0, 0, time.UTC),
		Close:        25000,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.PublishBar(bar)
	}
}

func BenchmarkSubscriberQueueSteadyState(b *testing.B) {
	s := newSubscriberState(128)
	event := BarEvent{
		Seq: 1,
		Bar: domain.Bar{
			InstrumentID: "NSE:NIFTY50",
			Timeframe:    "15s",
			OpenTime:     time.Date(2026, 10, 13, 9, 15, 0, 0, time.UTC),
			Close:        25000,
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.enqueue(event)
		s.next()
	}
}

func TestReplayRingGrowsWithoutChangingResumeWindow(t *testing.T) {
	ring := newReplayRing(65)
	if got := len(ring.items); got != 16 {
		t.Fatalf("initial storage=%d, want 16", got)
	}
	for seq := uint64(1); seq <= 260; seq++ {
		ring.Append(BarEvent{Seq: seq})
		wantLen := int(seq)
		if wantLen > 65 {
			wantLen = 65
		}
		if ring.Len() != wantLen {
			t.Fatalf("seq=%d replay length=%d want=%d", seq, ring.Len(), wantLen)
		}
		if len(ring.items) > ring.limit {
			t.Fatalf("replay capacity exceeds limit: %d > %d", len(ring.items), ring.limit)
		}
		first, ok := ring.First()
		if !ok || first.Seq != seq-uint64(wantLen)+1 {
			t.Fatalf("seq=%d wrong oldest replay event: %+v", seq, first)
		}
		last, ok := ring.Last()
		if !ok || last.Seq != seq {
			t.Fatalf("seq=%d wrong latest replay event: %+v", seq, last)
		}
		if seq == 17 || seq == 33 || seq == 65 || seq == 66 || seq == 260 {
			for i, event := range ring.Slice() {
				if want := seq-uint64(wantLen)+1+uint64(i); event.Seq != want {
					t.Fatalf("seq=%d replay index %d has %d, want %d", seq, i, event.Seq, want)
				}
			}
		}
	}
	if got := len(ring.items); got != 65 {
		t.Fatalf("grown replay storage=%d, want 65", got)
	}
}

func TestSubscriberQueueReusesBufferAndRetainsProtectedEvents(t *testing.T) {
	s := newSubscriberState(4)
	for seq := uint64(1); seq <= 1000; seq++ {
		original := BarEvent{Seq: seq, Bar: domain.Bar{Final: true}}
		if result := s.enqueue(original); result.overflow {
			t.Fatalf("unexpected overflow for seq %d", seq)
		}
		got, ok := s.next()
		if !ok || got.Seq != seq || !got.Bar.Final {
			t.Fatalf("lost/reordered protected event: got=%+v ok=%t want=%d", got, ok, seq)
		}
		if len(s.queue) != 0 || cap(s.queue) != cap(s.queueStorage) {
			t.Fatalf("subscriber did not reclaim its queue buffer at seq=%d", seq)
		}
	}

	// Keep a backlog while forcing the tail of the consumed slice past the
	// preallocated storage. This exercises compaction, not just empty resets.
	var expected uint64 = 1001
	var nextSeq uint64 = expected
	for i := 0; i < 4; i++ {
		s.enqueue(BarEvent{Seq: nextSeq, Bar: domain.Bar{Final: true}})
		nextSeq++
	}
	for i := 0; i < 100; i++ {
		got, ok := s.next()
		if !ok || got.Seq != expected {
			t.Fatalf("backlog delivery: got=%d want=%d", got.Seq, expected)
		}
		expected++
		s.enqueue(BarEvent{Seq: nextSeq, Bar: domain.Bar{Final: true}})
		nextSeq++
	}
	for len(s.queue) > 0 {
		got, ok := s.next()
		if !ok || got.Seq != expected {
			t.Fatalf("remaining backlog: got=%d want=%d", got.Seq, expected)
		}
		expected++
	}
	if expected != nextSeq {
		t.Fatalf("remaining protected event mismatch: expected=%d next=%d", expected, nextSeq)
	}
}

func TestPublishedStreamIdentityStableAcrossResume(t *testing.T) {
	broker := NewBroker(32, 4)
	instrumentID := "QNEXT:NIFTY-SYN+"
	timeframe := "15s"
	original := bar(instrumentID, timeframe, time.Date(2026, 10, 13, 3, 45, 0, 0, time.UTC), 25100)
	broker.PublishBar(original)
	broker.PublishBar(original)
	b := broker.streams[streamKey(instrumentID, timeframe)]
	if b.id != streamID(instrumentID, timeframe) {
		t.Fatalf("unexpected cached stream ID: %s", b.id)
	}
	if events := b.replay.Slice(); len(events) != 2 || events[0].StreamID != b.id || events[1].StreamID != b.id {
		t.Fatalf("published event stream IDs differ from cached identity: %+v", events)
	}
	after := uint64(1)
	sub, err := broker.Subscribe(instrumentID, timeframe, &after)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()
	if sub.ResyncRequired || len(sub.Replay) != 1 || sub.Replay[0].StreamID != b.id {
		t.Fatalf("resume changed cached stream identity: %+v", sub)
	}
}
