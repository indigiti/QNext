package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestBrokerStatsTrackStreamsSubscribersAndFreshness(t *testing.T) {
	broker := NewBroker(8, 4)
	open := time.Date(2026, 9, 30, 8, 30, 0, 0, time.UTC)
	broker.PublishBar(domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "15s",
		OpenTime:     open,
		CloseTime:    open.Add(15 * time.Second),
		Open:         25000,
		High:         25001,
		Low:          24999,
		Close:        25000.5,
	})

	subscription, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Cancel()

	stats := broker.Stats()
	if stats.Streams != 1 || stats.Subscribers != 1 || stats.ReplayEvents != 1 {
		t.Fatalf("unexpected broker stats: %+v", stats)
	}
	if len(stats.Details) != 1 {
		t.Fatalf("expected one stream detail, got %+v", stats.Details)
	}
	detail := stats.Details[0]
	if detail.InstrumentID != "NSE:NIFTY50" || detail.Timeframe != "15s" {
		t.Fatalf("unexpected stream detail: %+v", detail)
	}
	if detail.LastPublishedAtMS <= 0 {
		t.Fatalf("missing publish timestamp: %+v", detail)
	}
	if detail.LastBarOpenTimeMS != open.UnixMilli() {
		t.Fatalf("open time=%d want=%d", detail.LastBarOpenTimeMS, open.UnixMilli())
	}

	subscription.Cancel()
	if got := broker.Stats().Subscribers; got != 0 {
		t.Fatalf("subscribers after cancel=%d", got)
	}
}

func TestBrokerReplayCapacityAndConnectionAccounting(t *testing.T) {
	broker := NewBroker(32, 4)
	at := time.Date(2026, 10, 14, 3, 45, 0, 0, time.UTC)
	broker.PublishBar(domain.Bar{InstrumentID: "NSE:NIFTY50", Timeframe: "15s", OpenTime: at})
	stats := broker.Stats()
	if stats.ActiveStreams != 0 || stats.IdleStreams != 1 || stats.WebSocketConnections != 0 {
	t.Fatalf("unexpected idle stats: %+v", stats)
	}
	if stats.ReplayEvents != 1 || stats.ReplaySlots != 16 || stats.ReplayBytesEstimated <= 0 {
	t.Fatalf("unexpected replay footprint: %+v", stats)
	}
	for i := 0; i < 17; i++ {
	broker.PublishBar(domain.Bar{InstrumentID: "NSE:NIFTY50", Timeframe: "15s", OpenTime: at})
	}
	stats = broker.Stats()
	if stats.ReplaySlots != 32 || stats.ReplayEvents != 18 {
	t.Fatalf("replay growth changed: %+v", stats)
	}
	broker.webSocketConnections.Add(2)
	if got := broker.Stats().WebSocketConnections; got != 2 {
	t.Fatalf("physical connection count=%d", got)
	}
	broker.webSocketConnections.Add(-2)
}
