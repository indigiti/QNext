package upstox

import (
	"context"
	"testing"
)

func TestSubscriptionStateTracksDesiredKeys(t *testing.T) {
	state, err := NewSubscriptionState("qnext", ModeLTPC, []string{"NSE_INDEX|Nifty 50"})
	if err != nil {
		t.Fatal(err)
	}

	if err := state.Subscribe(context.Background(), []string{"NSE_FO|CE", "NSE_FO|PE"}); err != nil {
		t.Fatal(err)
	}
	update := <-state.Updates()
	if update.Method != MethodSubscribe || len(update.Data.InstrumentKeys) != 2 {
		t.Fatalf("unexpected subscribe update: %+v", update)
	}

	if err := state.Unsubscribe(context.Background(), []string{"NSE_FO|CE"}); err != nil {
		t.Fatal(err)
	}
	update = <-state.Updates()
	if update.Method != MethodUnsubscribe || len(update.Data.InstrumentKeys) != 1 {
		t.Fatalf("unexpected unsubscribe update: %+v", update)
	}

	snapshot := state.Snapshot()
	if len(snapshot.Data.InstrumentKeys) != 2 {
		t.Fatalf("unexpected desired snapshot: %+v", snapshot.Data.InstrumentKeys)
	}
}
