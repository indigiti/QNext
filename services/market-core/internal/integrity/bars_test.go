package integrity

import (
	"errors"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func finalBar(at time.Time) domain.Bar {
	return domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "1m",
		OpenTime:     at,
		CloseTime:    at.Add(time.Minute),
		Final:        true,
	}
}

func TestCheckBarContinuity(t *testing.T) {
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	if err := CheckBarContinuity([]domain.Bar{
		finalBar(base),
		finalBar(base.Add(time.Minute)),
		finalBar(base.Add(2 * time.Minute)),
	}, "1m"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckBarContinuityRejectsGap(t *testing.T) {
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	if err := CheckBarContinuity([]domain.Bar{
		finalBar(base),
		finalBar(base.Add(2 * time.Minute)),
	}, "1m"); err == nil {
		t.Fatal("expected gap failure")
	}
}

func TestHistoryLiveBoundary(t *testing.T) {
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	history := finalBar(base)
	live := finalBar(base.Add(time.Minute))
	live.Final = false
	if err := CheckHistoryLiveBoundary(history, live); err != nil {
		t.Fatal(err)
	}

	live.OpenTime = base.Add(2 * time.Minute)
	if err := CheckHistoryLiveBoundary(history, live); !errors.Is(err, ErrHistoryLiveDiscontinuity) {
		t.Fatalf("expected discontinuity, got %v", err)
	}
}
