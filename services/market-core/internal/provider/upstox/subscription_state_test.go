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

func TestReassertFullRepairsOnlyExistingDerivativeKeys(t *testing.T) {
	state, err := NewSubscriptionState("qnext", ModeLTPC, []string{"NSE_INDEX|Nifty 50"})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Subscribe(context.Background(), []string{"NSE_FO|101", "NSE_FO|100"}); err != nil {
		t.Fatal(err)
	}
	<-state.Updates() // original full-mode subscription
	if err := state.ReassertFull(context.Background(), []string{"NSE_FO|101", "NSE_INDEX|Nifty 50", "NSE_FO|100", "NOT_SUBSCRIBED"}); err != nil {
		t.Fatal(err)
	}
	msg := <-state.Updates()
	if msg.Method != MethodChangeMode || msg.Data.Mode != ModeFull ||
		len(msg.Data.InstrumentKeys) != 2 ||
		msg.Data.InstrumentKeys[0] != "NSE_FO|100" || msg.Data.InstrumentKeys[1] != "NSE_FO|101" {
		t.Fatalf("unexpected targeted full-mode repair: %+v", msg)
	}
	if err := state.ReassertFull(context.Background(), []string{"NOT_SUBSCRIBED"}); err == nil {
		t.Fatal("cannot repair an instrument outside desired subscriptions")
	}
	snapshot := state.Snapshot()
	if len(snapshot.Data.InstrumentKeys) != 3 {
		t.Fatalf("repair changed desired subscription set: %+v", snapshot)
	}
}
