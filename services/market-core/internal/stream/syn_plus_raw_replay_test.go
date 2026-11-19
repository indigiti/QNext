package stream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/pipeline"
	"github.com/indigiti/QNext/services/market-core/internal/provider/upstox"
	"github.com/indigiti/QNext/services/market-core/internal/research"
	"github.com/indigiti/QNext/services/market-core/internal/runtime"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
	"github.com/indigiti/QNext/services/market-core/internal/volumeledger"
	"github.com/indigiti/QNext/services/market-core/internal/volumeproxy"
	"google.golang.org/protobuf/encoding/protowire"
)

type replayUpstoxFrame struct {
	Schema   string `json:"schema"`
	Provider string `json:"provider"`
	AtMS     int64  `json:"captured_at_ms"`
	Payload  []byte `json:"payload_base64"`
}

// Preserve file order. captured_at_ms is the recorder's write clock, not
// exchange time or an exact measurement of live ingest latency.
func replayUpstoxFrames(r io.Reader, client *upstox.WireClient, sink *runtime.MarketSink) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var frame replayUpstoxFrame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return fmt.Errorf("capture line %d: %w", line, err)
		}
		if frame.Schema != "QNEXT.RAW.FRAME/1" || frame.Provider != "upstox" || frame.AtMS <= 0 || len(frame.Payload) == 0 {
			return fmt.Errorf("capture line %d: invalid Upstox frame", line)
		}
		at := time.UnixMilli(frame.AtMS).UTC()
		client.Now = func() time.Time { return at }
		ticks, err := client.HandleFrame(frame.Payload)
		if err != nil {
			return fmt.Errorf("capture line %d: %w", line, err)
		}
		for _, tick := range ticks {
			if err := sink.Handle(tick); err != nil {
				return fmt.Errorf("capture line %d: %w", line, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read capture after line %d: %w", line, err)
	}
	if line == 0 {
		return fmt.Errorf("empty Upstox capture")
	}
	return nil
}

type replayFutureSource struct{ contract upstox.FutureContract }

func (s replayFutureSource) Futures(context.Context, string, string, string) ([]upstox.FutureContract, error) {
	return []upstox.FutureContract{s.contract}, nil
}

type replaySubscriber struct{}

func (replaySubscriber) Subscribe(context.Context, []string) error   { return nil }
func (replaySubscriber) Unsubscribe(context.Context, []string) error { return nil }

// This is a synthetic wire-format fixture, not a captured market incident.
func TestNiftySynPlusRawReplayPublishesLateFuturesVolume(t *testing.T) {
	const futureKey = "NSE_FO|100"
	const indexID = "NSE:NIFTY50"
	const synID = "QNEXT:NIFTY-SYN+"
	const price = 25100.0

	t.Setenv("QNEXT_SYN_PLUS_ENABLED_MARKETS", "NIFTY")
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", synID)
	t.Setenv("QNEXT_MARKET_CONFIG", "")
	at := time.Date(2026, 10, 28, 3, 45, 0, 0, time.UTC)

	registry := symbol.NewRegistry()
	router, err := volumeproxy.New(replayFutureSource{upstox.FutureContract{
		Exchange: "NSE", Expiry: "2026-10-29", InstrumentKey: futureKey,
		TradingSymbol: "NIFTY FUT 29 OCT 26", InstrumentType: "FUT", UnderlyingSymbol: "NIFTY",
	}}, "offline-test", registry, replaySubscriber{}, []volumeproxy.Market{{
		Symbol: "NIFTY", Exchange: "NSE", CalendarID: "NSE_EQ", TargetInstrumentID: indexID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.Refresh(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	normalizer, err := upstox.NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}
	var seq uint64
	client := &upstox.WireClient{
		Decoder: upstox.ProtobufDecoder{}, Normalizer: normalizer,
		NextSequence: func() uint64 { seq++; return seq },
	}
	core, err := pipeline.New(candle.New("replay-test"), nil, []string{"5s", "15s", "30s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	ledger := volumeledger.New(t.TempDir())
	broker := NewBroker(64, 16)
	handler := NewWebSocketHandler(broker).(*WebSocketHandler)
	clockDone := make(chan struct{})
	close(clockDone) // No wall-clock candle finalization in a historical test.
	sink := &runtime.MarketSink{
		Pipeline: core, VolumeProxy: router, VolumeRecorder: ledger, Publisher: broker,
		DirectInstruments: map[string]bool{indexID: true}, ClockDone: clockDone,
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	read := func() serverMessage {
		t.Helper()
		var msg serverMessage
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		return msg
	}
	if msg := read(); msg.Op != "hello" {
		t.Fatalf("expected hello, got %+v", msg)
	}
	if err := conn.WriteJSON(clientMessage{
		Op: "subscribe", Channel: "bars", Symbol: synID, Timeframe: "15s",
	}); err != nil {
		t.Fatal(err)
	}
	if msg := read(); msg.Op != "subscribed" {
		t.Fatalf("expected subscribed, got %+v", msg)
	}
	if err := handler.shadow.observe(research.SynPlusSnapshot{
		Schema: research.SynPlusSnapshotSchema, InstrumentID: synID, Version: "replay-test",
		SnapshotAtMS: at.Add(time.Second).UnixMilli(), Quality: "GOOD", Value: price,
	}); err != nil {
		t.Fatal(err)
	}
	assertBar := func(msg serverMessage, volume float64, status string) {
		t.Helper()
		bar := msg.Bar
		if msg.Op != "update" || msg.Symbol != synID || msg.Timeframe != "15s" ||
			bar == nil || bar.Time != at.UnixMilli() || bar.Volume != volume ||
			bar.VolumeStatus != status || bar.Open != price || bar.High != price ||
			bar.Low != price || bar.Close != price {
			t.Fatalf("unexpected SYN+ update: %+v bar=%+v; want volume=%v status=%s", msg, bar, volume, status)
		}
	}
	assertBar(read(), 0, "UNKNOWN")

	var capture bytes.Buffer
	encoder := json.NewEncoder(&capture)
	for i, cumulative := range []uint64{1000, 1012, 1012, 1030} {
		eventMS := at.Add(time.Duration(i+2) * time.Second).UnixMilli()
		if err := encoder.Encode(replayUpstoxFrame{
			Schema: "QNEXT.RAW.FRAME/1", Provider: "upstox", AtMS: eventMS + 5,
			Payload: replayFuturesPayload(futureKey, uint64(eventMS), cumulative),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := replayUpstoxFrames(&capture, client, sink); err != nil {
		t.Fatal(err)
	}
	entries, err := ledger.Read(indexID, at, at.Add(15*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Quantity != 12 || entries[1].Quantity != 18 ||
		entries[0].CumulativeVolume != 1012 || entries[1].CumulativeVolume != 1030 {
		t.Fatalf("unexpected futures ledger: %+v", entries)
	}
	first, second := read(), read()
	assertBar(first, 12, "OBSERVED")
	assertBar(second, 30, "OBSERVED")
	if second.Seq <= first.Seq || second.Bar.Revision <= first.Bar.Revision ||
		second.Bar.VolumeSource != "FUTURES_PROXY" {
		t.Fatalf("expected advancing futures-proxy revision, got %+v then %+v", first.Bar, second.Bar)
	}
}

func replayFuturesPayload(key string, ms, cumulative uint64) []byte {
	uintField := func(dst []byte, field protowire.Number, value uint64) []byte {
		return protowire.AppendVarint(protowire.AppendTag(dst, field, protowire.VarintType), value)
	}
	bytesField := func(dst []byte, field protowire.Number, value []byte) []byte {
		return protowire.AppendBytes(protowire.AppendTag(dst, field, protowire.BytesType), value)
	}
	ltpc := protowire.AppendFixed64(protowire.AppendTag(nil, 1, protowire.Fixed64Type), math.Float64bits(25096))
	ltpc = uintField(ltpc, 2, ms)
	market := uintField(bytesField(nil, 1, ltpc), 6, cumulative)
	feed := uintField(bytesField(nil, 2, bytesField(nil, 1, market)), 4, 1)
	entry := bytesField(bytesField(nil, 1, []byte(key)), 2, feed)
	return uintField(bytesField(uintField(nil, 1, 1), 2, entry), 3, ms)
}
