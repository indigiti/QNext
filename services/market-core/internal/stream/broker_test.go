package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func bar(instrument, timeframe string, at time.Time, close float64) domain.Bar {
	return domain.Bar{
		InstrumentID: instrument,
		Timeframe:    timeframe,
		OpenTime:     at,
		CloseTime:    at.Add(time.Minute),
		Open:         close,
		High:         close,
		Low:          close,
		Close:        close,
		Quality:      domain.QualityGood,
	}
}

func TestBrokerReplayAndResume(t *testing.T) {
	broker := NewBroker(3, 4)
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		broker.PublishBar(bar("NSE:NIFTY50", "1m", base.Add(time.Duration(i)*time.Minute), 25100+float64(i)))
	}

	after := uint64(1)
	sub, err := broker.Subscribe("NSE:NIFTY50", "1m", &after)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()
	if sub.ResyncRequired {
		t.Fatal("replay should be available")
	}
	if len(sub.Replay) != 2 || sub.Replay[0].Seq != 2 || sub.Replay[1].Seq != 3 {
		t.Fatalf("unexpected replay: %+v", sub.Replay)
	}

	broker.PublishBar(bar("NSE:NIFTY50", "1m", base.Add(3*time.Minute), 25103))
	event := <-sub.Events
	if event.Seq != 4 || event.Bar.Close != 25103 {
		t.Fatalf("unexpected live event: %+v", event)
	}
}

func TestBrokerRequiresResyncWhenReplayExpired(t *testing.T) {
	broker := NewBroker(2, 4)
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		broker.PublishBar(bar("NSE:NIFTY50", "1m", base.Add(time.Duration(i)*time.Minute), 25100+float64(i)))
	}

	after := uint64(1)
	sub, err := broker.Subscribe("NSE:NIFTY50", "1m", &after)
	if err != nil {
		t.Fatal(err)
	}
	if !sub.ResyncRequired {
		t.Fatal("expected resync when requested sequence is older than replay buffer")
	}
}

func TestStreamIDRoundTrip(t *testing.T) {
	id := streamID("QNEXT:NIFTY-SYN", "30s")
	instrument, timeframe, err := parseStreamID(id)
	if err != nil {
		t.Fatal(err)
	}
	if instrument != "QNEXT:NIFTY-SYN" || timeframe != "30s" {
		t.Fatalf("unexpected stream id decode: %s %s", instrument, timeframe)
	}
}

func TestLatestBarReturnsMostRecentPublishedUpdate(t *testing.T) {
	broker := NewBroker(8, 8)
	at := time.Date(2026, 9, 25, 8, 30, 0, 0, time.UTC)
	broker.PublishBar(bar("NSE:NIFTY50", "15s", at, 23120))
	broker.PublishBar(bar("NSE:NIFTY50", "15s", at, 23122.7))

	latest, ok := broker.LatestBar("NSE:NIFTY50", "15s")
	if !ok {
		t.Fatal("expected latest bar")
	}
	if latest.Close != 23122.7 || latest.OpenTime != at {
		t.Fatalf("unexpected latest bar: %+v", latest)
	}

	if _, ok := broker.LatestBar("NSE:NIFTY50", "1m"); ok {
		t.Fatal("unexpected bar for unknown stream")
	}
}

func TestBrokerPublishesResyncControlWithoutReplacingLatestBar(t *testing.T) {
	broker := NewBroker(8, 8)
	at := time.Date(2026, 9, 25, 8, 30, 0, 0, time.UTC)
	broker.PublishBar(bar("NSE:NIFTY50", "1m", at, 23120))

	sub, err := broker.Subscribe("NSE:NIFTY50", "1m", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()

	broker.PublishResync("NSE:NIFTY50", "1m", "provider_gap_recovered")
	event := <-sub.Events
	if !event.ResyncRequired || event.Reason != "provider_gap_recovered" {
		t.Fatalf("unexpected resync event: %+v", event)
	}

	latest, ok := broker.LatestBar("NSE:NIFTY50", "1m")
	if !ok || latest.Close != 23120 {
		t.Fatalf("resync control must not replace latest bar: %+v ok=%v", latest, ok)
	}
}
