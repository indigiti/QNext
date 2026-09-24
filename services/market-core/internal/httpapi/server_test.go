package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
