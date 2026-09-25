package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type fakeHistory struct {
	bars []domain.Bar
}

func (f fakeHistory) LoadRange(string, string, time.Time, time.Time) ([]domain.Bar, error) {
	return f.bars, nil
}

func TestBarsEndpoint(t *testing.T) {
	at := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	handler := New(fakeHistory{bars: []domain.Bar{{
		InstrumentID:      "NSE:NIFTY50",
		Timeframe:         "30s",
		OpenTime:          at,
		Open:              25100,
		High:              25105,
		Low:               25099,
		Close:             25101,
		Final:             true,
		Quality:           domain.QualityGood,
		AuthorityProvider: "fixture",
	}}}, Options{Version: "test", Commit: "abc", StartedAt: at})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/bars?instrument_id=NSE%3ANIFTY50&timeframe=30s&from_ms=1790221500000&to_ms=1790221560000", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload barsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Bars) != 1 || payload.Bars[0].Close != 25101 || payload.Bars[0].Time != at.UnixMilli() {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestBarsEndpointRejectsInvalidQuery(t *testing.T) {
	handler := New(fakeHistory{}, Options{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/bars", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestFeedStatusEndpoint(t *testing.T) {
	handler := New(fakeHistory{}, Options{
		FeedStatus: func() any {
			return map[string]any{
				"live_configured":     true,
				"nifty_instrument_id": "NSE:NIFTY50",
			}
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/feed-status", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["live_configured"] != true || payload["nifty_instrument_id"] != "NSE:NIFTY50" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

type fakeLiveBars struct {
	bar domain.Bar
	ok  bool
}

func (f fakeLiveBars) LatestBar(string, string) (domain.Bar, bool) {
	return f.bar, f.ok
}

func TestBarsEndpointIncludesCurrentFormingBar(t *testing.T) {
	at := time.Date(2026, 9, 25, 8, 30, 0, 0, time.UTC)
	historyBar := domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "15s",
		OpenTime:    at.Add(-15 * time.Second),
		Open:        23118,
		High:        23120,
		Low:         23117,
		Close:       23119,
		Final:       true,
		Quality:     domain.QualityGood,
	}
	forming := domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "15s",
		OpenTime:    at,
		Open:        23119,
		High:        23124,
		Low:         23118.5,
		Close:       23122.7,
		Final:       false,
		Quality:     domain.QualityGood,
	}

	handler := New(fakeHistory{bars: []domain.Bar{historyBar}}, Options{
		LiveBars: fakeLiveBars{bar: forming, ok: true},
	})
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/bars?instrument_id=NSE%3ANIFTY50&timeframe=15s&from_ms="+
			strconv.FormatInt(at.Add(-time.Minute).UnixMilli(), 10)+
			"&to_ms="+strconv.FormatInt(at.Add(time.Minute).UnixMilli(), 10),
		nil,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload barsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Bars) != 2 {
		t.Fatalf("expected history + forming bar, got %+v", payload.Bars)
	}
	got := payload.Bars[1]
	if got.Time != at.UnixMilli() || got.Close != 23122.7 || got.Final {
		t.Fatalf("unexpected forming bar: %+v", got)
	}
}
