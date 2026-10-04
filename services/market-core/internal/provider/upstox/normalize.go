package upstox

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/observability"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

const ProviderName = "upstox"

type LTPC struct {
	LTP float64 `json:"ltp"`
	LTT string  `json:"ltt"`
	LTQ string  `json:"ltq,omitempty"`
	CP  float64 `json:"cp,omitempty"`
}

type Quote struct {
	BidQ int64   `json:"bid_q"`
	BidP float64 `json:"bid_p"`
	AskQ int64   `json:"ask_q"`
	AskP float64 `json:"ask_p"`
}

type OptionGreeks struct {
	Delta float64 `json:"delta"`
	Theta float64 `json:"theta"`
	Gamma float64 `json:"gamma"`
	Vega  float64 `json:"vega"`
	Rho   float64 `json:"rho"`
}

type MarketState struct {
	LTPC         *LTPC         `json:"ltpc,omitempty"`
	Depth        []Quote       `json:"depth,omitempty"`
	OptionGreeks *OptionGreeks `json:"option_greeks,omitempty"`
	ATP          float64       `json:"atp,omitempty"`
	VTT          int64         `json:"vtt,omitempty"`
	OI           float64       `json:"oi,omitempty"`
	IV           float64       `json:"iv,omitempty"`
	TBQ          float64       `json:"tbq,omitempty"`
	TSQ          float64       `json:"tsq,omitempty"`
}

type Feed struct {
	LTPC                 *LTPC            `json:"ltpc,omitempty"`
	FullFeed             *MarketState     `json:"full_feed,omitempty"`
	FirstLevelWithGreeks *MarketState     `json:"first_level_with_greeks,omitempty"`
	RequestMode          SubscriptionMode `json:"request_mode,omitempty"`
}

func (f Feed) ResearchState() (MarketState, bool) {
	if f.FullFeed != nil {
		return cloneMarketState(*f.FullFeed), true
	}
	if f.FirstLevelWithGreeks != nil {
		return cloneMarketState(*f.FirstLevelWithGreeks), true
	}
	if f.LTPC != nil {
		return MarketState{LTPC: cloneLTPC(f.LTPC)}, true
	}
	return MarketState{}, false
}

func cloneMarketState(state MarketState) MarketState {
	state.LTPC = cloneLTPC(state.LTPC)
	if state.Depth != nil {
		state.Depth = append([]Quote(nil), state.Depth...)
	}
	if state.OptionGreeks != nil {
		copyGreeks := *state.OptionGreeks
		state.OptionGreeks = &copyGreeks
	}
	return state
}

func cloneLTPC(ltpc *LTPC) *LTPC {
	if ltpc == nil {
		return nil
	}
	copyLTPC := *ltpc
	return &copyLTPC
}

type DecodedEnvelope struct {
	Type      string          `json:"type"`
	Feeds     map[string]Feed `json:"feeds"`
	CurrentTS string          `json:"currentTs"`
}

type Normalizer struct {
	registry *symbol.Registry

	volumeMu sync.Mutex
	volume   map[string]tradeVolumeState

	microprice atomic.Uint64
	midpoint   atomic.Uint64
	ltpOption  atomic.Uint64
}

type tradeVolumeState struct {
	session        string
	cumulativeVTT  int64
	cumulativeSeen bool
	lastTradeMS    int64
}

type OptionQuoteStats struct {
	Microprice  uint64 `json:"microprice"`
	Midpoint    uint64 `json:"midpoint"`
	LTPFallback uint64 `json:"ltp_fallback"`
}

type optionPriceSource uint8

const (
	optionPriceLTP optionPriceSource = iota
	optionPriceMicroprice
	optionPriceMidpoint
)

func NewNormalizer(registry *symbol.Registry) (*Normalizer, error) {
	if registry == nil {
		return nil, errors.New("symbol registry is required")
	}
	normalizer := &Normalizer{
		registry: registry,
		volume:   make(map[string]tradeVolumeState),
	}
	observability.SetQuotePricingSource(func() any { return normalizer.QuoteStats() })
	return normalizer, nil
}

