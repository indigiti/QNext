package upstox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type StreamRunner interface {
	Run(
		context.Context,
		string,
		SubscriptionRequest,
		func(domain.Tick) error,
	) error
}

type RecoveryRequest struct {
	Provider       string
	InstrumentKeys []string
	From           time.Time
	To             time.Time
	Cause          string
}

type GapRecovery interface {
	Recover(context.Context, RecoveryRequest) error
}

type Supervisor struct {
	Runner     StreamRunner
	Recovery   GapRecovery
	MinBackoff time.Duration
	MaxBackoff time.Duration
	ResetAfter time.Duration
	Now        func() time.Time
	Sleep      func(context.Context, time.Duration) error
}

func (s *Supervisor) Run(
	ctx context.Context,
	accessToken string,
	request SubscriptionRequest,
	onTick func(domain.Tick) error,
) error {
	if s.Runner == nil {
		return errors.New("stream runner is required")
	}
	if onTick == nil {
		return errors.New("tick sink is required")
	}

	minBackoff := s.MinBackoff
	if minBackoff <= 0 {
		minBackoff = time.Second
	}
	maxBackoff := s.MaxBackoff
	if maxBackoff < minBackoff {
		maxBackoff = 30 * time.Second
	}
	resetAfter := s.ResetAfter
	if resetAfter <= 0 {
		resetAfter = 30 * time.Second
	}

	now := s.Now
	if now == nil {
		now = time.Now
	}
	sleep := s.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	backoff := minBackoff
	var lastEventTime time.Time

	for {
		sessionStarted := now().UTC()
		err := s.Runner.Run(ctx, accessToken, request, func(tick domain.Tick) error {
			if tick.EventTime.After(lastEventTime) {
				lastEventTime = tick.EventTime
			}
			return onTick(tick)
		})

		if ctx.Err() != nil {
			return ctx.Err()
		}

		sessionEnded := now().UTC()
		if sessionEnded.Sub(sessionStarted) >= resetAfter {
			backoff = minBackoff
		}

		if err == nil {
			err = errors.New("Upstox market stream ended unexpectedly")
		}

		if err := sleep(ctx, backoff); err != nil {
			return err
		}

		recoveryTo := now().UTC()
		if s.Recovery != nil && !lastEventTime.IsZero() && recoveryTo.After(lastEventTime) {
			recovery := RecoveryRequest{
				Provider:       ProviderName,
				InstrumentKeys: append([]string(nil), request.Data.InstrumentKeys...),
				From:           lastEventTime,
				To:             recoveryTo,
				Cause:          err.Error(),
			}
			if recoverErr := s.Recovery.Recover(ctx, recovery); recoverErr != nil {
				return fmt.Errorf("recover Upstox market gap: %w", recoverErr)
			}
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
