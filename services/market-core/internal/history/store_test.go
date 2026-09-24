package history

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func testBar(at time.Time, close float64, revision uint32) domain.Bar {
	return domain.Bar{
		InstrumentID:        "NSE:NIFTY50",
		Timeframe:           "30s",
		OpenTime:            at,
		CloseTime:           at.Add(30 * time.Second),
		Open:                25100,
		High:                25105,
		Low:                 25099,
		Close:               close,
		Final:               true,
		Revision:            revision,
		AuthorityProvider:   "fixture",
		Quality:             domain.QualityGood,
		CandleEngineVersion: "candle-v1",
		Corrected:           revision > 0,
	}
}

func TestStoreKeepsLatestRevisionWithoutOverwritingHistory(t *testing.T) {
	store := New(t.TempDir())
	at := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)

	if err := store.AppendBar(testBar(at, 25101, 0)); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendBar(testBar(at, 25102, 1)); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendBar(testBar(at.Add(30*time.Second), 25103, 0)); err != nil {
		t.Fatal(err)
	}

	bars, err := store.LoadDay("NSE:NIFTY50", "30s", at)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 canonical bars, got %d", len(bars))
	}
	if bars[0].Revision != 1 || bars[0].Close != 25102 || !bars[0].Corrected {
		t.Fatalf("expected latest correction, got %+v", bars[0])
	}
	if !bars[0].OpenTime.Before(bars[1].OpenTime) {
		t.Fatal("bars are not ordered by open time")
	}
}

func TestStoreRejectsFormingBar(t *testing.T) {
	store := New(t.TempDir())
	bar := testBar(time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC), 25101, 0)
	bar.Final = false
	if err := store.AppendBar(bar); err == nil {
		t.Fatal("expected forming bar persistence to fail")
	}
}