func (n *Normalizer) NormalizeEnvelope(
	envelope DecodedEnvelope,
	processedAt time.Time,
	nextSequence func() uint64,
) ([]domain.Tick, error) {
	switch envelope.Type {
	case "", "initial_feed", "live_feed":
	default:
		return nil, errors.New("unsupported Upstox message type")
	}
	if nextSequence == nil {
		return nil, errors.New("sequence generator is required")
	}

	marketTime, err := parseMillis(envelope.CurrentTS)
	if err != nil {
		return nil, errors.New("invalid Upstox currentTs")
	}
	if processedAt.Before(marketTime) {
		return nil, errors.New("processed time precedes provider receive time")
	}

	keys := make([]string, 0, len(envelope.Feeds))
	for key := range envelope.Feeds {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	ticks := make([]domain.Tick, 0, len(keys))
	for _, providerKey := range keys {
		feed := envelope.Feeds[providerKey]
		if feed.LTPC == nil {
			continue
		}

		instrument, ok := n.registry.ResolveProviderKey(ProviderName, providerKey)
		if !ok {
			return nil, errors.New("unregistered Upstox provider key: " + providerKey)
		}

		price, source := normalizedPrice(instrument.AssetClass, feed)
		quoteDriven := source != optionPriceLTP
		if price <= 0 {
			return nil, errors.New("invalid Upstox price for " + providerKey)
		}
		if strings.EqualFold(strings.TrimSpace(instrument.AssetClass), "OPTION") {
			n.recordOptionPriceSource(source)
		}

		tradeTime, tradeErr := parseMillis(feed.LTPC.LTT)
		if tradeErr != nil && !quoteDriven {
			return nil, errors.New("invalid Upstox ltt for " + providerKey)
		}

		quantity, quantityErr := n.normalizedQuantity(
			instrument.ID,
			feed,
			marketTime,
			tradeTime,
		)
		if quantityErr != nil {
			return nil, errors.New("invalid Upstox volume for " + providerKey)
		}

		ticks = append(ticks, domain.Tick{
			InstrumentID:  instrument.ID,
			Provider:      ProviderName,
			Price:         price,
			Quantity:      quantity,
			EventTime:     marketTime,
			TradeTime:     tradeTime,
			ReceivedTime:  marketTime,
			ProcessedTime: processedAt.UTC(),
			Sequence:      nextSequence(),
			Quality:       domain.QualityGood,
		})
	}
	return ticks, nil
}

func (n *Normalizer) normalizedQuantity(
	instrumentID string,
	feed Feed,
	marketTime time.Time,
	tradeTime time.Time,
) (float64, error) {
	ltq, err := parseOptionalNonNegativeFloat(feed.LTPC.LTQ)
	if err != nil {
		return 0, err
	}

	var cumulativeVTT int64
	hasVTT := false
	if feed.FullFeed != nil && feed.FullFeed.VTT > 0 {
		cumulativeVTT = feed.FullFeed.VTT
		hasVTT = true
	} else if feed.FirstLevelWithGreeks != nil && feed.FirstLevelWithGreeks.VTT > 0 {
		cumulativeVTT = feed.FirstLevelWithGreeks.VTT
		hasVTT = true
	}

	session := marketTime.In(time.FixedZone("IST", 5*60*60+30*60)).Format("2006-01-02")

	n.volumeMu.Lock()
	defer n.volumeMu.Unlock()

	state := n.volume[instrumentID]
	if state.session != session {
		state = tradeVolumeState{session: session}
	}

	// VTT is cumulative exchange-traded volume for the session. Once available,
	// use only its positive delta. The first observation establishes a baseline,
	// and decreases (session reset/reconnect/provider correction) reset that
	// baseline instead of fabricating a large candle volume.
	if hasVTT {
		if !state.cumulativeSeen || cumulativeVTT < state.cumulativeVTT {
			state.cumulativeVTT = cumulativeVTT
			state.cumulativeSeen = true
			if !tradeTime.IsZero() {
				state.lastTradeMS = tradeTime.UnixMilli()
			}
			n.volume[instrumentID] = state
			return 0, nil
		}
		delta := cumulativeVTT - state.cumulativeVTT
		state.cumulativeVTT = cumulativeVTT
		state.cumulativeSeen = true
		if !tradeTime.IsZero() {
			state.lastTradeMS = tradeTime.UnixMilli()
		}
		n.volume[instrumentID] = state
		return float64(delta), nil
	}

	// LTQ is only a fallback for feeds without VTT. Upstox can repeat the same
	// last-trade fields across multiple market updates; count LTQ once per LTT
	// so candle volume is not multiplied by quote/update frequency.
	if ltq <= 0 || tradeTime.IsZero() {
		n.volume[instrumentID] = state
		return 0, nil
	}
	tradeMS := tradeTime.UnixMilli()
	if tradeMS == state.lastTradeMS {
		n.volume[instrumentID] = state
		return 0, nil
	}
	state.lastTradeMS = tradeMS
	n.volume[instrumentID] = state
	return ltq, nil
}

func (n *Normalizer) recordOptionPriceSource(source optionPriceSource) {
	switch source {
	case optionPriceMicroprice:
		n.microprice.Add(1)
	case optionPriceMidpoint:
		n.midpoint.Add(1)
	default:
		n.ltpOption.Add(1)
	}
}

func (n *Normalizer) QuoteStats() OptionQuoteStats {
	if n == nil {
		return OptionQuoteStats{}
	}
	return OptionQuoteStats{
		Microprice:  n.microprice.Load(),
		Midpoint:    n.midpoint.Load(),
		LTPFallback: n.ltpOption.Load(),
	}
}

func normalizedPrice(assetClass string, feed Feed) (float64, optionPriceSource) {
	if !strings.EqualFold(strings.TrimSpace(assetClass), "OPTION") {
		if feed.LTPC == nil {
			return 0, optionPriceLTP
		}
		return feed.LTPC.LTP, optionPriceLTP
	}

	state, ok := feed.ResearchState()
	if ok && len(state.Depth) > 0 {
		if price, ok := quoteMicroprice(state.Depth[0]); ok {
			return price, optionPriceMicroprice
		}
		if price, ok := quoteMidpoint(state.Depth[0]); ok {
			return price, optionPriceMidpoint
		}
	}
	if feed.LTPC == nil {
		return 0, optionPriceLTP
	}
	return feed.LTPC.LTP, optionPriceLTP
}

func quoteMicroprice(quote Quote) (float64, bool) {
	if !validTwoSidedQuote(quote) || quote.BidQ <= 0 || quote.AskQ <= 0 {
		return 0, false
	}
	total := float64(quote.BidQ + quote.AskQ)
	if total <= 0 {
		return 0, false
	}
	price := (quote.AskP*float64(quote.BidQ) + quote.BidP*float64(quote.AskQ)) / total
	if price < quote.BidP || price > quote.AskP {
		return 0, false
	}
	return price, true
}

func quoteMidpoint(quote Quote) (float64, bool) {
	if !validTwoSidedQuote(quote) {
		return 0, false
	}
	return (quote.BidP + quote.AskP) / 2, true
}

func validTwoSidedQuote(quote Quote) bool {
	return quote.BidP > 0 && quote.AskP > 0 && quote.AskP >= quote.BidP
}

func parseMillis(raw string) (time.Time, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return time.Time{}, errors.New("invalid millisecond timestamp")
	}
	return time.UnixMilli(value).UTC(), nil
}

func parseOptionalNonNegativeFloat(raw string) (float64, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 {
		return 0, errors.New("invalid numeric value")
	}
	return value, nil
}
