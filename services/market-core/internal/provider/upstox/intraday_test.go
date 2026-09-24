package upstox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIntradayClientFetchesAndSortsCandles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.EscapedPath(), "NSE_INDEX") || !strings.HasSuffix(r.URL.Path, "/minutes/1") {
			t.Fatalf("unexpected path %s", r.URL.EscapedPath())
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("missing auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{"candles":[
				["2026-09-24T09:16:00+05:30",25101,25103,25100,25102,200,0],
				["2026-09-24T09:15:00+05:30",25100,25102,25099,25101,100,0]
			]}
		}`))
	}))
	defer server.Close()

	client := IntradayClient{HTTP: server.Client(), BaseURL: server.URL}
	candles, err := client.Fetch(context.Background(), "token", "NSE_INDEX|Nifty 50", "1m")
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 2 {
		t.Fatalf("expected two candles, got %d", len(candles))
	}
	if !candles[0].OpenTime.Before(candles[1].OpenTime) || candles[0].Close != 25101 {
		t.Fatalf("unexpected candles: %+v", candles)
	}
}

func TestIntradayClientRejectsSubMinuteRecovery(t *testing.T) {
	client := IntradayClient{}
	if _, err := client.Fetch(context.Background(), "token", "NSE_INDEX|Nifty 50", "30s"); err == nil {
		t.Fatal("expected sub-minute recovery to fail")
	}
}
