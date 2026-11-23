package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyFailsClosedWithoutRuntimeAndPreservesLiveness(t *testing.T) {
	handler := New(fakeHistory{}, Options{})
	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusServiceUnavailable { t.Fatalf("unwired /ready returned %d", ready.Code) }
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK { t.Fatalf("liveness returned %d", health.Code) }
}

func TestReadyReportsRuntimeReasonsAndMethodSafety(t *testing.T) {
	handler := New(fakeHistory{}, Options{Readiness: func() (bool, []string) {
		return false, []string{"market_feed_stale", "history_persistence_error"}
	}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusServiceUnavailable { t.Fatalf("readiness code %d", response.Code) }
	var payload struct { Status string `json:"status"`; Reasons []string `json:"reasons"` }
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
	if payload.Status != "not_ready" || len(payload.Reasons) != 2 { t.Fatalf("unexpected readiness body: %+v", payload) }
	method := httptest.NewRecorder()
	handler.ServeHTTP(method, httptest.NewRequest(http.MethodPost, "/ready", nil))
	if method.Code != http.StatusMethodNotAllowed { t.Fatalf("POST readiness returned %d", method.Code) }
	okHandler := New(fakeHistory{}, Options{Readiness: func() (bool, []string) { return true, nil }})
	ok := httptest.NewRecorder()
	okHandler.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ok.Code != http.StatusOK { t.Fatalf("ready runtime returned %d", ok.Code) }
}
