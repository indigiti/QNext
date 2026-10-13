package runtime

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/feedstatus"
	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

func readinessConfig() marketconfig.Config {
	market := marketconfig.DefaultMarkets()[0]
	return marketconfig.Config{
		Timeframes:    []string{"1m"},
		Markets:       []marketconfig.MarketConfig{market},
		ActiveMarkets: []string{"NIFTY"},
	}
}

func TestReadinessRequiresLiveConfigurationWhenRequested(t *testing.T) {
	ready, snapshot := (ReadinessEvaluator{
		RequireLive: true,
		Now: func() time.Time {
			return time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
		},
	}).Snapshot()
	if ready {
		t.Fatal("expected live-required runtime without config to be not ready")
	}
	if len(snapshot.Reasons) != 1 || snapshot.Reasons[0] != "live_market_not_configured" {
		t.Fatalf("unexpected readiness reasons: %+v", snapshot.Reasons)
	}
}

func TestReadinessRequiresFreshUnderlyingDuringActiveSession(t *testing.T) {
	config := readinessConfig()
	tracker := feedstatus.New()
	now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC) // 09:30 IST.
	tracker.Observe(domain.Tick{
		InstrumentID:  "NSE:NIFTY50",
		Provider:      "upstox",
		Price:         25000,
		EventTime:     now,
		ReceivedTime:  now,
		ProcessedTime: now,
		Quality:       domain.QualityGood,
	})

	ready, snapshot := (ReadinessEvaluator{
		RequireLive: true,
		Config:      &config,
		Calendars:   marketcalendar.DefaultRegistry(),
		Feed:        tracker,
		History:     history.New(t.TempDir()),
		Now:         func() time.Time { return now },
	}).Snapshot()
	if !ready {
		t.Fatalf("expected fresh active-session runtime to be ready: %+v", snapshot)
	}

	staleNow := now.Add(2 * time.Minute)
	ready, snapshot = (ReadinessEvaluator{
		RequireLive: true,
		Config:      &config,
		Calendars:   marketcalendar.DefaultRegistry(),
		Feed:        tracker,
		History:     history.New(t.TempDir()),
		Now:         func() time.Time { return staleNow },
		MaxFeedAge:  60 * time.Second,
	}).Snapshot()
	if ready {
		t.Fatalf("expected stale active-session feed to be not ready: %+v", snapshot)
	}
	if len(snapshot.Reasons) == 0 || snapshot.Reasons[0] != "feed_stale:NSE:NIFTY50" {
		t.Fatalf("unexpected stale readiness reasons: %+v", snapshot.Reasons)
	}
}

func TestReadinessDoesNotRequireLiveTicksOutsideSession(t *testing.T) {
	config := readinessConfig()
	now := time.Date(2026, 10, 10, 4, 0, 0, 0, time.UTC) // Saturday.

	ready, snapshot := (ReadinessEvaluator{
		RequireLive: true,
		Config:      &config,
		Calendars:   marketcalendar.DefaultRegistry(),
		Feed:        feedstatus.New(),
		History:     history.New(t.TempDir()),
		Now:         func() time.Time { return now },
	}).Snapshot()
	if !ready {
		t.Fatalf("expected closed-market runtime to remain ready: %+v", snapshot)
	}
	if len(snapshot.ActiveMarkets) != 0 {
		t.Fatalf("expected no active markets outside session: %+v", snapshot.ActiveMarkets)
	}
}
