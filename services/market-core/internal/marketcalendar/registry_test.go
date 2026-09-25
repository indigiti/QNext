package marketcalendar

import (
	"testing"
	"time"
)

func TestNSECalendarSkipsWeekendAndCertifiedHoliday(t *testing.T) {
	registry := DefaultRegistry()
	from := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)

	windows, definition, err := registry.Windows("NSE_EQ", from, to, SessionRegular)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Version != "nse-equities-2026-v1" {
		t.Fatalf("unexpected calendar version: %s", definition.Version)
	}
	if len(windows) != 2 {
		t.Fatalf("expected Friday and Tuesday windows only, got %+v", windows)
	}

	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	first := windows[0].Start.In(location)
	if first.Weekday() != time.Friday || first.Hour() != 9 || first.Minute() != 15 {
		t.Fatalf("unexpected first session start: %v", first)
	}
	second := windows[1].Start.In(location)
	if second.Weekday() != time.Tuesday {
		t.Fatalf("expected Tuesday after Ganesh Chaturthi closure, got %v", second)
	}
}

func TestNSECalendarFailsClosedOutsideCertifiedYear(t *testing.T) {
	registry := DefaultRegistry()
	_, _, err := registry.Windows(
		"NSE_EQ",
		time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		SessionRegular,
	)
	if err == nil {
		t.Fatal("expected unsupported calendar range to fail")
	}
}

func TestWindowAtUsesCertifiedSessionAndFailsClosed(t *testing.T) {
	registry := DefaultRegistry()
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}

	inside := time.Date(2026, 9, 25, 10, 0, 0, 0, ist)
	window, definition, active, err := registry.WindowAt("NSE_EQ", inside, SessionRegular)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("expected regular session to be active")
	}
	if definition.Version != "nse-equities-2026-v1" {
		t.Fatalf("unexpected calendar version: %s", definition.Version)
	}
	if got := window.Start.In(ist); got.Hour() != 9 || got.Minute() != 15 {
		t.Fatalf("unexpected session start: %v", got)
	}
	if got := window.End.In(ist); got.Hour() != 15 || got.Minute() != 30 {
		t.Fatalf("unexpected session end: %v", got)
	}

	_, _, active, err = registry.WindowAt(
		"NSE_EQ",
		time.Date(2026, 9, 25, 16, 0, 0, 0, ist),
		SessionRegular,
	)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("after-hours timestamp must not be active")
	}

	_, _, active, err = registry.WindowAt(
		"NSE_EQ",
		time.Date(2026, 10, 2, 10, 0, 0, 0, ist),
		SessionRegular,
	)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("certified holiday must not be active")
	}

	_, _, _, err = registry.WindowAt(
		"NSE_EQ",
		time.Date(2027, 1, 2, 10, 0, 0, 0, ist),
		SessionRegular,
	)
	if err == nil {
		t.Fatal("calendar lookup outside the certified window must fail closed")
	}
}
