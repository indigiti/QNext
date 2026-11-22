package upstox

import (
	"context"
	"testing"
)

// A rejected subscription must not become an apparently-enabled key in the
// desired snapshot. Otherwise a discovery retry silently skips the missing
// futures feed until an operator toggles the symbol off and on.
func TestFailedSubscriptionQueueLeavesEnabledKeysUnchangedAndRetryable(t *testing.T) {
	s, err := NewSubscriptionState("qnext", ModeLTPC, []string{"NSE_INDEX|Nifty 50"})
	if err != nil {
		t.Fatal(err)
	}
	for len(s.updates) < cap(s.updates)-1 {
		s.updates <- SubscriptionRequest{GUID: "test", Method: MethodSubscribe,
			Data: SubscriptionData{Mode: ModeLTPC, InstrumentKeys: []string{"NSE_INDEX|Nifty 50"}}}
	}
	// Two modes require two slots; one is available, so neither key may
	// be optimistically added, even if one of the messages would fit.
	keys := []string{"NSE_FO|100", "NSE_EQ|ABC"}
	if err := s.Subscribe(context.Background(), keys); err == nil {
		t.Fatal("accepted partial two-mode subscription despite a full writer queue")
	}
	if len(s.updates) != cap(s.updates)-1 {
		t.Fatal("failed batch queued a partial subscribe")
	}
	if snap := s.Snapshot(); len(snap.Data.InstrumentKeys) != 1 {
		t.Fatalf("failed batch poisoned the reconnect desired keys: %+v", snap)
	}
	<-s.Updates()
	if err := s.Subscribe(context.Background(), keys); err != nil {
		t.Fatalf("failed to retry after capacity returned: %v", err)
	}
	snapshot := s.Snapshot()
	if len(snapshot.Data.InstrumentKeys) != 3 || snapshot.Data.Mode != ModeFull {
		t.Fatalf("retry did not enable both keys: %+v", snapshot)
	}
	// Consume the previous diagnostic messages; the final two outbound
	// messages must be distinct modes, and neither can be duplicated.
	for len(s.updates) > 2 {
		<-s.Updates()
	}
	first := <-s.Updates()
	second := <-s.Updates()
	if first.Method != MethodSubscribe || second.Method != MethodSubscribe ||
		first.Data.Mode != ModeLTPC || second.Data.Mode != ModeFull ||
		first.Data.InstrumentKeys[0] != "NSE_EQ|ABC" ||
		second.Data.InstrumentKeys[0] != "NSE_FO|100" {
		t.Fatalf("unexpected retry updates: %+v then %+v", first, second)
	}
}

func TestFailedUnsubscribeQueuePreservesReconnectState(t *testing.T) {
	s, err := NewSubscriptionState("qnext", ModeLTPC, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Subscribe(context.Background(), []string{"NSE_FO|100"}); err != nil {
		t.Fatal(err)
	}
	<-s.Updates()
	for len(s.updates) < cap(s.updates) {
		s.updates <- SubscriptionRequest{GUID: "test", Method: MethodChangeMode,
			Data: SubscriptionData{Mode: ModeFull, InstrumentKeys: []string{"NSE_FO|100"}}}
	}
	if err := s.Unsubscribe(context.Background(), []string{"NSE_FO|100"}); err == nil {
		t.Fatal("unsubscribed desired key when outbound queue was full")
	}
	if snap := s.Snapshot(); len(snap.Data.InstrumentKeys) != 1 {
		t.Fatalf("failed unsubscribe incorrectly changed desired state: %+v", snap)
	}
	<-s.Updates()
	if err := s.Unsubscribe(context.Background(), []string{"NSE_FO|100"}); err != nil {
		t.Fatalf("unsubscribe did not retry once capacity returned: %v", err)
	}
	if snap := s.Snapshot(); len(snap.Data.InstrumentKeys) != 0 {
		t.Fatalf("successful unsubscribe did not update desired keys: %+v", snap)
	}
	if len(s.updates) != cap(s.updates) {
		t.Fatal("successful retry failed to queue unsubscribe event")
	}
}
