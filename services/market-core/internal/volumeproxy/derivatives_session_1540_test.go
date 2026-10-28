package volumeproxy

import (
	"testing"
	"time"
)

func TestFuturesWatchdogRemainsActiveAfterCashCloseUntil1540(t *testing.T) {
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil { t.Fatal(err) }
	at := time.Date(2026, 10, 28, 15, 30, 0, 0, ist)
	if !inRegularSession(at) || !inRegularSession(at.Add(9*time.Minute)) ||
		inRegularSession(at.Add(10*time.Minute)) {
		t.Fatal("futures watchdog does not honor 15:40 end")
	}
	spot := at.Add(-3*time.Second)
	if !underlyingWasUsable(spot, at.Add(8*time.Minute)) {
		t.Fatal("fresh 15:29 spot must permit postclose futures recovery")
	}
	if underlyingWasUsable(at.Add(-30*time.Minute), at.Add(8*time.Minute)) {
		t.Fatal("stale midday spot cannot authorize recovery")
	}
	if underlyingWasUsable(spot, at.Add(11*time.Minute)) {
		t.Fatal("cash session state cannot carry beyond 15:40")
	}
}
