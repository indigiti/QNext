package okx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestOKXTradeSocketSendsPingAndProcessesPongAndTrades(t *testing.T) {
	var sentPing atomic.Bool
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			return
		}
		if req["op"] != "subscribe" {
			t.Errorf("missing subscription: %v", req)
			return
		}
		_ = conn.WriteJSON(map[string]any{"event": "subscribe", "arg": map[string]string{"channel": "trades-all", "instId": "XAU-USD_UM_XPERP"}})
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if string(msg) == "ping" {
				sentPing.Store(true)
				_ = conn.WriteMessage(websocket.TextMessage, []byte("pong"))
				trade := map[string]any{"arg": map[string]string{"channel": "trades-all", "instId": "XAU-USD_UM_XPERP"},
					"data": []map[string]string{{"tradeId": "1", "px": "4000.2", "sz": "7", "ts": "1795435200000"}}}
				raw, _ := json.Marshal(trade)
				_ = conn.WriteMessage(websocket.TextMessage, raw)
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	gotTick := make(chan domain.Tick, 1)
	subscribed := make(chan struct{}, 1)
	stream := Stream{Endpoint: "ws" + strings.TrimPrefix(server.URL, "http"),
		HeartbeatInterval: 40 * time.Millisecond, ReadTimeout: time.Second}
	done := make(chan error, 1)
	go func() {
		done <- stream.once(ctx, Contract{InstID: "XAU-USD_UM_XPERP", ContractValue: 0.001},
			func(id string, tick domain.Tick) error {
				if id != "1" {
					t.Errorf("wrong trade ID %q", id)
				}
				gotTick <- tick
				return nil
			}, func() { subscribed <- struct{}{} })
	}()
	select {
	case <-subscribed:
	case <-ctx.Done():
		t.Fatal("subscribe acknowledgement absent")
	}
	select {
	case tick := <-gotTick:
		if tick.InstrumentID != InstrumentID || tick.Price != 4000.2 || tick.Quantity != 0.007 {
			t.Fatalf("incorrect tick %+v", tick)
		}
		if !sentPing.Load() {
			t.Fatal("no idle heartbeat sent")
		}
	case <-ctx.Done():
		t.Fatal("no trade after OKX pong")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream ignored cancellation")
	}
}
