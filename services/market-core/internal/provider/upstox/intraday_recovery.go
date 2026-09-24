package upstox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type IntradayFetcher interface {
	Fetch(context.Context, string, string, string) ([]ProviderCandle, error)
}

type RecoveredHistoryWriter interface {
	AppendBar(domain.Bar) error
}

type IntradayRecovery struct {
	Client      IntradayFetcher
	AccessToken string
	Registry    *symbol.Registry
	History     RecoveredHistoryWriter
	Timeframes    []string
	InstrumentIDs map[string]bool
}

func (r *IntradayRecovery) Recover(ctx context.Context, request RecoveryRequest) error {
	if request.Provider != ProviderName {
		return fmt.Errorf("unsupported recovery provider %q", request.Provider)
	}
	if r.Client == nil || r.Registry == nil || r.History == nil {
		return errors.New("intraday recovery requires client, registry, and history")
	}
	if request.From.IsZero() || request.To.IsZero() || !request.From.Before(request.To) {
		return errors.New("invalid recovery window")
	}

	timeframes := r.Timeframes
	if len(timeframes) == 0 {
		timeframes = []string{"1m", "3m", "5m"}
	}

	for _, providerKey := range request.InstrumentKeys {
		instrument, ok := r.Registry.ResolveProviderKey(ProviderName, providerKey)
		if !ok {
			return fmt.Errorf("recovery instrument is not registered: %s", providerKey)
		}
		if len(r.InstrumentIDs) > 0 && !r.InstrumentIDs[instrument.ID] {
			continue
		}

		for _, timeframe := range timeframes {
			interval, err := minuteInterval(timeframe)
			if err != nil {
				return err
			}
			duration := time.Duration(interval) * time.Minute

			candles, err := r.Client.Fetch(ctx, r.AccessToken, providerKey, timeframe)
			if err != nil {
				return err
			}
			for _, candle := range candles {
				closeTime := candle.OpenTime.Add(duration)
				if !closeTime.After(request.From) || closeTime.After(request.To) {
					continue
				}
				bar := domain.Bar{
					InstrumentID:        instrument.ID,
					Timeframe:           timeframe,
					OpenTime:            candle.OpenTime.UTC(),
					CloseTime:           closeTime.UTC(),
					Open:                candle.Open,
					High:                candle.High,
					Low:                 candle.Low,
					Close:               candle.Close,
					Volume:              candle.Volume,
					Final:               true,
					Revision:            0,
					AuthorityProvider:   ProviderName,
					Quality:             domain.QualityRecovered,
					Recovered:           true,
					CandleEngineVersion: "provider-recovery-v1",
				}
				if err := r.History.AppendBar(bar); err != nil {
					return fmt.Errorf("persist recovered %s bar: %w", timeframe, err)
				}
			}
		}
	}
	return nil
}
