package upstox

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/autolegs"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
	"github.com/indigiti/QNext/services/market-core/internal/synthetic"
)

type OptionLegResolver struct {
	Client        OptionContractSource
	AccessToken   string
	UnderlyingKey string
	Registry      *symbol.Registry
}

func (r OptionLegResolver) Resolve(
	ctx context.Context,
	at time.Time,
	strikes []float64,
) (autolegs.Basket, error) {
	if r.Client == nil || r.Registry == nil {
		return autolegs.Basket{}, errors.New("option leg resolver requires client and symbol registry")
	}
	if len(strikes) == 0 {
		return autolegs.Basket{}, errors.New("option leg resolver requires strikes")
	}

	contracts, err := r.Client.Contracts(ctx, r.AccessToken, r.UnderlyingKey)
	if err != nil {
		return autolegs.Basket{}, err
	}

	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return autolegs.Basket{}, err
	}
	marketDate := at.In(location).Format("2006-01-02")

	expirySet := make(map[string]bool)
	for _, contract := range contracts {
		if contract.Expiry >= marketDate && strings.EqualFold(contract.UnderlyingKey, r.UnderlyingKey) {
			expirySet[contract.Expiry] = true
		}
	}
	expiries := make([]string, 0, len(expirySet))
	for expiry := range expirySet {
		expiries = append(expiries, expiry)
	}
	sort.Strings(expiries)

	for _, expiry := range expiries {
		legs, ok := completeBasket(contracts, r.UnderlyingKey, expiry, strikes)
		if !ok {
			continue
		}

		resolved := make([]autolegs.ResolvedLeg, 0, len(legs))
		for _, contract := range legs {
			side := synthetic.LegCall
			if strings.EqualFold(contract.InstrumentType, "PE") {
				side = synthetic.LegPut
			}

			instrumentID := canonicalOptionID(contract)
			if err := r.Registry.Ensure(symbol.Instrument{
				ID:         instrumentID,
				Symbol:     contract.TradingSymbol,
				Name:       contract.TradingSymbol,
				AssetClass: "OPTION",
				Exchange:   "NSE",
				Currency:   "INR",
				Timezone:   "Asia/Kolkata",
				CalendarID: "NSE_EQ",
				Visible:    false,
			}); err != nil {
				return autolegs.Basket{}, err
			}
			if err := r.Registry.EnsureProvider(symbol.ProviderInstrument{
				Provider:     ProviderName,
				InstrumentID: instrumentID,
				ProviderKey:  contract.InstrumentKey,
			}); err != nil {
				return autolegs.Basket{}, err
			}

			resolved = append(resolved, autolegs.ResolvedLeg{
				InstrumentID: instrumentID,
				ProviderKey:  contract.InstrumentKey,
				Strike:       contract.StrikePrice,
				Side:         side,
			})
		}
		return autolegs.Basket{Expiry: expiry, Legs: resolved}, nil
	}

	return autolegs.Basket{}, errors.New("no complete live option basket found for requested strikes")
}

func completeBasket(
	contracts []OptionContract,
	underlyingKey string,
	expiry string,
	strikes []float64,
) ([]OptionContract, bool) {
	type pair struct {
		call *OptionContract
		put  *OptionContract
	}
	pairs := make(map[float64]*pair, len(strikes))
	for _, strike := range strikes {
		pairs[strike] = &pair{}
	}

	for i := range contracts {
		contract := &contracts[i]
		if contract.Expiry != expiry || !strings.EqualFold(contract.UnderlyingKey, underlyingKey) {
			continue
		}
		strike, ok := requestedStrike(strikes, contract.StrikePrice)
		if !ok {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(contract.InstrumentType)) {
		case "CE":
			pairs[strike].call = contract
		case "PE":
			pairs[strike].put = contract
		}
	}

	result := make([]OptionContract, 0, len(strikes)*2)
	for _, strike := range strikes {
		pair := pairs[strike]
		if pair == nil || pair.call == nil || pair.put == nil {
			return nil, false
		}
		result = append(result, *pair.call, *pair.put)
	}
	return result, true
}

func requestedStrike(strikes []float64, value float64) (float64, bool) {
	for _, strike := range strikes {
		if math.Abs(strike-value) < 0.001 {
			return strike, true
		}
	}
	return 0, false
}

func canonicalOptionID(contract OptionContract) string {
	underlying := strings.ToUpper(strings.TrimSpace(contract.UnderlyingSymbol))
	if underlying == "" {
		underlying = "NIFTY"
	}
	return fmt.Sprintf(
		"NSE:%s:%s:%s:%s",
		underlying,
		contract.Expiry,
		strconv.FormatFloat(contract.StrikePrice, 'f', -1, 64),
		strings.ToUpper(strings.TrimSpace(contract.InstrumentType)),
	)
}
