package main

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
)

func TestReadinessChecksIngestionPersistenceCalendarAndSession(t *testing.T) {
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil { t.Fatal(err) }
	cal := marketcalendar.DefaultRegistry()
	day := time.Date(2026, 11, 23, 10, 30, 0, 0, ist)
	check := func(now, started time.Time, configured, stopping bool, recent int64, storageOK bool, storageReasons []string) (bool, []string) {
		return evaluateMarketReadiness(now, started, configured, stopping, cal, recent, storageOK, storageReasons)
	}
	if ok, reasons := check(day, day.Add(-3*time.Minute), true, false, day.Add(-time.Second).UnixMilli(), true, nil); !ok { t.Fatalf("healthy live session unready: %v", reasons) }
	if ok, _ := check(day, day.Add(-3*time.Minute), false, false, 0, true, nil); ok { t.Fatal("unconfigured market accepted") }
	if ok, _ := check(day, day.Add(-3*time.Minute), true, false, 0, true, nil); ok { t.Fatal("missing live ticks accepted") }
	if ok, _ := check(day, day.Add(-3*time.Minute), true, false, day.Add(-2*time.Minute).UnixMilli(), true, nil); ok { t.Fatal("stale ticks accepted") }
	if ok, _ := check(day, day.Add(-30*time.Second), true, false, 0, true, nil); !ok { t.Fatal("startup grace period should prevent premature stale readiness") }
	if ok, _ := check(day, day.Add(-3*time.Minute), true, false, day.UnixMilli(), false, []string{"history_persistence_error"}); ok { t.Fatal("disk errors accepted") }
	if ok, _ := check(day, day.Add(-3*time.Minute), true, true, day.UnixMilli(), true, nil); ok { t.Fatal("shutdown accepted") }

	afterClose := time.Date(2026, 11, 23, 17, 0, 0, 0, ist)
	if ok, reasons := check(afterClose, day, true, false, 0, true, nil); !ok { t.Fatalf("closed market incorrectly requires ticks: %v", reasons) }

	uncertified := time.Date(2027, 1, 4, 10, 30, 0, 0, ist)
	if ok, reasons := check(uncertified, uncertified.Add(-2*time.Minute), true, false, uncertified.UnixMilli(), true, nil); ok {
		t.Fatalf("2027 uncertified calendar allowed: %v", reasons)
	}
}
