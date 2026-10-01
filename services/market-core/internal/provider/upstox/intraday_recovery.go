package upstox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type IntradayFetcher interface {
	Fetch(context.Context, string, string, string) ([]ProviderCandle, error)
}

type RecoveredHistoryWriter interface {
	AppendBar(domain.Bar) error
}

type RecoveryResyncPublisher interface {
	PublishResync(instrumentID, timeframe, reason string)
}

type IntradayRecovery struct {
	Client              IntradayFetcher
	AccessToken         string
	Registry            *symbol.Registry
	History             RecoveredHistoryWriter
	Calendars           *marketcalendar.Registry
	Timeframes          []string
	DerivedTimeframes   []string
	InstrumentIDs       map[string]bool
	Resync              RecoveryResyncPublisher
	Status              *GapRecoveryTracker
}

func (r *IntradayRecovery) Recover(ctx context.Context, request RecoveryRequest) (err error) {
	var recoveredBars uint64
	defer func() {
		if r.Status != nil {
			r.Status.Observe(request, recoveredBars, err)
		}
	}()
	if request.Provider != ProviderName {
		return fmt.Errorf("unsupported recovery provider %q", request.Provider)
	}
	if r.Client == nil || r.Registry == nil || r.History == nil {
		return errors.New("intraday recovery requires client, registry, and history")
	}
	if request.From.IsZero() || request.To.IsZero() || !request.From.Before(request.To) {
		return errors.New("invalid recovery window")
	}

	// Upstox is the provider authority only for the canonical minute source.
	// Derived intraday candles are rebuilt below from repaired 1m history. This
	// keeps one canonical recovery source and prevents 3m/5m provider candles
	// from drifting from QNext's session-aligned rollup semantics.
	timeframes := r.Timeframes
	if len(timeframes) == 0 {
		timeframes = []string{"1m"}
	}

	for _, providerKey := range request.InstrumentKeys {
		instrument, ok := r.Registry.ResolveProviderKey(ProviderName, providerKey)
		if !ok {
			return fmt.Errorf("recovery instrument is not registered: %s", providerKey)
		}
		if len(r.InstrumentIDs) > 0 && !r.InstrumentIDs[instrument.ID] {
			continue
		}

		from := request.From
		if cursor := request.FromByInstrumentID[instrument.ID]; !cursor.IsZero() {
			from = cursor
		}
		if from.IsZero() || !from.Before(request.To) {
			continue
		}

		canonicalChanged := false
		for _, timeframe := range timeframes {
			interval, intervalErr := minuteInterval(timeframe)
			if intervalErr != nil {
				return intervalErr
			}
			duration := time.Duration(interval) * time.Minute

			candles, fetchErr := r.Client.Fetch(ctx, r.AccessToken, providerKey, timeframe)
			if fetchErr != nil {
				return fetchErr
			}
			candidates := make([]domain.Bar, 0, len(candles))
			for _, candle := range candles {
				closeTime := candle.OpenTime.Add(duration)
				if !closeTime.After(from) || closeTime.After(request.To) {
					continue
				}
				bar := providerBar(instrument.ID, timeframe, candle, duration)
				bar.CandleEngineVersion = "provider-gap-recovery-v2"
				candidates = append(candidates, bar)
			}

			changedCount, changed, repairErr := r.repairCandidates(from, request.To, candidates)
			if repairErr != nil {
				return fmt.Errorf("persist recovered %s bars: %w", timeframe, repairErr)
			}
			recoveredBars += changedCount
			if changed && r.Resync != nil {
				r.Resync.PublishResync(instrument.ID, timeframe, "provider_gap_recovered")
			}
			if timeframe == "1m" && changed {
				canonicalChanged = true
			}
		}

		if canonicalChanged {
			derivedCount, derivedErr := r.repairDerivedFromCanonical(
				instrument,
				from,
				request.To,
			)
			if derivedErr != nil {
				return derivedErr
			}
			recoveredBars += derivedCount
		}
	}
	return nil
}

