package candle

import (
	"errors"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestDerivativeSyntheticCandlesContinueUntil1540WithoutExtendingCashIndex(t *testing.T) {
	for _, test := range []struct {
		instrument string
		at string
		accepted bool
		wantOpen string
		wantEnd string
	}{
		{"NSE:NIFTY50", "2026-10-28T15:30:00+05:30", false, "", ""},
		{"QNEXT:NIFTY-SYN", "2026-10-28T15:30:00+05:30", true, "2026-10-28T15:30:00+05:30", "2026-10-28T15:30:15+05:30"},
		{"QNEXT:NIFTY-SYN+", "2026-10-28T15:39:59+05:30", true, "2026-10-28T15:39:45+05:30", "2026-10-28T15:40:00+05:30"},
		{"QNEXT:BANKNIFTY-SYN", "2026-10-28T15:39:59+05:30", true, "2026-10-28T15:39:45+05:30", "2026-10-28T15:40:00+05:30"},
		{"QNEXT:SENSEX-SYN+", "2026-10-28T15:40:00+05:30", false, "", ""},
	} {
		at, _ := time.Parse(time.RFC3339, test.at)
		open, closeAt, err := BucketForInstrument(test.instrument, at, "15s")
		if !test.accepted {
			if !errors.Is(err, ErrOutsideSession) {
				t.Fatalf("%s %s: expected closed session, got %v", test.instrument, test.at, err)
			}
			continue
		}
		wantOpen, _ := time.Parse(time.RFC3339, test.wantOpen)
		wantEnd, _ := time.Parse(time.RFC3339, test.wantEnd)
		if err != nil || !open.Equal(wantOpen) || !closeAt.Equal(wantEnd) {
			t.Fatalf("%s: got %v..%v err=%v", test.instrument, open, closeAt, err)
		}
	}
}

func TestDerivativeCandleClockFinalizesAt1540AndNot1530(t *testing.T) {
	engine := New("derivatives-1540")
	at, _ := time.Parse(time.RFC3339, "2026-10-28T15:29:55+05:30")
	tick := domain.Tick{InstrumentID: "QNEXT:NIFTY-SYN+", Provider: "test", Price: 25000, EventTime: at, Quality: domain.QualityGood}
	if _, err := engine.Apply(tick, "5s"); err != nil { t.Fatal(err) }
	cashClose, _ := time.Parse(time.RFC3339, "2026-10-28T15:30:00+05:30")
	bars, err := engine.Advance(tick.InstrumentID, "5s", cashClose, true)
	if err != nil || len(bars) != 2 || !bars[0].Final || bars[1].Final ||
		!bars[1].OpenTime.Equal(cashClose) {
		t.Fatalf("synthetic candle stopped at cash close: %+v %v", bars, err)
	}
	tick.EventTime = cashClose.Add(2*time.Second)
	tick.Price = 25005
	if _, err := engine.Apply(tick, "5s"); err != nil { t.Fatal(err) }
	tick.EventTime = cashClose.Add(9*time.Minute+55*time.Second)
	if _, err := engine.Apply(tick, "5s"); err != nil { t.Fatal(err) }
	foClose, _ := time.Parse(time.RFC3339, "2026-10-28T15:40:00+05:30")
	bars, err = engine.Advance(tick.InstrumentID, "5s", foClose, true)
	if err != nil || len(bars) != 1 || !bars[0].Final || !bars[0].CloseTime.Equal(foClose) {
		t.Fatalf("last F&O candle not finalized at 15:40: %+v %v", bars, err)
	}
	repeated, err := engine.Advance(tick.InstrumentID, "5s", foClose.Add(time.Minute), true)
	if err != nil || len(repeated) != 0 {
		t.Fatalf("derivative generated phantom afterclose candles: %+v %v", repeated, err)
	}
}
