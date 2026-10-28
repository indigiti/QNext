package marketcalendar

import (
	"testing"
	"time"
)

func TestEquityDerivativesCalendarClosesAt1540SinceAugust2026(t *testing.T) {
	registry := DefaultRegistry()
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"NSE_EQ", "BSE_EQ", "NSE_FO", "BSE_FO"} {
		at := time.Date(2026, 10, 28, 15, 25, 0, 0, ist)
		windows, _, err := registry.Windows(id, at, at.Add(30*time.Minute), SessionRegular)
		if err != nil || len(windows) != 1 {
			t.Fatalf("%s windows=%+v err=%v", id, windows, err)
		}
		want := 30
		if id == "NSE_FO" || id == "BSE_FO" {
			want = 40
		}
		end := windows[0].End.In(ist)
		if end.Hour() != 15 || end.Minute() != want {
			t.Fatalf("%s close=%v want 15:%d", id, end, want)
		}
	}
	july := time.Date(2026, 7, 30, 15, 20, 0, 0, ist)
	windows, _, err := registry.Windows("NSE_FO", july, july.Add(30*time.Minute), SessionRegular)
	if err != nil || len(windows) != 1 || windows[0].End.In(ist).Minute() != 30 {
		t.Fatalf("pre-August futures history changed: %+v err=%v", windows, err)
	}
}
