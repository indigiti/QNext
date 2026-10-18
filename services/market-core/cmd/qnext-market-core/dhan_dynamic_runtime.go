package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/authority"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
	"github.com/indigiti/QNext/services/market-core/internal/provider/dhan"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/resilience"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

const dhanOptionChainMinInterval = 3 * time.Second

type dynamicOptionID struct {
	Exchange   string
	Underlying string
	Expiry     string
	Strike     float64
	Side       string
}

type dynamicDhanMarket struct {
	underlyingID  string
	underlyingKey dhan.InstrumentKey
	optionSegment string
	aliases       map[string]bool
}

type dhanDynamicOptionMapper struct {
	mu sync.RWMutex

	chain        dhan.OptionChainSource
	accessToken  string
	clientID     string
	registry     *symbol.Registry
	authority    *authority.Resolver
	quoteKeys    *dhan.KeyRegistry
	maxStaleness time.Duration
	markets      map[string][]dynamicDhanMarket
	onError      func(error)
	now          func() time.Time

	queue       chan string
	pending     map[string]bool
	covered     map[string]bool
	chainCache  map[string]map[string]dhan.InstrumentKey
	lastRequest time.Time
}

func newDhanDynamicOptionMapper(
	marketConfig marketconfig.Config,
	resilienceConfig resilience.Config,
	chain dhan.OptionChainSource,
	accessToken string,
	clientID string,
	registry *symbol.Registry,
	resolver *authority.Resolver,
	quoteKeys *dhan.KeyRegistry,
	onError func(error),
) (*dhanDynamicOptionMapper, error) {
	if chain == nil || registry == nil || resolver == nil || quoteKeys == nil {
		return nil, errors.New("dynamic Dhan mapper requires chain source, registry, authority, and quote keys")
	}

	markets := make(map[string][]dynamicDhanMarket)
	for _, market := range marketConfig.EffectiveMarkets() {
		mapping, ok := resilienceConfig.Mapping(market.Underlying.InstrumentID)
		if !ok {
			continue
		}
		underlyingKey, err := dhan.ParseInstrumentKey(mapping.DhanProviderKey)
		if err != nil {
			return nil, fmt.Errorf("parse Dhan underlying mapping for %s: %w", market.Symbol, err)
		}
		optionSegment, ok := dhanOptionSegment(market.Exchange)
		if !ok {
			continue
		}
		aliases := make(map[string]bool)
		for _, value := range append(
			[]string{market.Symbol, market.Name},
			market.Aliases...,
		) {
			if normalized := normalizeUnderlyingAlias(value); normalized != "" {
				aliases[normalized] = true
			}
		}
		exchange := strings.ToUpper(strings.TrimSpace(market.Exchange))
		markets[exchange] = append(markets[exchange], dynamicDhanMarket{
			underlyingID:  market.Underlying.InstrumentID,
			underlyingKey: underlyingKey,
			optionSegment: optionSegment,
			aliases:       aliases,
		})
	}

	return &dhanDynamicOptionMapper{
		chain:        chain,
		accessToken:  accessToken,
		clientID:     clientID,
		registry:     registry,
		authority:    resolver,
		quoteKeys:    quoteKeys,
		maxStaleness: resilienceConfig.MaxStaleness(),
		markets:      markets,
		onError:      onError,
		now:          time.Now,
		queue:        make(chan string, 256),
		pending:      make(map[string]bool),
		covered:      make(map[string]bool),
		chainCache:   make(map[string]map[string]dhan.InstrumentKey),
	}, nil
}

func (m *dhanDynamicOptionMapper) Run(ctx context.Context) {
	if m == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case instrumentID := <-m.queue:
			err := m.resolve(ctx, instrumentID)
			m.mu.Lock()
			delete(m.pending, instrumentID)
			m.mu.Unlock()
			if err != nil && ctx.Err() == nil && m.onError != nil {
				m.onError(err)
			}
		}
	}
}

func (m *dhanDynamicOptionMapper) Observe(instrumentID string) {
	if m == nil || !m.Eligible(instrumentID) {
		return
	}

	m.mu.Lock()
	if m.covered[instrumentID] || m.pending[instrumentID] {
		m.mu.Unlock()
		return
	}
	m.pending[instrumentID] = true
	m.mu.Unlock()

	select {
	case m.queue <- instrumentID:
	default:
		m.mu.Lock()
		delete(m.pending, instrumentID)
		m.mu.Unlock()
		if m.onError != nil {
			m.onError(errors.New("dynamic Dhan option mapping queue is full"))
		}
	}
}

func (m *dhanDynamicOptionMapper) Eligible(instrumentID string) bool {
	if m == nil {
		return false
	}
	parsed, ok := parseDynamicOptionID(instrumentID)
	if !ok {
		return false
	}
	_, ok = m.market(parsed)
	return ok
}

func (m *dhanDynamicOptionMapper) Covered(instrumentID string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	covered := m.covered[instrumentID]
	m.mu.RUnlock()
	return covered
}

