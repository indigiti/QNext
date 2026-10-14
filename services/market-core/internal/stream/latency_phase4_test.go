package stream

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestPhase4TickAndDeliveryTimingMetrics(t *testing.T) {
	broker := NewBroker(32, 4)
	for i := 0; i < 16; i++ {
		broker.ObserveTickHandleLatency(2 * time.Microsecond)
		broker.publishToWriteLatency.observe(8 * time.Microsecond)
	}
	stats := broker.Stats()
	if stats.TickHandleLatency.Samples != 1 || stats.TickHandleLatency.P50US != 2 {
		t.Fatalf("unexpected tick handler sampling: %+v", stats.TickHandleLatency)
	}
	if stats.PublishToWriteLatency.Samples != 1 || stats.PublishToWriteLatency.P50US != 8 {
		t.Fatalf("unexpected WSS timing: %+v", stats.PublishToWriteLatency)
	}
	broker.PublishBar(domain.Bar{InstrumentID: "NSE:NIFTY50", Timeframe: "15s"})
	event, ok := broker.streams[streamKey("NSE:NIFTY50", "15s")].replay.Last()
	if !ok || event.PublishedAt.IsZero() {
		t.Fatalf("publish time missing on live bar: %+v", event)
	}
}

func TestPhase4TimingFieldsNeverLeakIntoWireProtocol(t *testing.T) {
	message := updateMessage(BarEvent{
		StreamID: "sample", Seq: 2, PublishedAt: time.Now(),
		Bar: domain.Bar{InstrumentID: "NSE:NIFTY50", Timeframe: "15s"},
	})
	message.LivePublishedAt = time.Now()
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["LivePublishedAt"]; ok {
		t.Fatalf("internal monotonic timing leaked into wire payload: %s", encoded)
	}
	if _, ok := decoded["live_published_at"]; ok {
		t.Fatalf("internal timing leaked into wire payload: %s", encoded)
	}
	if decoded["op"] != "update" || decoded["seq"] != float64(2) {
		t.Fatalf("wire bar semantics changed: %s", encoded)
	}
}