// repairCandidates uses the same append-only revision semantics as manual
// historical repair when the configured history implementation can read as
// well as write. Lightweight test/compatibility writers retain append-only
// behavior without enabling derived recovery.
func (r *IntradayRecovery) repairCandidates(
	from time.Time,
	to time.Time,
	candidates []domain.Bar,
) (uint64, bool, error) {
	if len(candidates) == 0 {
		return 0, false, nil
	}
	if store, ok := r.History.(HistoricalRepairStore); ok {
		repairer := HistoricalRepairer{History: store}
		counts, changed, err := repairer.repairBars(from, to, candidates)
		return counts.Missing + counts.Corrected, changed, err
	}

	for _, bar := range candidates {
		if err := r.History.AppendBar(bar); err != nil {
			return 0, false, err
		}
	}
	return uint64(len(candidates)), true, nil
}

func (r *IntradayRecovery) repairDerivedFromCanonical(
	instrument symbol.Instrument,
	from time.Time,
	to time.Time,
) (uint64, error) {
	store, ok := r.History.(HistoricalRepairStore)
	if !ok || r.Calendars == nil || len(r.DerivedTimeframes) == 0 {
		return 0, nil
	}
	definition, ok := r.Calendars.Definition(instrument.CalendarID)
	if !ok {
		return 0, fmt.Errorf("recovery calendar %s is not registered", instrument.CalendarID)
	}
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return 0, fmt.Errorf("load recovery calendar timezone: %w", err)
	}

	// Load from the beginning of the first affected local day so a repaired
	// minute can reconstruct a target bucket whose open precedes the outage.
	localFrom := from.In(location)
	loadFrom := time.Date(
		localFrom.Year(), localFrom.Month(), localFrom.Day(),
		0, 0, 0, 0, location,
	).UTC()
	canonical1m, err := store.LoadRange(instrument.ID, "1m", loadFrom, to)
	if err != nil {
		return 0, fmt.Errorf("load repaired %s 1m history: %w", instrument.ID, err)
	}

	repairer := HistoricalRepairer{History: store}
	var repaired uint64
	for _, target := range []struct {
		minutes   int
		timeframe string
	}{
		{2, "2m"}, {3, "3m"}, {5, "5m"}, {10, "10m"},
		{15, "15m"}, {30, "30m"}, {45, "45m"},
		{60, "1h"}, {120, "2h"}, {180, "3h"}, {240, "4h"},
	} {
		if !timeframeListed(r.DerivedTimeframes, target.timeframe) {
			continue
		}
		rollups, aggregateErr := aggregateMinuteBars(
			canonical1m,
			target.minutes,
			target.timeframe,
			definition,
			to,
		)
		if aggregateErr != nil {
			return repaired, fmt.Errorf("rebuild recovered %s %s: %w", instrument.ID, target.timeframe, aggregateErr)
		}
		counts, changed, repairErr := repairer.repairBars(from, to, rollups)
		if repairErr != nil {
			return repaired, fmt.Errorf("repair recovered %s %s: %w", instrument.ID, target.timeframe, repairErr)
		}
		repaired += counts.Missing + counts.Corrected
		if changed && r.Resync != nil {
			r.Resync.PublishResync(instrument.ID, target.timeframe, "provider_gap_recovered")
		}
	}

	if timeframeListed(r.DerivedTimeframes, "1D") {
		daily := aggregateDailyBars(canonical1m, definition, to)
		counts, changed, repairErr := repairer.repairBars(from, to, daily)
		if repairErr != nil {
			return repaired, fmt.Errorf("repair recovered %s 1D: %w", instrument.ID, repairErr)
		}
		repaired += counts.Missing + counts.Corrected
		if changed && r.Resync != nil {
			r.Resync.PublishResync(instrument.ID, "1D", "provider_gap_recovered")
		}
	}

	return repaired, nil
}

func timeframeListed(timeframes []string, wanted string) bool {
	for _, timeframe := range timeframes {
		if timeframe == wanted {
			return true
		}
	}
	return false
}
