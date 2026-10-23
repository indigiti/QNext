package stream

import (
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestUnifiedSYNVolumeMatchesIndexAcrossLiveCorrections(t *testing.T) {
	broker := NewBroker(64, 16)
	broker.ConfigureSyntheticVolume(map[string]string{
		"QNEXT:NIFTY-SYN":     "NSE:NIFTY50",
		"QNEXT:BANKNIFTY-SYN": "NSE:BANKNIFTY",
	})
	at := time.Date(2026, 10, 23, 3, 45, 0, 0, time.UTC)
	syn := domain.Bar{
		InstrumentID: "QNEXT:NIFTY-SYN", Timeframe: "15s",
		OpenTime: at, CloseTime: at.Add(15 * time.Second),
		Open: 25010, High: 25011, Low: 25009, Close: 25010,
		Volume: 130, Quality: domain.QualityGood,
	}
	broker.PublishBar(syn)
	latest, ok := broker.LatestBar(syn.InstrumentID, "15s")
	if !ok || latest.Volume != 0 || latest.VolumeStatus != "UNKNOWN" {
		t.Fatalf("option basket was mistaken for futures volume: %+v", latest)
	}
	source := domain.Bar{
		InstrumentID: "NSE:NIFTY50", Timeframe: "15s",
		OpenTime: at, CloseTime: at.Add(15 * time.Second),
		Volume: 45, VolumeSource: "FUTURES_PROXY", VolumeStatus: "OBSERVED",
	}
	broker.PublishBar(source)
	latest, _ = broker.LatestBar(syn.InstrumentID, "15s")
	if latest.Volume != 45 || latest.VolumeStatus != "OBSERVED" ||
		latest.Close != 25010 || latest.Open != 25010 || latest.Revision == 0 {
		t.Fatalf("late observed volume did not correct SYN: %+v", latest)
	}
	syn.Close = 25012
	syn.Volume = 300 // unrelated option basket activity.
	broker.PublishBar(syn)
	latest, _ = broker.LatestBar(syn.InstrumentID, "15s")
	if latest.Volume != 45 || latest.Close != 25012 {
		t.Fatalf("SYN price event rewound futures volume: %+v", latest)
	}
	broker.PublishBar(domain.Bar{
		InstrumentID: "NSE:BANKNIFTY", Timeframe: "15s",
		OpenTime: at, Volume: 900, VolumeSource: "FUTURES_PROXY",
	})
	latest, _ = broker.LatestBar(syn.InstrumentID, "15s")
	if latest.Volume != 45 {
		t.Fatalf("second market changed NIFTY proxy: %+v", latest)
	}
	source.Volume = 70
	source.Revision = 1
	source.Final = true
	broker.PublishBar(source)
	latest, _ = broker.LatestBar(syn.InstrumentID, "15s")
	if latest.Volume != 70 || latest.Close != 25012 {
		t.Fatalf("source revision did not reach SYN: %+v", latest)
	}
}

func TestSynVolumeDoesNotFabricateMissingSource(t *testing.T) {
	broker := NewBroker(8, 8)
	broker.ConfigureSyntheticVolume(map[string]string{
		"QNEXT:NIFTY-SYN": "NSE:NIFTY50",
	})
	at := time.Date(2026, 10, 23, 3, 45, 0, 0, time.UTC)
	broker.PublishBar(domain.Bar{
		InstrumentID: "QNEXT:NIFTY-SYN", Timeframe: "1m",
		OpenTime: at, Volume: 80, Open: 25000, Close: 25000,
	})
	latest, _ := broker.LatestBar("QNEXT:NIFTY-SYN", "1m")
	if latest.Volume != 0 || latest.VolumeStatus != "UNKNOWN" {
		t.Fatalf("unverified futures proxy must remain unknown: %+v", latest)
	}
}
