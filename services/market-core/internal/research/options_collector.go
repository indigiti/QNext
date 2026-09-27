package research

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
)

const OptionSnapshotSchema = "QNEXT.RESEARCH.OPTION_SNAPSHOT/1"

type OptionSnapshot struct {
	Schema            string         `json:"schema"`
	Interval          string         `json:"interval"`
	SnapshotAtMS      int64          `json:"snapshot_at_ms"`
	SourceCurrentTSMS int64          `json:"source_current_ts_ms"`
	Provider          string         `json:"provider"`
	ProviderKey       string         `json:"provider_key"`
	Underlying        string         `json:"underlying"`
	UnderlyingKey     string         `json:"underlying_key"`
	UnderlyingPrice   float64        `json:"underlying_price"`
	Expiry            string         `json:"expiry"`
	Strike            float64        `json:"strike"`
	Side              string         `json:"side"`
	TradingSymbol     string         `json:"trading_symbol,omitempty"`
	LTP               float64        `json:"ltp,omitempty"`
	LTTMS             int64          `json:"ltt_ms,omitempty"`
	LTQ               int64          `json:"ltq,omitempty"`
	ClosePrice        float64        `json:"close_price,omitempty"`
	Bid               float64        `json:"bid,omitempty"`
	BidQty            int64          `json:"bid_qty,omitempty"`
	Ask               float64        `json:"ask,omitempty"`
	AskQty            int64          `json:"ask_qty,omitempty"`
	Depth             []upstox.Quote `json:"depth,omitempty"`
	Mid               float64        `json:"mid,omitempty"`
	Spread            float64        `json:"spread,omitempty"`
	MicroPrice        float64        `json:"microprice,omitempty"`
	DepthImbalance    float64        `json:"depth_imbalance,omitempty"`
	Volume            int64          `json:"volume,omitempty"`
	OI                float64        `json:"oi,omitempty"`
	IV                float64        `json:"iv,omitempty"`
	Delta             float64        `json:"delta,omitempty"`
	Gamma             float64        `json:"gamma,omitempty"`
	Theta             float64        `json:"theta,omitempty"`
	Vega              float64        `json:"vega,omitempty"`
	Rho               float64        `json:"rho,omitempty"`
	ATP               float64        `json:"atp,omitempty"`
	TotalBidQty       float64        `json:"total_bid_qty,omitempty"`
	TotalAskQty       float64        `json:"total_ask_qty,omitempty"`
}

type OptionsCollector struct {
	root string
	mu   sync.Mutex

	spot       float64
	state      map[string]OptionSnapshot
	buckets    map[time.Duration]time.Time
	lastSource int64
	closed     bool
}

func NewOptionsCollector(root string) (*OptionsCollector, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("research storage root is required")
	}
	return &OptionsCollector{
		root:  root,
		state: make(map[string]OptionSnapshot),
		buckets: map[time.Duration]time.Time{
			time.Second:      {},
			15 * time.Second: {},
		},
	}, nil
}

func (c *OptionsCollector) Observe(envelope upstox.DecodedEnvelope, plan upstox.ResearchPlan) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("research collector is closed")
	}

	sourceMS, err := strconv.ParseInt(envelope.CurrentTS, 10, 64)
	if err != nil || sourceMS <= 0 {
		if envelope.Type == "market_info" {
			return nil
		}
		return errors.New("research envelope has invalid currentTs")
	}
	sourceTime := time.UnixMilli(sourceMS).UTC()
	if err := c.rollBuckets(sourceTime); err != nil {
		return err
	}
	c.lastSource = sourceMS

	for key := range c.state {
		if _, selected := plan.Contracts[key]; !selected {
			delete(c.state, key)
		}
	}

	if feed, ok := envelope.Feeds[plan.UnderlyingKey]; ok && feed.LTPC != nil && feed.LTPC.LTP > 0 {
		c.spot = feed.LTPC.LTP
		for key, snapshot := range c.state {
			snapshot.UnderlyingPrice = c.spot
			c.state[key] = snapshot
		}
	}

	for providerKey, contract := range plan.Contracts {
		feed, ok := envelope.Feeds[providerKey]
		if !ok {
			continue
		}
		market, ok := feed.ResearchState()
		if !ok {
			continue
		}
		snapshot := c.state[providerKey]
		snapshot.Schema = OptionSnapshotSchema
		snapshot.Provider = upstox.ProviderName
		snapshot.ProviderKey = providerKey
		snapshot.Underlying = contract.UnderlyingSymbol
		snapshot.UnderlyingKey = plan.UnderlyingKey
		snapshot.UnderlyingPrice = c.spot
		snapshot.Expiry = contract.Expiry
		snapshot.Strike = contract.StrikePrice
		snapshot.Side = normalizeSide(contract.InstrumentType)
		snapshot.TradingSymbol = contract.TradingSymbol
		snapshot.SourceCurrentTSMS = sourceMS
		applyMarketState(&snapshot, market)
		c.state[providerKey] = snapshot
	}
	return nil
}

