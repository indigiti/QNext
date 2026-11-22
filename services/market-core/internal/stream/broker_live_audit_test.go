package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

// The integrity monitor must never turn diagnostic congestion into a stalled
// market stream. The broker drops *audit copies*, not canonical bar events.
func TestLiveAuditOverflowDoesNotBlockCanonicalPublish(t *testing.T) {
	broker := NewBroker(8, 8)
	queue := make(chan BarEvent, 1)
	broker.SetAuditSink(queue, []string{"NSE:NIFTY50"})

	at := time.Date(2026, 10, 28, 3, 45, 0, 0, time.UTC)
	first := domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "15s",
		OpenTime:     at,
		CloseTime:    at.Add(15 * time.Second),
		Open:         25100,
		High:         25100,
		Low:          25100,
		Close:        25100,
		Quality:      domain.QualityGood,
	}
	broker.PublishBar(first)
	second := first
	second.OpenTime = at.Add(15 * time.Second)
	second.CloseTime = second.OpenTime.Add(15 * time.Second)
	second.Open, second.High, second.Low, second.Close = 25101, 25101, 25101, 25101

	done := make(chan struct{})
	go func() {
		broker.PublishBar(second)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("saturated audit queue blocked canonical bar publication")
	}
	if got := broker.AuditDrops(); got != 1 {
		t.Fatalf("expected one lost audit copy, got %d", got)
	}
	event := <-queue
	if event.Seq != 1 || event.Bar.Close != 25100 {
		t.Fatalf("first ordered audit event was corrupted: %+v", event)
	}
	latest, ok := broker.LatestBar("NSE:NIFTY50", "15s")
	if !ok || !latest.OpenTime.Equal(second.OpenTime) || latest.Close != 25101 {
		t.Fatalf("canonical stream failed despite audit-only loss: %+v ok=%v", latest, ok)
	}
}
