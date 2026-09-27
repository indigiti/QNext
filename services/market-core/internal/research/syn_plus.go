package research

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
)

const SynPlusSnapshotSchema = "QNEXT.RESEARCH.SYNTHETIC_SNAPSHOT/1"

type SynPlusConfig struct {
	InstrumentID           string
	Version                string
	MinimumValidCandidates int
	MaxLegAge              time.Duration
	MaxLegTimeSkew         time.Duration
}

type SynPlusCandidate struct {
	Strike          float64 `json:"strike"`
	CallPrice       float64 `json:"call_price,omitempty"`
	PutPrice        float64 `json:"put_price,omitempty"`
	CallPriceSource string  `json:"call_price_source,omitempty"`
	PutPriceSource  string  `json:"put_price_source,omitempty"`
	Value           float64 `json:"value,omitempty"`
	Accepted        bool    `json:"accepted"`
	RejectionReason string  `json:"rejection_reason,omitempty"`
}

type SynPlusSnapshot struct {
	Schema                  string             `json:"schema"`
	InstrumentID            string             `json:"instrument_id"`
	Version                 string             `json:"version"`
	SnapshotAtMS            int64              `json:"snapshot_at_ms"`
	SourceCurrentTSMS       int64              `json:"source_current_ts_ms"`
	Expiry                  string             `json:"expiry"`
	ATM                     float64            `json:"atm"`
	Spot                    float64            `json:"spot"`
	Value                   float64            `json:"value,omitempty"`
	BasisToSpot             float64            `json:"basis_to_spot,omitempty"`
	Quality                 string             `json:"quality"`
	ValidCandidates         int                `json:"valid_candidates"`
	RejectedCandidates      int                `json:"rejected_candidates"`
	MicropriceLegs          int                `json:"microprice_legs"`
	MidLegs                 int                `json:"mid_legs"`
	LTPLegs                 int                `json:"ltp_legs"`
	MedianAbsoluteDeviation float64            `json:"median_absolute_deviation,omitempty"`
	Candidates              []SynPlusCandidate `json:"candidates,omitempty"`
}

type synPlusLeg struct {
	state     upstox.MarketState
	updatedAt time.Time
}

type SynPlusCollector struct {
	store     *JSONLStore
	ownsStore bool
	cfg       SynPlusConfig

	mu         sync.Mutex
	legs       map[string]synPlusLeg
	spot       float64
	lastBucket time.Time
	lastSource int64
	closed     bool
}

func NewSynPlusCollector(root string, cfg SynPlusConfig) (*SynPlusCollector, error) {
	store, err := NewJSONLStore(root)
	if err != nil {
		return nil, err
	}
	collector, err := newSynPlusCollector(store, true, cfg)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return collector, nil
}

func NewSynPlusCollectorWithStore(store *JSONLStore, cfg SynPlusConfig) (*SynPlusCollector, error) {
	return newSynPlusCollector(store, false, cfg)
}

func newSynPlusCollector(store *JSONLStore, ownsStore bool, cfg SynPlusConfig) (*SynPlusCollector, error) {
	if store == nil {
		return nil, errors.New("SYN+ storage is required")
	}
	if strings.TrimSpace(cfg.InstrumentID) == "" {
		return nil, errors.New("SYN+ instrument id is required")
	}
	if strings.TrimSpace(cfg.Version) == "" {
		return nil, errors.New("SYN+ version is required")
	}
	if cfg.MinimumValidCandidates <= 0 {
		cfg.MinimumValidCandidates = 3
	}
	if cfg.MaxLegAge <= 0 {
		cfg.MaxLegAge = 2 * time.Second
	}
	if cfg.MaxLegTimeSkew <= 0 {
		cfg.MaxLegTimeSkew = time.Second
	}
	return &SynPlusCollector{
		store:     store,
		ownsStore: ownsStore,
		cfg:       cfg,
		legs:      make(map[string]synPlusLeg),
	}, nil
}

// Observe is retained for direct/test callers. Production uses one shared
// ResearchStateHub so rich feed normalization is performed only once.
func (c *SynPlusCollector) Observe(envelope upstox.DecodedEnvelope, plan upstox.ResearchPlan) error {
	return NewResearchStateHub(c).Observe(envelope, plan)
}

func (c *SynPlusCollector) ObserveState(update ResearchStateUpdate) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("SYN+ collector is closed")
	}

	plan := update.Plan
	for key := range c.legs {
		contract, selected := plan.Contracts[key]
		if !selected || contract.Expiry != plan.CurrentExpiry {
			delete(c.legs, key)
		}
	}

	bucket := update.SourceTime.Truncate(time.Second)
	if c.lastBucket.IsZero() {
		c.lastBucket = bucket
	} else if bucket.After(c.lastBucket) {
		snapshot := c.evaluate(plan, c.lastBucket.Add(time.Second), c.lastSource)
		if err := c.persist(snapshot); err != nil {
			return err
		}
		c.lastBucket = bucket
	}

	if update.SpotUpdated && update.Spot > 0 {
		c.spot = update.Spot
	}
	for key, entry := range update.Updates {
		contract, selected := plan.Contracts[key]
		if !selected || contract.Expiry != plan.CurrentExpiry {
			continue
		}
		c.legs[key] = synPlusLeg{state: entry.State, updatedAt: entry.UpdatedAt}
	}
	c.lastSource = update.SourceMS
	return nil
}

