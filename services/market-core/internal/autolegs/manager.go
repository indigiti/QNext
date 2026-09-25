package autolegs

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/synthetic"
)

type Config struct {
	UnderlyingInstrumentID string
	SyntheticInstrumentID  string
	Version                string
	StrikeInterval         float64
	ActiveStrikes          int
	WarmStrikes            int
	HysteresisPoints       float64
	Confirmation           time.Duration
	MinimumValidCandidates int
	MaxLegAge              time.Duration
	MaxLegTimeSkew         time.Duration
}

func (c Config) Validate() error {
	if c.UnderlyingInstrumentID == "" || c.SyntheticInstrumentID == "" || c.Version == "" {
		return errors.New("auto leg manager requires underlying, synthetic id, and version")
	}
	if c.StrikeInterval <= 0 {
		return errors.New("strike interval must be positive")
	}
	if c.ActiveStrikes < 3 || c.ActiveStrikes%2 == 0 {
		return errors.New("active strike count must be odd and at least 3")
	}
	if c.WarmStrikes < c.ActiveStrikes || c.WarmStrikes%2 == 0 {
		return errors.New("warm strike count must be odd and no smaller than active strike count")
	}
	if c.HysteresisPoints < 0 || c.HysteresisPoints >= c.StrikeInterval/2 {
		return errors.New("ATM hysteresis must be non-negative and below half the strike interval")
	}
	if c.Confirmation < 0 {
		return errors.New("ATM confirmation cannot be negative")
	}
	if c.MinimumValidCandidates <= 0 || c.MinimumValidCandidates > c.ActiveStrikes {
		return errors.New("minimum valid candidates must fit inside active strike count")
	}
	return nil
}

type ResolvedLeg struct {
	InstrumentID string
	ProviderKey  string
	Strike       float64
	Side         synthetic.LegSide
}

type Basket struct {
	Expiry string
	Legs   []ResolvedLeg
}

type Resolver interface {
	Resolve(context.Context, time.Time, []float64) (Basket, error)
}

type SubscriptionController interface {
	Subscribe(context.Context, []string) error
	Unsubscribe(context.Context, []string) error
}

type Status struct {
	ATM           float64
	Expiry        string
	Generation    uint64
	PendingATM    float64
	PendingExpiry string
}

type observation struct {
	price float64
	at    time.Time
}

type generation struct {
	atm       float64
	expiry    string
	number    uint64
	assembler *synthetic.Assembler
	warmKeys  map[string]bool
	activeIDs map[string]bool
	ready     *domain.Tick
}

type Manager struct {
	mu sync.Mutex

	cfg      Config
	resolver Resolver
	subs     SubscriptionController
	nextSeq  func() uint64
	onError  func(error)

	observations chan observation
	retire       chan []string

	current *generation
	pending *generation

	candidateATM   float64
	candidateSince time.Time
	latestSpot     observation
	latestTicks    map[string]domain.Tick
	planCounter    uint64
}

func New(
	cfg Config,
	resolver Resolver,
	subs SubscriptionController,
	nextSeq func() uint64,
	onError func(error),
) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if resolver == nil || subs == nil || nextSeq == nil {
		return nil, errors.New("auto leg manager requires resolver, subscriptions, and sequence generator")
	}
	return &Manager{
		cfg:          cfg,
		resolver:     resolver,
		subs:         subs,
		nextSeq:      nextSeq,
		onError:      onError,
		observations: make(chan observation, 1),
		retire:       make(chan []string, 8),
		latestTicks:  make(map[string]domain.Tick),
	}, nil
}

func (m *Manager) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case obs := <-m.observations:
			if err := m.consider(ctx, obs); err != nil {
				m.report(err)
			}
		case keys := <-m.retire:
			if len(keys) == 0 {
				continue
			}
			if err := m.subs.Unsubscribe(ctx, keys); err != nil && ctx.Err() == nil {
				m.report(fmt.Errorf("retire synthetic option legs: %w", err))
			}
		}
	}
}

