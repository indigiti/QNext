package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/pipeline"
	"github.com/indigiti/QNext/services/market-core/internal/provider/okx"
	qruntime "github.com/indigiti/QNext/services/market-core/internal/runtime"
	"github.com/indigiti/QNext/services/market-core/internal/stream"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func registerOKXGold(registry *symbol.Registry, contract okx.Contract) error {
	if contract.InstID == "" || contract.ContractValue <= 0 {
		return errors.New("validated OKX contract required")
	}
	if err := registry.Register(symbol.Instrument{
		ID: okx.InstrumentID, Symbol: "XAUUSD-XPERP", Name: "OKX Gold/USD X-Perpetual",
		AssetClass: "FUTURES", Exchange: "OKX", Currency: "USD", Timezone: "UTC",
		CalendarID: "OKX_CRYPTO_24_7", Visible: true,
		Aliases: []string{"XAUUSD", "OKX GOLD", "GOLD XPERP"},
	}); err != nil {
		return err
	}
	return registry.RegisterProvider(symbol.ProviderInstrument{
		Provider: okx.ProviderName, InstrumentID: okx.InstrumentID, ProviderKey: contract.InstID,
	})
}

// repairOKXGold stores only exchange-confirmed final bars. All historical 5s
// buckets must have five contiguous 1s sources or stay explicitly missing.
func repairOKXGold(ctx context.Context, client okx.Client, contract okx.Contract, history *history.Store, broker *stream.Broker, from, to time.Time) error {
	if history == nil || broker == nil {
		return errors.New("history and broker required")
	}

	// Keep source exchange final candles authoritative if an overlapping
	// locally formed bar is subsequently emitted with a lower revision.
	for _, tf := range []string{"1m", "5s"} {
		start := from
		if tf == "5s" && to.Sub(start) > 5*time.Minute {
			start = to.Add(-5 * time.Minute)
		}
		if !start.Before(to) {
			continue
		}
		bars, err := client.History(ctx, contract, tf, start, to)
		if err != nil {
			return fmt.Errorf("OKX %s backfill: %w", tf, err)
		}
		previous, err := history.LoadRange(okx.InstrumentID, tf, start, to)
		if err != nil {
			return fmt.Errorf("OKX read %s prior bars: %w", tf, err)
		}
		byOpen := make(map[int64]domain.Bar, len(previous))
		for _, prior := range previous {
			byOpen[prior.OpenTime.UnixMilli()] = prior
		}
		changed := false
		for _, bar := range bars {
			key := bar.OpenTime.UnixMilli()
			if prior, ok := byOpen[key]; ok {
				if prior.Final && prior.Open == bar.Open && prior.High == bar.High &&
					prior.Low == bar.Low && prior.Close == bar.Close && prior.Volume == bar.Volume {
					continue
				}
				if prior.Revision == ^uint32(0) {
					return errors.New("OKX candle revision overflow")
				}
				bar.Revision = prior.Revision + 1
				bar.Corrected = true
				bar.CorrectedAt = time.Now().UTC()
			} else {
				bar.Revision = 1
			}
			if err := history.AppendBar(bar); err != nil {
				return err
			}
			byOpen[key] = bar
			changed = true
		}
		// Historical bars are retrieved through REST. Do not inject an older
		// recovered bar into the monotonic live WebSocket bar stream.
		if changed {
			broker.PublishResync(okx.InstrumentID, tf, "okx_history_repair")
		}
	}
	return nil
}

// runOKXGold is an independent provider path; existing NSE/BSE feed
// supervision, synthetic volume routing and trading calendars are untouched.
func runOKXGold(ctx context.Context, client okx.Client, contract okx.Contract, history *history.Store, broker *stream.Broker, onTrade func(domain.Tick)) error {
	if history == nil || broker == nil {
		return errors.New("OKX history/broker missing")
	}
	clock, err := pipeline.New(candle.New("okx-gold-trades-v1"), history,
		[]string{"5s", "15s", "30s", "1m", "2m", "3m", "5m", "15m", "30m", "1h"})
	if err != nil {
		return err
	}
	sink := &qruntime.MarketSink{
		Pipeline: clock, Publisher: broker,
		DirectInstruments: map[string]bool{okx.InstrumentID: true},
		Observer:          onTrade,
		ClockDone:         ctx.Done(),
	}
	// Start with 24 hours of 1m candle context; the 1s endpoint repairs only
	// the most recent five minutes and is NOT claimed as indefinite tick history.
	now := time.Now().UTC()
	if err := repairOKXGold(ctx, client, contract, history, broker, now.Add(-24*time.Hour), now); err != nil {
		log.Printf("OKX initial historical backfill incomplete: %v", err)
	}
	streamer := okx.Stream{OnReconnect: func(ctx context.Context) error {
		to := time.Now().UTC()
		return repairOKXGold(ctx, client, contract, history, broker, to.Add(-5*time.Minute), to)
	}}
	return streamer.Run(ctx, contract, sink.Handle)
}

// Opt-in is intentionally exclusive: no gold instrument appears in QNext
// until the provider returns the uniquely identifiable, live X-Perp contract.
// This is not the OTC XAUUSD spot/broker bid-ask series.
func discoverOKXGold(ctx context.Context, registry *symbol.Registry) (*okx.Contract, error) {
	contract, err := (okx.Client{}).DiscoverXAUUSD(ctx)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(strings.ToUpper(contract.InstID), "XAU") {
		return nil, errors.New("unexpected non-gold OKX contract")
	}
	if err := registerOKXGold(registry, contract); err != nil {
		return nil, err
	}
	return &contract, nil
}
