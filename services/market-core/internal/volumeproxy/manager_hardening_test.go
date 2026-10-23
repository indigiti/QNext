package volumeproxy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type pausedDiscovery struct {
	contracts []upstox.FutureContract
	calls     int
	entered   chan struct{}
	release   chan struct{}
}

func (s *pausedDiscovery) Futures(context.Context, string, string, string) ([]upstox.FutureContract, error) {
	s.calls++
	if s.calls == 2 {
		close(s.entered)
		<-s.release
	}
	return append([]upstox.FutureContract(nil), s.contracts...), nil
}

func TestRefreshDoesNotEraseTicksArrivingDuringContractDiscovery(t *testing.T) {
	at := time.Date(2026, 10, 23, 9, 20, 0, 0, indiaLocation).UTC()
	source := &pausedDiscovery{
		contracts: []upstox.FutureContract{{
			Exchange: "NSE", Expiry: "2026-10-29",
			InstrumentKey: "NSE_FO|100", TradingSymbol: "NIFTY FUT",
			InstrumentType: "FUT", UnderlyingSymbol: "NIFTY",
		}},
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	subscriber := &subscriberStub{}
	manager, err := New(source, "token", symbol.NewRegistry(), subscriber, []Market{{
		Symbol: "NIFTY", Exchange: "NSE", CalendarID: "NSE_EQ",
		TargetInstrumentID: "NSE:NIFTY50",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	_, _ = manager.Route(spot(at))
	_, _ = manager.Route(future(at, 10))

	done := make(chan error, 1)
	go func() { done <- manager.Refresh(context.Background(), at.Add(time.Minute)) }()
	<-source.entered

	// These ticks arrive after Refresh captured its snapshot, while remote
	// contract discovery is still waiting. The final Refresh commit must not
	// roll back their proxy-volume and underlying-health timestamps.
	liveAt := at.Add(65 * time.Second)
	_, _ = manager.Route(spot(liveAt))
	_, _ = manager.Route(future(liveAt, 38))
	close(source.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	state := *manager.targets["NSE:NIFTY50"]
	manager.mu.Unlock()
	if !state.lastUnderlyingAt.Equal(liveAt) || !state.lastVolumeAt.Equal(liveAt) ||
		!state.lastActiveFeedAt.Equal(liveAt) {
		t.Fatalf("contract refresh erased live proxy state: %+v", state)
	}
	if len(subscriber.added) != 1 || len(subscriber.removed) != 0 {
		t.Fatalf("refresh duplicated subscriptions: %+v", subscriber)
	}
}

func TestRolloverResetsPriorContractVolumeHealth(t *testing.T) {
	at := time.Date(2026, 10, 23, 9, 20, 0, 0, indiaLocation).UTC()
	manager, err := New(futureSourceStub{contracts: []upstox.FutureContract{
		{Exchange: "NSE", Expiry: "2026-10-29", InstrumentKey: "NSE_FO|100",
			TradingSymbol: "NIFTY OCT FUT", InstrumentType: "FUT", UnderlyingSymbol: "NIFTY"},
		{Exchange: "NSE", Expiry: "2026-11-26", InstrumentKey: "NSE_FO|101",
			TradingSymbol: "NIFTY NOV FUT", InstrumentType: "FUT", UnderlyingSymbol: "NIFTY"},
	}}, "token", symbol.NewRegistry(), &subscriberStub{}, []Market{{
		Symbol: "NIFTY", Exchange: "NSE", TargetInstrumentID: "NSE:NIFTY50",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	_, _ = manager.Route(spot(at))
	_, _ = manager.Route(domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-10-29", EventTime: at,
		Quantity: 40, CumulativeVolume: 1000, OpenInterest: 1000,
	})
	rolledAt := at.Add(time.Second)
	_, _ = manager.Route(domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-11-26", EventTime: rolledAt,
		Quantity: 0, CumulativeVolume: 1200, OpenInterest: 1000,
	})
	manager.mu.Lock()
	state := *manager.targets["NSE:NIFTY50"]
	manager.mu.Unlock()
	if state.activeID != "NSE:FUT:NIFTY:2026-11-26" ||
		!state.lastVolumeAt.IsZero() || !state.lastActiveFeedAt.Equal(rolledAt) {
		t.Fatalf("rollover kept stale previous-contract volume: %+v", state)
	}
	nextAt := rolledAt.Add(time.Second)
	if value, ok := manager.Route(domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-11-26", EventTime: nextAt,
		Quantity: 21, CumulativeVolume: 1221, OpenInterest: 1000,
	}); !ok || value.Quantity != 21 {
		t.Fatalf("rollover lost next-contract volume: %+v, routed=%v", value, ok)
	}
	manager.mu.Lock()
	lastVolume := manager.targets["NSE:NIFTY50"].lastVolumeAt
	manager.mu.Unlock()
	if !lastVolume.Equal(nextAt) {
		t.Fatalf("next-contract volume health not updated: %s", lastVolume)
	}
}

type failFirstRepair struct {
	subscriberStub
	attempts int
}

func (s *failFirstRepair) ReassertFull(context.Context, []string) error {
	s.attempts++
	if s.attempts == 1 {
		return errors.New("temporary upstream writer failure")
	}
	return nil
}

func TestFailedRepairRetriesSoonerWithoutFloodingSubscriptions(t *testing.T) {
	m, _, at := watchdogManager(t)
	repairer := &failFirstRepair{}
	m.Subscriber = repairer
	_, _ = m.Route(spot(at))
	_, _ = m.Route(spot(at.Add(130 * time.Second)))
	if err := m.RepairSilent(context.Background(), at.Add(131*time.Second)); err == nil {
		t.Fatal("expected first repair failure")
	}
	_, _ = m.Route(spot(at.Add(170 * time.Second)))
	if err := m.RepairSilent(context.Background(), at.Add(171*time.Second)); err != nil {
		t.Fatal(err)
	}
	if repairer.attempts != 1 {
		t.Fatal("failed repair bypassed one-minute backoff")
	}
	_, _ = m.Route(spot(at.Add(192 * time.Second)))
	if err := m.RepairSilent(context.Background(), at.Add(193*time.Second)); err != nil {
		t.Fatal(err)
	}
	if repairer.attempts != 2 {
		t.Fatalf("failed repair not retried after one minute: %d", repairer.attempts)
	}
	_, _ = m.Route(spot(at.Add(250 * time.Second)))
	if err := m.RepairSilent(context.Background(), at.Add(251*time.Second)); err != nil {
		t.Fatal(err)
	}
	if repairer.attempts != 2 {
		t.Fatal("successful repair did not restore standard cooldown")
	}
	m.mu.Lock()
	failed := m.targets["NSE:NIFTY50"].lastRepairFailed
	attempts := m.targets["NSE:NIFTY50"].repairAttempts
	m.mu.Unlock()
	if failed || attempts != 2 {
		t.Fatalf("incorrect repair health after recovery: failed=%v attempts=%d", failed, attempts)
	}
}
