package main

import (
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
)

// evaluateMarketReadiness separates process liveness from ability to ingest
// and durably persist market data. During closed sessions provider freshness
// is irrelevant, but configuration, persistence and exchange calendar are not.
func evaluateMarketReadiness(
	now, started time.Time,
	liveConfigured, stopping bool,
	calendars *marketcalendar.Registry,
	latestProcessedMS int64,
	persistenceOK bool,
	persistenceReasons []string,
) (bool, []string) {
	var reasons []string
	if stopping {
		reasons = append(reasons, "shutting_down")
	}
	if !liveConfigured {
		reasons = append(reasons, "market_ingestion_not_configured")
	}
	if !persistenceOK {
		if len(persistenceReasons) == 0 {
			reasons = append(reasons, "history_persistence_unready")
		} else {
			reasons = append(reasons, persistenceReasons...)
		}
	}
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil || calendars == nil {
		reasons = append(reasons, "calendar_unavailable")
	} else {
		local := now.In(location)
		start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
		end := start.AddDate(0, 0, 1)
		for _, id := range []string{"NSE_EQ", "BSE_EQ", "NSE_FO", "BSE_FO"} {
			if _, _, calendarErr := calendars.Windows(id, start, end, marketcalendar.SessionRegular); calendarErr != nil {
				reasons = append(reasons, "exchange_calendar_uncertified")
				break
			}
		}
	}
	// A reconnect can transiently prevent ticks. Do not replace /health
	// with /ready as a service restart probe.
	if liveConfigured && regularMarketSessionActive(now) && now.Sub(started) >= 90*time.Second {
		if latestProcessedMS <= 0 ||
			now.Sub(time.UnixMilli(latestProcessedMS)) > 90*time.Second {
			reasons = append(reasons, "market_feed_stale")
		}
	}
	return len(reasons) == 0, reasons
}
