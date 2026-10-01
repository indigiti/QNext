package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

const fiveSecondHealthBucket = 5 * time.Second

type fiveSecondHealthResponse struct {
	GeneratedAtMS int64                    `json:"generated_at_ms"`
	BucketMS      int64                    `json:"bucket_ms"`
	Symbols       []fiveSecondSymbolHealth `json:"symbols"`
}

type fiveSecondSymbolHealth struct {
	InstrumentID     string                    `json:"instrument_id"`
	Symbol           string                    `json:"symbol"`
	CalendarID       string                    `json:"calendar_id"`
	SessionStartMS   int64                     `json:"session_start_ms,omitempty"`
	SessionEndMS     int64                     `json:"session_end_ms,omitempty"`
	ElapsedEndMS     int64                     `json:"elapsed_end_ms,omitempty"`
	Expected         int                       `json:"expected"`
	Present          int                       `json:"present"`
	Good             int                       `json:"good"`
	CarryForward     int                       `json:"carry_forward"`
	Recovered        int                       `json:"recovered"`
	Degraded         int                       `json:"degraded"`
	Missing          int                       `json:"missing"`
	HealthyPct       float64                   `json:"healthy_pct"`
	CompletenessPct  float64                   `json:"completeness_pct"`
	LastFiveSecondMS int64                     `json:"last_5s_ms,omitempty"`
	Status           string                    `json:"status"`
	Timeline         []fiveSecondHealthSegment `json:"timeline"`
}

type fiveSecondHealthSegment struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	State   string `json:"state"`
	Buckets int    `json:"buckets"`
	Reason  string `json:"reason,omitempty"`
}

func (s *Server) fiveSecondHealth(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.history == nil || s.options.Symbols == nil || s.options.Calendars == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "five_second_health_unavailable"})
		return
	}

	now := time.Now().UTC()
	instruments := s.options.Symbols.ListVisible()
	response := fiveSecondHealthResponse{
		GeneratedAtMS: now.UnixMilli(),
		BucketMS:      fiveSecondHealthBucket.Milliseconds(),
		Symbols:       make([]fiveSecondSymbolHealth, 0, len(instruments)),
	}

	for _, instrument := range instruments {
		if !includeFiveSecondHealthInstrument(instrument) {
			continue
		}
		health, err := s.currentDayFiveSecondHealth(instrument, now)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":         "five_second_health_failed",
				"instrument_id": instrument.ID,
				"message":       err.Error(),
			})
			return
		}
		response.Symbols = append(response.Symbols, health)
	}

	sort.SliceStable(response.Symbols, func(i, j int) bool {
		return fiveSecondHealthSortKey(response.Symbols[i].Symbol) < fiveSecondHealthSortKey(response.Symbols[j].Symbol)
	})
	writeJSON(w, http.StatusOK, response)
}

func includeFiveSecondHealthInstrument(instrument symbol.Instrument) bool {
	asset := strings.ToLower(strings.TrimSpace(instrument.AssetClass))
	if asset == "index" || strings.Contains(asset, "index") {
		return true
	}
	// Synthetic index instruments inherit the base asset class, but keep this
	// explicit fallback so future registry wording does not hide INDEX-SYN rows.
	return instrument.Synthetic && strings.Contains(strings.ToUpper(instrument.Symbol), "SYN")
}

func fiveSecondHealthSortKey(value string) string {
	symbol := strings.ToUpper(strings.TrimSpace(value))
	base := strings.TrimSuffix(strings.TrimSuffix(symbol, "+"), "-SYN")
	kind := "0"
	if strings.HasSuffix(symbol, "-SYN") {
		kind = "1"
	}
	if strings.HasSuffix(symbol, "-SYN+") {
		kind = "2"
	}
	return base + "\x00" + kind + "\x00" + symbol
}

