package market

import (
	"errors"
	"time"
)

const (
	SchemaCanonicalTickV1 = "QNEXT.MARKET.CANONICAL_TICK/1"
	SchemaBarV1           = "QNEXT.MARKET.BAR/1"
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

type CanonicalTick struct {
	Schema        string    `json:"schema"`
	InstrumentID  string    `json:"instrument_id"`
	Provider      string    `json:"provider"`
	Price         float64   `json:"price"`
	Volume        float64   `json:"volume,omitempty"`
	EventTime     time.Time `json:"event_time"`
	ReceivedTime  time.Time `json:"received_time"`
	ProcessedTime time.Time `json:"processed_time"`
	PublishedTime time.Time `json:"published_time,omitempty"`
	Sequence      uint64    `json:"sequence"`
	Quality       Quality   `json:"quality"`
}

func (t CanonicalTick) Validate() error {
	if t.Schema != SchemaCanonicalTickV1 {
		return errors.New("unsupported canonical tick schema")
	}
	if t.InstrumentID == "" {
		return errors.New("instrument_id is required")
	}
	if t.Provider == "" {
		return errors.New("provider is required")
	}
	if t.Price <= 0 {
		return errors.New("price must be positive")
	}
	if t.EventTime.IsZero() || t.ReceivedTime.IsZero() || t.ProcessedTime.IsZero() {
		return errors.New("event_time, received_time and processed_time are required")
	}
	if t.ReceivedTime.Before(t.EventTime) {
		return errors.New("received_time cannot precede event_time")
	}
	if t.ProcessedTime.Before(t.ReceivedTime) {
		return errors.New("processed_time cannot precede received_time")
	}
	if t.Sequence == 0 {
		return errors.New("sequence must be non-zero")
	}
	if t.Quality == "" {
		return errors.New("quality is required")
	}
	return nil
}

type Bar struct {
	Schema            string    `json:"schema"`
	InstrumentID      string    `json:"instrument_id"`
	Timeframe         string    `json:"timeframe"`
	Open              float64   `json:"open"`
	High              float64   `json:"high"`
	Low               float64   `json:"low"`
	Close             float64   `json:"close"`
	Volume            float64   `json:"volume"`
	OpenTime          time.Time `json:"open_time"`
	CloseTime         time.Time `json:"close_time"`
	Final             bool      `json:"final"`
	Revision          uint32    `json:"revision"`
	AuthorityProvider string    `json:"authority_provider"`
	Quality           Quality   `json:"quality"`
	Recovered         bool      `json:"recovered"`
	Corrected         bool      `json:"corrected"`
	SourceSequence    uint64    `json:"source_sequence"`
	CandleEngine      string    `json:"candle_engine_version"`
	SyntheticVersion  string    `json:"synthetic_version,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	CorrectedAt       time.Time `json:"corrected_at,omitempty"`
}
