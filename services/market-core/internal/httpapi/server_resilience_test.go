package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResilienceStatusEndpoint(t *testing.T) {
	handler := New(nil, Options{
		ResilienceStatus: func() any {
			return map[string]any{
				"authority_switches": float64(2),
				"active_authorities": map[string]string{"NSE:NIFTY50": "dhan"},
			}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/resilience", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["authority_switches"].(float64) != 2 {
		t.Fatalf("payload=%v", payload)
	}
}

func TestResilienceStatusEndpointRejectsWriteMethods(t *testing.T) {
	handler := New(nil, Options{ResilienceStatus: func() any { return map[string]any{} }})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/resilience", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rec.Code)
	}
}
