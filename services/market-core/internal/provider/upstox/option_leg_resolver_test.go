package upstox

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type fakeContractSource struct {
	contracts []OptionContract
}

func (f fakeContractSource) Contracts(context.Context, string, string) ([]OptionContract, error) {
	return f.contracts, nil
}

func TestOptionLegResolverChoosesNearestCompleteExpiry(t *testing.T) {
	var contracts []OptionContract
	for _, expiry := range []string{"2026-09-29", "2026-10-06"} {
		for _, strike := range []float64{24950, 25000, 25050, 25100, 25150, 25200, 25250} {
			for _, side := range []string{"CE", "PE"} {
				contracts = append(contracts, OptionContract{
					Expiry:           expiry,
					InstrumentKey:    fmt.Sprintf("NSE_FO|%s|%.0f|%s", expiry, strike, side),
					TradingSymbol:    fmt.Sprintf("NIFTY %.0f %s", strike, side),
					InstrumentType:   side,
					UnderlyingKey:    "NSE_INDEX|Nifty 50",
					UnderlyingSymbol: "NIFTY",
					StrikePrice:      strike,
				})
			}
		}
	}

	registry := symbol.NewRegistry()
	resolver := OptionLegResolver{
		Client:        fakeContractSource{contracts: contracts},
		AccessToken:   "token",
		UnderlyingKey: "NSE_INDEX|Nifty 50",
		Registry:      registry,
	}

	basket, err := resolver.Resolve(
		context.Background(),
		time.Date(2026, 9, 25, 9, 15, 0, 0, time.FixedZone("IST", 5*60*60+30*60)),
		[]float64{24950, 25000, 25050, 25100, 25150, 25200, 25250},
	)
	if err != nil {
		t.Fatal(err)
	}
	if basket.Expiry != "2026-09-29" || len(basket.Legs) != 14 {
		t.Fatalf("unexpected basket: expiry=%s legs=%d", basket.Expiry, len(basket.Legs))
	}
	if _, ok := registry.ProviderMapping(ProviderName, basket.Legs[0].InstrumentID); !ok {
		t.Fatal("expected dynamic provider mapping to be registered")
	}
}

func TestOptionLegResolverSkipsIncompleteNearestExpiry(t *testing.T) {
	contracts := []OptionContract{
		{
			Expiry: "2026-09-29", InstrumentKey: "NSE_FO|bad", TradingSymbol: "NIFTY CE",
			InstrumentType: "CE", UnderlyingKey: "NSE_INDEX|Nifty 50",
			UnderlyingSymbol: "NIFTY", StrikePrice: 25100,
		},
	}
	for _, strike := range []float64{25050, 25100, 25150} {
		for _, side := range []string{"CE", "PE"} {
			contracts = append(contracts, OptionContract{
				Expiry:           "2026-10-06",
				InstrumentKey:    fmt.Sprintf("NSE_FO|%0.f|%s", strike, side),
				TradingSymbol:    fmt.Sprintf("NIFTY %.0f %s", strike, side),
				InstrumentType:   side,
				UnderlyingKey:    "NSE_INDEX|Nifty 50",
				UnderlyingSymbol: "NIFTY",
				StrikePrice:      strike,
			})
		}
	}

	registry := symbol.NewRegistry()
	resolver := OptionLegResolver{
		Client:        fakeContractSource{contracts: contracts},
		AccessToken:   "token",
		UnderlyingKey: "NSE_INDEX|Nifty 50",
		Registry:      registry,
	}
	basket, err := resolver.Resolve(
		context.Background(),
		time.Date(2026, 9, 25, 9, 15, 0, 0, time.FixedZone("IST", 5*60*60+30*60)),
		[]float64{25050, 25100, 25150},
	)
	if err != nil {
		t.Fatal(err)
	}
	if basket.Expiry != "2026-10-06" {
		t.Fatalf("expected next complete expiry, got %s", basket.Expiry)
	}
}
