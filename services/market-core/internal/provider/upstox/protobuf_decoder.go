package upstox

import (
	"errors"
	"strconv"

	"google.golang.org/protobuf/encoding/protowire"
)

// ProtobufDecoder decodes the Upstox Market Data Feed V3 subset used by QNext.
// Unknown fields are skipped so the decoder stays forward-compatible with
// additive changes in the upstream protobuf contract.
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

		case 2:
			if typ != protowire.BytesType {
				return Feed{}, wireTypeError("Feed.fullFeed", typ)
			}
			value, m := protowire.ConsumeBytes(payload)
			if m < 0 {
				return Feed{}, protowire.ParseError(m)
			}
			state, err := parseFullFeed(value)
			if err != nil {
				return Feed{}, err
			}
			feed.FullFeed = &state
			feed.LTPC = cloneLTPC(state.LTPC)
			payload = payload[m:]

		case 3:
			if typ != protowire.BytesType {
				return Feed{}, wireTypeError("Feed.firstLevelWithGreeks", typ)
			}
			value, m := protowire.ConsumeBytes(payload)
			if m < 0 {
				return Feed{}, protowire.ParseError(m)
			}
			state, err := parseFirstLevelWithGreeks(value)
			if err != nil {
				return Feed{}, err
			}
			feed.FirstLevelWithGreeks = &state
			feed.LTPC = cloneLTPC(state.LTPC)
			payload = payload[m:]

		case 4:
			if typ != protowire.VarintType {
				return Feed{}, wireTypeError("Feed.requestMode", typ)
			}
			value, m := protowire.ConsumeVarint(payload)
			if m < 0 {
				return Feed{}, protowire.ParseError(m)
			}
			feed.RequestMode = requestModeName(value)
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