func (s *Server) currentDayFiveSecondHealth(instrument symbol.Instrument, now time.Time) (fiveSecondSymbolHealth, error) {
	result := fiveSecondSymbolHealth{
		InstrumentID: instrument.ID,
		Symbol:       instrument.Symbol,
		CalendarID:   instrument.CalendarID,
		Status:       "WAITING",
		Timeline:     []fiveSecondHealthSegment{},
	}
	if strings.TrimSpace(instrument.CalendarID) == "" {
		result.Status = "NO_CALENDAR"
		return result, nil
	}

	definition, ok := s.options.Calendars.Definition(instrument.CalendarID)
	if !ok {
		return result, fmt.Errorf("calendar %s is not registered", instrument.CalendarID)
	}
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return result, fmt.Errorf("load calendar timezone: %w", err)
	}
	localNow := now.In(location)
	localStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	localEnd := localStart.AddDate(0, 0, 1)
	windows, _, err := s.options.Calendars.Windows(
		instrument.CalendarID,
		localStart.UTC(),
		localEnd.UTC(),
		marketcalendar.SessionRegular,
	)
	if err != nil {
		return result, err
	}
	if len(windows) == 0 {
		result.Status = "CLOSED"
		return result, nil
	}

	window := windows[0]
	result.SessionStartMS = window.Start.UnixMilli()
	result.SessionEndMS = window.End.UnixMilli()
	elapsedEnd := now
	if elapsedEnd.Before(window.Start) {
		elapsedEnd = window.Start
	}
	if elapsedEnd.After(window.End) {
		elapsedEnd = window.End
	}
	completeDuration := elapsedEnd.Sub(window.Start)
	if completeDuration < 0 {
		completeDuration = 0
	}
	result.Expected = int(completeDuration / fiveSecondHealthBucket)
	elapsedEnd = window.Start.Add(time.Duration(result.Expected) * fiveSecondHealthBucket)
	result.ElapsedEndMS = elapsedEnd.UnixMilli()
	if result.Expected == 0 {
		return result, nil
	}

	bars, err := s.history.LoadRange(instrument.ID, "5s", window.Start, elapsedEnd)
	if err != nil {
		return result, err
	}
	return buildFiveSecondSymbolHealth(result, window.Start, elapsedEnd, bars), nil
}

func buildFiveSecondSymbolHealth(
	result fiveSecondSymbolHealth,
	start time.Time,
	elapsedEnd time.Time,
	bars []domain.Bar,
) fiveSecondSymbolHealth {
	byOpen := make(map[int64]domain.Bar, len(bars))
	for _, bar := range bars {
		if !bar.Final || bar.Timeframe != "5s" || bar.OpenTime.Before(start) || !bar.OpenTime.Before(elapsedEnd) {
			continue
		}
		key := bar.OpenTime.UTC().UnixMilli()
		if previous, ok := byOpen[key]; !ok || bar.Revision >= previous.Revision {
			byOpen[key] = bar
		}
		if key > result.LastFiveSecondMS {
			result.LastFiveSecondMS = key
		}
	}

	for index := 0; index < result.Expected; index++ {
		openTime := start.Add(time.Duration(index) * fiveSecondHealthBucket)
		closeTime := openTime.Add(fiveSecondHealthBucket)
		state := "MISSING"
		reason := "NO_PERSISTED_5S"
		if bar, ok := byOpen[openTime.UnixMilli()]; ok {
			result.Present++
			state, reason = classifyFiveSecondBar(bar)
		}
		switch state {
		case "GOOD":
			result.Good++
		case "CARRY_FORWARD":
			result.CarryForward++
		case "RECOVERED":
			result.Recovered++
		case "DEGRADED":
			result.Degraded++
		default:
			result.Missing++
		}
		result.Timeline = appendFiveSecondHealthSegment(result.Timeline, openTime, closeTime, state, reason)
	}

	healthy := result.Good + result.CarryForward + result.Recovered
	if result.Expected > 0 {
		result.HealthyPct = percent(healthy, result.Expected)
		result.CompletenessPct = percent(result.Present, result.Expected)
	}
	switch {
	case result.Missing > 0 || result.Degraded > 0:
		result.Status = "CHECK"
	default:
		result.Status = "HEALTHY"
	}
	return result
}

func classifyFiveSecondBar(bar domain.Bar) (string, string) {
	switch {
	case bar.CarryForward:
		return "CARRY_FORWARD", "NO_UPDATE"
	case bar.Recovered || bar.Quality == domain.QualityRecovered:
		return "RECOVERED", "RECOVERED"
	case bar.Quality == domain.QualityDegraded,
		bar.Quality == domain.QualityStale,
		bar.Quality == domain.QualityInvalid,
		bar.Quality == domain.QualityPartial:
		return "DEGRADED", "QUALITY_" + string(bar.Quality)
	default:
		return "GOOD", ""
	}
}

func appendFiveSecondHealthSegment(
	segments []fiveSecondHealthSegment,
	start time.Time,
	end time.Time,
	state string,
	reason string,
) []fiveSecondHealthSegment {
	if len(segments) > 0 {
		last := &segments[len(segments)-1]
		if last.State == state && last.Reason == reason && last.EndMS == start.UnixMilli() {
			last.EndMS = end.UnixMilli()
			last.Buckets++
			return segments
		}
	}
	return append(segments, fiveSecondHealthSegment{
		StartMS: start.UnixMilli(),
		EndMS:   end.UnixMilli(),
		State:   state,
		Buckets: 1,
		Reason:  reason,
	})
}

func percent(value, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}
