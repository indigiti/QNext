package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/research"
)

func TestSynPlusMirrorsSourceVolumeWithoutPriceSnapshot(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:NIFTY-SYN+")
	broker := NewBroker(64, 16)
	handler := NewWebSocketHandler(broker).(*WebSocketHandler)
	at := time.Date(2026, 10, 15, 3, 45, 0, 0, time.UTC)
	source := domain.Bar{
		InstrumentID:      "NSE:NIFTY50",
		Timeframe:         "1m",
		OpenTime:          at,
		CloseTime:         at.Add(time.Minute),
		Open:              25000,
		High:              25000,
		Low:               25000,
		Close:             25000,
		AuthorityProvider: "upstox",
		Quality:           domain.QualityGood,
	}
	// The initial source proxy is zero while the first 1m SYN+ price forms.
	broker.PublishBar(source)
	observe := func(at time.Time, value float64) {
		t.Helper()
		if err := handler.shadow.observe(research.SynPlusSnapshot{
			Schema:       research.SynPlusSnapshotSchema,
			InstrumentID: "QNEXT:NIFTY-SYN+",
			Version:      "test-syn-plus",
			SnapshotAtMS: at.UnixMilli(),
			Quality:      "GOOD",
			Value:        value,
		}); err != nil {
			t.Fatal(err)
		}
	}
	latest := func(wantVolume float64, wantAt time.Time) {
		t.Helper()
		bar, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", "1m")
		if !ok || bar.Volume != wantVolume || !bar.OpenTime.Equal(wantAt) {
			t.Fatalf("unexpected live SYN+ volume: %+v ok=%v, want volume %v at %s", bar, ok, wantVolume, wantAt)
		}
		if bar.Open != 25010 || bar.Close != 25010 {
			t.Fatalf("source volume changed SYN+ OHLC: %+v", bar)
		}
	}
	observe(at.Add(time.Second), 25010)
	latest(0, at)
	for _, volume := range []float64{100, 300, 350} {
		source.Volume = volume
		source.SourceSequence++
		source.Final = volume == 350
		broker.PublishBar(source) // No additional SYN+ snapshot.
		latest(volume, at)
	}
	// A stale VTT observation must not downgrade a newer source volume.
	stale := source
	stale.Volume = 100
	stale.Final = false
	stale.SourceSequence--
	broker.PublishBar(stale)
	latest(350, at)
}

func TestSynPlusCorrectsPreviousOneMinuteVolumeWithoutRewindingLatest(t *testing.T) {
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:NIFTY-SYN+")
	broker := NewBroker(64, 16)
	handler := NewWebSocketHandler(broker).(*WebSocketHandler)
	at := time.Date(2026, 10, 15, 3, 45, 0, 0, time.UTC)
	previous := domain.Bar{
		InstrumentID: "NSE:NIFTY50",
		Timeframe:    "1m",
		OpenTime:     at,
		CloseTime:    at.Add(time.Minute),
		Volume:       300,
		Final:        false,
	}
	broker.PublishBar(previous)
	for _, tick := range []struct {
		at    time.Time
		value float64
	}{
		{at.Add(time.Second), 25010},
		{at.Add(time.Minute + time.Second), 25015},
	} {
		if err := handler.shadow.observe(research.SynPlusSnapshot{
			Schema:       research.SynPlusSnapshotSchema,
			InstrumentID: "QNEXT:NIFTY-SYN+",
			Version:      "test-syn-plus",
			SnapshotAtMS: tick.at.UnixMilli(),
			Quality:      "GOOD",
			Value:        tick.value,
		}); err != nil {
			t.Fatal(err)
		}
	}
	currentAt := at.Add(time.Minute)
	current := previous
	current.OpenTime = currentAt
	current.CloseTime = currentAt.Add(time.Minute)
	current.Volume = 10
	broker.PublishBar(current)
	bar, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", "1m")
	if !ok || !bar.OpenTime.Equal(currentAt) || bar.Volume != 10 {
		t.Fatalf("current SYN+ bar missing proxy: %+v", bar)
	}
	listener, err := broker.Subscribe("QNEXT:NIFTY-SYN+", "1m", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Cancel()

	// Proxy finalization of the old minute can follow the first SYN+ snapshot
	// of the new minute. Emit a revision, but keep chronological LatestBar.
	previous.Volume = 350
	previous.Final = true
	previous.Revision = 1
	broker.PublishBar(previous)

	select {
	case update := <-listener.Events:
		if !update.Bar.OpenTime.Equal(at) || update.Bar.Volume != 350 ||
			!update.Bar.Final || update.Bar.Revision == 0 {
			t.Fatalf("expected revisioned prior-minute correction: %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("missing prior-minute volume correction")
	}
	latest, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", "1m")
	if !ok || !latest.OpenTime.Equal(currentAt) || latest.Volume != 10 {
		t.Fatalf("late source volume rewound latest SYN+ bar: %+v", latest)
	}
}

func TestBrokerLatestBarIgnoresOlderVolumeCorrections(t *testing.T) {
	broker := NewBroker(8, 8)
	at := time.Date(2026, 10, 15, 3, 45, 0, 0, time.UTC)
	current := bar("QNEXT:NIFTY-SYN+", "1m", at.Add(time.Minute), 25012)
	broker.PublishBar(current)
	older := bar("QNEXT:NIFTY-SYN+", "1m", at, 25010)
	older.Volume = 400
	older.Revision = 2
	broker.PublishBar(older)
	latest, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", "1m")
	if !ok || !latest.OpenTime.Equal(current.OpenTime) {
		t.Fatalf("late correction replaced chronological latest: %+v", latest)
	}
}
