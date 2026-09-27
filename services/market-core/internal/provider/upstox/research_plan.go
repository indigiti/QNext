package upstox

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

type researchPair struct {
	call *OptionContract
	put  *OptionContract
}

type ResearchPlan struct {
	UnderlyingKey string                    `json:"underlying_key"`
	Spot          float64                   `json:"spot"`
	ATM           float64                   `json:"atm"`
	CurrentExpiry string                    `json:"current_expiry"`
	NextExpiry    string                    `json:"next_expiry"`
	FullKeys      []string                  `json:"full_keys"`
	GreekKeys     []string                  `json:"greek_keys"`
	Contracts     map[string]OptionContract `json:"contracts"`
}

func BuildResearchPlan(
	contracts []OptionContract,
	underlyingKey string,
	spot float64,
	now time.Time,
	wingStrikes int,
	fullWingStrikes int,
) (ResearchPlan, error) {
	if strings.TrimSpace(underlyingKey) == "" || spot <= 0 {
		return ResearchPlan{}, errors.New("research plan requires underlying key and positive spot")
	}
	if wingStrikes < 1 || fullWingStrikes < 0 || fullWingStrikes > wingStrikes {
		return ResearchPlan{}, errors.New("invalid research strike wings")
	}

	today := now.In(time.FixedZone("IST", 5*60*60+30*60)).Format("2006-01-02")
	byExpiry := make(map[string]map[float64]*researchPair)
	for i := range contracts {
		contract := contracts[i]
		if contract.InstrumentKey == "" || contract.StrikePrice <= 0 || contract.Expiry == "" {
			continue
		}
		if contract.UnderlyingKey != "" && contract.UnderlyingKey != underlyingKey {
			continue
		}
		if contract.Expiry < today {
			continue
		}
		side := researchOptionSide(contract.InstrumentType)
		if side == "" {
			continue
		}
		strikes := byExpiry[contract.Expiry]
		if strikes == nil {
			strikes = make(map[float64]*researchPair)
			byExpiry[contract.Expiry] = strikes
		}
		entry := strikes[contract.StrikePrice]
		if entry == nil {
			entry = &researchPair{}
			strikes[contract.StrikePrice] = entry
		}
		copyContract := contract
		if side == "CALL" {
			entry.call = &copyContract
		} else {
			entry.put = &copyContract
		}
	}

	expiries := make([]string, 0, len(byExpiry))
	for expiry, strikes := range byExpiry {
		complete := false
		for _, entry := range strikes {
			if entry.call != nil && entry.put != nil {
				complete = true
				break
			}
		}
		if complete {
			expiries = append(expiries, expiry)
		}
	}
	sort.Strings(expiries)
	if len(expiries) < 2 {
		return ResearchPlan{}, errors.New("research plan requires current and next option expiry")
	}

	plan := ResearchPlan{
		UnderlyingKey: underlyingKey,
		Spot:          spot,
		CurrentExpiry: expiries[0],
		NextExpiry:    expiries[1],
		Contracts:     make(map[string]OptionContract),
	}
	fullSet := make(map[string]bool)
	greekSet := make(map[string]bool)
	for expiryIndex, expiry := range []string{plan.CurrentExpiry, plan.NextExpiry} {
		strikes := researchCompleteStrikes(byExpiry[expiry])
		if len(strikes) == 0 {
			return ResearchPlan{}, errors.New("research expiry has no complete call/put strikes")
		}
		center := researchNearestStrikeIndex(strikes, spot)
		if expiryIndex == 0 {
			plan.ATM = strikes[center]
		}
		lo := researchMaxInt(0, center-wingStrikes)
		hi := researchMinInt(len(strikes)-1, center+wingStrikes)
		for index := lo; index <= hi; index++ {
			strike := strikes[index]
			entry := byExpiry[expiry][strike]
			for _, contract := range []*OptionContract{entry.call, entry.put} {
				if contract == nil {
					continue
				}
				plan.Contracts[contract.InstrumentKey] = *contract
				isFull := expiryIndex == 0 && researchAbsInt(index-center) <= fullWingStrikes
				if isFull {
					fullSet[contract.InstrumentKey] = true
					delete(greekSet, contract.InstrumentKey)
				} else if !fullSet[contract.InstrumentKey] {
					greekSet[contract.InstrumentKey] = true
				}
			}
		}
	}
	plan.FullKeys = researchSortedKeySet(fullSet)
	plan.GreekKeys = researchSortedKeySet(greekSet)
	if len(plan.Contracts) == 0 {
		return ResearchPlan{}, errors.New("research plan selected no option contracts")
	}
	return plan, nil
}

func ResearchATM(contracts []OptionContract, expiry string, spot float64) (float64, bool) {
	if spot <= 0 || expiry == "" {
		return 0, false
	}
	pairs := make(map[float64]*researchPair)
	for i := range contracts {
		contract := contracts[i]
		if contract.Expiry != expiry || contract.StrikePrice <= 0 {
			continue
		}
		pair := pairs[contract.StrikePrice]
		if pair == nil {
			pair = &researchPair{}
			pairs[contract.StrikePrice] = pair
		}
		switch researchOptionSide(contract.InstrumentType) {
		case "CALL":
			pair.call = &contract
		case "PUT":
			pair.put = &contract
		}
	}
	values := researchCompleteStrikes(pairs)
	if len(values) == 0 {
		return 0, false
	}
	return values[researchNearestStrikeIndex(values, spot)], true
}

func researchOptionSide(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "CE", "CALL":
		return "CALL"
	case "PE", "PUT":
		return "PUT"
	default:
		return ""
	}
}

func researchCompleteStrikes(strikes map[float64]*researchPair) []float64 {
	values := make([]float64, 0, len(strikes))
	for strike, entry := range strikes {
		if entry != nil && entry.call != nil && entry.put != nil {
			values = append(values, strike)
		}
	}
	sort.Float64s(values)
	return values
}

func researchNearestStrikeIndex(strikes []float64, spot float64) int {
	best := 0
	bestDistance := math.Abs(strikes[0] - spot)
	for i := 1; i < len(strikes); i++ {
		distance := math.Abs(strikes[i] - spot)
		if distance < bestDistance || (distance == bestDistance && strikes[i] < strikes[best]) {
			best = i
			bestDistance = distance
		}
	}
	return best
}

func researchSortedKeySet(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func researchMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func researchMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func researchAbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
