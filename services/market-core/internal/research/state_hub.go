package research

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
)

type ResearchStateEntry struct {
	State     upstox.MarketState
	UpdatedAt time.Time
}

type ResearchStateUpdate struct {
	SourceMS    int64
	SourceTime  time.Time
	Plan        upstox.ResearchPlan
	Spot        float64
	SpotUpdated bool
	Updates     map[string]ResearchStateEntry
}

type ResearchStateConsumer interface {
	ObserveState(ResearchStateUpdate) error
}

// ResearchStateHub normalizes rich Upstox state once and fans the normalized
// update out to research consumers. It also retains the latest rich state for
// future research consumers without coupling them to the production feed.
type ResearchStateHub struct {
	mu sync.Mutex

	spot      float64
	state     map[string]ResearchStateEntry
	consumers []ResearchStateConsumer
}

func NewResearchStateHub(consumers ...ResearchStateConsumer) *ResearchStateHub {
	return &ResearchStateHub{
		state:     make(map[string]ResearchStateEntry),
		consumers: append([]ResearchStateConsumer(nil), consumers...),
	}
}

func (h *ResearchStateHub) Observe(envelope upstox.DecodedEnvelope, plan upstox.ResearchPlan) error {
	sourceMS, err := strconv.ParseInt(envelope.CurrentTS, 10, 64)
	if err != nil || sourceMS <= 0 {
		if envelope.Type == "market_info" {
			return nil
		}
		return errors.New("research envelope has invalid currentTs")
	}
	sourceTime := time.UnixMilli(sourceMS).UTC()

	h.mu.Lock()
	defer h.mu.Unlock()

	for key := range h.state {
		if _, selected := plan.Contracts[key]; !selected {
			delete(h.state, key)
		}
	}

	update := ResearchStateUpdate{
		SourceMS:   sourceMS,
		SourceTime: sourceTime,
		Plan:       plan,
		Spot:       h.spot,
		Updates:    make(map[string]ResearchStateEntry),
	}
	if feed, ok := envelope.Feeds[plan.UnderlyingKey]; ok && feed.LTPC != nil && feed.LTPC.LTP > 0 {
		h.spot = feed.LTPC.LTP
		update.Spot = h.spot
		update.SpotUpdated = true
	}

	for providerKey := range plan.Contracts {
		feed, ok := envelope.Feeds[providerKey]
		if !ok {
			continue
		}
		market, ok := feed.ResearchState()
		if !ok {
			continue
		}
		entry := ResearchStateEntry{State: market, UpdatedAt: sourceTime}
		h.state[providerKey] = entry
		update.Updates[providerKey] = entry
	}

	for _, consumer := range h.consumers {
		if consumer == nil {
			continue
		}
		if err := consumer.ObserveState(update); err != nil {
			return err
		}
	}
	return nil
}

func (h *ResearchStateHub) Snapshot() (float64, map[string]ResearchStateEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := make(map[string]ResearchStateEntry, len(h.state))
	for key, entry := range h.state {
		state[key] = entry
	}
	return h.spot, state
}
