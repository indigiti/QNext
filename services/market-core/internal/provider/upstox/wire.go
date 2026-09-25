package upstox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type MarketFeedAuthorizer interface {
	AuthorizedURI(context.Context, string) (string, error)
}

type BinaryConnection interface {
	WriteBinary(context.Context, []byte) error
	ReadBinary(context.Context) ([]byte, error)
	Close() error
}

type BinaryDialer interface {
	Dial(context.Context, string) (BinaryConnection, error)
}

type FeedDecoder interface {
	Decode([]byte) (DecodedEnvelope, error)
}

type WireClient struct {
	Authorizer          MarketFeedAuthorizer
	Dialer              BinaryDialer
	Decoder             FeedDecoder
	Normalizer          *Normalizer
	NextSequence        func() uint64
	Now                 func() time.Time
	CaptureFrame        func([]byte) error
	SubscriptionUpdates <-chan SubscriptionRequest
	InactivityTimeout   time.Duration
	WatchdogInterval    time.Duration
	WatchdogActive      func(time.Time) bool
}

func (c *WireClient) Open(
	ctx context.Context,
	accessToken string,
	request SubscriptionRequest,
) (BinaryConnection, error) {
	if c.Authorizer == nil || c.Dialer == nil {
		return nil, errors.New("authorizer and dialer are required")
	}

	uri, err := c.Authorizer.AuthorizedURI(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	connection, err := c.Dialer.Dial(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("dial Upstox market feed: %w", err)
	}

	payload, err := request.MarshalBinary()
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if err := connection.WriteBinary(ctx, payload); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("send Upstox subscription: %w", err)
	}
	return connection, nil
}

func (c *WireClient) HandleFrame(payload []byte) ([]domain.Tick, error) {
	if c.Decoder == nil || c.Normalizer == nil || c.NextSequence == nil {
		return nil, errors.New("decoder, normalizer, and sequence generator are required")
	}
	envelope, err := c.Decoder.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("decode Upstox frame: %w", err)
	}
	if envelope.Type == "market_info" {
		return nil, nil
	}

	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	return c.Normalizer.NormalizeEnvelope(envelope, now().UTC(), c.NextSequence)
}

type frameResult struct {
	payload []byte
	err     error
}

func (c *WireClient) Run(
	ctx context.Context,
	accessToken string,
	request SubscriptionRequest,
	onTick func(domain.Tick) error,
) error {
	if onTick == nil {
		return errors.New("tick sink is required")
	}
	connection, err := c.Open(ctx, accessToken, request)
	if err != nil {
		return err
	}
	defer connection.Close()

	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	lastFrameAt := now().UTC()

	var watchdogTicker *time.Ticker
	var watchdog <-chan time.Time
	if c.InactivityTimeout > 0 {
		interval := c.WatchdogInterval
		if interval <= 0 {
			interval = c.InactivityTimeout / 4
			if interval <= 0 {
				interval = time.Second
			}
		}
		watchdogTicker = time.NewTicker(interval)
		defer watchdogTicker.Stop()
		watchdog = watchdogTicker.C
	}

	frames := make(chan frameResult, 1)
	go func() {
		for {
			payload, readErr := connection.ReadBinary(ctx)
			frames <- frameResult{payload: payload, err: readErr}
			if readErr != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-watchdog:
			current := now().UTC()
			active := true
			if c.WatchdogActive != nil {
				active = c.WatchdogActive(current)
			}
			if active && current.Sub(lastFrameAt) >= c.InactivityTimeout {
				return fmt.Errorf(
					"Upstox market feed inactive for %s",
					current.Sub(lastFrameAt).Round(time.Second),
				)
			}

		case update, ok := <-c.SubscriptionUpdates:
			if !ok {
				c.SubscriptionUpdates = nil
				continue
			}
			payload, marshalErr := update.MarshalBinary()
			if marshalErr != nil {
				return fmt.Errorf("encode Upstox subscription update: %w", marshalErr)
			}
			if writeErr := connection.WriteBinary(ctx, payload); writeErr != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("send Upstox subscription update: %w", writeErr)
			}

		case result := <-frames:
			lastFrameAt = now().UTC()
			if result.err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("read Upstox market frame: %w", result.err)
			}
			if c.CaptureFrame != nil {
				if captureErr := c.CaptureFrame(result.payload); captureErr != nil {
					return fmt.Errorf("capture Upstox frame: %w", captureErr)
				}
			}
			ticks, handleErr := c.HandleFrame(result.payload)
			if handleErr != nil {
				return handleErr
			}
			for _, tick := range ticks {
				if sinkErr := onTick(tick); sinkErr != nil {
					return fmt.Errorf("deliver normalized tick: %w", sinkErr)
				}
			}
		}
	}
}
