package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/feedstatus"
	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

type ReadinessSnapshot struct {
	Status              string                   `json:"status"`
	LiveRequired        bool                     `json:"live_required"`
	LiveConfigured      bool                     `json:"live_configured"`
	ActiveMarkets       []string                 `json:"active_markets,omitempty"`
	FeedFreshnessMS     map[string]int64          `json:"feed_freshness_ms,omitempty"`
	Persistence         history.PersistenceStats `json:"persistence"`
	PersistenceQueueUse float64                  `json:"persistence_queue_utilization"`
	Reasons             []string                 `json:"reasons,omitempty"`
}

type ReadinessEvaluator struct {
	RequireLive           bool
	Config                *marketconfig.Config
	Calendars             *marketcalendar.Registry
	Feed                  *feedstatus.Tracker
	History               *history.Store
	Now                   func() time.Time
	MaxFeedAge            time.Duration
	MaxPersistenceLag     time.Duration
	MaxPersistenceQueueUse float64
}

func (r ReadinessEvaluator) Snapshot() (bool, ReadinessSnapshot) {
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	maxFeedAge := r.MaxFeedAge
	if maxFeedAge <= 0 {
		maxFeedAge = 60 * time.Second
	}
	maxPersistenceLag := r.MaxPersistenceLag
	if maxPersistenceLag <= 0 {
		maxPersistenceLag = 15 * time.Second
	}
	maxQueueUse := r.MaxPersistenceQueueUse
	if maxQueueUse <= 0 || maxQueueUse >= 1 {
		maxQueueUse = 0.90
	}

	snapshot := ReadinessSnapshot{
		Status:          "ready",
		LiveRequired:    r.RequireLive,
		LiveConfigured:  r.Config != nil,
		FeedFreshnessMS: map[string]int64{},
	}

	if r.RequireLive && r.Config == nil {
		snapshot.Reasons = append(snapshot.Reasons, "live_market_not_configured")
	}

	if r.Config != nil {
		calendars := r.Calendars
		if calendars == nil {
			calendars = marketcalendar.DefaultRegistry()
		}
		active := activeMarkets(*r.Config, calendars, now)
		snapshot.ActiveMarkets = make([]string, 0, len(active))
		for _, market := range active {
			snapshot.ActiveMarkets = append(snapshot.ActiveMarkets, market.Symbol)
		}

		if len(active) > 0 {
			if r.Feed == nil {
				snapshot.Reasons = append(snapshot.Reasons, "feed_tracker_unavailable")
			} else {
				feed := r.Feed.Snapshot()
				for _, market := range active {
					instrumentID := market.Underlying.InstrumentID
					state, ok := feed.Instruments[instrumentID]
					if !ok {
						snapshot.FeedFreshnessMS[instrumentID] = -1
						snapshot.Reasons = append(
							snapshot.Reasons,
							"feed_missing:"+instrumentID,
						)
						continue
					}
					age := feedstatus.AgeMS(state.LastReceivedTimeMS, now)
					snapshot.FeedFreshnessMS[instrumentID] = age
					if age < 0 || age > maxFeedAge.Milliseconds() {
						snapshot.Reasons = append(
							snapshot.Reasons,
							"feed_stale:"+instrumentID,
						)
					}
					if unusableReadinessQuality(state.Quality) {
						snapshot.Reasons = append(
							snapshot.Reasons,
							"feed_quality:"+instrumentID+":"+string(state.Quality),
						)
					}
				}
			}
		}
	}

	if r.History != nil {
		stats := r.History.PersistenceStats()
		snapshot.Persistence = stats
		if stats.Capacity > 0 {
			snapshot.PersistenceQueueUse = float64(stats.Queued) / float64(stats.Capacity)
		}
		if stats.Pending > 0 && strings.TrimSpace(stats.LastError) != "" {
			snapshot.Reasons = append(snapshot.Reasons, "persistence_error")
		}
		if stats.Pending > 0 && stats.FlushLagMS > maxPersistenceLag.Milliseconds() {
			snapshot.Reasons = append(snapshot.Reasons, "persistence_lag")
		}
		if stats.Capacity > 0 && snapshot.PersistenceQueueUse >= maxQueueUse {
			snapshot.Reasons = append(snapshot.Reasons, "persistence_backlog")
		}
	}

	if len(snapshot.Reasons) > 0 {
		snapshot.Status = "not_ready"
		return false, snapshot
	}
	return true, snapshot
}

func activeMarkets(
	config marketconfig.Config,
	calendars *marketcalendar.Registry,
	now time.Time,
) []marketconfig.MarketConfig {
	var result []marketconfig.MarketConfig
	for _, market := range config.EffectiveMarkets() {
		definition, ok := calendars.Definition(market.CalendarID)
		if !ok {
			continue
		}
		if regularSessionActive(definition, now) {
			result = append(result, market)
		}
	}
	return result
}

func regularSessionActive(definition marketcalendar.Definition, now time.Time) bool {
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return false
	}
	local := now.In(location)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	if _, closed := definition.ClosedDates[local.Format("2006-01-02")]; closed {
		return false
	}
	open := time.Date(
		local.Year(), local.Month(), local.Day(),
		definition.RegularOpen.Hour, definition.RegularOpen.Minute,
		0, 0, location,
	)
	closeAt := time.Date(
		local.Year(), local.Month(), local.Day(),
		definition.RegularClose.Hour, definition.RegularClose.Minute,
		0, 0, location,
	)
	return !local.Before(open) && local.Before(closeAt)
}

func unusableReadinessQuality(quality domain.Quality) bool {
	switch quality {
	case domain.QualityInvalid, domain.QualityDegraded, domain.QualityStale:
		return true
	default:
		return false
	}
}

func (r ReadinessSnapshot) String() string {
	return fmt.Sprintf("%s reasons=%v", r.Status, r.Reasons)
}
