package upstox

import (
	"context"
	"testing"
)

func TestRenewFullCyclesOnlyExistingEnabledFuturesKeys(t *testing.T) {
	s, err := NewSubscriptionState("qnext", ModeLTPC, []string{"NSE_INDEX|Nifty 50"})
	if err != nil { t.Fatal(err) }
	if err := s.Subscribe(context.Background(), []string{"NSE_FO|101", "NSE_FO|100"}); err != nil {
		t.Fatal(err)
	}
	<-s.Updates() // original subscription
	before := s.Snapshot()
	if err := s.RenewFull(context.Background(), []string{"NSE_FO|101", "NSE_FO|100"}); err != nil {
		t.Fatal(err)
	}
	unsubscribe := <-s.Updates()
	subscribe := <-s.Updates()
	if unsubscribe.Method != MethodUnsubscribe || subscribe.Method != MethodSubscribe ||
		subscribe.Data.Mode != ModeFull || len(unsubscribe.Data.InstrumentKeys) != 2 ||
		unsubscribe.Data.InstrumentKeys[0] != "NSE_FO|100" ||
		unsubscribe.Data.InstrumentKeys[1] != "NSE_FO|101" ||
		subscribe.Data.InstrumentKeys[0] != "NSE_FO|100" ||
		subscribe.Data.InstrumentKeys[1] != "NSE_FO|101" {
		t.Fatalf("incorrect unsubscribe/subscribe sequence: %+v then %+v", unsubscribe, subscribe)
	}
	after := s.Snapshot()
	if len(before.Data.InstrumentKeys) != len(after.Data.InstrumentKeys) {
		t.Fatalf("renewal changed desired enabled keys: before=%+v after=%+v", before, after)
	}
	for i := range before.Data.InstrumentKeys {
		if before.Data.InstrumentKeys[i] != after.Data.InstrumentKeys[i] {
			t.Fatalf("reconnect snapshot changed: before=%+v after=%+v", before, after)
		}
	}
	for _, invalid := range []string{"NSE_INDEX|Nifty 50", "NSE_FO|NOT_ENABLED"} {
		if err := s.RenewFull(context.Background(), []string{"NSE_FO|100", invalid}); err == nil {
			t.Fatalf("renewal allowed an inactive/non-futures key: %s", invalid)
		}
	}
	if len(s.Updates()) != 0 {
		t.Fatal("invalid renewal wrote a partial request")
	}
}

func TestRenewFullReservesBothQueueSlotsOrFailsClosed(t *testing.T) {
	s, err := NewSubscriptionState("qnext", ModeLTPC, nil)
	if err != nil { t.Fatal(err) }
	if err := s.Subscribe(context.Background(), []string{"NSE_FO|100"}); err != nil { t.Fatal(err) }
	<-s.Updates()
	for len(s.updates) < cap(s.updates)-1 {
		s.updates <- SubscriptionRequest{GUID: "test", Method: MethodChangeMode, Data: SubscriptionData{
			Mode: ModeFull, InstrumentKeys: []string{"NSE_FO|100"},
		}}
	}
	length := len(s.updates)
	if err := s.RenewFull(context.Background(), []string{"NSE_FO|100"}); err == nil {
		t.Fatal("renewal did not reject near-full writer queue")
	}
	if len(s.updates) != length {
		t.Fatal("failure queued a lone unsubscribe without its resubscribe")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.RenewFull(ctx, []string{"NSE_FO|100"}); err == nil {
		t.Fatal("cancelled renewal was accepted")
	}
	if len(s.updates) != length { t.Fatal("cancelled renewal modified queue") }
}
