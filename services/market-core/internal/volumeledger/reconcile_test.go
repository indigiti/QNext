package volumeledger

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
)

type resyncLog struct { calls []string }
func (r *resyncLog) PublishResync(id, timeframe, reason string) {
	r.calls = append(r.calls, id+"|"+timeframe+"|"+reason)
}

func TestReconcileRepairsExistingFifteenSecondAndMinuteVolume(t *testing.T) {
	root := t.TempDir()
	ledger, store := New(root), history.New(root)
	at := time.Date(2026, 10, 23, 3, 45, 0, 0, time.UTC)
	for _, tf := range []struct{
		name string
		length time.Duration
	}{{"15s", 15*time.Second}, {"1m", time.Minute}} {
		bar := domain.Bar{
			InstrumentID: "NSE:NIFTY50", Timeframe: tf.name,
			OpenTime: at, CloseTime: at.Add(tf.length),
			Open: 25000, High: 25003, Low: 24998, Close: 25002,
			Final: true, Quality: domain.QualityGood,
		}
		if err := store.AppendBar(bar); err != nil { t.Fatal(err) }
	}
	source := domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-10-29", Provider: "upstox",
		EventTime: at.Add(2*time.Second), Quantity: 40,
		CumulativeVolume: 1000,
	}
	proxy := source
	proxy.InstrumentID = "NSE:NIFTY50"
	proxy.Provider = "upstox-futures-proxy"
	if err := ledger.Record(source, proxy); err != nil { t.Fatal(err) }
	notify := &resyncLog{}
	stats, err := ledger.Reconcile(store, notify, "NSE:NIFTY50",
		[]string{"15s","1m","30s"}, at, at.Add(time.Minute))
	if err != nil || stats.Corrected != 2 || stats.Scanned != 2 {
		t.Fatalf("wrong reconciliation result: %+v err=%v", stats, err)
	}
	for _, tf := range []string{"15s", "1m"} {
		bars, err := store.LoadRange("NSE:NIFTY50", tf, at, at.Add(time.Minute))
		if err != nil || len(bars) != 1 { t.Fatalf("load %s: %+v %v", tf,bars,err) }
		if bars[0].Volume != 40 || bars[0].Close != 25002 ||
			bars[0].Revision != 1 || bars[0].VolumeStatus != "OBSERVED" {
			t.Fatalf("wrong %s correction: %+v", tf,bars[0])
		}
	}
	if len(notify.calls) != 2 { t.Fatalf("missing chart resync: %+v", notify.calls) }
	// No 30s price candle existed, so recovering 30s volume must never
	// synthesize a price candle.
	missing, err := store.LoadRange("NSE:NIFTY50","30s",at,at.Add(time.Minute))
	if err != nil || len(missing) != 0 { t.Fatalf("invented 30s candles: %+v %v", missing,err) }
	again, err := ledger.Reconcile(store, notify, "NSE:NIFTY50",
		[]string{"15s","1m"}, at, at.Add(time.Minute))
	if err != nil || again.Corrected != 0 { t.Fatalf("reconcile not idempotent: %+v %v",again,err) }
}