func (m *dhanDynamicOptionMapper) resolve(ctx context.Context, instrumentID string) error {
	parsed, ok := parseDynamicOptionID(instrumentID)
	if !ok {
		return errors.New("dynamic Dhan mapping received a non-option canonical id")
	}
	market, ok := m.market(parsed)
	if !ok {
		return fmt.Errorf("no Dhan-capable market mapping for %s", instrumentID)
	}
	if _, ok := m.registry.ProviderMapping(upstox.ProviderName, instrumentID); !ok {
		return fmt.Errorf("Upstox option mapping is not registered yet for %s", instrumentID)
	}

	chain, err := m.chainFor(ctx, market, parsed.Expiry)
	if err != nil {
		return err
	}
	key, ok := chain[dynamicOptionLegKey(parsed.Strike, parsed.Side)]
	if !ok {
		return fmt.Errorf("Dhan option chain has no %s %.2f leg for %s", parsed.Side, parsed.Strike, instrumentID)
	}

	if err := m.registry.EnsureProvider(symbol.ProviderInstrument{
		Provider:     dhan.ProviderName,
		InstrumentID: instrumentID,
		ProviderKey:  key.String(),
	}); err != nil {
		return fmt.Errorf("register Dhan dynamic option mapping: %w", err)
	}
	if err := m.authority.SetPolicy(instrumentID, []authority.Preference{
		{Provider: upstox.ProviderName, Priority: 10, MaxStaleness: m.maxStaleness},
		{Provider: dhan.ProviderName, Priority: 20, MaxStaleness: m.maxStaleness},
	}); err != nil {
		return fmt.Errorf("register dynamic option authority: %w", err)
	}
	m.quoteKeys.Ensure(key)

	m.mu.Lock()
	m.covered[instrumentID] = true
	m.mu.Unlock()
	return nil
}

func (m *dhanDynamicOptionMapper) chainFor(
	ctx context.Context,
	market dynamicDhanMarket,
	expiry string,
) (map[string]dhan.InstrumentKey, error) {
	cacheKey := market.underlyingID + "|" + expiry
	m.mu.RLock()
	cached := m.chainCache[cacheKey]
	m.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	if err := m.waitForChainRateLimit(ctx); err != nil {
		return nil, err
	}
	legs, err := m.chain.Chain(
		ctx,
		m.accessToken,
		m.clientID,
		market.underlyingKey,
		expiry,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve Dhan option chain for %s: %w", market.underlyingID, err)
	}

	resolved := make(map[string]dhan.InstrumentKey, len(legs))
	for _, leg := range legs {
		if leg.SecurityID <= 0 {
			continue
		}
		side := strings.ToUpper(strings.TrimSpace(leg.Side))
		if side != "CE" && side != "PE" {
			continue
		}
		resolved[dynamicOptionLegKey(leg.Strike, side)] = dhan.InstrumentKey{
			ExchangeSegment: market.optionSegment,
			SecurityID:      leg.SecurityID,
			Instrument:      "OPTIDX",
		}
	}
	if len(resolved) == 0 {
		return nil, errors.New("Dhan option chain did not contain usable index-option security ids")
	}

	m.mu.Lock()
	if existing := m.chainCache[cacheKey]; existing != nil {
		resolved = existing
	} else {
		m.chainCache[cacheKey] = resolved
	}
	m.mu.Unlock()
	return resolved, nil
}

func (m *dhanDynamicOptionMapper) waitForChainRateLimit(ctx context.Context) error {
	m.mu.Lock()
	now := m.now().UTC()
	wait := time.Duration(0)
	if !m.lastRequest.IsZero() {
		elapsed := now.Sub(m.lastRequest)
		if elapsed < dhanOptionChainMinInterval {
			wait = dhanOptionChainMinInterval - elapsed
		}
	}
	m.mu.Unlock()

	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	m.mu.Lock()
	m.lastRequest = m.now().UTC()
	m.mu.Unlock()
	return nil
}

func (m *dhanDynamicOptionMapper) market(parsed dynamicOptionID) (dynamicDhanMarket, bool) {
	candidates := m.markets[parsed.Exchange]
	underlying := normalizeUnderlyingAlias(parsed.Underlying)
	for _, market := range candidates {
		if market.aliases[underlying] {
			return market, true
		}
	}
	return dynamicDhanMarket{}, false
}

func parseDynamicOptionID(instrumentID string) (dynamicOptionID, bool) {
	parts := strings.Split(strings.TrimSpace(instrumentID), ":")
	if len(parts) != 5 {
		return dynamicOptionID{}, false
	}
	expiry := strings.TrimSpace(parts[2])
	if _, err := time.Parse("2006-01-02", expiry); err != nil {
		return dynamicOptionID{}, false
	}
	strike, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err != nil || strike <= 0 {
		return dynamicOptionID{}, false
	}
	side := strings.ToUpper(strings.TrimSpace(parts[4]))
	if side != "CE" && side != "PE" {
		return dynamicOptionID{}, false
	}
	exchange := strings.ToUpper(strings.TrimSpace(parts[0]))
	underlying := strings.TrimSpace(parts[1])
	if exchange == "" || underlying == "" {
		return dynamicOptionID{}, false
	}
	return dynamicOptionID{
		Exchange:   exchange,
		Underlying: underlying,
		Expiry:     expiry,
		Strike:     strike,
		Side:       side,
	}, true
}

func normalizeUnderlyingAlias(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func dhanOptionSegment(exchange string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "NSE":
		return "NSE_FNO", true
	case "BSE":
		return "BSE_FNO", true
	default:
		return "", false
	}
}

func dynamicOptionLegKey(strike float64, side string) string {
	return strconv.FormatFloat(strike, 'f', -1, 64) + "|" + strings.ToUpper(strings.TrimSpace(side))
}
