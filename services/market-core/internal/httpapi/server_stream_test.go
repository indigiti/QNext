package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/indigiti/QNext/services/market-core/internal/stream"
)

func TestServerMountsRealtimeStream(t *testing.T) {
	handler := New(nil, Options{
		StreamHandler: stream.NewWebSocketHandler(stream.NewBroker(8, 8)),
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	connection, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/stream",
		http.Header{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	var hello map[string]any
	if err := connection.ReadJSON(&hello); err != nil {
		t.Fatal(err)
	}
	if hello["op"] != "hello" || hello["protocol"] != "QNEXT.STREAM/1" {
		t.Fatalf("unexpected hello: %+v", hello)
	}
}
