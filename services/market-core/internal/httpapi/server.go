package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type HistoryReader interface {
	LoadRange(instrumentID, timeframe string, from, to time.Time) ([]domain.Bar, error)
}

type Options struct {
	Version       string
	Commit        string
	StartedAt     time.Time
	StreamHandler http.Handler
	Symbols       *symbol.Registry
	Calendars        *marketcalendar.Registry
	ResilienceStatus func() any
}

type Server struct {
	history HistoryReader
	options Options
	mux     *http.ServeMux
}

type barsResponse struct {
	Bars []barResponse `json:"bars"`
}

type barResponse struct {
	Time              int64          `json:"time"`
	Open              float64        `json:"open"`
	High              float64        `json:"high"`
	Low               float64        `json:"low"`
	Close             float64        `json:"close"`
	Volume            float64        `json:"volume"`
	Final             bool           `json:"final"`
	Revision          uint32         `json:"revision"`
	Quality           domain.Quality `json:"quality"`
	AuthorityProvider string         `json:"authority_provider"`
}

type symbolsResponse struct {
	Symbols []symbolResponse `json:"symbols"`
}

type symbolResponse struct {
	InstrumentID string `json:"instrument_id"`
	Ticker       string `json:"ticker"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	Prefix       string `json:"prefix"`
	Currency     string `json:"currency"`
	Timezone     string `json:"timezone"`
	CalendarID   string `json:"calendar_id"`
	Synthetic    bool   `json:"synthetic"`
}

type calendarResponse struct {
	CalendarID string     `json:"calendar_id"`
	Version    string     `json:"version"`
	Timezone   string     `json:"timezone"`
	Session    string     `json:"session"`
	Windows    [][2]int64 `json:"windows"`
}

func New(history HistoryReader, options Options) http.Handler {
	if options.StartedAt.IsZero() {
		options.StartedAt = time.Now().UTC()
	}
	server := &Server{
		history: history,
		options: options,
		mux:     http.NewServeMux(),
	}
	server.routes()
	return server.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.health)
	s.mux.HandleFunc("/ready", s.ready)
	s.mux.HandleFunc("/version", s.version)
	s.mux.HandleFunc("/api/v1/bars", s.bars)
	s.mux.HandleFunc("/api/v1/symbols", s.symbols)
	s.mux.HandleFunc("/api/v1/calendar", s.calendar)
	if s.options.ResilienceStatus != nil {
		s.mux.HandleFunc("/api/v1/resilience", s.resilience)
	}
	if s.options.StreamHandler != nil {
		s.mux.Handle("/api/v1/stream", s.options.StreamHandler)
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":    "qnext-market-core",
		"version":    s.options.Version,
		"commit":     s.options.Commit,
		"started_at": s.options.StartedAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) resilience(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.options.ResilienceStatus == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "resilience_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, s.options.ResilienceStatus())
}

func (s *Server) bars(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.history == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "history_unavailable"})
		return
	}

	query := r.URL.Query()
	instrumentID := query.Get("instrument_id")
	timeframe := query.Get("timeframe")
	fromMS, fromOK := parseMilliseconds(query.Get("from_ms"))
	toMS, toOK := parseMilliseconds(query.Get("to_ms"))
	if instrumentID == "" || timeframe == "" || !fromOK || !toOK || fromMS >= toMS {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_query"})
		return
	}

	bars, err := s.history.LoadRange(
		instrumentID,
		timeframe,
		time.UnixMilli(fromMS).UTC(),
		time.UnixMilli(toMS).UTC(),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "history_read_failed"})
		return
	}

	response := barsResponse{Bars: make([]barResponse, 0, len(bars))}
	for _, bar := range bars {
		response.Bars = append(response.Bars, barResponse{
			Time:              bar.OpenTime.UnixMilli(),
			Open:              bar.Open,
			High:              bar.High,
			Low:               bar.Low,
			Close:             bar.Close,
			Volume:            bar.Volume,
			Final:             bar.Final,
			Revision:          bar.Revision,
			Quality:           bar.Quality,
			AuthorityProvider: bar.AuthorityProvider,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) symbols(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.options.Symbols == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "symbols_unavailable"})
		return
	}

	instruments := s.options.Symbols.ListVisible()
	response := symbolsResponse{Symbols: make([]symbolResponse, 0, len(instruments))}
	for _, instrument := range instruments {
		response.Symbols = append(response.Symbols, symbolResponse{
			InstrumentID: instrument.ID,
			Ticker:       instrument.Symbol,
			Description:  instrument.Name,
			Type:         strings.ToLower(instrument.AssetClass),
			Prefix:       instrument.Exchange,
			Currency:     instrument.Currency,
			Timezone:     instrument.Timezone,
			CalendarID:   instrument.CalendarID,
			Synthetic:    instrument.Synthetic,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.options.Symbols == nil || s.options.Calendars == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "calendar_unavailable"})
		return
	}

	query := r.URL.Query()
	instrumentID := query.Get("instrument_id")
	fromMS, fromOK := parseMilliseconds(query.Get("from_ms"))
	toMS, toOK := parseMilliseconds(query.Get("to_ms"))
	session := marketcalendar.Session(strings.ToLower(strings.TrimSpace(query.Get("session"))))
	if session == "" {
		session = marketcalendar.SessionRegular
	}
	if instrumentID == "" || !fromOK || !toOK || fromMS >= toMS {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_query"})
		return
	}

	instrument, ok := s.options.Symbols.Instrument(instrumentID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "instrument_not_found"})
		return
	}
	if instrument.CalendarID == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "calendar_not_configured"})
		return
	}

	windows, definition, err := s.options.Calendars.Windows(
		instrument.CalendarID,
		time.UnixMilli(fromMS).UTC(),
		time.UnixMilli(toMS).UTC(),
		session,
	)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "calendar_range_unavailable",
			"message": err.Error(),
		})
		return
	}

	response := calendarResponse{
		CalendarID: definition.ID,
		Version:    definition.Version,
		Timezone:   definition.Timezone,
		Session:    string(session),
		Windows:    make([][2]int64, 0, len(windows)),
	}
	for _, window := range windows {
		response.Windows = append(response.Windows, [2]int64{
			window.Start.UnixMilli(),
			window.End.UnixMilli(),
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
	return false
}

func parseMilliseconds(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
