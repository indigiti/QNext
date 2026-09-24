package integrity

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestDedupeSinkDropsRepeatedLTPCSnapshot(t *testing.T) {
	var delivered []domain.Tick
	sink := NewDedupeSink(8, func(tick domain.Tick) error {
		delivered = append(delivered, tick)
		return nil
	})

	tick := domain.Tick{
		Provider:     "upstox",
		InstrumentID: "NSE:NIFTY50",
		Price:        25100,
		EventTime:    time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC),
	}
	if err := sink.Handle(tick); err != nil {
		t.Fatal(err)
	}
	if err := sink.Handle(tick); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 1 {
		t.Fatalf("expected one delivered tick, got %d", len(delivered))
	}

	tick.Price = 25101
	if err := sink.Handle(tick); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 2 {
		t.Fatalf("price change must remain observable, got %d ticks", len(delivered))
	}
}
