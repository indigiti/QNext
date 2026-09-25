package stream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketMultiplexesBarSubscriptions(t *testing.T) {
	broker := NewBroker(16, 16)
	server := httptest.NewServer(NewWebSocketHandler(broker))
	defer server.Close()

	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	readMessage := func() serverMessage {
		t.Helper()
		var message serverMessage
		if err := connection.ReadJSON(&message); err != nil {
			t.Fatal(err)
		}
		return message
	}

	if got := readMessage(); got.Op != "hello" {
		t.Fatalf("expected hello, got %+v", got)
	}

	if err := connection.WriteJSON(clientMessage{
		Op: "subscribe", Channel: "bars", Symbol: "NSE:NIFTY50", Timeframe: "1m",
	}); err != nil {
		t.Fatal(err)
	}
	first := readMessage()
	if first.Op != "subscribed" {
		t.Fatalf("unexpected first subscription response: %+v", first)
	}

	if err := connection.WriteJSON(clientMessage{
		Op: "subscribe", Channel: "bars", Symbol: "QNEXT:NIFTY-SYN", Timeframe: "1m",
	}); err != nil {
		t.Fatal(err)
	}
	second := readMessage()
	if second.Op != "subscribed" || second.StreamID == first.StreamID {
		t.Fatalf("unexpected second subscription response: %+v", second)
	}

	at := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	broker.PublishBar(bar("NSE:NIFTY50", "1m", at, 25100))
	broker.PublishBar(bar("QNEXT:NIFTY-SYN", "1m", at, 25101))

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		message := readMessage()
		if message.Op != "update" || message.Bar == nil {
			t.Fatalf("unexpected update: %+v", message)
		}
		seen[message.Symbol] = true
	}
	if !seen["NSE:NIFTY50"] || !seen["QNEXT:NIFTY-SYN"] {
		t.Fatalf("multiplexed streams missing: %+v", seen)
	}
}

func TestWebSocketResumeReplaysMissedBars(t *testing.T) {
	broker := NewBroker(8, 8)
	at := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	broker.PublishBar(bar("NSE:NIFTY50", "1m", at, 25100))
	broker.PublishBar(bar("NSE:NIFTY50", "1m", at.Add(time.Minute), 25101))

	initial, err := broker.Subscribe("NSE:NIFTY50", "1m", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamID := initial.StreamID
	initial.Cancel()

	server := httptest.NewServer(NewWebSocketHandler(broker))
	defer server.Close()
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	var message serverMessage
	if err := connection.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	after := uint64(0)
	if err := connection.WriteJSON(clientMessage{
		Op: "resume", StreamID: streamID, AfterSeq: &after,
	}); err != nil {
		t.Fatal(err)
	}

	if err := connection.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	if message.Op != "subscribed" {
		t.Fatalf("expected subscribed, got %+v", message)
	}

	for want := uint64(1); want <= 2; want++ {
		if err := connection.ReadJSON(&message); err != nil {
			t.Fatal(err)
		}
		if message.Op != "update" || message.Seq != want {
			t.Fatalf("expected replay seq %d, got %+v", want, message)
		}
	}
}


func TestSameOriginAcceptsForwardedPublicHost(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18080/api/v1/stream", nil)
	request.Host = "127.0.0.1:18080"
	request.Header.Set("Origin", "https://stage.digiti.in")
	request.Header.Set("X-Forwarded-Host", "stage.digiti.in")

	if !sameOrigin(request) {
		t.Fatal("expected forwarded public host to satisfy same-origin check")
	}
}

func TestSameOriginRejectsUnrelatedForwardedHost(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18080/api/v1/stream", nil)
	request.Host = "127.0.0.1:18080"
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("X-Forwarded-Host", "stage.digiti.in")

	if sameOrigin(request) {
		t.Fatal("expected unrelated origin to be rejected")
	}
}
