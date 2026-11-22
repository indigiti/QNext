package volumeproxy

import (
	"context"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type watchdogRenewSubscriber struct {
	watchdogSubscriber
	renewed [][]string
}

func (s *watchdogRenewSubscriber) RenewFull(_ context.Context, keys []string) error {
	s.renewed = append(s.renewed, append([]string(nil), keys...))
	return nil
}

func TestSilentEnabledFuturesEscalatesOnlyAfterSoftRecovery(t *testing.T) {
	m, _, at := watchdogManager(t)
	recoverer := &watchdogRenewSubscriber{}
	m.Subscriber = recoverer
	m.AutoRenew = true
	_, _ = m.Route(spot(at))

	_, _ = m.Route(spot(at.Add(130 * time.Second)))
	if err := m.RepairSilent(context.Background(), at.Add(131*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(recoverer.repairs) != 1 || len(recoverer.renewed) != 0 {
		t.Fatalf("must try change_mode before renew: repairs=%d renewals=%d", len(recoverer.repairs), len(recoverer.renewed))
	}
	_, _ = m.Route(spot(at.Add(8 * time.Minute)))
	if err := m.RepairSilent(context.Background(), at.Add(8*time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(recoverer.renewed) != 1 || len(recoverer.renewed[0]) != 1 ||
		recoverer.renewed[0][0] != "NSE_FO|100" {
		t.Fatalf("did not renew only the previously enabled futures key: %+v", recoverer.renewed)
	}
	_, _ = m.Route(spot(at.Add(9 * time.Minute)))
	if err := m.RepairSilent(context.Background(), at.Add(9*time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(recoverer.renewed) != 1 {
		t.Fatal("renewal ignored repair cooldown")
	}
	// Any fresh active-futures frame (even if no trade occurred) proves the
	// feed is no longer silent; do not keep cycling the same subscription.
	_, _ = m.Route(future(at.Add(9*time.Minute+2*time.Second), 0))
	_, _ = m.Route(spot(at.Add(14 * time.Minute)))
	if err := m.RepairSilent(context.Background(), at.Add(14*time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(recoverer.renewed) != 1 || len(recoverer.repairs) != 2 {
		t.Fatalf("fresh futures frame failed to reset escalation: repairs=%d renewed=%d", len(recoverer.repairs), len(recoverer.renewed))
	}
}

func TestVolumeOnlyMissingDoesNotTriggerDestructiveRenewal(t *testing.T) {
	m, _, at := watchdogManager(t)
	recoverer := &watchdogRenewSubscriber{}
	m.Subscriber = recoverer
	m.AutoRenew = true
	_, _ = m.Route(spot(at))
	for _, minutes := range []int{5, 10, 16} {
		when := at.Add(time.Duration(minutes) * time.Minute)
		_, _ = m.Route(future(when, 0)) // live futures feed, but no traded delta
		_, _ = m.Route(spot(when))
		if err := m.RepairSilent(context.Background(), when.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if len(recoverer.renewed) != 0 || len(recoverer.repairs) < 2 {
		t.Fatalf("quiet but live future was unnecessarily cycled: repairs=%d renewed=%d",
			len(recoverer.repairs), len(recoverer.renewed))
	}
}

func TestHardRenewalLimitedToTwoAttemptsPerMarketSession(t *testing.T) {
	m, _, at := watchdogManager(t)
	recoverer := &watchdogRenewSubscriber{}
	m.Subscriber = recoverer
	m.AutoRenew = true
	_, _ = m.Route(spot(at))
	for _, minutes := range []int{3, 9, 15, 25, 31, 38} {
		when := at.Add(time.Duration(minutes) * time.Minute)
		_, _ = m.Route(spot(when))
		if err := m.RepairSilent(context.Background(), when.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if len(recoverer.renewed) != 2 {
		t.Fatalf("renewal not capped at two per session: %+v", recoverer.renewed)
	}
	m.mu.Lock()
	state := *m.targets["NSE:NIFTY50"]
	m.mu.Unlock()
	if state.renewAttempts != 2 || state.lastRecoveryAction != "change_mode" {
		t.Fatalf("unexpected recovery state after reaching cap: %+v", state)
	}
}

func TestHardRenewalDefaultsOffForHealthyOrUnconfiguredFeeds(t *testing.T) {
	m, _, at := watchdogManager(t)
	recoverer := &watchdogRenewSubscriber{}
	m.Subscriber = recoverer
	_, _ = m.Route(spot(at))
	for _, minutes := range []int{3, 9, 15} {
		when := at.Add(time.Duration(minutes) * time.Minute)
		_, _ = m.Route(spot(when))
		if err := m.RepairSilent(context.Background(), when.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if len(recoverer.renewed) != 0 {
		t.Fatal("hard renewal ran while staging flag was off")
	}
	if err := m.RepairSilent(context.Background(), at.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(recoverer.renewed) != 0 {
		t.Fatal("hard renewal ran outside market session")
	}
}

func TestRenewalDoesNotToggleHealthyNextContract(t *testing.T) {
	at := time.Date(2026, 10, 21, 9, 15, 0, 0, indiaLocation).UTC()
	subscriber := &watchdogRenewSubscriber{}
	m, err := New(futureSourceStub{contracts: []upstox.FutureContract{
		{Exchange: "NSE", Expiry: "2026-10-29", InstrumentKey: "NSE_FO|100",
			TradingSymbol: "NIFTY OCT FUT", InstrumentType: "FUT", UnderlyingSymbol: "NIFTY"},
		{Exchange: "NSE", Expiry: "2026-11-26", InstrumentKey: "NSE_FO|101",
			TradingSymbol: "NIFTY NOV FUT", InstrumentType: "FUT", UnderlyingSymbol: "NIFTY"},
	}}, "token", symbol.NewRegistry(), subscriber, []Market{{
		Symbol: "NIFTY", Exchange: "NSE", TargetInstrumentID: "NSE:NIFTY50",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	m.AutoRenew = true
	_, _ = m.Route(spot(at))
	_, _ = m.Route(domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-10-29", EventTime: at.Add(time.Second),
		Price: 25100, CumulativeVolume: 1000, OpenInterest: 1000,
	})
	_, _ = m.Route(spot(at.Add(3 * time.Minute)))
	if err := m.RepairSilent(context.Background(), at.Add(3*time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}

	_, _ = m.Route(domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-11-26", EventTime: at.Add(8 * time.Minute),
		Price: 25200, CumulativeVolume: 900, OpenInterest: 1000,
	})
	_, _ = m.Route(spot(at.Add(9 * time.Minute)))
	if err := m.RepairSilent(context.Background(), at.Add(9*time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(subscriber.renewed) != 1 || len(subscriber.renewed[0]) != 1 ||
		subscriber.renewed[0][0] != "NSE_FO|100" {
		t.Fatalf("healthy next contract was cycled: %+v", subscriber.renewed)
	}
	if len(subscriber.removed) != 0 {
		t.Fatalf("manager altered desired futures contract set: %+v", subscriber.removed)
	}
}
