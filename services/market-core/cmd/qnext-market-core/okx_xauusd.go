package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/provider/directfeed"
	"github.com/indigiti/QNext/services/market-core/internal/provider/okx"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func okxGoldInstrument() symbol.Instrument {
	return symbol.Instrument{
		ID: okx.InstrumentID, Symbol: "XAUUSD-XPERP", Name: "OKX Gold/USD X-Perpetual",
		AssetClass: "FUTURES", MarketCategory: "Commodities", ProductType: "perpetual", DataMode: "stream", Exchange: "OKX", Currency: "USD", Timezone: "UTC",
		CalendarID: "OKX_CRYPTO_24_7", Visible: true,
		Aliases: []string{"XAUUSD", "OKX GOLD", "GOLD XPERP"},
	}
}

func okxSwapInstrument(contract okx.Contract) symbol.Instrument {
	if contract.InstrumentID == okx.SwapInstrumentID {
		return symbol.Instrument{
			ID: okx.SwapInstrumentID, Symbol: "XAU-USDT-SWAP", Name: "OKX Gold/USDT Perpetual Swap",
			AssetClass: "FUTURES", MarketCategory: "Commodities", ProductType: "perpetual", DataMode: "stream", Exchange: "OKX", Currency: "USDT", Timezone: "UTC",
			CalendarID: "OKX_CRYPTO_24_7", Visible: true,
			Aliases: []string{"XAUUSDT-SWAP", "OKX GOLD USDT"},
		}
	}
	parts := strings.Split(contract.InstID, "-")
	return symbol.Instrument{
		ID: contract.InstrumentID, Symbol: contract.InstID, Name: "OKX " + contract.InstID,
		AssetClass: "FUTURES", MarketCategory: "Crypto", ProductType: "perpetual", DataMode: "stream", Exchange: "OKX", Currency: parts[1], Timezone: "UTC",
		CalendarID: "OKX_CRYPTO_24_7", Visible: true,
	}
}

func registerOKXGold(registry *symbol.Registry, contract okx.Contract) error {
	if contract.InstID == "" || contract.ContractValue <= 0 {
		return errors.New("validated OKX contract required")
	}
	return registry.RegisterMapped(okxGoldInstrument(), symbol.ProviderInstrument{
		Provider: okx.ProviderName, InstrumentID: okx.InstrumentID, ProviderKey: contract.InstID,
	})
}

func registerOKXSwap(registry *symbol.Registry, contract okx.Contract) error {
	if contract.InstrumentID != okx.SwapInstrumentID || contract.InstID != okx.SwapProviderID || contract.ContractValue <= 0 {
		return errors.New("validated live OKX XAU-USDT-SWAP required")
	}
	return registry.RegisterMapped(okxSwapInstrument(contract), symbol.ProviderInstrument{
		Provider: okx.ProviderName, InstrumentID: okx.SwapInstrumentID, ProviderKey: contract.InstID,
	})
}

func okxBinding(client okx.Client, contract okx.Contract, instrument symbol.Instrument) directfeed.Binding {
	return directfeed.Binding{
		Instrument: instrument, Provider: okx.ProviderName, Key: contract.InstID,
		History: func(ctx context.Context, tf string, from, to time.Time) ([]domain.Bar, error) {
			return client.History(ctx, contract, tf, from, to)
		},
		Run: func(ctx context.Context, sink func(domain.Tick) error, repair func(context.Context) error) error {
			return (okx.Stream{OnReconnect: repair}).Run(ctx, contract, sink)
		},
	}
}

func newDirectAdapters(client okx.Client) *directfeed.Registry {
	r := directfeed.NewRegistry()
	if err := r.Register("okx-xperp", func(ctx context.Context, spec directfeed.Spec) (directfeed.Binding, error) {
		if spec.InstrumentID != okx.InstrumentID || spec.ProviderKey != "" {
			return directfeed.Binding{}, errors.New("unsupported OKX X-Perp identity")
		}
		contract, err := client.DiscoverXAUUSD(ctx)
		if err != nil {
			return directfeed.Binding{}, err
		}
		return okxBinding(client, contract, okxGoldInstrument()), nil
	}); err != nil {
		panic(err)
	}
	if err := r.Register("okx-linear-swap", func(ctx context.Context, spec directfeed.Spec) (directfeed.Binding, error) {
		if spec.InstrumentID != "OKX:"+spec.ProviderKey {
			return directfeed.Binding{}, errors.New("OKX canonical and provider identity mismatch")
		}
		contract, err := client.DiscoverLinearSwap(ctx, spec.ProviderKey)
		if err != nil {
			return directfeed.Binding{}, err
		}
		return okxBinding(client, contract, okxSwapInstrument(contract)), nil
	}); err != nil {
		panic(err)
	}
	return r
}
