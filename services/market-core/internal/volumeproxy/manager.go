package volumeproxy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

const ProxyProvider = "upstox-futures-proxy"

var indiaLocation = time.FixedZone("IST", 5*60*60+30*60)

type Market struct {
	Symbol             string
	Exchange           string
	CalendarID         string
	TargetInstrumentID string
}

type Subscriber interface {
	Subscribe(context.Context, []string) error
	Unsubscribe(context.Context, []string) error
}

type Manager struct {
	mu sync.Mutex

	Source      upstox.FutureContractSource
	AccessToken string
	Registry    *symbol.Registry
	Subscriber  Subscriber
	Markets     []Market

	byInstrument   map[string]binding
	targets        map[string]*targetState
	providerKeys   map[string]bool
	lastRefreshDay string
}

type binding struct {
	targetID    string
	providerKey string
	expiry      string
}

type liquidity struct {
	seen             bool
	cumulativeVolume float64
	openInterest     float64
}

type targetState struct {
	currentID string
	nextID    string
	activeID  string
	current   liquidity
	next      liquidity
	session   string
}

func New(
	source upstox.FutureContractSource,
	accessToken string,
	registry *symbol.Registry,
	subscriber Subscriber,
	markets []Market,
) (*Manager, error) {
	if source == nil || registry == nil || subscriber == nil {
		return nil, errors.New("volume proxy requires source, registry, and subscriber")
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("volume proxy requires Upstox access token")
	}
	return &Manager{
		Source:       source,
		AccessToken:  accessToken,
		Registry:     registry,
		Subscriber:   subscriber,
		Markets:      append([]Market(nil), markets...),
		byInstrument: make(map[string]binding),
		targets:      make(map[string]*targetState),
		providerKeys: make(map[string]bool),
	}, nil
}

func (m *Manager) Refresh(ctx context.Context, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	today := at.In(indiaLocation).Format("2006-01-02")

	nextBindings := make(map[string]binding)
	nextTargets := make(map[string]*targetState)
	nextProviderKeys := make(map[string]bool)
	var refreshErr error

	for _, market := range m.Markets {
		contracts, err := m.Source.Futures(
			ctx,
			m.AccessToken,
			market.Symbol,
			market.Exchange,
		)
		if err != nil {
			refreshErr = errors.Join(refreshErr, fmt.Errorf("%s futures: %w", market.Symbol, err))
			continue
		}
		selected := selectLiveFutures(contracts, market.Symbol, today)
		if len(selected) == 0 {
			refreshErr = errors.Join(refreshErr, fmt.Errorf("%s futures: no live contracts", market.Symbol))
			continue
		}

		ids := make([]string, 0, len(selected))
		for _, contract := range selected {
			instrumentID := canonicalFutureID(contract, market.Symbol)
			exchange := strings.ToUpper(strings.TrimSpace(contract.Exchange))
			if exchange == "" {
				exchange = strings.ToUpper(strings.TrimSpace(market.Exchange))
			}
			if err := m.Registry.Ensure(symbol.Instrument{
				ID:         instrumentID,
				Symbol:     contract.TradingSymbol,
				Name:       contract.TradingSymbol,
				AssetClass: "FUTURE",
				Exchange:   exchange,
				Currency:   "INR",
				Timezone:   "Asia/Kolkata",
				CalendarID: market.CalendarID,
				Visible:    false,
			}); err != nil {
				refreshErr = errors.Join(refreshErr, fmt.Errorf("%s future registry: %w", market.Symbol, err))
				continue
			}
			if err := m.Registry.EnsureProvider(symbol.ProviderInstrument{
				Provider:     upstox.ProviderName,
				InstrumentID: instrumentID,
				ProviderKey:  contract.InstrumentKey,
			}); err != nil {
				refreshErr = errors.Join(refreshErr, fmt.Errorf("%s future mapping: %w", market.Symbol, err))
				continue
			}

			ids = append(ids, instrumentID)
			nextBindings[instrumentID] = binding{
				targetID:    market.TargetInstrumentID,
				providerKey: contract.InstrumentKey,
				expiry:      contract.Expiry,
			}
			nextProviderKeys[contract.InstrumentKey] = true
		}
		if len(ids) == 0 {
			continue
		}

		state := &targetState{currentID: ids[0], activeID: ids[0]}
		if len(ids) > 1 {
			state.nextID = ids[1]
		}

		m.mu.Lock()
		previous := m.targets[market.TargetInstrumentID]
		m.mu.Unlock()
		if previous != nil &&
			(previous.activeID == state.currentID || previous.activeID == state.nextID) {
			state.activeID = previous.activeID
		}
		nextTargets[market.TargetInstrumentID] = state
	}

	m.mu.Lock()
	oldProviderKeys := make(map[string]bool, len(m.providerKeys))
	for key := range m.providerKeys {
		oldProviderKeys[key] = true
	}
	m.mu.Unlock()

	var added []string
	for key := range nextProviderKeys {
		if !oldProviderKeys[key] {
			added = append(added, key)
		}
	}
	var removed []string
	for key := range oldProviderKeys {
		if !nextProviderKeys[key] {
			removed = append(removed, key)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)

	if len(added) > 0 {
		if err := m.Subscriber.Subscribe(ctx, added); err != nil {
			return errors.Join(refreshErr, err)
		}
	}
	if len(removed) > 0 {
		if err := m.Subscriber.Unsubscribe(ctx, removed); err != nil {
			return errors.Join(refreshErr, err)
		}
	}

	m.mu.Lock()
	m.byInstrument = nextBindings
	m.targets = nextTargets
	m.providerKeys = nextProviderKeys
	m.lastRefreshDay = today
	m.mu.Unlock()
	return refreshErr
}

func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			day := now.In(indiaLocation).Format("2006-01-02")
			m.mu.Lock()
			last := m.lastRefreshDay
			m.mu.Unlock()
			if day != last {
				_ = m.Refresh(ctx, now.UTC())
			}
		}
	}
}

