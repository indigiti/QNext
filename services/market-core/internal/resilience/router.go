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

type AuthorityState string

const (
	StatePrimaryHealthy  AuthorityState = "PRIMARY_HEALTHY"
	StatePrimarySuspect  AuthorityState = "PRIMARY_SUSPECT"
	StateFailoverPending AuthorityState = "FAILOVER_PENDING"
	StateSecondaryActive AuthorityState = "SECONDARY_ACTIVE"
	StateFailbackPending AuthorityState = "FAILBACK_PENDING"
)

type TransitionPolicy struct {
	PrimaryProvider string
	FailoverPending time.Duration
	FailbackPending time.Duration
	Cooldown        time.Duration
	PolicyVersion   string
}

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
	mu             sync.Mutex
	resolver       *authority.Resolver
	downstream     TickSink
	recoverer      GapRecoverer
	metrics        *Metrics
	gapThreshold   time.Duration
	recoverable    map[string]bool
	active         map[string]string
	lastAccepted   map[string]time.Time
	state          map[string]AuthorityState
	candidate      map[string]string
	candidateSince map[string]time.Time
	lastSwitch     map[string]time.Time
	policy         TransitionPolicy
}

func NewRouter(resolver *authority.Resolver, downstream TickSink, metrics *Metrics, gapThreshold time.Duration, recoverable map[string]bool, recoverer GapRecoverer) (*Router, error) {
	return newRouter(resolver, downstream, metrics, gapThreshold, recoverable, recoverer, TransitionPolicy{})
}

func NewRouterWithPolicy(resolver *authority.Resolver, downstream TickSink, metrics *Metrics, gapThreshold time.Duration, recoverable map[string]bool, recoverer GapRecoverer, policy TransitionPolicy) (*Router, error) {
	if policy.PrimaryProvider == "" {
		return nil, errors.New("transition policy requires primary provider")
	}
	if policy.FailoverPending < 0 || policy.FailbackPending < 0 || policy.Cooldown < 0 {
		return nil, errors.New("transition durations cannot be negative")
	}
	policy.PrimaryProvider = strings.ToLower(strings.TrimSpace(policy.PrimaryProvider))
	if policy.PolicyVersion == "" {
		policy.PolicyVersion = "q3-authority-v1"
	}
	return newRouter(resolver, downstream, metrics, gapThreshold, recoverable, recoverer, policy)
}

func newRouter(resolver *authority.Resolver, downstream TickSink, metrics *Metrics, gapThreshold time.Duration, recoverable map[string]bool, recoverer GapRecoverer, policy TransitionPolicy) (*Router, error) {
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
	return &Router{
		resolver:       resolver,
		downstream:     downstream,
		recoverer:      recoverer,
		metrics:        metrics,
		gapThreshold:   gapThreshold,
		recoverable:    cp,
		active:         map[string]string{},
		lastAccepted:   map[string]time.Time{},
		state:          map[string]AuthorityState{},
		candidate:      map[string]string{},
		candidateSince: map[string]time.Time{},
		lastSwitch:     map[string]time.Time{},
		policy:         policy,
	}, nil
}

