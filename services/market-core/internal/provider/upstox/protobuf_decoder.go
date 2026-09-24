package upstox

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"google.golang.org/protobuf/encoding/protowire"
)

// ProtobufDecoder decodes the LTPC subset of Upstox Market Data Feed V3.
//
// QNext intentionally requests LTPC mode for the first NIFTY/NIFTY-SYN stream.
// Unknown protobuf fields are skipped so the decoder remains forward-compatible
// with richer feed payloads that are not part of this first slice.
type ProtobufDecoder struct{}

func (ProtobufDecoder) Decode(payload []byte) (DecodedEnvelope, error) {
	if len(payload) == 0 {
		return DecodedEnvelope{}, errors.New("empty Upstox protobuf frame")
	}

	envelope := DecodedEnvelope{
		Type:  "initial_feed",
		Feeds: make(map[string]Feed),
	}

	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return DecodedEnvelope{}, protowire.ParseError(n)
		}
		payload = payload[n:]

		switch num {
		case 1:
			if typ != protowire.VarintType {
				return DecodedEnvelope{}, wireTypeError("FeedResponse.type", typ)
			}
			value, m := protowire.ConsumeVarint(payload)
			if m < 0 {
				return DecodedEnvelope{}, protowire.ParseError(m)
			}
			envelope.Type = feedTypeName(value)
			payload = payload[m:]

		case 2:
			if typ != protowire.BytesType {
				return DecodedEnvelope{}, wireTypeError("FeedResponse.feeds", typ)
			}
			entry, m := protowire.ConsumeBytes(payload)
			if m < 0 {
				return DecodedEnvelope{}, protowire.ParseError(m)
			}
			key, feed, err := parseFeedEntry(entry)
			if err != nil {
				return DecodedEnvelope{}, err
			}
			envelope.Feeds[key] = feed
			payload = payload[m:]

		case 3:
			if typ != protowire.VarintType {
				return DecodedEnvelope{}, wireTypeError("FeedResponse.currentTs", typ)
			}
			value, m := protowire.ConsumeVarint(payload)
			if m < 0 {
				return DecodedEnvelope{}, protowire.ParseError(m)
			}
			envelope.CurrentTS = strconv.FormatUint(value, 10)
			payload = payload[m:]

		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return DecodedEnvelope{}, err
			}
			payload = rest
		}
	}

	return envelope, nil
}

func parseFeedEntry(payload []byte) (string, Feed, error) {
	var key string
	var feed Feed

	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return "", Feed{}, protowire.ParseError(n)
		}
		payload = payload[n:]

		switch num {
		case 1:
			if typ != protowire.BytesType {
				return "", Feed{}, wireTypeError("feeds.key", typ)
			}
			value, m := protowire.ConsumeString(payload)
			if m < 0 {
				return "", Feed{}, protowire.ParseError(m)
			}
			key = value
			payload = payload[m:]

		case 2:
			if typ != protowire.BytesType {
				return "", Feed{}, wireTypeError("feeds.value", typ)
			}
			value, m := protowire.ConsumeBytes(payload)
			if m < 0 {
				return "", Feed{}, protowire.ParseError(m)
			}
			parsed, err := parseFeed(value)
			if err != nil {
				return "", Feed{}, err
			}
			feed = parsed
			payload = payload[m:]

		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return "", Feed{}, err
			}
			payload = rest
		}
	}

	if key == "" {
		return "", Feed{}, errors.New("Upstox feed map entry is missing instrument key")
	}
	return key, feed, nil
}

func parseFeed(payload []byte) (Feed, error) {
	var feed Feed

	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return Feed{}, protowire.ParseError(n)
		}
		payload = payload[n:]

		switch num {
		case 1:
			if typ != protowire.BytesType {
				return Feed{}, wireTypeError("Feed.ltpc", typ)
			}
			value, m := protowire.ConsumeBytes(payload)
			if m < 0 {
				return Feed{}, protowire.ParseError(m)
			}
			ltpc, err := parseLTPC(value)
			if err != nil {
				return Feed{}, err
			}
			feed.LTPC = &ltpc
			payload = payload[m:]

		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return Feed{}, err
			}
			payload = rest
		}
	}

	return feed, nil
}

func parseLTPC(payload []byte) (LTPC, error) {
	var ltpc LTPC

	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return LTPC{}, protowire.ParseError(n)
		}
		payload = payload[n:]

		switch num {
		case 1:
			if typ != protowire.Fixed64Type {
				return LTPC{}, wireTypeError("LTPC.ltp", typ)
			}
			value, m := protowire.ConsumeFixed64(payload)
			if m < 0 {
				return LTPC{}, protowire.ParseError(m)
			}
			ltpc.LTP = math.Float64frombits(value)
			payload = payload[m:]

		case 2:
			if typ != protowire.VarintType {
				return LTPC{}, wireTypeError("LTPC.ltt", typ)
			}
			value, m := protowire.ConsumeVarint(payload)
			if m < 0 {
				return LTPC{}, protowire.ParseError(m)
			}
			ltpc.LTT = strconv.FormatUint(value, 10)
			payload = payload[m:]

		case 3:
			if typ != protowire.VarintType {
				return LTPC{}, wireTypeError("LTPC.ltq", typ)
			}
			value, m := protowire.ConsumeVarint(payload)
			if m < 0 {
				return LTPC{}, protowire.ParseError(m)
			}
			ltpc.LTQ = strconv.FormatUint(value, 10)
			payload = payload[m:]

		case 4:
			if typ != protowire.Fixed64Type {
				return LTPC{}, wireTypeError("LTPC.cp", typ)
			}
			value, m := protowire.ConsumeFixed64(payload)
			if m < 0 {
				return LTPC{}, protowire.ParseError(m)
			}
			ltpc.CP = math.Float64frombits(value)
			payload = payload[m:]

		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return LTPC{}, err
			}
			payload = rest
		}
	}

	return ltpc, nil
}

func skipField(num protowire.Number, typ protowire.Type, payload []byte) ([]byte, error) {
	n := protowire.ConsumeFieldValue(num, typ, payload)
	if n < 0 {
		return nil, protowire.ParseError(n)
	}
	return payload[n:], nil
}

func feedTypeName(value uint64) string {
	switch value {
	case 0:
		return "initial_feed"
	case 1:
		return "live_feed"
	case 2:
		return "market_info"
	default:
		return fmt.Sprintf("unknown_%d", value)
	}
}

func wireTypeError(field string, typ protowire.Type) error {
	return fmt.Errorf("%s has unexpected protobuf wire type %d", field, typ)
}
