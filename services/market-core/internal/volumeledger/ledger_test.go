package volumeledger

import (
	"math"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestLedgerPersistsIndependentIndexSourcesAndReplaysExactSubminute(t *testing.T) {
	root := t.TempDir()
	ledger := New(root)
	at := time.Date(2026, 10, 23, 3, 45, 0, 0, time.UTC)
	add := func(sourceID, targetID string, offset time.Duration, qty, cumulative float64) {
		t.Helper()
		source := domain.Tick{
			InstrumentID: sourceID, Provider: "upstox",
			EventTime: at.Add(offset), Quantity: qty,
			CumulativeVolume: cumulative, Sequence: uint64(offset / time.Second),
		}
		routed := source
		routed.InstrumentID = targetID
		routed.Provider = "upstox-futures-proxy"
		if err := ledger.Record(source, routed); err != nil {
			t.Fatal(err)
		}
	}
	add("NSE:FUT:NIFTY:2026-10-29", "NSE:NIFTY50", 2*time.Second, 10, 110)
	add("NSE:FUT:NIFTY:2026-10-29", "NSE:NIFTY50", 16*time.Second, 20, 130)
	add("NSE:FUT:NIFTY:2026-10-29", "NSE:NIFTY50", 31*time.Second, 30, 160)
	add("NSE:FUT:BANKNIFTY:2026-10-29", "NSE:BANKNIFTY", 2*time.Second, 500, 900)

	// Exact observed trade placement is retained, not divided equally across
	// four 15-second buckets after receiving an aggregate 1-minute candle.
	reloaded := New(root)
	for _, tt := range []struct {
		open time.Duration
		want float64
		ok   bool
	}{
		{0, 10, true}, {15 * time.Second, 20, true},
		{30 * time.Second, 30, true}, {45 * time.Second, 0, false},
	} {
		got, ok, err := reloaded.Sum("NSE:NIFTY50", at.Add(tt.open), at.Add(tt.open+15*time.Second))
		if err != nil || got != tt.want || ok != tt.ok {
			t.Fatalf("wrong subminute replay %+v: got=%v found=%v err=%v", tt, got, ok, err)
		}
	}
	nifty, _, err := reloaded.Sum("NSE:NIFTY50", at, at.Add(time.Minute))
	if err != nil || nifty != 60 {
		t.Fatalf("wrong NIFTY minute replay: %v %v", nifty, err)
	}
	bank, _, err := reloaded.Sum("NSE:BANKNIFTY", at, at.Add(time.Minute))
	if err != nil || bank != 500 {
		t.Fatalf("cross-market volume leaked: %v %v", bank, err)
	}
}

func TestLedgerRejectsFabricatedOrInvalidVolume(t *testing.T) {
	l := New(t.TempDir())
	now := time.Date(2026, 10, 23, 3, 45, 0, 0, time.UTC)
	source := domain.Tick{InstrumentID: "NSE:FUT:NIFTY:2026-10-29", EventTime: now}
	proxy := domain.Tick{
		InstrumentID: "NSE:NIFTY50", EventTime: now,
		Provider: "upstox-futures-proxy", Quantity: math.NaN(),
	}
	if err := l.Record(source, proxy); err == nil {
		t.Fatal("accepted NaN volume")
	}
	proxy.Quantity = 12
	proxy.Provider = "upstox"
	if err := l.Record(source, proxy); err == nil {
		t.Fatal("accepted non-futures-proxy volume")
	}
	if _, present, err := l.Sum(proxy.InstrumentID, now, now.Add(time.Minute)); err != nil || present {
		t.Fatalf("a rejected write created a volume record: %v %v", present, err)
	}
}
