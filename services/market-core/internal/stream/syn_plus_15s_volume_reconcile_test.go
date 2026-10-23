package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/research"
)

// Regression: SYN+ receives independent price snapshots and futures-proxy
// volume updates. A zero-volume opening candle must acquire real source volume
// without needing a new synthetic price tick or a manual chart refresh.
func TestSynPlusFifteenSecondVolumeLateArrivalAndRollover(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:NIFTY-SYN+")
	broker := NewBroker(64, 16)
	handler := NewWebSocketHandler(broker).(*WebSocketHandler)
	at := time.Date(2026, 10, 23, 3, 45, 0, 0, time.UTC)
	source := domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "15s",
		OpenTime:     at,
		CloseTime:    at.Add(15 * time.Second),
		Volume:       0,
		Quality:      domain.QualityGood,
	}
	broker.PublishBar(source)

	observe := func(when time.Time, price float64) {
		t.Helper()
		err := handler.shadow.observe(research.SynPlusSnapshot{
			Schema:       research.SynPlusSnapshotSchema,
			InstrumentID: "QNEXT:NIFTY-SYN+",
			Version:      "test-syn-plus",
			SnapshotAtMS: when.UnixMilli(),
			Quality:      "GOOD",
			Value:        price,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	latest := func(wantOpen time.Time, wantVolume float64) {
		t.Helper()
		bar, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", "15s")
		if !ok || !bar.OpenTime.Equal(wantOpen) || bar.Volume != wantVolume {
			t.Fatalf("SYN+ 15s volume mismatch: %+v ok=%v, want open=%s volume=%v", bar, ok, wantOpen, wantVolume)
		}
	}
	observe(at.Add(time.Second), 25010)
	latest(at, 0)
	listener, err := broker.Subscribe("QNEXT:NIFTY-SYN+", "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Cancel()

	// Volume reaches the canonical NIFTY futures proxy after the SYN+ candle
	// already opened. The bridge must immediately publish an update for 15s.
	source.Volume = 45
	source.SourceSequence = 1
	broker.PublishBar(source)
	select {
	case update := <-listener.Events:
		if !update.Bar.OpenTime.Equal(at) || update.Bar.Volume != 45 ||
			update.Bar.Open != 25010 || update.Bar.Close != 25010 {
			t.Fatalf("15s live update changed price or lost source volume: %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("missing 15s live volume update")
	}
	latest(at, 45)

	// Synthetic price moves to the next bucket before source volume for the
	// previous bucket finalizes.
	nextAt := at.Add(15 * time.Second)
	observe(nextAt.Add(time.Second), 25012)
	latest(nextAt, 0)
	source.Volume = 120
	source.Final = true
	source.Revision = 1
	source.SourceSequence = 2
	broker.PublishBar(source)

	// A late correction must reach 15s subscribers but must not replace the
	// chronological latest bar or alter SYN+ OHLC.
	var corrected bool
	for i := 0; i < 3; i++ {
		select {
		case update := <-listener.Events:
			if update.Bar.OpenTime.Equal(at) && update.Bar.Volume == 120 {
				if !update.Bar.Final || !update.Bar.Corrected || update.Bar.Open != 25010 {
					t.Fatalf("invalid 15s finalized correction: %+v", update)
				}
				corrected = true
			}
		case <-time.After(time.Second):
			t.Fatal("missing 15s finalized volume correction")
		}
		if corrected {
			break
		}
	}
	if !corrected {
		t.Fatal("prior 15s bucket was not corrected")
	}
	latest(nextAt, 0)

	// The next source bucket must also be mirrored without another price tick.
	next := source
	next.OpenTime = nextAt
	next.CloseTime = nextAt.Add(15 * time.Second)
	next.Volume = 23
	next.Final = false
	next.Revision = 0
	next.SourceSequence = 3
	broker.PublishBar(next)
	latest(nextAt, 23)

	// Out-of-order same-revision volume cannot erase the valid proxy.
	stale := next
	stale.Volume = 0
	broker.PublishBar(stale)
	latest(nextAt, 23)
}
