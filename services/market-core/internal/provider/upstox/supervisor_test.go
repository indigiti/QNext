package upstox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type fakeStreamRunner struct {
	calls  int
	cancel context.CancelFunc
	event  time.Time
}

func (r *fakeStreamRunner) Run(
	_ context.Context,
	_ string,
	_ SubscriptionRequest,
	onTick func(domain.Tick) error,
) error {
	r.calls++
	if r.calls == 1 {
		if err := onTick(domain.Tick{
			InstrumentID: "NSE:NIFTY50",
			Provider:     ProviderName,
			Price:        25100,
			EventTime:    r.event,
			Quality:      domain.QualityGood,
		}); err != nil {
			return err
		}
		return errors.New("socket disconnected")
	}

	r.cancel()
	return context.Canceled
}

type fakeRecovery struct {
	requests []RecoveryRequest
	err      error
}

func (r *fakeRecovery) Recover(_ context.Context, request RecoveryRequest) error {
	r.requests = append(r.requests, request)
	return r.err
}

func TestSupervisorRecoversGapBeforeReconnect(t *testing.T) {
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	clock := base

	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakeStreamRunner{cancel: cancel, event: base}
	recovery := &fakeRecovery{}

	supervisor := &Supervisor{
		Runner:     runner,
		Recovery:   recovery,
		MinBackoff: time.Second,
		MaxBackoff: 4 * time.Second,
		Now: func() time.Time {
			return clock
		},
		Sleep: func(_ context.Context, delay time.Duration) error {
			clock = clock.Add(delay)
			return nil
		},
	}

	request := SubscriptionRequest{
		GUID:   "qnext-test",
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode:           ModeLTPC,
			InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		},
	}

	var ticks int
	err := supervisor.Run(ctx, "token", request, func(domain.Tick) error {
		ticks++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if ticks != 1 || runner.calls != 2 {
		t.Fatalf("unexpected calls: ticks=%d runner=%d", ticks, runner.calls)
	}
	if len(recovery.requests) != 1 {
		t.Fatalf("expected one recovery request, got %d", len(recovery.requests))
	}

	got := recovery.requests[0]
	if got.Provider != ProviderName || !got.From.Equal(base) || !got.To.Equal(base.Add(time.Second)) {
		t.Fatalf("unexpected recovery window: %+v", got)
	}
	if len(got.InstrumentKeys) != 1 || got.InstrumentKeys[0] != "NSE_INDEX|Nifty 50" {
		t.Fatalf("unexpected recovery instruments: %+v", got.InstrumentKeys)
	}
}

func TestSupervisorFailsClosedWhenRecoveryFails(t *testing.T) {
	base := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	clock := base
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := &fakeStreamRunner{cancel: cancel, event: base}
	recovery := &fakeRecovery{err: errors.New("history unavailable")}

	supervisor := &Supervisor{
		Runner:     runner,
		Recovery:   recovery,
		MinBackoff: time.Second,
		MaxBackoff: time.Second,
		Now:        func() time.Time { return clock },
		Sleep: func(_ context.Context, delay time.Duration) error {
			clock = clock.Add(delay)
			return nil
		},
	}

	err := supervisor.Run(ctx, "token", SubscriptionRequest{
		GUID:   "qnext-test",
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode:           ModeLTPC,
			InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		},
	}, func(domain.Tick) error { return nil })
	if err == nil || err.Error() != "recover Upstox market gap: history unavailable" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type multiInstrumentRunner struct {
	calls  int
	cancel context.CancelFunc
	base   time.Time
}

func (r *multiInstrumentRunner) Run(
	_ context.Context,
	_ string,
	_ SubscriptionRequest,
	onTick func(domain.Tick) error,
) error {
	r.calls++
	if r.calls == 1 {
		for _, tick := range []domain.Tick{
			{
				InstrumentID: "NSE:NIFTY50",
				Provider:     ProviderName,
				Price:        25100,
				EventTime:    r.base,
				Quality:      domain.QualityGood,
			},
			{
				InstrumentID: "NSE:BANKNIFTY",
				Provider:     ProviderName,
				Price:        55100,
				EventTime:    r.base.Add(-5 * time.Second),
				Quality:      domain.QualityGood,
			},
		} {
			if err := onTick(tick); err != nil {
				return err
			}
		}
		return errors.New("socket disconnected")
	}
	r.cancel()
	return context.Canceled
}

func TestSupervisorTracksRecoveryCursorPerInstrument(t *testing.T) {
	base := time.Date(2026, 9, 24, 3, 45, 10, 0, time.UTC)
	clock := base

	ctx, cancel := context.WithCancel(context.Background())
	runner := &multiInstrumentRunner{cancel: cancel, base: base}
	recovery := &fakeRecovery{}

	supervisor := &Supervisor{
		Runner:     runner,
		Recovery:   recovery,
		MinBackoff: time.Second,
		MaxBackoff: time.Second,
		Now: func() time.Time {
			return clock
		},
		Sleep: func(_ context.Context, delay time.Duration) error {
			clock = clock.Add(delay)
			return nil
		},
	}

	err := supervisor.Run(ctx, "token", SubscriptionRequest{
		GUID:   "qnext-test",
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode: ModeLTPC,
			InstrumentKeys: []string{
				"NSE_INDEX|Nifty 50",
				"NSE_INDEX|Nifty Bank",
			},
		},
	}, func(domain.Tick) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if len(recovery.requests) != 1 {
		t.Fatalf("expected one recovery request, got %d", len(recovery.requests))
	}

	request := recovery.requests[0]
	if got := request.FromByInstrumentID["NSE:NIFTY50"]; !got.Equal(base) {
		t.Fatalf("NIFTY cursor=%v want %v", got, base)
	}
	wantBank := base.Add(-5 * time.Second)
	if got := request.FromByInstrumentID["NSE:BANKNIFTY"]; !got.Equal(wantBank) {
		t.Fatalf("BANKNIFTY cursor=%v want %v", got, wantBank)
	}
	if !request.From.Equal(wantBank) {
		t.Fatalf("global fallback cursor=%v want earliest %v", request.From, wantBank)
	}
}