func (m *Manager) Route(tick domain.Tick) (domain.Tick, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	binding, ok := m.byInstrument[tick.InstrumentID]
	if !ok {
		return domain.Tick{}, false
	}
	state := m.targets[binding.targetID]
	if state == nil {
		return domain.Tick{}, false
	}

	session := tick.EventTime.In(indiaLocation).Format("2006-01-02")
	if state.session != session {
		state.session = session
		state.current = liquidity{}
		state.next = liquidity{}
	}

	switch tick.InstrumentID {
	case state.currentID:
		state.current = liquidity{
			seen:             true,
			cumulativeVolume: tick.CumulativeVolume,
			openInterest:     tick.OpenInterest,
		}
	case state.nextID:
		state.next = liquidity{
			seen:             true,
			cumulativeVolume: tick.CumulativeVolume,
			openInterest:     tick.OpenInterest,
		}
	}

	if state.activeID == state.currentID && shouldRoll(state.current, state.next) {
		state.activeID = state.nextID
	}
	if tick.InstrumentID != state.activeID || tick.Quantity <= 0 {
		return domain.Tick{}, false
	}

	volumeTick := tick
	volumeTick.InstrumentID = binding.targetID
	volumeTick.Provider = ProxyProvider
	volumeTick.Price = 0
	return volumeTick, true
}

func shouldRoll(current, next liquidity) bool {
	if !current.seen || !next.seen || next.cumulativeVolume <= 0 {
		return false
	}
	if current.cumulativeVolume <= 0 {
		return false
	}
	volumeDominant := next.cumulativeVolume >= current.cumulativeVolume*1.05
	oiSupported := current.openInterest <= 0 ||
		next.openInterest >= current.openInterest*0.80
	return volumeDominant && oiSupported
}

func selectLiveFutures(
	contracts []upstox.FutureContract,
	symbol string,
	today string,
) []upstox.FutureContract {
	var filtered []upstox.FutureContract
	for _, contract := range contracts {
		if !strings.EqualFold(strings.TrimSpace(contract.InstrumentType), "FUT") ||
			contract.Expiry < today ||
			!strings.EqualFold(strings.TrimSpace(contract.UnderlyingSymbol), strings.TrimSpace(symbol)) ||
			strings.TrimSpace(contract.InstrumentKey) == "" {
			continue
		}
		filtered = append(filtered, contract)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Expiry == filtered[j].Expiry {
			return filtered[i].InstrumentKey < filtered[j].InstrumentKey
		}
		return filtered[i].Expiry < filtered[j].Expiry
	})
	if len(filtered) > 2 {
		filtered = filtered[:2]
	}
	return filtered
}

func canonicalFutureID(contract upstox.FutureContract, fallbackSymbol string) string {
	exchange := strings.ToUpper(strings.TrimSpace(contract.Exchange))
	if exchange == "" {
		exchange = "NSE"
	}
	symbolValue := strings.ToUpper(strings.TrimSpace(contract.UnderlyingSymbol))
	if symbolValue == "" {
		symbolValue = strings.ToUpper(strings.TrimSpace(fallbackSymbol))
	}
	return fmt.Sprintf("%s:FUT:%s:%s", exchange, symbolValue, contract.Expiry)
}
