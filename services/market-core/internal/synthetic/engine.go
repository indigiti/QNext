package synthetic

import (
	"errors"
	"sort"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type Definition struct {
	ID                    string
	Version               string
	MinimumValidCandidates int
	MaxLegAge             time.Duration
	MaxLegTimeSkew        time.Duration
}

type Candidate struct {
	Strike        float64
	CallPrice     float64
	PutPrice      float64
	CallEventTime time.Time
	PutEventTime  time.Time
}

type EvaluatedCandidate struct {
	Candidate
	Value           float64
	Accepted        bool
	RejectionReason string
}

type Observation struct {
	SyntheticID       string
	DefinitionVersion string
	EventTime         time.Time
	Value             float64
	Quality           domain.Quality
	Candidates        []EvaluatedCandidate
}

func Compute(def Definition, at time.Time, candidates []Candidate) (Observation, error) {
	if def.ID == "" || def.Version == "" {
		return Observation{}, errors.New("synthetic definition requires id and version")
	}
	if def.MinimumValidCandidates <= 0 {
		return Observation{}, errors.New("minimum valid candidates must be positive")
	}

	out := Observation{
		SyntheticID:       def.ID,
		DefinitionVersion: def.Version,
		EventTime:         at.UTC(),
		Quality:           domain.QualityInvalid,
	}

	values := make([]float64, 0, len(candidates))
	for _, c := range candidates {
		e := EvaluatedCandidate{Candidate: c}
		switch {
		case c.CallEventTime.IsZero() || c.PutEventTime.IsZero():
			e.RejectionReason = "MISSING_TIME"
		case def.MaxLegAge > 0 && (at.Sub(c.CallEventTime) > def.MaxLegAge || at.Sub(c.PutEventTime) > def.MaxLegAge):
			e.RejectionReason = "STALE_LEG"
		case def.MaxLegTimeSkew > 0 && absDuration(c.CallEventTime.Sub(c.PutEventTime)) > def.MaxLegTimeSkew:
			e.RejectionReason = "LEG_TIME_SKEW"
		default:
			e.Accepted = true
			e.Value = c.Strike + c.CallPrice - c.PutPrice
			values = append(values, e.Value)
		}
		out.Candidates = append(out.Candidates, e)
	}

	if len(values) < def.MinimumValidCandidates {
		out.Quality = domain.QualityDegraded
		return out, nil
	}

	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		out.Value = values[mid]
	} else {
		out.Value = (values[mid-1] + values[mid]) / 2
	}
	out.Quality = domain.QualityGood
	return out, nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