func (c *SynPlusCollector) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	if c.ownsStore {
		return c.store.Close()
	}
	return nil
}

func (c *SynPlusCollector) evaluate(plan upstox.ResearchPlan, at time.Time, sourceMS int64) SynPlusSnapshot {
	spot := c.spot
	if spot <= 0 {
		spot = plan.Spot
	}
	snapshot := SynPlusSnapshot{
		Schema:            SynPlusSnapshotSchema,
		InstrumentID:      c.cfg.InstrumentID,
		Version:           c.cfg.Version,
		SnapshotAtMS:      at.UnixMilli(),
		SourceCurrentTSMS: sourceMS,
		Expiry:            plan.CurrentExpiry,
		ATM:               plan.ATM,
		Spot:              spot,
		Quality:           "DEGRADED",
	}

	type pair struct {
		callKey string
		putKey  string
	}
	pairs := make(map[float64]pair)
	for key, contract := range plan.Contracts {
		if contract.Expiry != plan.CurrentExpiry {
			continue
		}
		entry := pairs[contract.StrikePrice]
		switch strings.ToUpper(strings.TrimSpace(contract.InstrumentType)) {
		case "CE", "CALL":
			entry.callKey = key
		case "PE", "PUT":
			entry.putKey = key
		}
		pairs[contract.StrikePrice] = entry
	}

	strikes := make([]float64, 0, len(pairs))
	for strike := range pairs {
		strikes = append(strikes, strike)
	}
	sort.Float64s(strikes)
	values := make([]float64, 0, len(strikes))

	for _, strike := range strikes {
		entry := pairs[strike]
		candidate := SynPlusCandidate{Strike: strike}
		call, callOK := c.legs[entry.callKey]
		put, putOK := c.legs[entry.putKey]
		switch {
		case entry.callKey == "" || entry.putKey == "" || !callOK || !putOK:
			candidate.RejectionReason = "MISSING_LEG"
		case at.Sub(call.updatedAt) > c.cfg.MaxLegAge || at.Sub(put.updatedAt) > c.cfg.MaxLegAge:
			candidate.RejectionReason = "STALE_LEG"
		case absDuration(call.updatedAt.Sub(put.updatedAt)) > c.cfg.MaxLegTimeSkew:
			candidate.RejectionReason = "LEG_TIME_SKEW"
		default:
			callPrice, callSource, okCall := synPlusFairPrice(call.state)
			putPrice, putSource, okPut := synPlusFairPrice(put.state)
			if !okCall || !okPut {
				candidate.RejectionReason = "NO_FAIR_PRICE"
				break
			}
			candidate.Accepted = true
			candidate.CallPrice = callPrice
			candidate.PutPrice = putPrice
			candidate.CallPriceSource = callSource
			candidate.PutPriceSource = putSource
			candidate.Value = strike + callPrice - putPrice
			values = append(values, candidate.Value)
			countPriceSource(&snapshot, callSource)
			countPriceSource(&snapshot, putSource)
		}
		if candidate.Accepted {
			snapshot.ValidCandidates++
		} else {
			snapshot.RejectedCandidates++
		}
		snapshot.Candidates = append(snapshot.Candidates, candidate)
	}

	if len(values) < c.cfg.MinimumValidCandidates {
		return snapshot
	}

	snapshot.Value = median(values)
	snapshot.BasisToSpot = snapshot.Value - snapshot.Spot
	deviations := make([]float64, 0, len(values))
	for _, value := range values {
		deviations = append(deviations, math.Abs(value-snapshot.Value))
	}
	snapshot.MedianAbsoluteDeviation = median(deviations)
	snapshot.Quality = "GOOD"
	return snapshot
}

func synPlusFairPrice(state upstox.MarketState) (float64, string, bool) {
	if len(state.Depth) > 0 {
		quote := state.Depth[0]
		if quote.BidP > 0 && quote.AskP > 0 && quote.AskP >= quote.BidP {
			quantity := quote.BidQ + quote.AskQ
			if quantity > 0 {
				microprice := (quote.AskP*float64(quote.BidQ) + quote.BidP*float64(quote.AskQ)) / float64(quantity)
				if microprice > 0 {
					return microprice, "MICROPRICE", true
				}
			}
			return (quote.BidP + quote.AskP) / 2, "MID", true
		}
	}
	if state.LTPC != nil && state.LTPC.LTP > 0 {
		return state.LTPC.LTP, "LTP", true
	}
	return 0, "", false
}

func countPriceSource(snapshot *SynPlusSnapshot, source string) {
	switch source {
	case "MICROPRICE":
		snapshot.MicropriceLegs++
	case "MID":
		snapshot.MidLegs++
	case "LTP":
		snapshot.LTPLegs++
	}
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	mid := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[mid]
	}
	return (copyValues[mid-1] + copyValues[mid]) / 2
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func (c *SynPlusCollector) persist(snapshot SynPlusSnapshot) error {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(snapshot); err != nil {
		return err
	}
	at := time.UnixMilli(snapshot.SnapshotAtMS).UTC()
	path := filepath.Join(
		"research",
		"synthetic",
		"1s",
		at.Format("2006"),
		at.Format("01"),
		at.Format("2006-01-02")+".jsonl",
	)
	return c.store.Append(path, buffer.Bytes())
}
