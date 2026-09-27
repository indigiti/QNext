package research

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
)

func TestOptionsCollectorPersistsOneAndFifteenSecondSnapshots(t *testing.T) {
	root := t.TempDir()
	collector, err := NewOptionsCollector(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "NSE_FO|NIFTY_TEST_CE"
	plan := upstox.ResearchPlan{
		UnderlyingKey: "NSE_INDEX|Nifty 50",
		Contracts: map[string]upstox.OptionContract{
			key: {
				InstrumentKey:    key,
				TradingSymbol:    "NIFTY TEST CE",
				InstrumentType:   "CE",
				UnderlyingSymbol: "NIFTY",
				Expiry:           "2026-10-01",
				StrikePrice:      24000,
			},
		},
	}
	base := time.Date(2026, 9, 28, 3, 45, 0, 100*int(time.Millisecond), time.UTC)
	if err := collector.Observe(researchEnvelope(base, plan.UnderlyingKey, key, 24010, 123.5), plan); err != nil {
		t.Fatal(err)
	}
	if err := collector.Observe(researchEnvelope(base.Add(time.Second), plan.UnderlyingKey, key, 24012, 124.0), plan); err != nil {
		t.Fatal(err)
	}
	if err := collector.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "research", "options", "1s", "2026", "09", "2026-09-28.jsonl")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("missing 1s snapshot")
	}
	var snapshot OptionSnapshot
	if err := json.Unmarshal(scanner.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Schema != OptionSnapshotSchema || snapshot.Interval != "1s" || snapshot.ProviderKey != key {
		t.Fatalf("unexpected snapshot identity: %+v", snapshot)
	}
	if snapshot.UnderlyingPrice != 24010 || snapshot.IV != 0.18 || snapshot.Delta != 0.52 || snapshot.OI != 4500 {
		t.Fatalf("unexpected research fields: %+v", snapshot)
	}
	if snapshot.Bid != 123.4 || snapshot.Ask != 123.6 || snapshot.DepthImbalance == 0 {
		t.Fatalf("unexpected microstructure fields: %+v", snapshot)
	}
	wantMicro := (123.6*100 + 123.4*50) / 150
	if math.Abs(snapshot.MicroPrice-wantMicro) > 1e-9 {
		t.Fatalf("microprice = %v, want %v", snapshot.MicroPrice, wantMicro)
	}

	fifteen := filepath.Join(root, "research", "options", "15s", "2026", "09", "2026-09-28.jsonl")
	if _, err := os.Stat(fifteen); err != nil {
		t.Fatalf("missing 15s snapshot: %v", err)
	}
}

func researchEnvelope(at time.Time, underlyingKey, optionKey string, spot, optionLTP float64) upstox.DecodedEnvelope {
	ms := at.UnixMilli()
	return upstox.DecodedEnvelope{
		Type:      "live_feed",
		CurrentTS: formatMillis(ms),
		Feeds: map[string]upstox.Feed{
			underlyingKey: {LTPC: &upstox.LTPC{LTP: spot, LTT: formatMillis(ms - 5)}},
			optionKey: {
				FullFeed: &upstox.MarketState{
					LTPC: &upstox.LTPC{LTP: optionLTP, LTT: formatMillis(ms - 10), LTQ: "25", CP: 120},
					Depth: []upstox.Quote{
						{BidQ: 100, BidP: 123.4, AskQ: 50, AskP: 123.6},
						{BidQ: 80, BidP: 123.3, AskQ: 60, AskP: 123.7},
					},
					OptionGreeks: &upstox.OptionGreeks{Delta: 0.52, Gamma: 0.001, Theta: -4, Vega: 11, Rho: 0.04},
					VTT:          12000,
					OI:           4500,
					IV:           0.18,
					ATP:          122.9,
				},
			},
		},
	}
}

func formatMillis(value int64) string {
	return strconv.FormatInt(value, 10)
}
