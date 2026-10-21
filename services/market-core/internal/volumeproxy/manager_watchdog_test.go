package volumeproxy

import (
	"context"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type watchdogSubscriber struct {
	subscriberStub
	repairs [][]string
}

func (s *watchdogSubscriber) ReassertFull(_ context.Context, keys []string) error {
	s.repairs = append(s.repairs, append([]string(nil), keys...))
	return nil
}

func watchdogManager(t *testing.T) (*Manager, *watchdogSubscriber, time.Time) {
	t.Helper()
	at := time.Date(2026, 10, 21, 9, 15, 0, 0, indiaLocation).UTC()
	subscriber := &watchdogSubscriber{}
	manager, err := New(futureSourceStub{contracts: []upstox.FutureContract{{
		Exchange:         "NSE",
		Expiry:           "2026-10-29",
		InstrumentKey:    "NSE_FO|100",
		TradingSymbol:    "NIFTY FUT",
		InstrumentType:   "FUT",
		UnderlyingSymbol: "NIFTY",
	}}}, "token", symbol.NewRegistry(), subscriber, []Market{{
		Symbol:             "NIFTY",
		Exchange:           "NSE",
		CalendarID:         "NSE_EQ",
		TargetInstrumentID: "NSE:NIFTY50",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	return manager, subscriber, at
}

func spot(at time.Time) domain.Tick {
	return domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Provider:     upstox.ProviderName,
		Price:        25100,
		EventTime:    at,
	}
}

func future(at time.Time, quantity float64) domain.Tick {
	return domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-10-29",
		Provider:     upstox.ProviderName,
		Price:        25200,
		Quantity:     quantity,
		EventTime:    at,
	}
}

func TestVolumeWatchdogReassertsSilentFuturesWithFreshIndex(t *testing.T) {
	m, sub, at := watchdogManager(t)
	_, _ = m.Route(spot(at))
	if err := m.RepairSilent(context.Background(), at.Add(90 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(sub.repairs) != 0 {
		t.Fatal("watchdog triggered before missing-stream threshold")
	}
	_, _ = m.Route(spot(at.Add(130 * time.Second)))
	if err := m.RepairSilent(context.Background(), at.Add(131 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(sub.repairs) != 1 || len(sub.repairs[0]) != 1 || sub.repairs[0][0] != "NSE_FO|100" {
		t.Fatalf("watchdog failed to repair only existing futures key: %+v", sub.repairs)
	}
	_, _ = m.Route(spot(at.Add(160 * time.Second)))
	if err := m.RepairSilent(context.Background(), at.Add(161 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(sub.repairs) != 1 {
		t.Fatalf("watchdog ignored cooldown: %+v", sub.repairs)
	}
	if len(sub.added) != 1 {
		t.Fatalf("watchdog created new upstream subscriptions: %+v", sub.added)
	}
}

func TestVolumeWatchdogDoesNotTreatMissingIndexOrNormalVolumeAsFailure(t *testing.T) {
	m, sub, at := watchdogManager(t)
	if err := m.RepairSilent(context.Background(), at.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Route(spot(at))
	_, _ = m.Route(future(at.Add(time.Second), 10))
	_, _ = m.Route(spot(at.Add(90 * time.Second)))
	_, _ = m.Route(future(at.Add(90 * time.Second), 0))
	if err := m.RepairSilent(context.Background(), at.Add(91 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(sub.repairs) != 0 {
		t.Fatalf("healthy futures should not be repaired: %+v", sub.repairs)
	}
	// Stale index price should not cause a repair during feed-wide outages.
	if err := m.RepairSilent(context.Background(), at.Add(7 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(sub.repairs) != 0 {
		t.Fatal("watchdog reacted to a stale index feed")
	}
}

func TestVolumeWatchdogReassertsFullModeWhenFuturesTicksCarryNoVolume(t *testing.T) {
	m, sub, at := watchdogManager(t)
	_, _ = m.Route(spot(at))
	for minute := 1; minute <= 5; minute++ {
		instant := at.Add(time.Duration(minute) * time.Minute)
		_, _ = m.Route(spot(instant))
		_, _ = m.Route(future(instant, 0))
	}
	if err := m.RepairSilent(context.Background(), at.Add(5 * time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(sub.repairs) != 1 {
		t.Fatalf("source present but quantity missing was not repaired: %+v", sub.repairs)
	}
	_, _ = m.Route(future(at.Add(5 * time.Minute+2 * time.Second), 50))
	_, _ = m.Route(spot(at.Add(6 * time.Minute)))
	if err := m.RepairSilent(context.Background(), at.Add(6 * time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(sub.repairs) != 1 {
		t.Fatal("watchdog reasserted healthy futures volume")
	}
	snapshot, ok := m.Status().(map[string]any)
	if !ok || snapshot["NIFTY"] == nil {
		t.Fatalf("no operator-facing volume diagnostic: %+v", snapshot)
	}
}

func TestVolumeWatchdogSkipsMarketClosed(t *testing.T) {
	m, sub, at := watchdogManager(t)
	_, _ = m.Route(spot(at))
	_, _ = m.Route(spot(at.Add(10 * time.Minute)))
	if err := m.RepairSilent(context.Background(), at.Add(24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(sub.repairs) != 0 {
		t.Fatal("stale prior-day price triggered futures repair")
	}
	if inRegularSession(time.Date(2026, 10, 24, 10, 0, 0, 0, indiaLocation)) {
		t.Fatal("weekend must be excluded")
	}
}
