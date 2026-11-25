package main

import (
	"context"
	"errors"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/provider/bitunix"
	"github.com/indigiti/QNext/services/market-core/internal/provider/directfeed"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func registerBitunixAdapter(r *directfeed.Registry) error {
	client := bitunix.Client{}
	return r.Register("bitunix-perpetual", func(ctx context.Context, spec directfeed.Spec) (directfeed.Binding, error) {
		if spec.InstrumentID != "BITUNIX:"+spec.ProviderKey {
			return directfeed.Binding{}, errors.New("Bitunix canonical and provider identity mismatch")
		}
		contract, err := client.Discover(ctx, spec.ProviderKey)
		if err != nil {
			return directfeed.Binding{}, err
		}
		return directfeed.Binding{
			Instrument: symbol.Instrument{
				ID: contract.ID(), Symbol: contract.Symbol, Name: "Bitunix " + contract.Symbol,
				AssetClass: "FUTURES", MarketCategory: "Crypto", ProductType: "perpetual", DataMode: "stream", Exchange: "BITUNIX", Currency: contract.Quote, Timezone: "UTC",
				CalendarID: "BITUNIX_CRYPTO_24_7", Visible: true,
			},
			Provider: bitunix.ProviderName, Key: contract.Symbol, HistoryTimeframes: []string{"1m"},
			History: func(ctx context.Context, tf string, from, to time.Time) ([]domain.Bar, error) {
				return client.History(ctx, contract, tf, from, to)
			},
			Run: func(ctx context.Context, sink func(domain.Tick) error, repair func(context.Context) error) error {
				return (bitunix.Stream{}).Run(ctx, contract, sink, repair)
			},
		}, nil
	})
}