func (m *Manager) Apply(tick domain.Tick) (domain.Tick, bool, error) {
	if tick.InstrumentID == m.cfg.UnderlyingInstrumentID && tick.Price > 0 && !tick.EventTime.IsZero() {
		m.observe(observation{price: tick.Price, at: tick.EventTime.UTC()})
	}

	m.mu.Lock()
	m.latestTicks[tick.InstrumentID] = tick

	if m.pending != nil && m.pending.ready != nil {
		out := *m.pending.ready
		retire := m.activatePendingLocked()
		m.mu.Unlock()
		m.enqueueRetire(retire)
		return out, true, nil
	}

	var currentTick domain.Tick
	var currentEmitted bool
	if m.current != nil {
		var err error
		currentTick, currentEmitted, err = m.current.assembler.Apply(tick)
		if err != nil {
			m.mu.Unlock()
			return domain.Tick{}, false, err
		}
	}

	if m.pending != nil {
		pendingTick, pendingEmitted, err := m.pending.assembler.Apply(tick)
		if err != nil {
			m.mu.Unlock()
			return domain.Tick{}, false, err
		}
		if pendingEmitted {
			retire := m.activatePendingLocked()
			m.mu.Unlock()
			m.enqueueRetire(retire)
			return pendingTick, true, nil
		}
	}
	m.mu.Unlock()
	return currentTick, currentEmitted, nil
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	var status Status
	if m.current != nil {
		status.ATM = m.current.atm
		status.Expiry = m.current.expiry
		status.Generation = m.current.number
	}
	if m.pending != nil {
		status.PendingATM = m.pending.atm
		status.PendingExpiry = m.pending.expiry
	}
	return status
}

func (m *Manager) consider(ctx context.Context, obs observation) error {
	if obs.price <= 0 || obs.at.IsZero() {
		return nil
	}

	m.mu.Lock()
	m.latestSpot = obs
	if m.pending != nil {
		m.mu.Unlock()
		return nil
	}

	currentATM := 0.0
	if m.current != nil {
		currentATM = m.current.atm
	}
	target := desiredATM(currentATM, obs.price, m.cfg.StrikeInterval, m.cfg.HysteresisPoints)
	if target == currentATM && currentATM != 0 {
		m.candidateATM = 0
		m.candidateSince = time.Time{}
		m.mu.Unlock()
		return nil
	}

	// Initial startup should resolve immediately. Hysteresis confirmation is
	// only needed when replacing an already authoritative generation.
	if currentATM == 0 {
		m.mu.Unlock()
		return m.prepare(ctx, target, obs.at)
	}

	if m.candidateATM != target {
		m.candidateATM = target
		m.candidateSince = obs.at
		if m.cfg.Confirmation == 0 {
			m.candidateATM = 0
			m.candidateSince = time.Time{}
			m.mu.Unlock()
			return m.prepare(ctx, target, obs.at)
		}
		m.mu.Unlock()
		return nil
	}
	if obs.at.Sub(m.candidateSince) < m.cfg.Confirmation {
		m.mu.Unlock()
		return nil
	}

	m.candidateATM = 0
	m.candidateSince = time.Time{}
	m.mu.Unlock()
	return m.prepare(ctx, target, obs.at)
}

