package synthetic

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

const SyntheticProvider = "qnext-synthetic"

type LegSide string

const (
	LegCall LegSide = "CALL"
	LegPut  LegSide = "PUT"
)

type LegBinding struct {
	InstrumentID string
	Strike       float64
	Side         LegSide
}

type Assembler struct {
	mu       sync.Mutex
	def      Definition
	bindings map[string]LegBinding
	byStrike map[float64]map[LegSide]string
	latest   map[string]domain.Tick
	nextSeq  func() uint64
}

func NewAssembler(def Definition, legs []LegBinding, nextSeq func() uint64) (*Assembler, error) {
	if def.ID == "" || def.Version == "" {
		return nil, errors.New("synthetic definition requires id and version")
	}
	if nextSeq == nil {
		return nil, errors.New("synthetic sequence generator is required")
	}
	if len(legs) == 0 {
		return nil, errors.New("synthetic legs are required")
	}

	bindings := make(map[string]LegBinding, len(legs))
	byStrike := make(map[float64]map[LegSide]string)
	for _, leg := range legs {
		if leg.InstrumentID == "" || leg.Strike <= 0 {
			return nil, errors.New("synthetic leg requires instrument and positive strike")
		}
		if leg.Side != LegCall && leg.Side != LegPut {
			return nil, errors.New("synthetic leg side must be CALL or PUT")
		}
		if _, exists := bindings[leg.InstrumentID]; exists {
			return nil, errors.New("duplicate synthetic leg instrument")
		}
		if byStrike[leg.Strike] == nil {
			byStrike[leg.Strike] = make(map[LegSide]string)
		}
		if _, exists := byStrike[leg.Strike][leg.Side]; exists {
			return nil, errors.New("duplicate synthetic leg side for strike")
		}
		bindings[leg.InstrumentID] = leg
		byStrike[leg.Strike][leg.Side] = leg.InstrumentID
	}
	for _, pair := range byStrike {
		if pair[LegCall] == "" || pair[LegPut] == "" {
			return nil, errors.New("every synthetic strike requires call and put legs")
		}
	}

	return &Assembler{
		def:      def,
		bindings: bindings,
		byStrike: byStrike,
		latest:   make(map[string]domain.Tick),
		nextSeq:  nextSeq,
	}, nil
}

func (a *Assembler) Apply(tick domain.Tick) (domain.Tick, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.bindings[tick.InstrumentID]; !ok {
		return domain.Tick{}, false, nil
	}
	if tick.EventTime.IsZero() || tick.Price <= 0 {
		return domain.Tick{}, false, errors.New("synthetic leg tick requires event time and positive price")
	}
	a.latest[tick.InstrumentID] = tick

	strikes := make([]float64, 0, len(a.byStrike))
	for strike := range a.byStrike {
		strikes = append(strikes, strike)
	}
	sort.Float64s(strikes)

	at := tick.EventTime.UTC()
	for _, latest := range a.latest {
		if latest.EventTime.After(at) {
			at = latest.EventTime.UTC()
		}
	}

	candidates := make([]Candidate, 0, len(strikes))
	for _, strike := range strikes {
		pair := a.byStrike[strike]
		call := a.latest[pair[LegCall]]
		put := a.latest[pair[LegPut]]
		candidates = append(candidates, Candidate{
			Strike:        strike,
			CallPrice:     call.Price,
			PutPrice:      put.Price,
			CallEventTime: call.EventTime,
			PutEventTime:  put.EventTime,
		})
	}

	observation, err := Compute(a.def, at, candidates)
	if err != nil {
		return domain.Tick{}, false, err
	}
	if observation.Quality != domain.QualityGood {
		return domain.Tick{}, false, nil
	}

	received := tick.ReceivedTime
	processed := tick.ProcessedTime
	if processed.IsZero() {
		processed = time.Now().UTC()
	}

	return domain.Tick{
		InstrumentID:     a.def.ID,
		Provider:         SyntheticProvider,
		Price:            observation.Value,
		EventTime:        observation.EventTime,
		ReceivedTime:     received,
		ProcessedTime:    processed,
		Sequence:         a.nextSeq(),
		Quality:          observation.Quality,
		SyntheticVersion: a.def.Version,
	}, true, nil
}
