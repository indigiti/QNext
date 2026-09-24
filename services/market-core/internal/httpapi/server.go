package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type HistoryReader interface {
	LoadRange(instrumentID, timeframe string, from, to time.Time) ([]domain.Bar, error)
}

type Options struct {
	Version       string
	Commit        string
	StartedAt     time.Time
	StreamHandler http.Handler
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
	writeJSON(w, http.StatusOK, s.options.ResilienceStatus())
}

func (s *Server) bars(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
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

func parseMilliseconds(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
