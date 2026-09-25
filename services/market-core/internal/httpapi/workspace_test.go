package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func TestWorkspaceSymbolsAndCalendarEndpoints(t *testing.T) {
	registry := symbol.NewRegistry()
	for _, instrument := range []symbol.Instrument{
		{
			ID:         "NSE:NIFTY50",
			Symbol:     "NIFTY",
			Name:       "Nifty 50",
			AssetClass: "INDEX",
			Exchange:   "NSE",
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
			CalendarID: "NSE_EQ",
			Visible:    true,
		},
		{
			ID:         "QNEXT:NIFTY-SYN",
			Symbol:     "NIFTY-SYN",
			Name:       "QNext Nifty Synthetic",
			AssetClass: "INDEX",
			Exchange:   "QNEXT",
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
			CalendarID: "NSE_EQ",
			Synthetic:  true,
			Visible:    true,
		},
		{
			ID:         "NIFTY:25000:CE",
			Symbol:     "NIFTY:25000:CE",
			Name:       "NIFTY:25000:CE",
			AssetClass: "OPTION",
			Exchange:   "NSE",
			Currency:   "INR",
			Timezone:   "Asia/Kolkata",
			CalendarID: "NSE_EQ",
			Visible:    false,
		},
	} {
		if err := registry.Register(instrument); err != nil {
			t.Fatal(err)
		}
	}

	handler := New(nil, Options{
		Symbols:           registry,
		Calendars:         marketcalendar.DefaultRegistry(),
		ChartTimeframes:   []string{"15s", "30s", "1m", "2m", "3m", "5m", "15m", "30m", "1h", "1D"},
	})

	symbolRequest := httptest.NewRequest(http.MethodGet, "/api/v1/symbols", nil)
	symbolRecorder := httptest.NewRecorder()
	handler.ServeHTTP(symbolRecorder, symbolRequest)
	if symbolRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected symbols status %d: %s", symbolRecorder.Code, symbolRecorder.Body.String())
	}

	var symbols symbolsResponse
	if err := json.Unmarshal(symbolRecorder.Body.Bytes(), &symbols); err != nil {
		t.Fatal(err)
	}
	if len(symbols.Symbols) != 2 {
		t.Fatalf("expected only two visible workspace symbols, got %+v", symbols.Symbols)
	}
	if symbols.Symbols[0].Ticker != "NIFTY" || symbols.Symbols[1].Ticker != "NIFTY-SYN" {
		t.Fatalf("unexpected symbol ordering: %+v", symbols.Symbols)
	}

	timeframeRequest := httptest.NewRequest(http.MethodGet, "/api/v1/timeframes", nil)
	timeframeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(timeframeRecorder, timeframeRequest)
	if timeframeRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected timeframes status %d: %s", timeframeRecorder.Code, timeframeRecorder.Body.String())
	}
	var timeframes timeframesResponse
	if err := json.Unmarshal(timeframeRecorder.Body.Bytes(), &timeframes); err != nil {
		t.Fatal(err)
	}
	if len(timeframes.Timeframes) != 10 || timeframes.Timeframes[2] != "1m" || timeframes.Timeframes[9] != "1D" {
		t.Fatalf("unexpected enabled timeframes: %+v", timeframes.Timeframes)
	}

	from := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	query := url.Values{}
	query.Set("instrument_id", "NSE:NIFTY50")
	query.Set("from_ms", formatMillis(from.UnixMilli()))
	query.Set("to_ms", formatMillis(to.UnixMilli()))
	query.Set("session", "regular")

	calendarRequest := httptest.NewRequest(http.MethodGet, "/api/v1/calendar?"+query.Encode(), nil)
	calendarRecorder := httptest.NewRecorder()
	handler.ServeHTTP(calendarRecorder, calendarRequest)
	if calendarRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected calendar status %d: %s", calendarRecorder.Code, calendarRecorder.Body.String())
	}

	var calendar calendarResponse
	if err := json.Unmarshal(calendarRecorder.Body.Bytes(), &calendar); err != nil {
		t.Fatal(err)
	}
	if calendar.CalendarID != "NSE_EQ" || calendar.Version != "nse-equities-2026-v1" {
		t.Fatalf("unexpected calendar metadata: %+v", calendar)
	}
	if len(calendar.Windows) != 2 {
		t.Fatalf("expected Friday and Tuesday windows around weekend + holiday, got %+v", calendar.Windows)
	}
}

func formatMillis(value int64) string {
	return strconv.FormatInt(value, 10)
}
