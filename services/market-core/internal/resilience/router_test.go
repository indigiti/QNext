package resilience

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/authority"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func testRouter(t *testing.T, gap time.Duration, recovery GapRecoverer) (*Router, *Metrics, *atomic.Uint64) {
	t.Helper()
	reg := symbol.NewRegistry()
	_ = reg.Register(symbol.Instrument{ID: "NSE:NIFTY50", Symbol: "NIFTY", Name: "Nifty 50", AssetClass: "INDEX", Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata"})
	_ = reg.RegisterProvider(symbol.ProviderInstrument{Provider: "upstox", InstrumentID: "NSE:NIFTY50", ProviderKey: "UP"})
	_ = reg.RegisterProvider(symbol.ProviderInstrument{Provider: "dhan", InstrumentID: "NSE:NIFTY50", ProviderKey: "IDX_I|13|INDEX"})
	resolver, err := authority.NewResolver(reg, map[string][]authority.Preference{"NSE:NIFTY50": {{Provider: "upstox", Priority: 10, MaxStaleness: time.Second}, {Provider: "dhan", Priority: 20, MaxStaleness: 2 * time.Second}}})
	if err != nil {
		t.Fatal(err)
	}
	metrics := NewMetrics()
	var accepted atomic.Uint64
	r, err := NewRouter(resolver, func(domain.Tick) error { accepted.Add(1); return nil }, metrics, gap, map[string]bool{"NSE:NIFTY50": true}, recovery)
	if err != nil {
		t.Fatal(err)
	}
	return r, metrics, &accepted
}
func tick(provider string, at time.Time) domain.Tick {
	return domain.Tick{InstrumentID: "NSE:NIFTY50", Provider: provider, Price: 25000, EventTime: at, ReceivedTime: at, ProcessedTime: at, Quality: domain.QualityGood}
}
func TestRouterFailoverGapRecovery(t *testing.T) {
	base := time.Date(2026, 9, 25, 9, 15, 0, 0, time.UTC)
	var calls atomic.Uint64
	r, m, a := testRouter(t, 2*time.Second, RecoveryFunc(func(_ context.Context, g Gap) error {
		calls.Add(1)
		if g.Provider != "dhan" || g.From != base || g.To != base.Add(3*time.Second) {
			t.Fatalf("unexpected gap: %+v", g)
		}
		return nil
	}))
	if err := r.Handle(tick("upstox", base)); err != nil {
		t.Fatal(err)
	}
	if err := r.Handle(tick("dhan", base.Add(500*time.Millisecond))); err != nil {
		t.Fatal(err)
	}
	if err := r.Handle(tick("dhan", base.Add(3*time.Second))); err != nil {
		t.Fatal(err)
	}
	s := m.Snapshot()
	if a.Load() != 2 {
		t.Fatalf("accepted=%d", a.Load())
	}
	if calls.Load() != 1 || s.AuthoritySwitches != 1 || s.GapDetections != 1 || s.RecoverySuccesses != 1 {
		t.Fatalf("snapshot=%+v calls=%d", s, calls.Load())
	}
	if s.ActiveAuthorities["NSE:NIFTY50"] != "dhan" {
		t.Fatalf("authority=%s", s.ActiveAuthorities["NSE:NIFTY50"])
	}
}
func TestRouterRecoveryFailureFailsClosed(t *testing.T) {
	base := time.Date(2026, 9, 25, 9, 15, 0, 0, time.UTC)
	r, m, a := testRouter(t, time.Second, RecoveryFunc(func(context.Context, Gap) error { return errors.New("recovery failed") }))
	_ = r.Handle(tick("upstox", base))
	if err := r.Handle(tick("dhan", base.Add(3*time.Second))); err == nil {
		t.Fatal("expected recovery failure")
	}
	s := m.Snapshot()
	if a.Load() != 1 || s.RecoveryFailures != 1 {
		t.Fatalf("accepted=%d snapshot=%+v", a.Load(), s)
	}
}
func TestRouterLoadHotStandbyNoLoss(t *testing.T) {
	base := time.Date(2026, 9, 25, 9, 15, 0, 0, time.UTC)
	r, m, a := testRouter(t, 2*time.Second, nil)
	const cycles = 20000
	for i := 0; i < cycles; i++ {
		at := base.Add(time.Duration(i) * 10 * time.Millisecond)
		if err := r.Handle(tick("upstox", at)); err != nil {
			t.Fatal(err)
		}
		if err := r.Handle(tick("dhan", at.Add(time.Millisecond))); err != nil {
			t.Fatal(err)
		}
	}
	s := m.Snapshot()
	if got := a.Load(); got != cycles {
		t.Fatalf("accepted=%d want=%d", got, cycles)
	}
	if s.Provider["upstox"].Accepted != cycles || s.Provider["dhan"].DroppedNonAuthority != cycles {
		t.Fatalf("snapshot=%+v", s)
	}
}

func TestRouterAuthorityStateMachineHysteresis(t *testing.T) {
	base := time.Date(2026, 9, 25, 9, 15, 0, 0, time.UTC)

	reg := symbol.NewRegistry()
	_ = reg.Register(symbol.Instrument{ID: "NSE:NIFTY50", Symbol: "NIFTY", Name: "Nifty 50", AssetClass: "INDEX", Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata"})
	_ = reg.RegisterProvider(symbol.ProviderInstrument{Provider: "upstox", InstrumentID: "NSE:NIFTY50", ProviderKey: "UP"})
	_ = reg.RegisterProvider(symbol.ProviderInstrument{Provider: "dhan", InstrumentID: "NSE:NIFTY50", ProviderKey: "IDX_I|13|INDEX"})
	resolver, err := authority.NewResolver(reg, map[string][]authority.Preference{
		"NSE:NIFTY50": {
			{Provider: "upstox", Priority: 10, MaxStaleness: time.Second},
			{Provider: "dhan", Priority: 20, MaxStaleness: 2 * time.Second},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	metrics := NewMetrics()
	var accepted atomic.Uint64
	router, err := NewRouterWithPolicy(
		resolver,
		func(domain.Tick) error { accepted.Add(1); return nil },
		metrics,
		2*time.Second,
		map[string]bool{"NSE:NIFTY50": false},
		nil,
		TransitionPolicy{
			PrimaryProvider: "upstox",
			FailoverPending: time.Second,
			FailbackPending: 2 * time.Second,
			PolicyVersion:   "test-policy-v1",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	must := func(provider string, offset time.Duration) {
		if err := router.Handle(tick(provider, base.Add(offset))); err != nil {
			t.Fatal(err)
		}
	}

	must("upstox", 0)
	must("dhan", 3*time.Second)
	if got := metrics.Snapshot().AuthorityStates["NSE:NIFTY50"]; got != string(StateFailoverPending) {
		t.Fatalf("state=%s", got)
	}

	// Primary recovers before the failover dwell completes, cancelling the candidate.
	must("upstox", 3500*time.Millisecond)
	if got := metrics.Snapshot().ActiveAuthorities["NSE:NIFTY50"]; got != "upstox" {
		t.Fatalf("authority=%s", got)
	}

	must("dhan", 6*time.Second)
	must("dhan", 7100*time.Millisecond)
	if got := metrics.Snapshot().ActiveAuthorities["NSE:NIFTY50"]; got != "dhan" {
		t.Fatalf("authority=%s", got)
	}

	// During failback dwell, secondary remains accepted so there is no avoidable outage.
	must("upstox", 7200*time.Millisecond)
	must("dhan", 8*time.Second)
	must("upstox", 8900*time.Millisecond)
	must("dhan", 9*time.Second)
	must("upstox", 9300*time.Millisecond)

	snapshot := metrics.Snapshot()
	if snapshot.ActiveAuthorities["NSE:NIFTY50"] != "upstox" {
		t.Fatalf("authority=%s", snapshot.ActiveAuthorities["NSE:NIFTY50"])
	}
	if snapshot.AuthorityStates["NSE:NIFTY50"] != string(StatePrimaryHealthy) {
		t.Fatalf("state=%s", snapshot.AuthorityStates["NSE:NIFTY50"])
	}
	if snapshot.AuthoritySwitches != 2 {
		t.Fatalf("switches=%d", snapshot.AuthoritySwitches)
	}
	if len(snapshot.RecentSwitches) != 2 ||
		snapshot.RecentSwitches[0].Reason != "PRIMARY_STALE" ||
		snapshot.RecentSwitches[1].Reason != "PRIMARY_RECOVERED" ||
		snapshot.RecentSwitches[0].PolicyVersion != "test-policy-v1" {
		t.Fatalf("events=%+v", snapshot.RecentSwitches)
	}
	if accepted.Load() != 6 {
		t.Fatalf("accepted=%d", accepted.Load())
	}
}
