package pipeline

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
)

// The canonical 1m volume must equal the twelve 5s child volumes, not the
// number of forming updates or a repeated cumulative VTT observation.
func TestOneMinuteVolumeExactlySumsCanonicalFiveSecondChildren(t *testing.T) {
	store := history.New(t.TempDir())
	pipe, err := New(candle.New("1m-volume-regression"), syncHistoryWriter{store: store}, []string{"1m", "5m"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 15, 3, 45, 0, 0, time.UTC)
	var final domain.Bar
	for i := 0; i <= 12; i++ {
		at := start.Add(time.Duration(i) * 5 * time.Second)
		tick := domain.Tick{
			InstrumentID: "NSE:NIFTY50",
			Provider:     "test",
			EventTime:    at,
			Price:        25000 + float64(i),
			Quantity:     float64(i + 1),
			Sequence:     uint64(i + 1),
			Quality:      domain.QualityGood,
		}
		updates, applyErr := pipe.ApplyTick(tick)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		for _, update := range updates {
			if update.Timeframe == "1m" && update.Final {
				final = update
			}
		}
	}
	// The thirteenth tick belongs to the next minute; only first twelve
	// children contribute to the completed minute.
	if !final.Final || final.Volume != 78 || !final.OpenTime.Equal(start) {
		t.Fatalf("incorrect finalized 1m volume: %+v", final)
	}
	persisted, err := store.LoadDay("NSE:NIFTY50", "1m", start)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || persisted[0].Volume != final.Volume {
		t.Fatalf("1m history diverged from live final: %+v", persisted)
	}
}
