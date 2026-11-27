package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestReplaySuffixAcrossWrapAndCallerIsolation(t *testing.T) {
	ring := newReplayRing(65)
	for seq := uint64(1); seq <= 200; seq++ {
		ring.Append(BarEvent{Seq: seq})
	}
	for _, tc := range []struct {
		after uint64
		first uint64
		count int
	}{
		{0, 136, 65},
		{135, 136, 65},
		{150, 151, 50},
		{195, 196, 5},
		{199, 200, 1},
		{200, 0, 0},
		{201, 0, 0},
	} {
		got := ring.after(tc.after)
		if len(got) != tc.count {
			t.Fatalf("after %d: got %d events, want %d", tc.after, len(got), tc.count)
		}
		for i, event := range got {
			if event.Seq != tc.first+uint64(i) {
				t.Fatalf("after %d: event %d has seq %d", tc.after, i, event.Seq)
			}
		}
	}
	saved := ring.after(198)
	saved[0].Seq = 999
	if got := ring.after(198); got[0].Seq != 199 {
		t.Fatal("caller mutated replay ring")
	}
	for seq := uint64(201); seq <= 270; seq++ {
		ring.Append(BarEvent{Seq: seq})
	}
	if saved[1].Seq != 200 {
		t.Fatal("ring overwrite mutated caller replay")
	}
}

func TestSubscriberWakesOnlyWhenWorkBecomesAvailable(t *testing.T) {
	s := newSubscriberState(2)
	at := time.Date(2026, 11, 27, 3, 45, 0, 0, time.UTC)
	for seq := uint64(1); seq <= 3; seq++ {
		s.enqueue(BarEvent{Seq: seq, Bar: domain.Bar{OpenTime: at}})
		if seq == 1 {
			select {
			case <-s.wake:
			default:
				t.Fatal("empty-to-nonempty transition did not signal")
			}
		}
	}
	select {
	case <-s.wake:
		t.Fatal("backlog append or replacement signaled unnecessarily")
	default:
	}
	for _, want := range []uint64{1, 3} {
		got, ok := s.next()
		if !ok || got.Seq != want {
			t.Fatalf("queue sequence: got %d want %d", got.Seq, want)
		}
	}
	// An actually waiting consumer must receive the next protected event.
	go s.run()
	defer s.stop()
	s.enqueue(BarEvent{Seq: 4, Bar: domain.Bar{Final: true}})
	select {
	case got := <-s.out:
		if got.Seq != 4 || !got.Bar.Final {
			t.Fatalf("protected event changed: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("consumer did not wake")
	}
}

func BenchmarkBrokerResumeRecentSuffix(b *testing.B) {
	broker := NewBroker(1024, 128)
	at := time.Date(2026, 11, 27, 3, 45, 0, 0, time.UTC)
	for i := 0; i < 2048; i++ {
		broker.PublishBar(bar("NSE:NIFTY50", "5s", at, float64(i)))
	}
	after := uint64(2046)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sub, err := broker.Subscribe("NSE:NIFTY50", "5s", &after)
		if err != nil || sub.ResyncRequired || len(sub.Replay) != 2 {
			b.Fatalf("resume: sub=%+v err=%v", sub, err)
		}
		sub.Cancel()
	}
}
