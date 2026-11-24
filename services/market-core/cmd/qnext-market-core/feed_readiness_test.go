package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
)

func TestIndependentFeedReadiness(t *testing.T) {
	cal := marketcalendar.DefaultRegistry()
	now := time.Date(2026, 11, 23, 5, 0, 0, 0, time.UTC)
	started := now.Add(-5 * time.Minute)
	targets := []independentFeedTarget{
		{ID: "NSE:NIFTY50", CalendarID: "NSE_EQ", Requested: true, Active: true},
		{ID: "OKX:XAU-USDT-SWAP", CalendarID: "OKX_CRYPTO_24_7", Requested: true, Active: true},
		{ID: "disabled", CalendarID: "missing"},
	}
	fresh := map[string]int64{"OKX:XAU-USDT-SWAP": now.UnixMilli()}
	read := func(id string) int64 { return fresh[id] }
	check := func(at, start time.Time, want []string) {
		t.Helper()
		if got := independentFeedReadiness(at, start, cal, targets, read); !reflect.DeepEqual(got, want) {
			t.Fatalf("readiness: got %v, want %v", got, want)
		}
	}
	check(now, started, []string{"feed_stale:NSE:NIFTY50"})
	fresh["NSE:NIFTY50"] = now.UnixMilli()
	check(now, started, nil)
	delete(fresh, "OKX:XAU-USDT-SWAP")
	check(now, started, []string{"feed_stale:OKX:XAU-USDT-SWAP"})
	check(now, now.Add(-30*time.Second), nil)
	// Saturday: Indian market closed, continuous feed remains mandatory.
	weekend := now.AddDate(0, 0, 5)
	check(weekend, started, []string{"feed_stale:OKX:XAU-USDT-SWAP"})
	targets[1].Active = false
	check(weekend, started, []string{"feed_unavailable:OKX:XAU-USDT-SWAP"})
	targets[1].Active = true
	targets[1].CalendarID = "missing"
	check(weekend, started, []string{"feed_calendar_uncertified:OKX:XAU-USDT-SWAP"})
}
