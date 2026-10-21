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

type futureSourceStub struct {
	contracts []upstox.FutureContract
}

func (s futureSourceStub) Futures(
	context.Context,
	string,
	string,
	string,
) ([]upstox.FutureContract, error) {
	return append([]upstox.FutureContract(nil), s.contracts...), nil
}

type subscriberStub struct {
	added   []string
	removed []string
}

func (s *subscriberStub) Subscribe(_ context.Context, keys []string) error {
	s.added = append(s.added, keys...)
	return nil
}

func (s *subscriberStub) Unsubscribe(_ context.Context, keys []string) error {
	s.removed = append(s.removed, keys...)
	return nil
}

func TestManagerRoutesCurrentFutureAndRollsOnLiquidity(t *testing.T) {
	registry := symbol.NewRegistry()
	subscriber := &subscriberStub{}
	manager, err := New(
		futureSourceStub{contracts: []upstox.FutureContract{
			{
				Exchange:         "NSE",
				Expiry:           "2026-10-29",
				InstrumentKey:    "NSE_FO|100",
				TradingSymbol:    "NIFTY FUT 29 OCT 26",
				InstrumentType:   "FUT",
				UnderlyingSymbol: "NIFTY",
			},
			{
				Exchange:         "NSE",
				Expiry:           "2026-11-26",
				InstrumentKey:    "NSE_FO|101",
				TradingSymbol:    "NIFTY FUT 26 NOV 26",
				InstrumentType:   "FUT",
				UnderlyingSymbol: "NIFTY",
			},
		}},
		"token",
		registry,
		subscriber,
		[]Market{{
			Symbol:             "NIFTY",
			Exchange:           "NSE",
			CalendarID:         "NSE_EQ",
			TargetInstrumentID: "NSE:NIFTY50",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 10, 5, 9, 16, 0, 0, indiaLocation).UTC()
	if err := manager.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	if len(subscriber.added) != 2 ||
		subscriber.added[0] != "NSE_FO|100" ||
		subscriber.added[1] != "NSE_FO|101" {
		t.Fatalf("unexpected subscriptions: %+v", subscriber.added)
	}

	currentID := "NSE:FUT:NIFTY:2026-10-29"
	nextID := "NSE:FUT:NIFTY:2026-11-26"

	currentTick := domain.Tick{
		InstrumentID:     currentID,
		Provider:         upstox.ProviderName,
		Quantity:         25,
		CumulativeVolume: 1000,
		OpenInterest:     1000,
		EventTime:        at,
		Sequence:         1,
		Quality:          domain.QualityGood,
	}
	volumeTick, ok := manager.Route(currentTick)
	if !ok {
		t.Fatal("current future should route before rollover")
	}
	if volumeTick.InstrumentID != "NSE:NIFTY50" ||
		volumeTick.Provider != ProxyProvider ||
		volumeTick.Quantity != 25 ||
		volumeTick.Price != 0 {
		t.Fatalf("unexpected proxy tick: %+v", volumeTick)
	}

	nextTick := domain.Tick{
		InstrumentID:     nextID,
		Provider:         upstox.ProviderName,
		Quantity:         30,
		CumulativeVolume: 1100,
		OpenInterest:     900,
		EventTime:        at.Add(time.Second),
		Sequence:         2,
		Quality:          domain.QualityGood,
	}
	volumeTick, ok = manager.Route(nextTick)
	if !ok {
		t.Fatal("next future should route once liquidity rollover is confirmed")
	}
	if volumeTick.InstrumentID != "NSE:NIFTY50" || volumeTick.Quantity != 30 {
		t.Fatalf("unexpected rolled proxy tick: %+v", volumeTick)
	}

	currentTick.Quantity = 40
	currentTick.CumulativeVolume = 1200
	currentTick.EventTime = at.Add(2 * time.Second)
	if _, ok := manager.Route(currentTick); ok {
		t.Fatal("manager must not switch back to current future during the same contract set")
	}
}

func TestManagerDoesNotRollWithoutOISupport(t *testing.T) {
	registry := symbol.NewRegistry()
	subscriber := &subscriberStub{}
	manager, err := New(
		futureSourceStub{contracts: []upstox.FutureContract{
			{
				Exchange:         "NSE",
				Expiry:           "2026-10-29",
				InstrumentKey:    "NSE_FO|100",
				TradingSymbol:    "NIFTY FUT 29 OCT 26",
				InstrumentType:   "FUT",
				UnderlyingSymbol: "NIFTY",
			},
			{
				Exchange:         "NSE",
				Expiry:           "2026-11-26",
				InstrumentKey:    "NSE_FO|101",
				TradingSymbol:    "NIFTY FUT 26 NOV 26",
				InstrumentType:   "FUT",
				UnderlyingSymbol: "NIFTY",
			},
		}},
		"token",
		registry,
		subscriber,
		[]Market{{
			Symbol:             "NIFTY",
			Exchange:           "NSE",
			CalendarID:         "NSE_EQ",
			TargetInstrumentID: "NSE:NIFTY50",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 10, 5, 9, 16, 0, 0, indiaLocation).UTC()
	if err := manager.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}

	_, _ = manager.Route(domain.Tick{
		InstrumentID:     "NSE:FUT:NIFTY:2026-10-29",
		Quantity:         20,
		CumulativeVolume: 1000,
		OpenInterest:     1000,
		EventTime:        at,
	})
	if _, ok := manager.Route(domain.Tick{
		InstrumentID:     "NSE:FUT:NIFTY:2026-11-26",
		Quantity:         30,
		CumulativeVolume: 1200,
		OpenInterest:     500,
		EventTime:        at.Add(time.Second),
	}); ok {
		t.Fatal("low next-month OI must not trigger rollover")
	}
}

type intermittentFutureSource struct {
	fail      bool
	contracts []upstox.FutureContract
}

func (s *intermittentFutureSource) Futures(_ context.Context, _, _, _ string) ([]upstox.FutureContract, error) {
	if s.fail {
		return nil, errors.New("temporary Upstox discovery failure")
	}
	return append([]upstox.FutureContract(nil), s.contracts...), nil
}

func newRecoveryManager(t *testing.T, source upstox.FutureContractSource, subscriber *subscriberStub) *Manager {
	t.Helper()
	manager, err := New(source, "token", symbol.NewRegistry(), subscriber, []Market{{
		Symbol:             "NIFTY",
		Exchange:           "NSE",
		CalendarID:         "NSE_EQ",
		TargetInstrumentID: "NSE:NIFTY50",
	}})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestRefreshRetriesSameDayAfterFailedStartup(t *testing.T) {
	at := time.Date(2026, 10, 21, 9, 20, 0, 0, indiaLocation).UTC()
	source := &intermittentFutureSource{
		fail: true,
		contracts: []upstox.FutureContract{{
			Exchange:         "NSE",
			Expiry:           "2026-10-29",
			InstrumentKey:    "NSE_FO|100",
			TradingSymbol:    "NIFTY FUT",
			InstrumentType:   "FUT",
			UnderlyingSymbol: "NIFTY",
		}},
	}
	sub := &subscriberStub{}
	manager := newRecoveryManager(t, source, sub)
	if err := manager.Refresh(context.Background(), at); err == nil {
		t.Fatal("expected discovery failure")
	}
	if !manager.refreshDue(at.Add(time.Minute)) {
		t.Fatal("failed discovery must retry on same day")
	}
	source.fail = false
	if err := manager.Refresh(context.Background(), at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if manager.refreshDue(at.Add(2 * time.Minute)) {
		t.Fatal("successful discovery should not repeat on same day")
	}
	if len(sub.added) != 1 || sub.added[0] != "NSE_FO|100" {
		t.Fatalf("unexpected subscriptions: %+v", sub.added)
	}
}

func TestRefreshPreservesUnexpiredVolumeRouteDuringTemporaryFailure(t *testing.T) {
	at := time.Date(2026, 10, 21, 9, 20, 0, 0, indiaLocation).UTC()
	source := &intermittentFutureSource{
		contracts: []upstox.FutureContract{{
			Exchange:         "NSE",
			Expiry:           "2026-10-29",
			InstrumentKey:    "NSE_FO|100",
			TradingSymbol:    "NIFTY FUT",
			InstrumentType:   "FUT",
			UnderlyingSymbol: "NIFTY",
		}},
	}
	sub := &subscriberStub{}
	manager := newRecoveryManager(t, source, sub)
	if err := manager.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	source.fail = true
	if err := manager.Refresh(context.Background(), at.Add(time.Minute)); err == nil {
		t.Fatal("expected temporary failure")
	}
	if len(sub.removed) != 0 {
		t.Fatalf("temporary failure unsubscribed valid futures: %+v", sub.removed)
	}
	if !manager.refreshDue(at.Add(2 * time.Minute)) {
		t.Fatal("failure should remain retryable")
	}
	if got, ok := manager.Route(domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-10-29",
		EventTime:    at.Add(2 * time.Minute),
		Quantity:     42,
	}); !ok || got.Quantity != 42 || got.InstrumentID != "NSE:NIFTY50" {
		t.Fatalf("valid proxy route lost: %+v, %v", got, ok)
	}
	if err := manager.Refresh(context.Background(), at.Add(3*time.Minute)); err == nil {
		t.Fatal("expected temporary failure")
	}
	if len(sub.added) != 1 {
		t.Fatalf("duplicate subscription detected: %+v", sub.added)
	}
}

func TestRefreshDoesNotRetainExpiredProxy(t *testing.T) {
	at := time.Date(2026, 10, 21, 9, 20, 0, 0, indiaLocation).UTC()
	source := &intermittentFutureSource{
		contracts: []upstox.FutureContract{{
			Exchange:         "NSE",
			Expiry:           "2026-10-21",
			InstrumentKey:    "NSE_FO|100",
			TradingSymbol:    "NIFTY FUT",
			InstrumentType:   "FUT",
			UnderlyingSymbol: "NIFTY",
		}},
	}
	sub := &subscriberStub{}
	manager := newRecoveryManager(t, source, sub)
	if err := manager.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	source.fail = true
	if err := manager.Refresh(context.Background(), at.Add(24*time.Hour)); err == nil {
		t.Fatal("expected failure")
	}
	if len(sub.removed) != 1 {
		t.Fatalf("expired future was not unsubscribed: %+v", sub.removed)
	}
	if _, ok := manager.Route(domain.Tick{
		InstrumentID: "NSE:FUT:NIFTY:2026-10-21",
		Quantity:     20,
		EventTime:    at.Add(24 * time.Hour),
	}); ok {
		t.Fatal("expired volume route must not be used")
	}
}
