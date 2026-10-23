package httpapi

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestMissingIndexSourceNeverFallsBackToOptionBasketVolume(t *testing.T) {
	at := time.Date(2026, 10, 23, 3, 45, 0, 0, time.UTC)
	bars := []domain.Bar{{
		InstrumentID: "QNEXT:NIFTY-SYN", Timeframe: "15s",
		OpenTime: at, CloseTime: at.Add(15 * time.Second),
		Volume: 500, Open: 25000, Close: 25001,
	}}
	server := &Server{}
	server.overlayVolume(bars, "NSE:NIFTY50", "15s",
		at.Add(-time.Second).UnixMilli(), at.Add(time.Minute).UnixMilli())
	if bars[0].Volume != 0 || bars[0].VolumeStatus != "UNKNOWN" ||
		bars[0].Open != 25000 || bars[0].Close != 25001 {
		t.Fatalf("unverified option-basket volume leaked into SYN: %+v", bars[0])
	}
}
