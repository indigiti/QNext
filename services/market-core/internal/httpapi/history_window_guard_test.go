package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type boundedHistoryProbe struct { calls int }

func (p *boundedHistoryProbe) LoadRange(_ string, _ string, _, _ time.Time) ([]domain.Bar, error) {
	p.calls++
	return nil, nil
}

func TestBarsRejectOversizedPublicHistoryBeforeStore(t *testing.T) {
	cases := []struct {
		timeframe string
		maxDays   int
	}{
		{"5s", 31}, {"15s", 31}, {"30s", 31},
		{"1m", 400}, {"2m", 400}, {"5m", 400},
		{"1h", 1826}, {"4h", 1826},
		{"1D", 7305}, {"1W", 7305}, {"1M", 7305},
		{"malformed", 400},
	}
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		t.Run(tc.timeframe, func(t *testing.T) {
			probe := &boundedHistoryProbe{}
			handler := New(probe, Options{})
			requestURL := func(days int) string {
				return fmt.Sprintf("/api/v1/bars?instrument_id=NSE%%3ANIFTY50&timeframe=%s&from_ms=%d&to_ms=%d&limit=5000",
					tc.timeframe, from.UnixMilli(), from.AddDate(0, 0, days).UnixMilli())
			}
			rejected := httptest.NewRecorder()
			handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, requestURL(tc.maxDays+1), nil))
			if rejected.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("oversized range status=%d payload=%s", rejected.Code, rejected.Body.String())
			}
			var body struct {
				Error   string `json:"error"`
				MaxDays int    `json:"max_days"`
			}
			if err := json.Unmarshal(rejected.Body.Bytes(), &body); err != nil { t.Fatal(err) }
			if body.Error != "history_range_too_large" || body.MaxDays != tc.maxDays {
				t.Fatalf("unexpected range response: %+v", body)
			}
			if probe.calls != 0 {
				t.Fatalf("oversized query scanned history %d times", probe.calls)
			}

			allowed := httptest.NewRecorder()
			handler.ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, requestURL(tc.maxDays), nil))
			if allowed.Code != http.StatusOK || probe.calls != 1 {
				t.Fatalf("max legal request failed status=%d scans=%d payload=%s",
					allowed.Code, probe.calls, allowed.Body.String())
			}
		})
	}
}
