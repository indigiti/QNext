package research

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
)

type recordingStateConsumer struct {
	updates []ResearchStateUpdate
}

func (c *recordingStateConsumer) ObserveState(update ResearchStateUpdate) error {
	c.updates = append(c.updates, update)
	return nil
}

func TestResearchStateHubNormalizesOnceAndRetainsSpot(t *testing.T) {
	consumer := &recordingStateConsumer{}
	hub := NewResearchStateHub(consumer)
	plan := upstox.ResearchPlan{
		UnderlyingKey: "NSE_INDEX|Nifty 50",
		CurrentExpiry: "2026-10-01",
		Contracts: map[string]upstox.OptionContract{
			"CE": {InstrumentKey: "CE", Expiry: "2026-10-01", StrikePrice: 25000, InstrumentType: "CE"},
		},
	}
	first := time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC)
	if err := hub.Observe(upstox.DecodedEnvelope{
		Type:      "live_feed",
		CurrentTS: "1790568000000",
		Feeds: map[string]upstox.Feed{
			"NSE_INDEX|Nifty 50": {LTPC: &upstox.LTPC{LTP: 25010}},
			"CE": {FirstLevelWithGreeks: &upstox.MarketState{
				LTPC: &upstox.LTPC{LTP: 110},
			}},
		},
	}, plan); err != nil {
		t.Fatal(err)
	}
	if len(consumer.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(consumer.updates))
	}
	update := consumer.updates[0]
	if !update.SpotUpdated || update.Spot != 25010 {
		t.Fatalf("spot = %v updated=%v, want 25010 true", update.Spot, update.SpotUpdated)
	}
	if entry, ok := update.Updates["CE"]; !ok || entry.State.LTPC == nil || entry.State.LTPC.LTP != 110 {
		t.Fatalf("CE update = %+v, want LTP 110", entry)
	}

	spot, state := hub.Snapshot()
	if spot != 25010 || len(state) != 1 {
		t.Fatalf("snapshot spot/state = %v/%d, want 25010/1", spot, len(state))
	}
	if state["CE"].UpdatedAt.IsZero() {
		t.Fatal("cached CE update time is zero")
	}
	_ = first
}

func TestJSONLStoreReusesOpenFileAndPersists(t *testing.T) {
	root := t.TempDir()
	store, err := NewJSONLStore(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("research", "options", "1s", "2026", "09", "2026-09-28.jsonl")
	if err := store.Append(path, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(path, []byte("two\n")); err != nil {
		t.Fatal(err)
	}
	if len(store.files) != 1 {
		t.Fatalf("open files = %d, want 1", len(store.files))
	}
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\ntwo\n" {
		t.Fatalf("content = %q, want two durable appends", string(data))
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(path, []byte("three\n")); err == nil {
		t.Fatal("append after close succeeded")
	}
}
