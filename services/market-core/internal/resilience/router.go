package resilience

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/authority"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type Gap struct {
	InstrumentID string
	Provider     string
	From         time.Time
	To           time.Time
}
type GapRecoverer interface {
	Recover(context.Context, Gap) error
}
type TickSink func(domain.Tick) error

type Router struct {
	mu           sync.Mutex
	resolver     *authority.Resolver
	downstream   TickSink
	recoverer    GapRecoverer
	metrics      *Metrics
	gapThreshold time.Duration
	recoverable  map[string]bool
	active       map[string]string
	lastAccepted map[string]time.Time
}

func NewRouter(resolver *authority.Resolver, downstream TickSink, metrics *Metrics, gapThreshold time.Duration, recoverable map[string]bool, recoverer GapRecoverer) (*Router, error) {
	if resolver == nil {
		return nil, errors.New("authority resolver is required")
	}
	if downstream == nil {
		return nil, errors.New("downstream tick sink is required")
	}
	if metrics == nil {
		metrics = NewMetrics()
	}
	if gapThreshold <= 0 {
		return nil, errors.New("positive gap threshold is required")
	}
	cp := map[string]bool{}
	for k, v := range recoverable {
		cp[k] = v
	}
	return &Router{resolver: resolver, downstream: downstream, recoverer: recoverer, metrics: metrics, gapThreshold: gapThreshold, recoverable: cp, active: map[string]string{}, lastAccepted: map[string]time.Time{}}, nil
}

func (r *Router) Handle(tick domain.Tick) error { return r.HandleContext(context.Background(), tick) }
func (r *Router) HandleContext(ctx context.Context, tick domain.Tick) error {
	provider := strings.ToLower(strings.TrimSpace(tick.Provider))
	if tick.InstrumentID == "" || provider == "" || tick.EventTime.IsZero() || tick.Price <= 0 {
		return errors.New("canonical tick requires instrument, provider, event time, and positive price")
	}
	now := tick.ReceivedTime.UTC()
	if now.IsZero() {
		now = tick.EventTime.UTC()
	}
	if now.Before(tick.EventTime) {
		now = tick.EventTime.UTC()
	}
	tick.Provider = provider

	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.received(provider, tick.EventTime)
	if err := r.resolver.UpdateState(tick.InstrumentID, provider, authority.ProviderState{Healthy: true, Entitled: true, LastEventTime: tick.EventTime.UTC()}); err != nil {
		r.metrics.ObserveError(provider)
		return err
	}
	selected, ok := r.resolver.Resolve(tick.InstrumentID, now)
	if !ok || selected.Provider != provider {
		r.metrics.dropped(provider)
		return nil
	}

	previous := r.active[tick.InstrumentID]
	last := r.lastAccepted[tick.InstrumentID]
	switching := previous != "" && previous != provider
	gap := time.Duration(0)
	if !last.IsZero() && tick.EventTime.After(last) {
		gap = tick.EventTime.Sub(last)
	}
	if switching && gap > r.gapThreshold {
		r.metrics.gap()
		if r.recoverable[tick.InstrumentID] && r.recoverer != nil {
			err := r.recoverer.Recover(ctx, Gap{InstrumentID: tick.InstrumentID, Provider: provider, From: last, To: tick.EventTime.UTC()})
			r.metrics.recovery(err == nil)
			if err != nil {
				r.metrics.ObserveError(provider)
				return err
			}
		}
	}
	if switching {
		r.metrics.switched(SwitchEvent{InstrumentID: tick.InstrumentID, From: previous, To: provider, AtMS: now.UnixMilli(), GapMS: gap.Milliseconds()})
	} else if previous == "" {
		r.metrics.setAuthority(tick.InstrumentID, provider)
	}
	r.active[tick.InstrumentID] = provider
	if err := r.downstream(tick); err != nil {
		r.metrics.ObserveError(provider)
		return err
	}
	if last.IsZero() || tick.EventTime.After(last) {
		r.lastAccepted[tick.InstrumentID] = tick.EventTime.UTC()
	}
	r.metrics.accepted(provider)
	return nil
}
func (r *Router) Snapshot() Snapshot { return r.metrics.Snapshot() }

type RecoveryFunc func(context.Context, Gap) error

func (f RecoveryFunc) Recover(ctx context.Context, gap Gap) error { return f(ctx, gap) }
