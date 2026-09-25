package upstox

import (
	"context"
	"errors"
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
	Provider           string
	InstrumentKeys     []string
	From               time.Time
	FromByInstrumentID map[string]time.Time
	To                 time.Time
	Cause              string
}

type GapRecovery interface {
	Recover(context.Context, RecoveryRequest) error
}

type Supervisor struct {
	Runner          StreamRunner
	Recovery        GapRecovery
	RequestSnapshot func() SubscriptionRequest
	MinBackoff      time.Duration
	MaxBackoff      time.Duration
	ResetAfter      time.Duration
	Now             func() time.Time
	Sleep           func(context.Context, time.Duration) error
	OnRecoveryError func(error)
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
	lastEventByInstrument := make(map[string]time.Time)

	for {
		runRequest := request
		if s.RequestSnapshot != nil {
			runRequest = s.RequestSnapshot()
		}

		sessionStarted := now().UTC()
		err := s.Runner.Run(ctx, accessToken, runRequest, func(tick domain.Tick) error {
			if tick.EventTime.After(lastEventTime) {
				lastEventTime = tick.EventTime
			}
			if tick.InstrumentID != "" {
				if previous := lastEventByInstrument[tick.InstrumentID]; tick.EventTime.After(previous) {
					lastEventByInstrument[tick.InstrumentID] = tick.EventTime
				}
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
			cursors := make(map[string]time.Time, len(lastEventByInstrument))
			for instrumentID, at := range lastEventByInstrument {
				cursors[instrumentID] = at
			}
			recovery := RecoveryRequest{
				Provider:           ProviderName,
				InstrumentKeys:     append([]string(nil), runRequest.Data.InstrumentKeys...),
				From:               earliestRecoveryCursor(lastEventTime, cursors),
				FromByInstrumentID: cursors,
				To:                 recoveryTo,
				Cause:              err.Error(),
			}
			if recoverErr := s.Recovery.Recover(ctx, recovery); recoverErr != nil {
				// Recovery failure must not terminate the supervisor. The live stream can
				// reconnect independently and a later recovery attempt can heal the gap.
				// Returning here used to bubble up to main and shut down the entire HTTP
				// service, turning otherwise readable history endpoints into 502s.
				if s.OnRecoveryError != nil {
					s.OnRecoveryError(recoverErr)
				}
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
}

func earliestRecoveryCursor(fallback time.Time, cursors map[string]time.Time) time.Time {
	earliest := fallback
	for _, at := range cursors {
		if at.IsZero() {
			continue
		}
		if earliest.IsZero() || at.Before(earliest) {
			earliest = at
		}
	}
	return earliest
}
