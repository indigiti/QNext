package upstox

import (
	"context"
	"testing"
)

func TestSubscriptionStateUsesFullModeForDynamicDerivativeKeys(t *testing.T) {
	state, err := NewSubscriptionState("qnext-test", ModeLTPC, []string{"NSE_INDEX|Nifty 50"})
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Snapshot().Data.Mode; got != ModeLTPC {
		t.Fatalf("expected LTPC base snapshot, got %q", got)
	}

	optionKey := "NSE_FO|NIFTY22600CE"
	if err := state.Subscribe(context.Background(), []string{optionKey}); err != nil {
		t.Fatal(err)
	}
	update := <-state.Updates()
	if update.Method != MethodSubscribe || update.Data.Mode != ModeFull {
		t.Fatalf("expected full-mode derivative subscription, got %+v", update)
	}
	if len(update.Data.InstrumentKeys) != 1 || update.Data.InstrumentKeys[0] != optionKey {
		t.Fatalf("unexpected derivative update keys: %+v", update.Data.InstrumentKeys)
	}

	// A reconnect uses one Upstox subscription request. While rich derivative
	// demand exists, promote the small active set to Full so option quote
	// quality survives reconnect instead of silently degrading to LTP.
	if got := state.Snapshot().Data.Mode; got != ModeFull {
		t.Fatalf("expected reconnect snapshot to preserve rich mode, got %q", got)
	}

	if err := state.Unsubscribe(context.Background(), []string{optionKey}); err != nil {
		t.Fatal(err)
	}
	<-state.Updates()
	if got := state.Snapshot().Data.Mode; got != ModeLTPC {
		t.Fatalf("expected LTPC snapshot after derivative demand release, got %q", got)
	}
}

func TestDerivativeProviderKeyDetection(t *testing.T) {
	cases := map[string]bool{
		"NSE_FO|123":            true,
		"BSE_FO|456":            true,
		"NSE_INDEX|Nifty 50":    false,
		"NSE_EQ|INE000000000":   false,
		"  nse_fo|NIFTY22600PE": true,
	}
	for key, want := range cases {
		if got := derivativeProviderKey(key); got != want {
			t.Fatalf("derivativeProviderKey(%q)=%v want %v", key, got, want)
		}
	}
}
