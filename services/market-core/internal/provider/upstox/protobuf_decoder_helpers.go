package upstox

import (
	"fmt"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

func consumeBytesField(field string, typ protowire.Type, payload []byte) ([]byte, []byte, error) {
	if typ != protowire.BytesType {
		return nil, nil, wireTypeError(field, typ)
	}
	value, n := protowire.ConsumeBytes(payload)
	if n < 0 {
		return nil, nil, protowire.ParseError(n)
	}
	return value, payload[n:], nil
}

func consumeDoubleField(field string, typ protowire.Type, payload []byte) (float64, []byte, error) {
	if typ != protowire.Fixed64Type {
		return 0, nil, wireTypeError(field, typ)
	}
	value, n := protowire.ConsumeFixed64(payload)
	if n < 0 {
		return 0, nil, protowire.ParseError(n)
	}
	return math.Float64frombits(value), payload[n:], nil
}

func consumeInt64Field(field string, typ protowire.Type, payload []byte) (int64, []byte, error) {
	if typ != protowire.VarintType {
		return 0, nil, wireTypeError(field, typ)
	}
	value, n := protowire.ConsumeVarint(payload)
	if n < 0 {
		return 0, nil, protowire.ParseError(n)
	}
	return int64(value), payload[n:], nil
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

func requestModeName(value uint64) SubscriptionMode {
	switch value {
	case 0:
		return ModeLTPC
	case 1:
		return ModeFull
	case 2:
		return ModeOptionGreeks
	case 3:
		return ModeFullD30
	default:
		return ""
	}
}

func wireTypeError(field string, typ protowire.Type) error {
	return fmt.Errorf("%s has unexpected protobuf wire type %d", field, typ)
}
