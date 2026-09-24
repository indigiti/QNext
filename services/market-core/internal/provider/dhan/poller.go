package dhan

import (
	"context"
	"errors"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type Poller struct {
	Client       QuoteFetcher
	Registry     *symbol.Registry
	AccessToken  string
	ClientID     string
	Keys         []InstrumentKey
	Interval     time.Duration
	NextSequence func() uint64
	Now          func() time.Time
	OnError      func(error)
}

func (p *Poller) Run(ctx context.Context, onTick func(domain.Tick) error) error {
	if p.Client == nil || p.Registry == nil || p.NextSequence == nil {
		return errors.New("Dhan poller requires client, registry, and sequence generator")
	}
	if onTick == nil {
		return errors.New("Dhan poller tick sink is required")
	}
	interval := p.Interval
	if interval < time.Second {
		interval = time.Second
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			snapshots, err := p.Client.Fetch(ctx, p.AccessToken, p.ClientID, p.Keys)
			if err != nil {
				if p.OnError != nil {
					p.OnError(err)
				}
			} else {
				for _, s := range snapshots {
					instrument, ok := p.Registry.ResolveProviderKey(ProviderName, s.ProviderKey)
					if !ok {
						if p.OnError != nil {
							p.OnError(errors.New("Dhan snapshot provider key is not registered"))
						}
						continue
					}
					processed := now().UTC()
					tick := domain.Tick{InstrumentID: instrument.ID, Provider: ProviderName, Price: s.Price, EventTime: s.ObservedAt.UTC(), ReceivedTime: s.ObservedAt.UTC(), ProcessedTime: processed, Sequence: p.NextSequence(), Quality: domain.QualityPartial}
					if err := onTick(tick); err != nil {
						if p.OnError != nil {
							p.OnError(err)
						}
						continue
					}
				}
			}
			timer.Reset(interval)
		}
	}
}
