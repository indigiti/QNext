package upstox

import (
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestProtobufDecoderDecodesFullResearchFeed(t *testing.T) {
	payload := makeRichFeedResponse("NSE_FO|TEST", 1760000000123)
	envelope, err := (ProtobufDecoder{}).Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	feed := envelope.Feeds["NSE_FO|TEST"]
	state, ok := feed.ResearchState()
	if !ok || state.LTPC == nil {
		t.Fatalf("missing research state: %+v", feed)
	}
	if feed.RequestMode != ModeFull || state.LTPC.LTP != 123.5 || len(state.Depth) != 2 {
		t.Fatalf("unexpected rich feed: %+v", feed)
	}
	if state.OI != 4567 || state.IV != 0.1825 || state.VTT != 98765 {
		t.Fatalf("unexpected market fields: %+v", state)
	}
	if state.OptionGreeks == nil || state.OptionGreeks.Delta != 0.51 || state.OptionGreeks.Rho != 0.04 {
		t.Fatalf("unexpected greeks: %+v", state.OptionGreeks)
	}
	if state.Depth[0].BidP != 123.4 || state.Depth[0].AskP != 123.6 || state.Depth[0].BidQ != 75 || state.Depth[0].AskQ != 50 {
		t.Fatalf("unexpected depth: %+v", state.Depth)
	}
}

func makeRichFeedResponse(key string, currentTS uint64) []byte {
	ltpc := richLTPC(123.5, currentTS-100, 25, 120.0)
	level := richAppendQuote(nil, 75, 123.4, 50, 123.6)
	level = richAppendQuote(level, 120, 123.3, 90, 123.7)
	greeks := richAppendDouble(nil, 1, 0.51)
	greeks = richAppendDouble(greeks, 2, -4.2)
	greeks = richAppendDouble(greeks, 3, 0.0012)
	greeks = richAppendDouble(greeks, 4, 11.3)
	greeks = richAppendDouble(greeks, 5, 0.04)

	var market []byte
	market = richAppendBytes(market, 1, ltpc)
	market = richAppendBytes(market, 2, level)
	market = richAppendBytes(market, 3, greeks)
	market = richAppendDouble(market, 5, 122.8)
	market = richAppendInt64(market, 6, 98765)
	market = richAppendDouble(market, 7, 4567)
	market = richAppendDouble(market, 8, 0.1825)
	market = richAppendDouble(market, 9, 12000)
	market = richAppendDouble(market, 10, 9800)

	var full []byte
	full = richAppendBytes(full, 1, market)
	var feed []byte
	feed = richAppendBytes(feed, 2, full)
	feed = richAppendInt64(feed, 4, 1)
	var entry []byte
	entry = protowire.AppendTag(entry, 1, protowire.BytesType)
	entry = protowire.AppendString(entry, key)
	entry = richAppendBytes(entry, 2, feed)
	var response []byte
	response = richAppendInt64(response, 1, 1)
	response = richAppendBytes(response, 2, entry)
	response = richAppendInt64(response, 3, int64(currentTS))
	return response
}

func richLTPC(ltp float64, ltt uint64, ltq uint64, cp float64) []byte {
	var payload []byte
	payload = richAppendDouble(payload, 1, ltp)
	payload = richAppendInt64(payload, 2, int64(ltt))
	payload = richAppendInt64(payload, 3, int64(ltq))
	payload = richAppendDouble(payload, 4, cp)
	return payload
}

func richAppendQuote(payload []byte, bidQ int64, bidP float64, askQ int64, askP float64) []byte {
	var quote []byte
	quote = richAppendInt64(quote, 1, bidQ)
	quote = richAppendDouble(quote, 2, bidP)
	quote = richAppendInt64(quote, 3, askQ)
	quote = richAppendDouble(quote, 4, askP)
	return richAppendBytes(payload, 1, quote)
}

func richAppendBytes(payload []byte, field protowire.Number, value []byte) []byte {
	payload = protowire.AppendTag(payload, field, protowire.BytesType)
	return protowire.AppendBytes(payload, value)
}

func richAppendDouble(payload []byte, field protowire.Number, value float64) []byte {
	payload = protowire.AppendTag(payload, field, protowire.Fixed64Type)
	return protowire.AppendFixed64(payload, math.Float64bits(value))
}

func richAppendInt64(payload []byte, field protowire.Number, value int64) []byte {
	payload = protowire.AppendTag(payload, field, protowire.VarintType)
	return protowire.AppendVarint(payload, uint64(value))
}
