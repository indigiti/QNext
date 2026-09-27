package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
)

func TestSynPlusFairPricePreference(t *testing.T) {
	micro, source, ok := synPlusFairPrice(marketState(100, 102, 3, 1, 99))
	if !ok || source != "MICROPRICE" || micro != 101.5 {
		t.Fatalf("microprice = %v %q %v", micro, source, ok)
	}

	mid, source, ok := synPlusFairPrice(marketState(100, 102, 0, 0, 99))
	if !ok || source != "MID" || mid != 101 {
		t.Fatalf("mid = %v %q %v", mid, source, ok)
	}

	ltp, source, ok := synPlusFairPrice(upstox.MarketState{LTPC: &upstox.LTPC{LTP: 99}})
	if !ok || source != "LTP" || ltp != 99 {
		t.Fatalf("ltp = %v %q %v", ltp, source, ok)
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
	plan := testPlan()

	addPair(collector, &plan, at, "a", 25000, 110, 95, plan.CurrentExpiry)
	addPair(collector, &plan, at, "b", 25050, 85, 120, plan.CurrentExpiry)
	addPair(collector, &plan, at, "c", 25100, 60, 145, plan.CurrentExpiry)
	addPair(collector, &plan, at, "next", 25000, 500, 1, plan.NextExpiry)

	snapshot := collector.evaluate(plan, at, at.UnixMilli())
	if snapshot.Quality != "GOOD" || snapshot.Value != 25015 {
		t.Fatalf("snapshot quality/value = %s/%v", snapshot.Quality, snapshot.Value)
	}
	if snapshot.Spot != 25014 || snapshot.BasisToSpot != 1 {
		t.Fatalf("spot/basis = %v/%v", snapshot.Spot, snapshot.BasisToSpot)
	}
	if snapshot.ValidCandidates != 3 || snapshot.MicropriceLegs != 6 {
		t.Fatalf("valid/micro legs = %d/%d", snapshot.ValidCandidates, snapshot.MicropriceLegs)
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

	plan := testPlan()
	plan.NextExpiry = ""
	plan.Contracts = make(map[string]upstox.OptionContract)
	now := time.Date(2026, 9, 28, 4, 0, 0, 200_000_000, time.UTC)
	addContractPair(&plan, "p", 25000, plan.CurrentExpiry)

	if err := collector.Observe(testEnvelope(now, 25010, 110, 100), plan); err != nil {
		t.Fatal(err)
	}
	if err := collector.Observe(testEnvelope(now.Add(time.Second), 25012, 111, 99), plan); err != nil {
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
	if snapshot.InstrumentID != "QNEXT:NIFTY-SYN+" || snapshot.Value != 25010 || snapshot.Spot != 25010 {
		t.Fatalf("unexpected persisted snapshot: %+v", snapshot)
	}
}

func testPlan() upstox.ResearchPlan {
	return upstox.ResearchPlan{
		UnderlyingKey: "NSE_INDEX|Nifty 50",
		Spot:          25000,
		ATM:           25000,
		CurrentExpiry: "2026-10-01",
		NextExpiry:    "2026-10-08",
		Contracts:     make(map[string]upstox.OptionContract),
	}
}

func addPair(collector *SynPlusCollector, plan *upstox.ResearchPlan, at time.Time, prefix string, strike, callPrice, putPrice float64, expiry string) {
	callKey, putKey := addContractPair(plan, prefix, strike, expiry)
	collector.legs[callKey] = synPlusLeg{state: microState(callPrice), updatedAt: at.Add(-100 * time.Millisecond)}
	collector.legs[putKey] = synPlusLeg{state: microState(putPrice), updatedAt: at.Add(-100 * time.Millisecond)}
}

func addContractPair(plan *upstox.ResearchPlan, prefix string, strike float64, expiry string) (string, string) {
	callKey := prefix + "-CE"
	putKey := prefix + "-PE"
	plan.Contracts[callKey] = upstox.OptionContract{InstrumentKey: callKey, Expiry: expiry, StrikePrice: strike, InstrumentType: "CE"}
	plan.Contracts[putKey] = upstox.OptionContract{InstrumentKey: putKey, Expiry: expiry, StrikePrice: strike, InstrumentType: "PE"}
	return callKey, putKey
}

func microState(price float64) upstox.MarketState {
	return marketState(price-1, price+1, 1, 1, price)
}

func marketState(bid, ask float64, bidQty, askQty int64, ltp float64) upstox.MarketState {
	return upstox.MarketState{
		LTPC:  &upstox.LTPC{LTP: ltp},
		Depth: []upstox.Quote{{BidP: bid, AskP: ask, BidQ: bidQty, AskQ: askQty}},
	}
}

func testEnvelope(at time.Time, spot, call, put float64) upstox.DecodedEnvelope {
	return upstox.DecodedEnvelope{
		Type:      "live_feed",
		CurrentTS: strconv.FormatInt(at.UnixMilli(), 10),
		Feeds: map[string]upstox.Feed{
			"NSE_INDEX|Nifty 50": {LTPC: &upstox.LTPC{LTP: spot}},
			"p-CE":               {FirstLevelWithGreeks: statePtr(microState(call))},
			"p-PE":               {FirstLevelWithGreeks: statePtr(microState(put))},
		},
	}
}

func statePtr(state upstox.MarketState) *upstox.MarketState {
	return &state
}
