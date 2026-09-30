package domain

import (
	"fmt"
	"time"
)

type Quality string

const (
	QualityGood      Quality = "GOOD"
	QualityRecovered Quality = "RECOVERED"
	QualityPartial   Quality = "PARTIAL"
	QualityStale     Quality = "STALE"
	QualityDegraded  Quality = "DEGRADED"
	QualityInvalid   Quality = "INVALID"
)

type Tick struct {
	InstrumentID     string
	Provider         string
	Price            float64
	Quantity         float64
	// EventTime is the provider market-data clock used for sequencing and
	// candle bucketing. It must advance with fresh market-data messages even
	// when the last traded time does not (notably for index feeds).
	EventTime time.Time
	// TradeTime preserves the provider's last-traded timestamp when one is
	// available. It is informational and must not drive candle chronology.
	TradeTime        time.Time
	ReceivedTime     time.Time
	ProcessedTime    time.Time
	PublishedTime    time.Time
	Sequence         uint64
	Quality          Quality
	SyntheticVersion string
}

type Bar struct {
	InstrumentID        string
	Timeframe           string
	OpenTime            time.Time
	CloseTime           time.Time
	Open                float64
	High                float64
	Low                 float64
	Close               float64
	Volume              float64
	Final               bool
	Revision            uint32
	AuthorityProvider   string
	Quality             Quality
	Recovered           bool
	Corrected           bool
	SourceSequence      uint64
	CandleEngineVersion string
	SyntheticVersion    string
	CreatedAt           time.Time
	CorrectedAt         time.Time
}

func (b Bar) Key() string {
	return fmt.Sprintf("%s|%s|%d", b.InstrumentID, b.Timeframe, b.OpenTime.UnixMilli())
}
