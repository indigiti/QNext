package upstox

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var errResearchReplan = errors.New("research subscription replan required")

type ResearchSink interface {
	Observe(DecodedEnvelope, ResearchPlan) error
}

type ResearchRunner struct {
	Authorizer      MarketFeedAuthorizer
	Dialer          BinaryDialer
	Decoder         FeedDecoder
	Contracts       OptionContractSource
	Now             func() time.Time
	CaptureFrame    func([]byte) error
	OnError         func(error)
	GUID            string
	WingStrikes     int
	FullWingStrikes int
	ReconnectMin    time.Duration
	ReconnectMax    time.Duration
}

func (r *ResearchRunner) Run(
	ctx context.Context,
	accessToken string,
	underlyingKey string,
	sink ResearchSink,
) error {
	if sink == nil {
		return errors.New("research sink is required")
	}
	if r.Authorizer == nil || r.Dialer == nil || r.Decoder == nil || r.Contracts == nil {
		return errors.New("research runner requires authorizer, dialer, decoder, and option contracts source")
	}
	minBackoff := r.ReconnectMin
	if minBackoff <= 0 {
		minBackoff = time.Second
	}
	maxBackoff := r.ReconnectMax
	if maxBackoff < minBackoff {
		maxBackoff = 15 * time.Second
	}
	backoff := minBackoff

	for {
		err := r.runSession(ctx, accessToken, underlyingKey, sink)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errResearchReplan) {
			backoff = minBackoff
			continue
		}
		if err != nil && r.OnError != nil {
			r.OnError(err)
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (r *ResearchRunner) runSession(
	ctx context.Context,
	accessToken string,
	underlyingKey string,
	sink ResearchSink,
) error {
	uri, err := r.Authorizer.AuthorizedURI(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("authorize Upstox research feed: %w", err)
	}
	connection, err := r.Dialer.Dial(ctx, uri)
	if err != nil {
		return fmt.Errorf("dial Upstox research feed: %w", err)
	}
	defer connection.Close()

	guid := r.GUID
	if guid == "" {
		guid = "qnext-data-collector"
	}
	if err := writeResearchSubscription(ctx, connection, SubscriptionRequest{
		GUID:   guid + "-spot",
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode:           ModeLTPC,
			InstrumentKeys: []string{underlyingKey},
		},
	}); err != nil {
		return err
	}

	var spot float64
	var firstEnvelope DecodedEnvelope
	for spot <= 0 {
		payload, err := connection.ReadBinary(ctx)
		if err != nil {
			return fmt.Errorf("read Upstox research spot: %w", err)
		}
		if err := r.capture(payload); err != nil {
			return err
		}
		envelope, err := r.Decoder.Decode(payload)
		if err != nil {
			return fmt.Errorf("decode Upstox research spot: %w", err)
		}
		feed, ok := envelope.Feeds[underlyingKey]
		if !ok || feed.LTPC == nil || feed.LTPC.LTP <= 0 {
			continue
		}
		spot = feed.LTPC.LTP
		firstEnvelope = envelope
	}

	contracts, err := r.Contracts.Contracts(ctx, accessToken, underlyingKey)
	if err != nil {
		return err
	}
	wing := r.WingStrikes
	if wing <= 0 {
		wing = 20
	}
	fullWing := r.FullWingStrikes
	if fullWing < 0 {
		fullWing = 0
	}
	if r.FullWingStrikes == 0 {
		fullWing = 5
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	plan, err := BuildResearchPlan(contracts, underlyingKey, spot, now().UTC(), wing, fullWing)
	if err != nil {
		return fmt.Errorf("build Upstox research plan: %w", err)
	}

	if len(plan.FullKeys) > 0 {
		if err := writeResearchSubscription(ctx, connection, SubscriptionRequest{
			GUID:   guid + "-full",
			Method: MethodSubscribe,
			Data:   SubscriptionData{Mode: ModeFull, InstrumentKeys: plan.FullKeys},
		}); err != nil {
			return err
		}
	}
	if len(plan.GreekKeys) > 0 {
		if err := writeResearchSubscription(ctx, connection, SubscriptionRequest{
			GUID:   guid + "-greeks",
			Method: MethodSubscribe,
			Data:   SubscriptionData{Mode: ModeOptionGreeks, InstrumentKeys: plan.GreekKeys},
		}); err != nil {
			return err
		}
	}
	if err := sink.Observe(firstEnvelope, plan); err != nil {
		return fmt.Errorf("store research envelope: %w", err)
	}

	for {
		payload, err := connection.ReadBinary(ctx)
		if err != nil {
			return fmt.Errorf("read Upstox research feed: %w", err)
		}
		if err := r.capture(payload); err != nil {
			return err
		}
		envelope, err := r.Decoder.Decode(payload)
		if err != nil {
			return fmt.Errorf("decode Upstox research feed: %w", err)
		}
		if err := sink.Observe(envelope, plan); err != nil {
			return fmt.Errorf("store research envelope: %w", err)
		}
		if feed, ok := envelope.Feeds[underlyingKey]; ok && feed.LTPC != nil && feed.LTPC.LTP > 0 {
			atm, ok := ResearchATM(contracts, plan.CurrentExpiry, feed.LTPC.LTP)
			if ok && atm != plan.ATM {
				return errResearchReplan
			}
		}
	}
}

func (r *ResearchRunner) capture(payload []byte) error {
	if r.CaptureFrame == nil {
		return nil
	}
	if err := r.CaptureFrame(payload); err != nil {
		return fmt.Errorf("capture Upstox research frame: %w", err)
	}
	return nil
}

func writeResearchSubscription(ctx context.Context, connection BinaryConnection, request SubscriptionRequest) error {
	payload, err := request.MarshalBinary()
	if err != nil {
		return err
	}
	if err := connection.WriteBinary(ctx, payload); err != nil {
		return fmt.Errorf("send Upstox research subscription: %w", err)
	}
	return nil
}
