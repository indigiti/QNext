package market

import (
	"errors"
	"time"
)

type CandleBuilder struct {
	timeframe time.Duration
	version   string
	current   *Bar
}

func NewCandleBuilder(timeframe time.Duration, version string) (*CandleBuilder, error) {
	if timeframe <= 0 {
		return nil, errors.New("timeframe must be positive")
	}
	if version == "" {
		return nil, errors.New("candle engine version is required")
	}
	return &CandleBuilder{timeframe: timeframe, version: version}, nil
}

func (b *CandleBuilder) Apply(t CanonicalTick) (updated Bar, finalized *Bar, err error) {
	if err := t.Validate(); err != nil {
		return Bar{}, nil, err
	}

	openTime := t.EventTime.Truncate(b.timeframe)
	closeTime := openTime.Add(b.timeframe)

	if b.current == nil {
		b.current = b.newBar(t, openTime, closeTime)
		return *b.current, nil, nil
	}

	if t.EventTime.Before(b.current.OpenTime) {
		return Bar{}, nil, errors.New("out-of-order tick predates current candle")
	}

	if !t.EventTime.Before(b.current.CloseTime) {
		closed := *b.current
		closed.Final = true
		b.current = b.newBar(t, openTime, closeTime)
		return *b.current, &closed, nil
	}

	if t.Price > b.current.High {
		b.current.High = t.Price
	}
	if t.Price < b.current.Low {
		b.current.Low = t.Price
	}
	b.current.Close = t.Price
	b.current.Volume += t.VolumeDelta
	b.current.SourceSequence = t.Sequence
	b.current.Quality = worstQuality(b.current.Quality, t.Quality)
	return *b.current, nil, nil
}

func (b *CandleBuilder) newBar(t CanonicalTick, openTime, closeTime time.Time) *Bar {
	return &Bar{
		Schema:            SchemaBarV1,
		InstrumentID:      t.InstrumentID,
		Timeframe:         b.timeframe.String(),
		Open:              t.Price,
		High:              t.Price,
		Low:               t.Price,
		Close:             t.Price,
		Volume:            t.VolumeDelta,
		OpenTime:          openTime,
		CloseTime:         closeTime,
		Final:             false,
		Revision:          0,
		AuthorityProvider: t.Provider,
		Quality:           t.Quality,
		SourceSequence:    t.Sequence,
		CandleEngine:      b.version,
		CreatedAt:         t.ProcessedTime,
	}
}

func worstQuality(a, b Quality) Quality {
	rank := map[Quality]int{
		QualityGood:      0,
		QualityRecovered: 1,
		QualityPartial:   2,
		QualityStale:     3,
		QualityDegraded:  4,
		QualityInvalid:   5,
	}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
