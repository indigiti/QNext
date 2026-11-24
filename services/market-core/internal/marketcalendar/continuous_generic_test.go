package marketcalendar

import (
	"testing"
	"time"
)

func TestContinuousCalendarIsNotBoundToExchangeName(t *testing.T) {
	registry := NewRegistry()
	definition := OKXCrypto24x7()
	definition.ID = "TEST_CONTINUOUS"
	definition.Timezone = "Asia/Kolkata"
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 11, 28, 17, 0, 0, 0, time.UTC)
	to := from.Add(4 * time.Hour)
	windows, _, err := registry.Windows(definition.ID, from, to, SessionRegular)
	if err != nil {
		t.Fatal(err)
	}
	midnight := time.Date(2026, 11, 28, 18, 30, 0, 0, time.UTC)
	if len(windows) != 2 || !windows[0].Start.Equal(from) || !windows[0].End.Equal(midnight) ||
		!windows[1].Start.Equal(midnight) || !windows[1].End.Equal(to) {
		t.Fatalf("continuous coverage or local midnight incorrect: %+v", windows)
	}
}
