package stream

import (
	"testing"
	"time"
)

func TestBrokerCoalescesFormingUpdatesForSlowSubscriber(t *testing.T) {
	broker := NewBroker(32, 2)
	sub, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()

	at := time.Date(2026, 9, 30, 9, 15, 0, 0, time.UTC)
	for revision := uint32(1); revision <= 20; revision++ {
		update := bar("NSE:NIFTY50", "15s", at, 25000+float64(revision))
		update.Revision = revision
		broker.PublishBar(update)
	}

	stats := broker.Stats()
	if stats.Subscribers != 1 {
		t.Fatalf("slow subscriber should remain connected, stats=%+v", stats)
	}
	if stats.CoalescedForming == 0 {
		t.Fatalf("expected forming revisions to coalesce, stats=%+v", stats)
	}
	if stats.SlowDisconnects != 0 {
		t.Fatalf("forming pressure must not disconnect subscriber, stats=%+v", stats)
	}

	deadline := time.After(time.Second)
	latest := 0.0
	for latest != 25020 {
		select {
		case event, ok := <-sub.Events:
			if !ok {
				t.Fatal("subscriber closed while draining coalesced forming updates")
			}
			latest = event.Bar.Close
		case <-deadline:
			t.Fatalf("latest forming revision was not delivered; last close %.2f", latest)
		}
	}
}

func TestBrokerProtectsFinalCandleUnderFormingPressure(t *testing.T) {
	broker := NewBroker(32, 2)
	sub, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()

	base := time.Date(2026, 9, 30, 9, 15, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		forming := bar("NSE:NIFTY50", "15s", base.Add(time.Duration(i)*15*time.Second), 25000+float64(i))
		forming.Revision = 1
		broker.PublishBar(forming)
	}
	final := bar("NSE:NIFTY50", "15s", base.Add(11*15*time.Second), 25011.5)
	final.Final = true
	final.Revision = 2
	broker.PublishBar(final)

	deadline := time.After(time.Second)
	for {
		select {
		case event, ok := <-sub.Events:
			if !ok {
				t.Fatal("subscriber closed before protected final was delivered")
			}
			if event.Bar.Final && event.Bar.OpenTime.Equal(final.OpenTime) {
				stats := broker.Stats()
				if stats.SlowDisconnects != 0 {
					t.Fatalf("subscriber disconnected despite recoverable forming pressure: %+v", stats)
				}
				if stats.DroppedForming == 0 {
					t.Fatalf("expected stale forming updates to be discarded for final priority: %+v", stats)
				}
				return
			}
		case <-deadline:
			t.Fatal("protected final candle was not delivered")
		}
	}
}

func TestBrokerDisconnectsOnlyAfterProtectedQueueExhaustionAndReplayRecovers(t *testing.T) {
	broker := NewBroker(64, 1)
	sub, err := broker.Subscribe("NSE:NIFTY50", "1m", nil)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 30, 9, 15, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		final := bar("NSE:NIFTY50", "1m", base.Add(time.Duration(i)*time.Minute), 25000+float64(i))
		final.Final = true
		broker.PublishBar(final)
	}

	stats := broker.Stats()
	if stats.Subscribers != 0 || stats.SlowDisconnects != 1 {
		t.Fatalf("expected pathological protected-event backlog to disconnect once: %+v", stats)
	}

	select {
	case _, ok := <-sub.Events:
		if ok {
			for range sub.Events {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("slow subscriber delivery channel did not close")
	}

	after := uint64(0)
	resumed, err := broker.Subscribe("NSE:NIFTY50", "1m", &after)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Cancel()
	if resumed.ResyncRequired {
		t.Fatal("protected finals should remain recoverable from broker replay")
	}
	if len(resumed.Replay) != 20 {
		t.Fatalf("unexpected replay length after slow-client disconnect: %d", len(resumed.Replay))
	}
	for index, event := range resumed.Replay {
		if !event.Bar.Final || event.Seq != uint64(index+1) {
			t.Fatalf("unexpected replay event at %d: %+v", index, event)
		}
	}
}

func TestReplayRingRetainsNewestEventsInOrder(t *testing.T) {
	ring := newReplayRing(3)
	for seq := uint64(1); seq <= 5; seq++ {
		ring.Append(BarEvent{Seq: seq})
	}
	got := ring.Slice()
	if len(got) != 3 || got[0].Seq != 3 || got[1].Seq != 4 || got[2].Seq != 5 {
		t.Fatalf("unexpected replay ring: %+v", got)
	}
	first, ok := ring.First()
	if !ok || first.Seq != 3 {
		t.Fatalf("unexpected first event: %+v ok=%v", first, ok)
	}
	last, ok := ring.Last()
	if !ok || last.Seq != 5 {
		t.Fatalf("unexpected last event: %+v ok=%v", last, ok)
	}
}
