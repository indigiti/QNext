package upstox

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestGorillaDialerUsesBinaryFrames(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()

		messageType, payload, err := connection.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.BinaryMessage {
			return
		}
		_ = connection.WriteMessage(websocket.BinaryMessage, payload)
	}))
	defer server.Close()

	uri := "ws" + strings.TrimPrefix(server.URL, "http")
	connection, err := (GorillaDialer{}).Dial(context.Background(), uri)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	want := []byte{1, 2, 3, 4}
	if err := connection.WriteBinary(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := connection.ReadBinary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected payload: %v", got)
	}
}
