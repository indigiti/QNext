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

type keyedHistory struct {
	bars map[string][]domain.Bar
}

func (h keyedHistory) LoadRange(instrumentID, timeframe string, _ time.Time, _ time.Time) ([]domain.Bar, error) {
	return append([]domain.Bar(nil), h.bars[instrumentID+"|"+timeframe]...), nil
}

func TestBarsEndpointOverlaysConfiguredProxyVolume(t *testing.T) {
	at := time.Date(2026, 9, 24, 3, 45, 0, 0, time.UTC)
	history := keyedHistory{bars: map[string][]domain.Bar{
		"QNEXT:NIFTY-SYN+|15s": {{
			InstrumentID: "QNEXT:NIFTY-SYN+",
			Timeframe:    "15s",
			OpenTime:     at,
			Open:         25101,
			High:         25105,
			Low:          25099,
			Close:        25103,
			Volume:       0,
			Final:        true,
			Quality:      domain.QualityGood,
		}},
		"NSE:NIFTY50|15s": {{
			InstrumentID: "NSE:NIFTY50",
			Timeframe:    "15s",
			OpenTime:     at,
			Open:         25098,
			High:         25102,
			Low:          25097,
			Close:        25100,
			Volume:       3770,
			Final:        true,
			Quality:      domain.QualityGood,
		}},
	}}

	handler := New(history, Options{
		VolumeAliases: map[string]string{
			"QNEXT:NIFTY-SYN+": "NSE:NIFTY50",
		},
	})
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/bars?instrument_id=QNEXT%3ANIFTY-SYN%2B&timeframe=15s&from_ms="+
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
	if len(payload.Bars) != 1 {
		t.Fatalf("unexpected bars: %+v", payload.Bars)
	}
	if payload.Bars[0].Volume != 3770 {
		t.Fatalf("expected mirrored NIFTY proxy volume, got %+v", payload.Bars[0])
	}
	if payload.Bars[0].Close != 25103 {
		t.Fatalf("volume overlay changed SYN+ price: %+v", payload.Bars[0])
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
		OpenTime:     at.Add(-15 * time.Second),
		Open:         23118,
		High:         23120,
		Low:          23117,
		Close:        23119,
		Final:        true,
		Quality:      domain.QualityGood,
	}
	forming := domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "15s",
		OpenTime:     at,
		Open:         23119,
		High:         23124,
		Low:          23118.5,
		Close:        23122.7,
		Final:        false,
		Quality:      domain.QualityGood,
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

func TestReadyReflectsRuntimePersistenceHealth(t *testing.T) {
	handler := New(fakeHistory{}, Options{
		Readiness: func() (bool, []string) {
			return false, []string{"history_persistence_error", "history_queue_pressure"}
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected readiness 503, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Status string `json:"status"`
		Reasons []string `json:"reasons"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "not_ready" || len(payload.Reasons) != 2 {
		t.Fatalf("unexpected readiness: %+v", payload)
	}
}

func TestReadyAllowsHealthyPersistence(t *testing.T) {
	handler := New(fakeHistory{}, Options{
		Readiness: func() (bool, []string) { return true, nil },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("healthy runtime must be ready: %d %s", response.Code, response.Body.String())
	}
}

func TestBarsRejectOversizedRangeWithoutReadingHistory(t *testing.T) {
	for _, testcase := range []struct {
		timeframe string
		days int
	}{
		{"15s", 32},
		{"1m", 401},
		{"1D", 21*365},
	} {
		handler := New(fakeHistory{}, Options{})
		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := from.Add(time.Duration(testcase.days) * 24 * time.Hour)
		path := "/api/v1/bars?instrument_id=NSE%3ANIFTY50&timeframe=" + testcase.timeframe +
			"&from_ms=" + strconv.FormatInt(from.UnixMilli(), 10) +
			"&to_ms=" + strconv.FormatInt(to.UnixMilli(), 10)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("%s: expected 413, got %d: %s", testcase.timeframe, response.Code, response.Body.String())
		}
	}
}

func TestBarsKeepsThirtyDaySubminuteRecoveryAvailable(t *testing.T) {
	handler := New(fakeHistory{}, Options{})
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(30 * 24 * time.Hour)
	path := "/api/v1/bars?instrument_id=NSE%3ANIFTY50&timeframe=15s" +
		"&from_ms=" + strconv.FormatInt(from.UnixMilli(), 10) +
		"&to_ms=" + strconv.FormatInt(to.UnixMilli(), 10)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("30-day chart recovery should remain available: %d %s", response.Code, response.Body.String())
	}
}
