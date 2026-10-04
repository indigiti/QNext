package candle

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestApplyVolumeDoesNotChangeOHLC(t *testing.T) {
	engine := New("test")
	at := time.Date(2026, 10, 5, 9, 15, 1, 0, indiaLocation).UTC()

	priceTick := domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "upstox",
		Price:        25000,
		EventTime:    at,
		Sequence:     1,
		Quality:      domain.QualityGood,
	}
	bars, err := engine.Apply(priceTick, "5s")
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one price bar, got %d", len(bars))
	}

	volumeTick := domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "upstox-futures-proxy",
		Quantity:     125,
		EventTime:    at.Add(time.Second),
		Sequence:     2,
		Quality:      domain.QualityGood,
	}
	updates, err := engine.ApplyVolume(volumeTick, "5s")
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected one volume update, got %d", len(updates))
	}
	got := updates[0]
	if got.Open != 25000 || got.High != 25000 || got.Low != 25000 || got.Close != 25000 {
		t.Fatalf("volume overlay changed spot OHLC: %+v", got)
	}
	if got.Volume != 125 {
		t.Fatalf("unexpected proxy volume: %v", got.Volume)
	}
}

func TestApplyVolumeQueuesUntilPriceBucketExists(t *testing.T) {
	engine := New("test")
	at := time.Date(2026, 10, 5, 9, 15, 6, 0, indiaLocation).UTC()

	volumeTick := domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "upstox-futures-proxy",
		Quantity:     80,
		EventTime:    at,
		Sequence:     10,
		Quality:      domain.QualityGood,
	}
	updates, err := engine.ApplyVolume(volumeTick, "5s")
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 0 {
		t.Fatalf("volume-only update must not fabricate a price bar: %+v", updates)
	}

	priceTick := domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     "upstox",
		Price:        25010,
		EventTime:    at.Add(time.Second),
		Sequence:     11,
		Quality:      domain.QualityGood,
	}
	bars, err := engine.Apply(priceTick, "5s")
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].Volume != 80 {
		t.Fatalf("pending proxy volume was not attached: %+v", bars)
	}
	if bars[0].Open != 25010 || bars[0].Close != 25010 {
		t.Fatalf("pending volume changed price: %+v", bars[0])
	}
}
