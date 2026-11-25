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
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/pipeline"
	"github.com/indigiti/QNext/services/market-core/internal/provider/directfeed"
	qruntime "github.com/indigiti/QNext/services/market-core/internal/runtime"
	"github.com/indigiti/QNext/services/market-core/internal/stream"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type directFeedState struct {
	Spec    directfeed.Spec    `json:"spec"`
	Active  bool               `json:"active"`
	Error   string             `json:"error,omitempty"`
	Binding directfeed.Binding `json:"-"`
}

func prepareDirectFeeds(ctx context.Context, adapters *directfeed.Registry, symbols *symbol.Registry, calendars *marketcalendar.Registry, specs []directfeed.Spec) []directFeedState {
	states := make([]directFeedState, 0, len(specs))
	// Bound aggregate startup discovery rather than multiplying timeout by symbols.
	discoveryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, spec := range specs {
		state := directFeedState{Spec: spec}
		if spec.Enabled {
			single, stop := context.WithTimeout(discoveryCtx, 15*time.Second)
			binding, err := adapters.Prepare(single, spec, symbols, calendars)
			stop()
			if err != nil {
				state.Error = err.Error()
				log.Printf("direct feed %s unavailable: %v", spec.InstrumentID, err)
			} else {
				state.Active = true
				state.Binding = binding
			}
		}
		states = append(states, state)
	}
	return states
}

func configuredDirectFeeds(path string, reserved ...string) ([]directfeed.Spec, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	specs, err := directfeed.Load(path)
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		for _, id := range reserved {
			if spec.InstrumentID == id {
				return nil, fmt.Errorf("legacy direct feed %s must use its existing admin setting", id)
			}
		}
	}
	return specs, nil
}

func runDirectFeeds(ctx context.Context, states []directFeedState, store *history.Store, broker *stream.Broker, observe func(domain.Tick)) error {
	ids := make(map[string]bool)
	for _, state := range states {
		if state.Active {
			ids[state.Spec.InstrumentID] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	// These direct adapters currently use the same certified UTC candle policy.
	// One pipeline and finalization clock for all compatible direct instruments.
	clock, err := pipeline.New(candle.NewWithUTCInstruments("okx-gold-trades-v1", ids), store,
		[]string{"5s", "15s", "30s", "1m", "2m", "3m", "5m", "15m", "30m", "1h"})
	if err != nil {
		return err
	}
	sink := &qruntime.MarketSink{
		Pipeline: clock, Publisher: broker, DirectInstruments: ids,
		Observer: observe, ClockDone: ctx.Done(),
	}
	// Limit simultaneous initial REST backfills; no goroutine per timeframe.
	initial := make(chan struct{}, 2)
	for _, state := range states {
		if !state.Active {
			continue
		}
		binding := state.Binding
		go func() {
			select {
			case initial <- struct{}{}:
			case <-ctx.Done():
				return
			}
			now := time.Now().UTC()
			if err := repairDirectFeed(ctx, binding, store, broker, now.Add(-24*time.Hour), now); err != nil {
				log.Printf("direct feed %s initial backfill incomplete: %v", binding.Instrument.ID, err)
			}
			<-initial
			handle := func(tick domain.Tick) error {
				if tick.InstrumentID != binding.Instrument.ID || tick.Provider != binding.Provider {
					return errors.New("direct adapter tick identity mismatch")
				}
				return sink.Handle(tick)
			}
			if err := binding.Run(ctx, handle, func(ctx context.Context) error {
				to := time.Now().UTC()
				return repairDirectFeed(ctx, binding, store, broker, to.Add(-5*time.Minute), to)
			}); err != nil && ctx.Err() == nil {
				log.Printf("direct feed %s ingestion stopped: %v", binding.Instrument.ID, err)
			}
		}()
	}
	return nil
}

func repairDirectFeed(ctx context.Context, binding directfeed.Binding, history *history.Store, broker *stream.Broker, from, to time.Time) error {
	if binding.Instrument.ID == "" {
		return errors.New("direct feed canonical instrument ID missing")
	}
	if history == nil || broker == nil {
		return errors.New("history and broker required")
	}

	// Keep source exchange final candles authoritative if an overlapping
	// locally formed bar is subsequently emitted with a lower revision.
	timeframes := binding.HistoryTimeframes
	if timeframes == nil {
		timeframes = []string{"1m", "5s"}
	}
	for _, tf := range timeframes {
		start := from
		if tf == "5s" && to.Sub(start) > 5*time.Minute {
			start = to.Add(-5 * time.Minute)
		}
		if !start.Before(to) {
			continue
		}
		bars, err := binding.History(ctx, tf, start, to)
		if err != nil {
			return fmt.Errorf("direct feed %s backfill: %w", tf, err)
		}
		previous, err := history.LoadRange(binding.Instrument.ID, tf, start, to)
		if err != nil {
			return fmt.Errorf("direct feed read %s prior bars: %w", tf, err)
		}
		byOpen := make(map[int64]domain.Bar, len(previous))
		for _, prior := range previous {
			byOpen[prior.OpenTime.UnixMilli()] = prior
		}
		changed := false
		for _, bar := range bars {
			if bar.InstrumentID != binding.Instrument.ID || bar.Timeframe != tf ||
				bar.AuthorityProvider != binding.Provider || !bar.Final ||
				bar.OpenTime.Before(start) || !bar.OpenTime.Before(to) || bar.CloseTime.After(to) {
				return errors.New("direct history returned mismatched or unconfirmed bar")
			}
			key := bar.OpenTime.UnixMilli()
			if prior, ok := byOpen[key]; ok {
				if prior.Final && prior.Open == bar.Open && prior.High == bar.High &&
					prior.Low == bar.Low && prior.Close == bar.Close && prior.Volume == bar.Volume {
					continue
				}
				if prior.Revision == ^uint32(0) {
					return errors.New("direct feed candle revision overflow")
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
			broker.PublishResync(binding.Instrument.ID, tf, binding.Provider+"_history_repair")
		}
	}
	return nil
}
