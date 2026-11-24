package main

import (
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
)

// independentFeedTarget describes a configured source without coupling readiness
// to a provider name, exchange-specific symbol, or a second candle engine.
type independentFeedTarget struct {
	ID         string
	CalendarID string
	Requested  bool
	Active     bool
}

func independentFeedReadiness(now, started time.Time, calendars *marketcalendar.Registry, targets []independentFeedTarget, processed func(string) int64) []string {
	var reasons []string
	for _, target := range targets {
		if !target.Requested {
			continue
		}
		if !target.Active {
			reasons = append(reasons, "feed_unavailable:"+target.ID)
			continue
		}
		if calendars == nil {
			reasons = append(reasons, "feed_calendar_uncertified:"+target.ID)
			continue
		}
		windows, _, err := calendars.Windows(target.CalendarID, now, now.Add(time.Nanosecond), marketcalendar.SessionRegular)
		if err != nil {
			reasons = append(reasons, "feed_calendar_uncertified:"+target.ID)
			continue
		}
		if len(windows) == 0 || now.Sub(started) < 90*time.Second {
			continue
		}
		at := processed(target.ID)
		if at <= 0 || now.Sub(time.UnixMilli(at)) > 90*time.Second {
			reasons = append(reasons, "feed_stale:"+target.ID)
		}
	}
	return reasons
}
