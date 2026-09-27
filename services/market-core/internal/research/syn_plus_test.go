package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
)

func TestSynPlusFairPricePreference(t *testing.T) {
	micro, source, ok := synPlusFairPrice(upstox.MarketState{
		LTPC:  &upstox.LTPC{LTP: 99},
		Depth: []upstox.Quote{{BidP: 100, AskP: 102, BidQ: 3, AskQ: 1}},
	})
	if !ok || source != "MICROPRICE" || micro != 101.5 {
		t.Fatalf("microprice = %v %q %v, want 101.5 MICROPRICE true", micro, source, ok)
	}

	mid, source, ok := synPlusFairPrice(upstox.MarketState{
		LTPC:  &upstox.LTPC{LTP: 99},
		Depth: []upstox.Quote{{BidP: 100, AskP: 102}},
	})
	if !ok || source != "MID" || mid != 101 {
		t.Fatalf("mid = %v %q %v, want 101 MID true", mid, source, ok)
	}

	ltp, source, ok := synPlusFairPrice(upstox.MarketState{LTPC: &upstox.LTPC{LTP: 99}})
	if !ok || source != "LTP" || ltp != 99 {
		t.Fatalf("ltp = %v %q %v, want 99 LTP true", ltp, source, ok)
	}
}

func TestSynPlusEvaluateUsesCurrentExpiryMedian(t *testing.T) {
	at := time.Date(2026, 9, 28, 4, 0, 1, 0, time.UTC)
	collector := &SynPlusCollector{
		cfg: SynPlusConfig{
			InstrumentID:           "QNEXT:NIFTY-SYN+",
			Version:                "nifty-syn-plus-v1",
			MinimumValidCandidates: 3,
			MaxLegAge:              2 * time.Second,
			MaxLegTimeSkew:         time.Second,
		},
		spot: 25014,
		legs: make(map[string]synPlusLeg),
	}
	plan := upstox.ResearchPlan{
		UnderlyingKey: "NSE_INDEX|Nifty 50",
		Spot:          25000,
		ATM:           25000,
		CurrentExpiry: "2026-10-01",
		NextExpiry:    "2026-10-08",
		Contracts:     make(map[string]upstox.OptionContract),
	}

	addPair := func(prefix string, strike, callPrice, putPrice float64, expiry string) {
		callKey := prefix + "-CE"
		putKey := prefix + "-PE"
		plan.Contracts[callKey] = upstox.OptionContract{InstrumentKey: callKey, Expiry: expiry, StrikePrice: strike, InstrumentType: "CE"}
		plan.Contracts[putKey] = upstox.OptionContract{InstrumentKey: putKey, Expiry: expiry, StrikePrice: strike, InstrumentType: "PE"}
		collector.legs[callKey] = synPlusLeg{state: microState(callPrice), updatedAt: at.Add(-100 * time.Millisecond)}
		collector.legs[putKey] = synPlusLeg{state: microState(putPrice), updatedAt: at.Add(-100 * time.Millisecond)}
	}

	addPair("a", 25000, 110, 95, plan.CurrentExpiry)
	addPair("b", 25050, 85, 120, plan.CurrentExpiry)
	addPair("c", 25100, 60, 145, plan.CurrentExpiry)
	addPair("next", 25000, 500, 1, plan.NextExpiry)

	snapshot := collector.evaluate(plan, at, at.UnixMilli())
	if snapshot.Quality != "GOOD" {
		t.Fatalf("quality = %q, want GOOD", snapshot.Quality)
	}
	if snapshot.Value != 25015 {
		t.Fatalf("value = %v, want 25015", snapshot.Value)
	}
	if snapshot.Spot != 25014 || snapshot.BasisToSpot != 1 {
		t.Fatalf("spot/basis = %v/%v, want 25014/1", snapshot.Spot, snapshot.BasisToSpot)
	}
	if snapshot.ValidCandidates != 3 {
		t.Fatalf("valid candidates = %d, want 3", snapshot.ValidCandidates)
	}
	if snapshot.MicropriceLegs != 6 {
		t.Fatalf("microprice legs = %d, want 6", snapshot.MicropriceLegs)
	}
}

func TestSynPlusObservePersistsShadowSnapshot(t *testing.T) {
	root := t.TempDir()
	collector, err := NewSynPlusCollector(root, SynPlusConfig{
		InstrumentID:           "QNEXT:NIFTY-SYN+",
		Version:                "nifty-syn-plus-v1",
		MinimumValidCandidates: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	plan := upstox.ResearchPlan{
		UnderlyingKey: "NSE_INDEX|Nifty 50",
		Spot:          25000,
		ATM:           25000,
		CurrentExpiry: "2026-10-01",
		Contracts: map[string]upstox.OptionContract{
			"CE": {InstrumentKey: "CE", Expiry: "2026-10-01", StrikePrice: 25000, InstrumentType: "CE"},
			"PE": {InstrumentKey: "PE", Expiry: "2026-10-01", StrikePrice: 25000, InstrumentType: "PE"},
		},
	}
	first := time.Date(2026, 9, 28, 4, 0, 0, 200_000_000, time.UTC)
	if err := collector.Observe(envelope(first, 25010, 110, 100), plan); err != nil {
		t.Fatal(err)
	}
	second := first.Add(time.Second)
	if err := collector.Observe(envelope(second, 25012, 111, 99), plan); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "research", "synthetic", "1s", "2026", "09", "2026-09-28.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot SynPlusSnapshot
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.InstrumentID != "QNEXT:NIFTY-SYN+" || snapshot.Value != 25010 {
		t.Fatalf("snapshot = %+v, want NIFTY-SYN+ value 25010", snapshot)
	}
	if snapshot.Spot != 25010 {
		t.Fatalf("snapshot spot = %v, want live spot 25010", snapshot.Spot)
	}
}

func microState(price float64) upstox.MarketState {
	return upstox.MarketState{
		LTPC: &upstox.LTPC{LTP: price},
		Depth: []upstox.Quote{{
			BidP: price - 1,
			AskP: price + 1,
			BidQ: 1,
			AskQ: 1,
		}},
	}
}

func envelope(at time.Time, spot, call, put float64) upstox.DecodedEnvelope {
	return upstox.DecodedEnvelope{
		Type:      "live_feed",
		CurrentTS: strconvFormatInt(at.UnixMilli()),
		Feeds: map[string]upstox.Feed{
			"NSE_INDEX|Nifty 50": {LTPC: &upstox.LTPC{LTP: spot}},
			"CE": {FirstLevelWithGreeks: statePtr(microState(call))},
			"PE": {FirstLevelWithGreeks: statePtr(microState(put))},
		},
	}
}

func statePtr(state upstox.MarketState) *upstox.MarketState {
	return &state
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