func (r *Router) Handle(tick domain.Tick) error {
	return r.HandleContext(context.Background(), tick)
}

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
	if err := r.resolver.UpdateState(tick.InstrumentID, provider, authority.ProviderState{
		Healthy:       true,
		Entitled:      true,
		LastEventTime: tick.EventTime.UTC(),
	}); err != nil {
		r.metrics.ObserveError(provider)
		return err
	}

	selected, ok := r.resolver.Resolve(tick.InstrumentID, now)
	active := r.active[tick.InstrumentID]

	if active == "" {
		if !ok || selected.Provider != provider {
			r.metrics.dropped(provider)
			return nil
		}
		r.active[tick.InstrumentID] = provider
		r.metrics.setAuthority(tick.InstrumentID, provider)
		r.setStableState(tick.InstrumentID, provider)
		return r.accept(tick)
	}

	if provider == active {
		if ok && selected.Provider == active {
			r.clearCandidate(tick.InstrumentID)
			r.setStableState(tick.InstrumentID, active)
		}
		return r.accept(tick)
	}

	if !ok || selected.Provider != provider {
		r.metrics.dropped(provider)
		return nil
	}

	if !r.transitionReady(tick.InstrumentID, active, provider, now) {
		r.metrics.dropped(provider)
		return nil
	}

	last := r.lastAccepted[tick.InstrumentID]
	gap := time.Duration(0)
	if !last.IsZero() && tick.EventTime.After(last) {
		gap = tick.EventTime.Sub(last)
	}
	if gap > r.gapThreshold {
		r.metrics.gap()
		if r.recoverable[tick.InstrumentID] && r.recoverer != nil {
			err := r.recoverer.Recover(ctx, Gap{
				InstrumentID: tick.InstrumentID,
				Provider:     provider,
				From:         last,
				To:           tick.EventTime.UTC(),
			})
			r.metrics.recovery(err == nil)
			if err != nil {
				r.metrics.ObserveError(provider)
				return err
			}
		}
	}

	reason := "PRIMARY_RECOVERED"
	nextState := StatePrimaryHealthy
	if provider != r.policy.PrimaryProvider {
		reason = "PRIMARY_STALE"
		nextState = StateSecondaryActive
	}
	r.active[tick.InstrumentID] = provider
	r.lastSwitch[tick.InstrumentID] = now
	r.clearCandidate(tick.InstrumentID)
	r.state[tick.InstrumentID] = nextState
	r.metrics.switched(SwitchEvent{
		InstrumentID:  tick.InstrumentID,
		From:          active,
		To:            provider,
		Reason:        reason,
		PolicyVersion: r.policy.PolicyVersion,
		AtMS:          now.UnixMilli(),
		GapMS:         gap.Milliseconds(),
	}, nextState)
	return r.accept(tick)
}

func (r *Router) transitionReady(instrumentID, active, target string, now time.Time) bool {
	if r.policy.PrimaryProvider == "" {
		return true
	}

	if last := r.lastSwitch[instrumentID]; !last.IsZero() && r.policy.Cooldown > 0 && now.Sub(last) < r.policy.Cooldown {
		return false
	}

	pending := r.policy.FailoverPending
	state := StateFailoverPending
	if target == r.policy.PrimaryProvider {
		pending = r.policy.FailbackPending
		state = StateFailbackPending
	} else if active == r.policy.PrimaryProvider {
		r.state[instrumentID] = StatePrimarySuspect
		r.metrics.setState(instrumentID, StatePrimarySuspect)
	}

	if r.candidate[instrumentID] != target {
		r.candidate[instrumentID] = target
		r.candidateSince[instrumentID] = now
		r.state[instrumentID] = state
		r.metrics.setState(instrumentID, state)
		return pending == 0
	}

	r.state[instrumentID] = state
	r.metrics.setState(instrumentID, state)
	return now.Sub(r.candidateSince[instrumentID]) >= pending
}

func (r *Router) accept(tick domain.Tick) error {
	if err := r.downstream(tick); err != nil {
		r.metrics.ObserveError(tick.Provider)
		return err
	}
	last := r.lastAccepted[tick.InstrumentID]
	if last.IsZero() || tick.EventTime.After(last) {
		r.lastAccepted[tick.InstrumentID] = tick.EventTime.UTC()
	}
	r.metrics.accepted(tick.Provider)
	return nil
}

func (r *Router) setStableState(instrumentID, provider string) {
	state := StateSecondaryActive
	if r.policy.PrimaryProvider == "" || provider == r.policy.PrimaryProvider {
		state = StatePrimaryHealthy
	}
	r.state[instrumentID] = state
	r.metrics.setState(instrumentID, state)
}

func (r *Router) clearCandidate(instrumentID string) {
	delete(r.candidate, instrumentID)
	delete(r.candidateSince, instrumentID)
}

func (r *Router) Snapshot() Snapshot {
	return r.metrics.Snapshot()
}

type RecoveryFunc func(context.Context, Gap) error

func (f RecoveryFunc) Recover(ctx context.Context, gap Gap) error {
	return f(ctx, gap)
}
