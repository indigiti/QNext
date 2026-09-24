package upstox

import (
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/market"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

const ProviderName = "upstox"

type LTPC struct {
	LTP float64 `json:"ltp"`
	LTT string  `json:"ltt"`
	LTQ string  `json:"ltq,omitempty"`
	CP  float64 `json:"cp,omitempty"`
}

type Feed struct {
	LTPC *LTPC `json:"ltpc,omitempty"`
}

type DecodedEnvelope struct {
	Type      string          `json:"type"`
	Feeds     map[string]Feed `json:"feeds"`
	CurrentTS string          `json:"currentTs"`
}

type Normalizer struct {
	registry *symbol.Registry
}

func NewNormalizer(registry *symbol.Registry) (*Normalizer, error) {
	if registry == nil {
		return nil, errors.New("symbol registry is required")
	}
	return &Normalizer{registry: registry}, nil
}

func (n *Normalizer) NormalizeEnvelope(
	envelope DecodedEnvelope,
	processedAt time.Time,
	nextSequence func() uint64,
) ([]market.CanonicalTick, error) {
	if envelope.Type != "" && envelope.Type != "live_feed" {
		return nil, errors.New("unsupported Upstox message type")
	}
	if nextSequence == nil {
		return nil, errors.New("sequence generator is required")
	}

	receivedTime, err := parseMillis(envelope.CurrentTS)
	if err != nil {
		return nil, errors.New("invalid Upstox currentTs")
	}
	if processedAt.Before(receivedTime) {
		return nil, errors.New("processed time precedes provider receive time")
	}

	keys := make([]string, 0, len(envelope.Feeds))
	for key := range envelope.Feeds {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	ticks := make([]market.CanonicalTick, 0, len(keys))
	for _, providerKey := range keys {
		feed := envelope.Feeds[providerKey]
		if feed.LTPC == nil {
			continue
		}

		instrument, ok := n.registry.ResolveProviderKey(ProviderName, providerKey)
		if !ok {
			return nil, errors.New("unregistered Upstox provider key: " + providerKey)
		}

		eventTime, err := parseMillis(feed.LTPC.LTT)
		if err != nil {
			return nil, errors.New("invalid Upstox ltt for " + providerKey)
		}

		lastQuantity, err := parseOptionalFloat(feed.LTPC.LTQ)
		if err != nil {
			return nil, errors.New("invalid Upstox ltq for " + providerKey)
		}

		tick := market.CanonicalTick{
			Schema:        market.SchemaCanonicalTickV1,
			InstrumentID:  instrument.ID,
			Provider:      ProviderName,
			ProviderKey:   providerKey,
			Price:         feed.LTPC.LTP,
			LastQuantity:  lastQuantity,
			EventTime:     eventTime,
			ReceivedTime:  receivedTime,
			ProcessedTime: processedAt,
			Sequence:      nextSequence(),
			Quality:       market.QualityGood,
		}
		if err := tick.Validate(); err != nil {
			return nil, err
		}
		ticks = append(ticks, tick)
	}
	return ticks, nil
}

func parseMillis(raw string) (time.Time, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return time.Time{}, errors.New("invalid millisecond timestamp")
	}
	return time.UnixMilli(value).UTC(), nil
}

func parseOptionalFloat(raw string) (float64, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 {
		return 0, errors.New("invalid numeric value")
	}
	return value, nil
}
