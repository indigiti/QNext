package upstox

import (
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestProtobufDecoderDecodesLTPCFeed(t *testing.T) {
	payload := makeFeedResponse(
		1,
		1740729566039,
		"NSE_INDEX|Nifty 50",
		219.3,
		1740729552723,
		75,
		494.05,
	)

	envelope, err := (ProtobufDecoder{}).Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Type != "live_feed" || envelope.CurrentTS != "1740729566039" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}

	feed, ok := envelope.Feeds["NSE_INDEX|Nifty 50"]
	if !ok || feed.LTPC == nil {
		t.Fatalf("missing LTPC feed: %+v", envelope.Feeds)
	}
	if feed.LTPC.LTP != 219.3 || feed.LTPC.LTT != "1740729552723" || feed.LTPC.LTQ != "75" || feed.LTPC.CP != 494.05 {
		t.Fatalf("unexpected LTPC: %+v", feed.LTPC)
	}
}

func TestProtobufDecoderRecognizesMarketInfo(t *testing.T) {
	var payload []byte
	payload = protowire.AppendTag(payload, 1, protowire.VarintType)
	payload = protowire.AppendVarint(payload, 2)
	payload = protowire.AppendTag(payload, 3, protowire.VarintType)
	payload = protowire.AppendVarint(payload, 1732775008661)

	envelope, err := (ProtobufDecoder{}).Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Type != "market_info" {
		t.Fatalf("unexpected type: %q", envelope.Type)
	}
}

func makeFeedResponse(
	feedType uint64,
	currentTS uint64,
	key string,
	ltp float64,
	ltt uint64,
	ltq uint64,
	cp float64,
) []byte {
	var ltpc []byte
	ltpc = protowire.AppendTag(ltpc, 1, protowire.Fixed64Type)
	ltpc = protowire.AppendFixed64(ltpc, math.Float64bits(ltp))
	ltpc = protowire.AppendTag(ltpc, 2, protowire.VarintType)
	ltpc = protowire.AppendVarint(ltpc, ltt)
	ltpc = protowire.AppendTag(ltpc, 3, protowire.VarintType)
	ltpc = protowire.AppendVarint(ltpc, ltq)
	ltpc = protowire.AppendTag(ltpc, 4, protowire.Fixed64Type)
	ltpc = protowire.AppendFixed64(ltpc, math.Float64bits(cp))

	var feed []byte
	feed = protowire.AppendTag(feed, 1, protowire.BytesType)
	feed = protowire.AppendBytes(feed, ltpc)

	var entry []byte
	entry = protowire.AppendTag(entry, 1, protowire.BytesType)
	entry = protowire.AppendString(entry, key)
	entry = protowire.AppendTag(entry, 2, protowire.BytesType)
	entry = protowire.AppendBytes(entry, feed)

	var response []byte
	response = protowire.AppendTag(response, 1, protowire.VarintType)
	response = protowire.AppendVarint(response, feedType)
	response = protowire.AppendTag(response, 2, protowire.BytesType)
	response = protowire.AppendBytes(response, entry)
	response = protowire.AppendTag(response, 3, protowire.VarintType)
	response = protowire.AppendVarint(response, currentTS)
	return response
}
