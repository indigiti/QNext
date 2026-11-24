package bitunix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestDiscoverExactCryptoIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/futures/market/trading_pairs" || r.URL.Query().Get("symbols") != "BTCUSDT" {
			t.Errorf("unexpected discovery request: %s", r.URL)
		}
		fmt.Fprint(w, `{"code":0,"data":[{"symbol":"BTCUSDT","base":"BTC","quote":"USDT","symbolStatus":"OPEN"}]}`)
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL}
	got, err := client.Discover(context.Background(), "BTCUSDT")
	if err != nil || got.ID() != "BITUNIX:BTCUSDT" {
		t.Fatalf("discovery: %+v %v", got, err)
	}
	if _, err := client.Discover(context.Background(), "XAUUSDT"); err == nil {
		t.Fatal("unreviewed metal session enabled")
	}
}

func TestClosedHistoryAndVolumeUnits(t *testing.T) {
	start := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("interval") != "1m" || q.Get("type") != "LAST_PRICE" || q.Get("endTime") != fmt.Sprint(start.Add(time.Minute).UnixMilli()) {
			t.Errorf("unexpected history query: %v", q)
		}
		fmt.Fprintf(w, `{"code":0,"data":[{"time":%d,"type":"LAST_PRICE","open":"100","high":"102","low":"99","close":"101","baseVol":"2","quoteVol":"201"}]}`, start.UnixMilli())
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL}
	c := Contract{Symbol: "BTCUSDT", Base: "BTC", Quote: "USDT"}
	bars, err := client.History(context.Background(), c, "1m", start, start.Add(time.Minute))
	if err != nil || len(bars) != 1 || bars[0].Volume != 2 || bars[0].InstrumentID != c.ID() || !bars[0].Final {
		t.Fatalf("history: %+v %v", bars, err)
	}
	if _, err := client.History(context.Background(), c, "5s", start, start.Add(time.Minute)); err == nil {
		t.Fatal("unsupported five-second recovery")
	}
	var row candleRow
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"time":%d,"type":"LAST_PRICE","open":"100","high":"102","low":"99","close":"101","baseVol":"201","quoteVol":"2"}`, start.UnixMilli())), &row); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeBar(c, row); err == nil {
		t.Fatal("ambiguous volume must not be guessed")
	}
	row.Base = json.RawMessage(`"NaN"`)
	if _, err := normalizeBar(c, row); err == nil {
		t.Fatal("non-finite volume accepted")
	}
}

func TestTradeStreamKeepsIdenticalExecutionsAndFiltersSymbols(t *testing.T) {
	upgrader := websocket.Upgrader{}
	now := time.Now().UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var subscribe struct {
			Op   string `json:"op"`
			Args []struct {
				Symbol string `json:"symbol"`
				Ch     string `json:"ch"`
			} `json:"args"`
		}
		if err := conn.ReadJSON(&subscribe); err != nil || subscribe.Op != "subscribe" || len(subscribe.Args) != 1 || subscribe.Args[0].Ch != "trade" || subscribe.Args[0].Symbol != "BTCUSDT" {
			t.Errorf("wrong subscription: %+v %v", subscribe, err)
			return
		}
		for _, symbol := range []string{"ETHUSDT", "BTCUSDT"} {
			row := map[string]string{"p": "100", "v": "0.25", "t": now.Format(time.RFC3339)}
			if err := conn.WriteJSON(map[string]any{"ch": "trade", "symbol": symbol, "data": []map[string]string{row, row}}); err != nil {
				return
			}
		}
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var got []domain.Tick
	s := Stream{Endpoint: "ws" + server.URL[4:]}
	err := s.Run(ctx, Contract{Symbol: "BTCUSDT"}, func(tick domain.Tick) error {
		got = append(got, tick)
		if len(got) == 2 {
			cancel()
		}
		return nil
	}, nil)
	if err != nil || len(got) != 2 || got[1].Sequence != 2 || got[1].InstrumentID != "BITUNIX:BTCUSDT" ||
		got[1].Quantity != 0.25 || got[1].Quality != domain.QualityDegraded {
		t.Fatalf("trade observations: %+v %v", got, err)
	}
}

func TestTradeRejectsInvalidTimeAndNumericValues(t *testing.T) {
	now := time.Now().UTC()
	row := trade{Price: json.RawMessage(`"100"`), Size: json.RawMessage(`"0.25"`), Time: json.RawMessage(`"bad"`)}
	if _, err := parseTrade(Contract{Symbol: "BTCUSDT"}, row, now); err == nil {
		t.Fatal("invalid event time accepted")
	}
	row.Time = json.RawMessage(fmt.Sprintf("%d", now.UnixMilli()))
	row.Price = json.RawMessage(`"Inf"`)
	if _, err := parseTrade(Contract{Symbol: "BTCUSDT"}, row, now); err == nil {
		t.Fatal("non-finite price accepted")
	}
}
