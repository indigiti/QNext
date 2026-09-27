package upstox

import (
	"fmt"
	"testing"
	"time"
)

func TestBuildResearchPlanTiersCurrentAndNextExpiry(t *testing.T) {
	contracts := makeResearchContracts("NSE_INDEX|Nifty 50", []string{"2026-10-01", "2026-10-08"}, 24000, 50, 25)
	plan, err := BuildResearchPlan(
		contracts,
		"NSE_INDEX|Nifty 50",
		24012,
		time.Date(2026, 9, 27, 6, 0, 0, 0, time.UTC),
		20,
		5,
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ATM != 24000 || plan.CurrentExpiry != "2026-10-01" || plan.NextExpiry != "2026-10-08" {
		t.Fatalf("unexpected plan identity: %+v", plan)
	}
	if len(plan.FullKeys) != 22 {
		t.Fatalf("full tier = %d, want 22", len(plan.FullKeys))
	}
	if len(plan.GreekKeys) != 142 {
		t.Fatalf("greek tier = %d, want 142", len(plan.GreekKeys))
	}
	if len(plan.Contracts) != 164 {
		t.Fatalf("selected contracts = %d, want 164", len(plan.Contracts))
	}
}

func TestBuildResearchPlanRequiresTwoExpiries(t *testing.T) {
	contracts := makeResearchContracts("NSE_INDEX|Nifty 50", []string{"2026-10-01"}, 24000, 50, 25)
	_, err := BuildResearchPlan(
		contracts,
		"NSE_INDEX|Nifty 50",
		24000,
		time.Date(2026, 9, 27, 6, 0, 0, 0, time.UTC),
		20,
		5,
	)
	if err == nil {
		t.Fatal("expected two-expiry validation error")
	}
}

func makeResearchContracts(underlying string, expiries []string, atm, interval float64, wings int) []OptionContract {
	var contracts []OptionContract
	for _, expiry := range expiries {
		for offset := -wings; offset <= wings; offset++ {
			strike := atm + float64(offset)*interval
			for _, side := range []string{"CE", "PE"} {
				key := fmt.Sprintf("NSE_FO|%s|%.0f|%s", expiry, strike, side)
				contracts = append(contracts, OptionContract{
					Name:             "NIFTY",
					Expiry:           expiry,
					InstrumentKey:    key,
					TradingSymbol:    key,
					InstrumentType:   side,
					UnderlyingKey:    underlying,
					UnderlyingSymbol: "NIFTY",
					StrikePrice:      strike,
				})
			}
		}
	}
	return contracts
}