func (m *Manager) prepare(ctx context.Context, atm float64, at time.Time) error {
	warm := centeredStrikes(atm, m.cfg.StrikeInterval, m.cfg.WarmStrikes)
	basket, err := m.resolver.Resolve(ctx, at, warm)
	if err != nil {
		return fmt.Errorf("resolve auto synthetic basket: %w", err)
	}
	if basket.Expiry == "" {
		return errors.New("resolved synthetic basket has no expiry")
	}

	warmSet := strikeSet(warm)
	warmKeys := make(map[string]bool, len(basket.Legs))
	for _, leg := range basket.Legs {
		if warmSet[leg.Strike] && leg.InstrumentID != "" && leg.ProviderKey != "" {
			warmKeys[leg.ProviderKey] = true
		}
	}
	if len(warmKeys) != m.cfg.WarmStrikes*2 {
		return fmt.Errorf("resolved warm basket is incomplete: got %d legs, want %d", len(warmKeys), m.cfg.WarmStrikes*2)
	}

	keys := sortedKeys(warmKeys)
	if err := m.subs.Subscribe(ctx, keys); err != nil {
		return fmt.Errorf("subscribe synthetic warm basket: %w", err)
	}

	active := centeredStrikes(atm, m.cfg.StrikeInterval, m.cfg.ActiveStrikes)
	activeSet := strikeSet(active)
	bindings := make([]synthetic.LegBinding, 0, m.cfg.ActiveStrikes*2)
	activeIDs := make(map[string]bool, m.cfg.ActiveStrikes*2)
	for _, leg := range basket.Legs {
		if !activeSet[leg.Strike] {
			continue
		}
		bindings = append(bindings, synthetic.LegBinding{
			InstrumentID: leg.InstrumentID,
			Strike:       leg.Strike,
			Side:         leg.Side,
		})
		activeIDs[leg.InstrumentID] = true
	}
	if len(bindings) != m.cfg.ActiveStrikes*2 {
		return fmt.Errorf("resolved active basket is incomplete: got %d legs, want %d", len(bindings), m.cfg.ActiveStrikes*2)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending != nil {
		return errors.New("synthetic basket changed while preparing a pending generation")
	}

	m.planCounter++
	number := m.planCounter
	version := fmt.Sprintf(
		"%s|expiry=%s|atm=%s|gen=%d",
		m.cfg.Version,
		basket.Expiry,
		formatStrike(atm),
		number,
	)
	assembler, err := synthetic.NewAssembler(synthetic.Definition{
		ID:                     m.cfg.SyntheticInstrumentID,
		Version:                version,
		MinimumValidCandidates: m.cfg.MinimumValidCandidates,
		MaxLegAge:              m.cfg.MaxLegAge,
		MaxLegTimeSkew:         m.cfg.MaxLegTimeSkew,
	}, bindings, m.nextSeq)
	if err != nil {
		return err
	}

	gen := &generation{
		atm:       atm,
		expiry:    basket.Expiry,
		number:    number,
		assembler: assembler,
		warmKeys:  warmKeys,
		activeIDs: activeIDs,
	}

	cached := make([]domain.Tick, 0, len(activeIDs))
	for instrumentID := range activeIDs {
		if tick, ok := m.latestTicks[instrumentID]; ok {
			cached = append(cached, tick)
		}
	}
	sort.Slice(cached, func(i, j int) bool {
		if cached[i].EventTime.Equal(cached[j].EventTime) {
			return cached[i].InstrumentID < cached[j].InstrumentID
		}
		return cached[i].EventTime.Before(cached[j].EventTime)
	})
	for _, tick := range cached {
		out, emitted, applyErr := gen.assembler.Apply(tick)
		if applyErr != nil {
			return applyErr
		}
		if emitted {
			copyOfTick := out
			gen.ready = &copyOfTick
		}
	}

	m.pending = gen
	return nil
}

func (m *Manager) activatePendingLocked() []string {
	old := m.current
	m.current = m.pending
	m.pending = nil
	m.candidateATM = 0
	m.candidateSince = time.Time{}

	if old == nil {
		return nil
	}
	var retire []string
	for key := range old.warmKeys {
		if !m.current.warmKeys[key] {
			retire = append(retire, key)
		}
	}
	sort.Strings(retire)
	return retire
}

func (m *Manager) observe(obs observation) {
	select {
	case m.observations <- obs:
	default:
		select {
		case <-m.observations:
		default:
		}
		select {
		case m.observations <- obs:
		default:
		}
	}
}

func (m *Manager) enqueueRetire(keys []string) {
	if len(keys) == 0 {
		return
	}
	select {
	case m.retire <- keys:
	default:
		m.report(errors.New("synthetic retirement queue is full"))
	}
}

func (m *Manager) report(err error) {
	if err != nil && m.onError != nil {
		m.onError(err)
	}
}

func desiredATM(currentATM, spot, interval, hysteresis float64) float64 {
	nearest := math.Floor(spot/interval+0.5) * interval
	if currentATM == 0 {
		return nearest
	}
	upper := currentATM + interval/2 + hysteresis
	lower := currentATM - interval/2 - hysteresis
	if spot > upper || spot < lower {
		return nearest
	}
	return currentATM
}

func centeredStrikes(atm, interval float64, count int) []float64 {
	half := count / 2
	result := make([]float64, 0, count)
	for offset := -half; offset <= half; offset++ {
		result = append(result, atm+float64(offset)*interval)
	}
	return result
}

func strikeSet(strikes []float64) map[float64]bool {
	result := make(map[float64]bool, len(strikes))
	for _, strike := range strikes {
		result[strike] = true
	}
	return result
}

func sortedKeys(keys map[string]bool) []string {
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func formatStrike(strike float64) string {
	if math.Mod(strike, 1) == 0 {
		return fmt.Sprintf("%.0f", strike)
	}
	return fmt.Sprintf("%.2f", strike)
}
