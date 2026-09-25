package dhan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseInstrumentKey(t *testing.T) {
	k, err := ParseInstrumentKey("IDX_I|13|INDEX")
	if err != nil {
		t.Fatal(err)
	}
	if k.SecurityID != 13 || k.String() != "IDX_I|13|INDEX" {
		t.Fatalf("key=%+v", k)
	}
}
func TestQuoteClientFetch(t *testing.T) {
	now := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("access-token") != "token" || r.Header.Get("client-id") != "client" {
			t.Fatal("missing Dhan auth headers")
		}
		var body map[string][]int64
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body["IDX_I"]) != 1 || body["IDX_I"][0] != 13 {
			t.Fatalf("body=%v", body)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"IDX_I":{"13":{"last_price":25001.25}}}}`))
	}))
	defer srv.Close()
	c := QuoteClient{URL: srv.URL, HTTPClient: srv.Client(), Now: func() time.Time { return now }}
	rows, err := c.Fetch(context.Background(), "token", "client", []InstrumentKey{{ExchangeSegment: "IDX_I", SecurityID: 13, Instrument: "INDEX"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Price != 25001.25 || !rows[0].ObservedAt.Equal(now) {
		t.Fatalf("rows=%+v", rows)
	}
}
func TestHistoryClientAggregatesThreeMinuteBars(t *testing.T) {
	base := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"open": []float64{100, 101, 102}, "high": []float64{102, 103, 105}, "low": []float64{99, 100, 101}, "close": []float64{101, 102, 104}, "volume": []float64{10, 20, 30}, "timestamp": []int64{base.Unix(), base.Add(time.Minute).Unix(), base.Add(2 * time.Minute).Unix()}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	c := HistoryClient{URL: srv.URL, HTTPClient: srv.Client()}
	rows, err := c.Fetch(context.Background(), "token", InstrumentKey{ExchangeSegment: "IDX_I", SecurityID: 13, Instrument: "INDEX"}, "3m", base, base.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%+v", rows)
	}
	r := rows[0]
	if r.Open != 100 || r.High != 105 || r.Low != 99 || r.Close != 104 || r.Volume != 60 {
		t.Fatalf("bar=%+v", r)
	}
}


func TestOptionChainClientResolvesSecurityIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("access-token") != "token" || r.Header.Get("client-id") != "client" {
			t.Fatal("missing Dhan option-chain auth headers")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["UnderlyingSeg"] != "IDX_I" || body["Expiry"] != "2026-09-29" {
			t.Fatalf("unexpected option-chain request: %+v", body)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"last_price":25110,"oc":{"25100.000000":{"ce":{"security_id":42529},"pe":{"security_id":42530}}}}}`))
	}))
	defer srv.Close()

	client := OptionChainClient{URL: srv.URL, HTTPClient: srv.Client()}
	legs, err := client.Chain(
		context.Background(),
		"token",
		"client",
		InstrumentKey{ExchangeSegment: "IDX_I", SecurityID: 13, Instrument: "INDEX"},
		"2026-09-29",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(legs) != 2 {
		t.Fatalf("expected CE+PE option-chain legs, got %+v", legs)
	}
	seen := map[string]int64{}
	for _, leg := range legs {
		seen[leg.Side] = leg.SecurityID
		if leg.Strike != 25100 {
			t.Fatalf("unexpected strike: %+v", leg)
		}
	}
	if seen["CE"] != 42529 || seen["PE"] != 42530 {
		t.Fatalf("unexpected security ids: %+v", seen)
	}
}

func TestKeyRegistryExpandsDeterministically(t *testing.T) {
	registry := NewKeyRegistry([]InstrumentKey{
		{ExchangeSegment: "IDX_I", SecurityID: 13, Instrument: "INDEX"},
	})
	registry.Ensure(
		InstrumentKey{ExchangeSegment: "NSE_FNO", SecurityID: 42530, Instrument: "OPTIDX"},
		InstrumentKey{ExchangeSegment: "NSE_FNO", SecurityID: 42529, Instrument: "OPTIDX"},
		InstrumentKey{ExchangeSegment: "NSE_FNO", SecurityID: 42529, Instrument: "OPTIDX"},
	)

	keys := registry.Snapshot()
	if len(keys) != 3 {
		t.Fatalf("expected deduplicated dynamic keys, got %+v", keys)
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1].String() >= keys[i].String() {
			t.Fatalf("key snapshot must be deterministic and sorted: %+v", keys)
		}
	}
}