func (c *OptionsCollector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	for _, interval := range []time.Duration{time.Second, 15 * time.Second} {
		bucket := c.buckets[interval]
		if bucket.IsZero() || len(c.state) == 0 {
			continue
		}
		if err := c.flush(interval, bucket.Add(interval)); err != nil {
			return err
		}
	}
	return nil
}

func (c *OptionsCollector) rollBuckets(sourceTime time.Time) error {
	for _, interval := range []time.Duration{time.Second, 15 * time.Second} {
		bucket := sourceTime.Truncate(interval)
		previous := c.buckets[interval]
		if previous.IsZero() {
			c.buckets[interval] = bucket
			continue
		}
		if !bucket.After(previous) {
			continue
		}
		if len(c.state) > 0 {
			if err := c.flush(interval, previous.Add(interval)); err != nil {
				return err
			}
		}
		c.buckets[interval] = bucket
	}
	return nil
}

func (c *OptionsCollector) flush(interval time.Duration, snapshotAt time.Time) error {
	if len(c.state) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.state))
	for key := range c.state {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	for _, key := range keys {
		snapshot := c.state[key]
		snapshot.Schema = OptionSnapshotSchema
		snapshot.Interval = intervalLabel(interval)
		snapshot.SnapshotAtMS = snapshotAt.UnixMilli()
		if snapshot.SourceCurrentTSMS == 0 {
			snapshot.SourceCurrentTSMS = c.lastSource
		}
		if err := encoder.Encode(snapshot); err != nil {
			return err
		}
	}

	path := filepath.Join(
		c.root,
		"research",
		"options",
		intervalLabel(interval),
		snapshotAt.Format("2006"),
		snapshotAt.Format("01"),
		snapshotAt.Format("2006-01-02")+".jsonl",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	defer file.Close()
	written, err := io.Copy(file, &buffer)
	if err != nil {
		return err
	}
	if written <= 0 {
		return io.ErrShortWrite
	}
	return file.Sync()
}

func applyMarketState(snapshot *OptionSnapshot, state upstox.MarketState) {
	if state.LTPC != nil {
		snapshot.LTP = state.LTPC.LTP
		snapshot.ClosePrice = state.LTPC.CP
		if value, err := strconv.ParseInt(state.LTPC.LTT, 10, 64); err == nil {
			snapshot.LTTMS = value
		}
		if value, err := strconv.ParseInt(state.LTPC.LTQ, 10, 64); err == nil {
			snapshot.LTQ = value
		}
	}
	if state.Depth != nil {
		snapshot.Depth = append(snapshot.Depth[:0], state.Depth...)
	} else {
		snapshot.Depth = nil
		snapshot.Bid = 0
		snapshot.BidQty = 0
		snapshot.Ask = 0
		snapshot.AskQty = 0
		snapshot.Mid = 0
		snapshot.Spread = 0
		snapshot.MicroPrice = 0
		snapshot.DepthImbalance = 0
	}
	if len(snapshot.Depth) > 0 {
		top := snapshot.Depth[0]
		snapshot.Bid = top.BidP
		snapshot.BidQty = top.BidQ
		snapshot.Ask = top.AskP
		snapshot.AskQty = top.AskQ
		if top.BidP > 0 && top.AskP > 0 {
			snapshot.Mid = (top.BidP + top.AskP) / 2
			snapshot.Spread = top.AskP - top.BidP
		}
		qty := top.BidQ + top.AskQ
		if qty > 0 && top.BidP > 0 && top.AskP > 0 {
			snapshot.MicroPrice = (top.AskP*float64(top.BidQ) + top.BidP*float64(top.AskQ)) / float64(qty)
		}
		var bidDepth, askDepth int64
		for _, quote := range snapshot.Depth {
			bidDepth += quote.BidQ
			askDepth += quote.AskQ
		}
		if bidDepth+askDepth > 0 {
			snapshot.DepthImbalance = float64(bidDepth-askDepth) / float64(bidDepth+askDepth)
		}
	}
	snapshot.Volume = state.VTT
	snapshot.OI = state.OI
	snapshot.IV = state.IV
	snapshot.ATP = state.ATP
	snapshot.TotalBidQty = state.TBQ
	snapshot.TotalAskQty = state.TSQ
	if state.OptionGreeks != nil {
		snapshot.Delta = state.OptionGreeks.Delta
		snapshot.Gamma = state.OptionGreeks.Gamma
		snapshot.Theta = state.OptionGreeks.Theta
		snapshot.Vega = state.OptionGreeks.Vega
		snapshot.Rho = state.OptionGreeks.Rho
	}
}

func normalizeSide(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "CE", "CALL":
		return "CALL"
	case "PE", "PUT":
		return "PUT"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

func intervalLabel(interval time.Duration) string {
	if interval == time.Second {
		return "1s"
	}
	if interval == 15*time.Second {
		return "15s"
	}
	return interval.String()
}
