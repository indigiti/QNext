package upstox

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketcalendar"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

var ErrHistoricalRepairRunning = errors.New("historical repair is already running")

type HistoricalRangeFetcher interface {
	FetchRange(
		context.Context,
		string,
		string,
		time.Time,
		time.Time,
	) ([]ProviderCandle, error)
	FetchWeeklyRange(
		context.Context,
		string,
		string,
		time.Time,
		time.Time,
	) ([]ProviderCandle, error)
	FetchMonthlyRange(
		context.Context,
		string,
		string,
		time.Time,
		time.Time,
	) ([]ProviderCandle, error)
}

type HistoricalRepairStore interface {
	LoadRange(instrumentID, timeframe string, from, to time.Time) ([]domain.Bar, error)
	AppendBar(domain.Bar) error
}

type HistoricalRepairResync interface {
	PublishResync(instrumentID, timeframe, reason string)
}

type HistoricalRepairRequest struct {
	Days    int      `json:"days"`
	Markets []string `json:"markets,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

type HistoricalRepairCounts struct {
	Scanned   uint64 `json:"scanned"`
	Missing   uint64 `json:"missing"`
	Corrected uint64 `json:"corrected"`
	Unchanged uint64 `json:"unchanged"`
}

type HistoricalRepairMarketResult struct {
	Symbol       string                            `json:"symbol"`
	InstrumentID string                            `json:"instrument_id"`
	ProviderKey  string                            `json:"provider_key"`
	Timeframes   map[string]HistoricalRepairCounts `json:"timeframes"`
}

type HistoricalRepairResult struct {
	Days          int                            `json:"days"`
	Markets       []HistoricalRepairMarketResult `json:"markets"`
	StartedAtMS   int64                          `json:"started_at_ms"`
	CompletedAtMS int64                          `json:"completed_at_ms"`
	Reason        string                         `json:"reason,omitempty"`
}

type HistoricalRepairStatus struct {
	Running       bool                    `json:"running"`
	Days          int                     `json:"days,omitempty"`
	Reason        string                  `json:"reason,omitempty"`
	StartedAtMS   int64                   `json:"started_at_ms,omitempty"`
	CompletedAtMS int64                   `json:"completed_at_ms,omitempty"`
	LastError     string                  `json:"last_error,omitempty"`
	LastResult    *HistoricalRepairResult `json:"last_result,omitempty"`
}

type HistoricalRepairer struct {
	Client      HistoricalRangeFetcher
	AccessToken string
	History     HistoricalRepairStore
	Calendars   *marketcalendar.Registry
	Markets     []marketconfig.MarketConfig
	Resync      HistoricalRepairResync
	Now         func() time.Time

	runMu    sync.Mutex
	statusMu sync.RWMutex
	status   HistoricalRepairStatus
}

func (r *HistoricalRepairer) Repair(
	ctx context.Context,
	request HistoricalRepairRequest,
) (HistoricalRepairResult, error) {
	if r == nil || r.Client == nil || r.History == nil || r.Calendars == nil {
		return HistoricalRepairResult{}, errors.New("historical repair is not configured")
	}
	if !validRepairDays(request.Days) {
		return HistoricalRepairResult{}, errors.New("historical repair days must be one of 3, 7, 15, or 30")
	}
	if !r.runMu.TryLock() {
		return HistoricalRepairResult{}, ErrHistoricalRepairRunning
	}
	defer r.runMu.Unlock()

	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	current := now().UTC()

	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return HistoricalRepairResult{}, err
	}
	localNow := current.In(location)
	localStart := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(),
		0, 0, 0, 0, location,
	).AddDate(0, 0, -(request.Days - 1))
	from := localStart.UTC()
	to := current

	selected, err := r.selectMarkets(request.Markets)
	if err != nil {
		return HistoricalRepairResult{}, err
	}

	result := HistoricalRepairResult{
		Days:        request.Days,
		Markets:     make([]HistoricalRepairMarketResult, 0, len(selected)),
		StartedAtMS: current.UnixMilli(),
		Reason:      strings.TrimSpace(request.Reason),
	}
	r.setRunning(result)

	for _, market := range selected {
		if err := ctx.Err(); err != nil {
			r.finish(result, err)
			return result, err
		}

		providerCandles, err := r.Client.FetchRange(
			ctx,
			r.AccessToken,
			market.Underlying.ProviderKey,
			from,
			to,
		)
		if err != nil {
			wrapped := fmt.Errorf("repair %s 1m source: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}

		oneMinute := make([]domain.Bar, 0, len(providerCandles))
		for _, candle := range providerCandles {
			if candle.OpenTime.Add(time.Minute).After(to) {
				continue
			}
			oneMinute = append(oneMinute, providerBar(
				market.Underlying.InstrumentID,
				"1m",
				candle,
				time.Minute,
			))
		}

		marketResult := HistoricalRepairMarketResult{
			Symbol:       market.Symbol,
			InstrumentID: market.Underlying.InstrumentID,
			ProviderKey:  market.Underlying.ProviderKey,
			Timeframes:   make(map[string]HistoricalRepairCounts, 3),
		}

		counts, changed, err := r.repairBars(from, to, oneMinute)
		if err != nil {
			wrapped := fmt.Errorf("repair %s 1m: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}
		marketResult.Timeframes["1m"] = counts
		if changed && r.Resync != nil {
			r.Resync.PublishResync(market.Underlying.InstrumentID, "1m", "historical_repair")
		}

		canonical1m, err := r.History.LoadRange(
			market.Underlying.InstrumentID,
			"1m",
			from,
			to,
		)
		if err != nil {
			wrapped := fmt.Errorf("load repaired %s 1m: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}

		definition, ok := r.Calendars.Definition(market.CalendarID)
		if !ok {
			wrapped := fmt.Errorf("calendar %s is not registered", market.CalendarID)
			r.finish(result, wrapped)
			return result, wrapped
		}

		for _, target := range []struct {
			minutes   int
			timeframe string
		}{
			{2, "2m"}, {3, "3m"}, {5, "5m"}, {10, "10m"},
			{15, "15m"}, {30, "30m"}, {45, "45m"},
			{60, "1h"}, {120, "2h"}, {180, "3h"}, {240, "4h"},
		} {
			rollups, err := aggregateMinuteBars(
				canonical1m,
				target.minutes,
				target.timeframe,
				definition,
				to,
			)
			if err != nil {
				wrapped := fmt.Errorf("aggregate %s %s: %w", market.Symbol, target.timeframe, err)
				r.finish(result, wrapped)
				return result, wrapped
			}
			counts, changed, err := r.repairBars(from, to, rollups)
			if err != nil {
				wrapped := fmt.Errorf("repair %s %s: %w", market.Symbol, target.timeframe, err)
				r.finish(result, wrapped)
				return result, wrapped
			}
			marketResult.Timeframes[target.timeframe] = counts
			if changed && r.Resync != nil {
				r.Resync.PublishResync(
					market.Underlying.InstrumentID,
					target.timeframe,
					"historical_repair",
				)
			}
		}

		daily := aggregateDailyBars(canonical1m, definition, to)
		counts, changed, err = r.repairBars(from, to, daily)
		if err != nil {
			wrapped := fmt.Errorf("repair %s 1D: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}
		marketResult.Timeframes["1D"] = counts
		if changed && r.Resync != nil {
			r.Resync.PublishResync(market.Underlying.InstrumentID, "1D", "historical_repair")
		}

		weeklySource, err := r.Client.FetchWeeklyRange(
			ctx,
			r.AccessToken,
			market.Underlying.ProviderKey,
			from.AddDate(0, 0, -7),
			to,
		)
		if err != nil {
			wrapped := fmt.Errorf("repair %s weekly source: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}
		weekly := providerCalendarBars(
			market.Underlying.InstrumentID,
			"1W",
			weeklySource,
			to,
		)
		counts, changed, err = r.repairBars(from, to, weekly)
		if err != nil {
			wrapped := fmt.Errorf("repair %s 1W: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}
		marketResult.Timeframes["1W"] = counts
		if changed && r.Resync != nil {
			r.Resync.PublishResync(market.Underlying.InstrumentID, "1W", "historical_repair")
		}

		monthlySource, err := r.Client.FetchMonthlyRange(
			ctx,
			r.AccessToken,
			market.Underlying.ProviderKey,
			from.AddDate(-1, 0, 0),
			to,
		)
		if err != nil {
			wrapped := fmt.Errorf("repair %s monthly source: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}
		monthly := providerMonthlyBars(market.Underlying.InstrumentID, monthlySource, to)
		counts, changed, err = r.repairBars(from, to, monthly)
		if err != nil {
			wrapped := fmt.Errorf("repair %s 1M: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}
		marketResult.Timeframes["1M"] = counts
		if changed && r.Resync != nil {
			r.Resync.PublishResync(market.Underlying.InstrumentID, "1M", "historical_repair")
		}

		canonicalMonthly, err := r.History.LoadRange(
			market.Underlying.InstrumentID,
			"1M",
			from.AddDate(-1, 0, 0),
			to,
		)
		if err != nil {
			wrapped := fmt.Errorf("load repaired %s 1M: %w", market.Symbol, err)
			r.finish(result, wrapped)
			return result, wrapped
		}
		for _, timeframe := range []string{"3M", "6M", "12M"} {
			rollups := aggregateCalendarBars(canonicalMonthly, timeframe, to)
			counts, changed, err := r.repairBars(from, to, rollups)
			if err != nil {
				wrapped := fmt.Errorf("repair %s %s: %w", market.Symbol, timeframe, err)
				r.finish(result, wrapped)
				return result, wrapped
			}
			marketResult.Timeframes[timeframe] = counts
			if changed && r.Resync != nil {
				r.Resync.PublishResync(
					market.Underlying.InstrumentID,
					timeframe,
					"historical_repair",
				)
			}
		}

		result.Markets = append(result.Markets, marketResult)
	}

	result.CompletedAtMS = now().UTC().UnixMilli()
	r.finish(result, nil)
	return result, nil
}

func (r *HistoricalRepairer) Status() HistoricalRepairStatus {
	if r == nil {
		return HistoricalRepairStatus{}
	}
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	status := r.status
	if r.status.LastResult != nil {
		copyResult := *r.status.LastResult
		copyResult.Markets = append([]HistoricalRepairMarketResult(nil), r.status.LastResult.Markets...)
		status.LastResult = &copyResult
	}
	return status
}

func (r *HistoricalRepairer) selectMarkets(requested []string) ([]marketconfig.MarketConfig, error) {
	if len(requested) == 0 {
		result := make([]marketconfig.MarketConfig, len(r.Markets))
		copy(result, r.Markets)
		return result, nil
	}
	wanted := make(map[string]bool, len(requested))
	for _, symbol := range requested {
		key := strings.ToUpper(strings.TrimSpace(symbol))
		if key == "" {
			return nil, errors.New("historical repair market cannot be empty")
		}
		wanted[key] = true
	}
	result := make([]marketconfig.MarketConfig, 0, len(wanted))
	for _, market := range r.Markets {
		key := strings.ToUpper(strings.TrimSpace(market.Symbol))
		if wanted[key] {
			result = append(result, market)
			delete(wanted, key)
		}
	}
	if len(wanted) > 0 {
		return nil, errors.New("historical repair requested an inactive or unknown market")
	}
	return result, nil
}

func (r *HistoricalRepairer) repairBars(
	from time.Time,
	to time.Time,
	candidates []domain.Bar,
) (HistoricalRepairCounts, bool, error) {
	filtered := make([]domain.Bar, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.CloseTime.After(from) && !candidate.CloseTime.After(to) {
			filtered = append(filtered, candidate)
		}
	}
	counts := HistoricalRepairCounts{Scanned: uint64(len(filtered))}
	if len(filtered) == 0 {
		return counts, false, nil
	}

	instrumentID := filtered[0].InstrumentID
	timeframe := filtered[0].Timeframe
	loadFrom := from
	for _, candidate := range filtered {
		if candidate.OpenTime.Before(loadFrom) {
			loadFrom = candidate.OpenTime
		}
	}
	existing, err := r.History.LoadRange(instrumentID, timeframe, loadFrom, to)
	if err != nil {
		return counts, false, err
	}
	byOpen := make(map[int64]domain.Bar, len(existing))
	for _, bar := range existing {
		byOpen[bar.OpenTime.UnixMilli()] = bar
	}

	changed := false
	for _, candidate := range filtered {
		previous, ok := byOpen[candidate.OpenTime.UnixMilli()]
		if !ok {
			candidate.Revision = 0
			candidate.Recovered = true
			candidate.Quality = domain.QualityRecovered
			if err := r.History.AppendBar(candidate); err != nil {
				return counts, changed, err
			}
			byOpen[candidate.OpenTime.UnixMilli()] = candidate
			counts.Missing++
			changed = true
			continue
		}

		if sameHistoricalBar(previous, candidate) {
			counts.Unchanged++
			continue
		}

		candidate.Revision = previous.Revision + 1
		candidate.Recovered = true
		candidate.Corrected = true
		candidate.CorrectedAt = time.Now().UTC()
		candidate.Quality = domain.QualityRecovered
		if r.Now != nil {
			candidate.CorrectedAt = r.Now().UTC()
		}
		if err := r.History.AppendBar(candidate); err != nil {
			return counts, changed, err
		}
		byOpen[candidate.OpenTime.UnixMilli()] = candidate
		counts.Corrected++
		changed = true
	}
	return counts, changed, nil
}

func providerBar(
	instrumentID string,
	timeframe string,
	candle ProviderCandle,
	duration time.Duration,
) domain.Bar {
	return domain.Bar{
		InstrumentID:        instrumentID,
		Timeframe:           timeframe,
		OpenTime:            candle.OpenTime.UTC(),
		CloseTime:           candle.OpenTime.UTC().Add(duration),
		Open:                candle.Open,
		High:                candle.High,
		Low:                 candle.Low,
		Close:               candle.Close,
		Volume:              candle.Volume,
		Final:               true,
		AuthorityProvider:   ProviderName,
		Quality:             domain.QualityRecovered,
		Recovered:           true,
		CandleEngineVersion: "historical-repair-v1",
	}
}

func aggregateMinuteBars(
	bars []domain.Bar,
	interval int,
	timeframe string,
	definition marketcalendar.Definition,
	now time.Time,
) ([]domain.Bar, error) {
	if interval <= 1 || interval > 240 {
		return nil, errors.New("historical intraday rollup interval must be between 2 and 240 minutes")
	}
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return nil, err
	}

	sorted := append([]domain.Bar(nil), bars...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OpenTime.Before(sorted[j].OpenTime)
	})

	type bucket struct {
		start time.Time
		bars  []domain.Bar
	}
	buckets := make(map[int64]*bucket)

	for _, bar := range sorted {
		if !bar.Final || bar.Timeframe != "1m" {
			continue
		}
		local := bar.OpenTime.In(location)
		dateKey := local.Format("2006-01-02")
		if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
			continue
		}
		if _, closed := definition.ClosedDates[dateKey]; closed {
			continue
		}

		sessionOpen := time.Date(
			local.Year(), local.Month(), local.Day(),
			definition.RegularOpen.Hour, definition.RegularOpen.Minute,
			0, 0, location,
		)
		sessionClose := time.Date(
			local.Year(), local.Month(), local.Day(),
			definition.RegularClose.Hour, definition.RegularClose.Minute,
			0, 0, location,
		)
		if local.Before(sessionOpen) || !local.Before(sessionClose) {
			continue
		}

		minutes := int(local.Sub(sessionOpen) / time.Minute)
		groupStart := sessionOpen.Add(
			time.Duration((minutes/interval)*interval) * time.Minute,
		).UTC()
		key := groupStart.UnixMilli()
		if buckets[key] == nil {
			buckets[key] = &bucket{start: groupStart}
		}
		buckets[key].bars = append(buckets[key].bars, bar)
	}

	keys := make([]int64, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	result := make([]domain.Bar, 0, len(keys))
	for _, key := range keys {
		group := buckets[key]
		sort.Slice(group.bars, func(i, j int) bool {
			return group.bars[i].OpenTime.Before(group.bars[j].OpenTime)
		})
		groupStartLocal := group.start.In(location)
		sessionClose := time.Date(
			groupStartLocal.Year(), groupStartLocal.Month(), groupStartLocal.Day(),
			definition.RegularClose.Hour, definition.RegularClose.Minute,
			0, 0, location,
		).UTC()
		closeTime := group.start.Add(time.Duration(interval) * time.Minute)
		if closeTime.After(sessionClose) {
			closeTime = sessionClose
		}
		expectedCount := int(closeTime.Sub(group.start) / time.Minute)
		if expectedCount <= 0 || len(group.bars) != expectedCount {
			continue
		}

		complete := true
		for i, bar := range group.bars {
			expected := group.start.Add(time.Duration(i) * time.Minute)
			if !bar.OpenTime.Equal(expected) {
				complete = false
				break
			}
		}
		if !complete || closeTime.After(now.UTC()) {
			continue
		}

		first := group.bars[0]
		last := group.bars[len(group.bars)-1]
		high := first.High
		low := first.Low
		volume := float64(0)
		for _, bar := range group.bars {
			if bar.High > high {
				high = bar.High
			}
			if bar.Low < low {
				low = bar.Low
			}
			volume += bar.Volume
		}

		result = append(result, domain.Bar{
			InstrumentID:        first.InstrumentID,
			Timeframe:           timeframe,
			OpenTime:            group.start,
			CloseTime:           closeTime,
			Open:                first.Open,
			High:                high,
			Low:                 low,
			Close:               last.Close,
			Volume:              volume,
			Final:               true,
			AuthorityProvider:   ProviderName,
			Quality:             domain.QualityRecovered,
			Recovered:           true,
			CandleEngineVersion: "historical-rollup-v1",
		})
	}
	return result, nil
}

func aggregateDailyBars(
	bars []domain.Bar,
	definition marketcalendar.Definition,
	now time.Time,
) []domain.Bar {
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return nil
	}
	byDay := make(map[string][]domain.Bar)
	for _, bar := range bars {
		if !bar.Final || bar.Timeframe != "1m" {
			continue
		}
		local := bar.OpenTime.In(location)
		key := local.Format("2006-01-02")
		byDay[key] = append(byDay[key], bar)
	}

	keys := make([]string, 0, len(byDay))
	for key := range byDay {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]domain.Bar, 0, len(keys))
	for _, key := range keys {
		group := byDay[key]
		sort.Slice(group, func(i, j int) bool { return group[i].OpenTime.Before(group[j].OpenTime) })
		if len(group) == 0 {
			continue
		}
		local := group[0].OpenTime.In(location)
		sessionOpen := time.Date(
			local.Year(), local.Month(), local.Day(),
			definition.RegularOpen.Hour, definition.RegularOpen.Minute,
			0, 0, location,
		)
		sessionClose := time.Date(
			local.Year(), local.Month(), local.Day(),
			definition.RegularClose.Hour, definition.RegularClose.Minute,
			0, 0, location,
		)
		expected := int(sessionClose.Sub(sessionOpen) / time.Minute)
		if len(group) != expected || sessionClose.After(now.In(location)) {
			continue
		}

		openTime := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).UTC()
		closeTime := openTime.In(location).AddDate(0, 0, 1).UTC()
		result = append(result, aggregateBarGroup(group, "1D", openTime, closeTime))
	}
	return result
}

func providerCalendarBars(
	instrumentID string,
	timeframe string,
	candles []ProviderCandle,
	now time.Time,
) []domain.Bar {
	location := time.FixedZone("IST", 5*60*60+30*60)
	result := make([]domain.Bar, 0, len(candles))
	for _, candle := range candles {
		local := candle.OpenTime.In(location)
		var openTime, closeTime time.Time
		switch timeframe {
		case "1W":
			day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
			offset := (int(day.Weekday()) + 6) % 7
			openTime = day.AddDate(0, 0, -offset)
			closeTime = openTime.AddDate(0, 0, 7)
		default:
			continue
		}
		if closeTime.After(now.In(location)) {
			continue
		}
		result = append(result, domain.Bar{
			InstrumentID:        instrumentID,
			Timeframe:           timeframe,
			OpenTime:            openTime.UTC(),
			CloseTime:           closeTime.UTC(),
			Open:                candle.Open,
			High:                candle.High,
			Low:                 candle.Low,
			Close:               candle.Close,
			Volume:              candle.Volume,
			Final:               true,
			AuthorityProvider:   ProviderName,
			Quality:             domain.QualityRecovered,
			Recovered:           true,
			CandleEngineVersion: "historical-calendar-v1",
		})
	}
	return result
}

func providerMonthlyBars(
	instrumentID string,
	candles []ProviderCandle,
	now time.Time,
) []domain.Bar {
	location := time.FixedZone("IST", 5*60*60+30*60)
	result := make([]domain.Bar, 0, len(candles))
	for _, candle := range candles {
		local := candle.OpenTime.In(location)
		openTime := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
		closeTime := openTime.AddDate(0, 1, 0)
		if closeTime.After(now.In(location)) {
			continue
		}
		result = append(result, domain.Bar{
			InstrumentID:        instrumentID,
			Timeframe:           "1M",
			OpenTime:            openTime.UTC(),
			CloseTime:           closeTime.UTC(),
			Open:                candle.Open,
			High:                candle.High,
			Low:                 candle.Low,
			Close:               candle.Close,
			Volume:              candle.Volume,
			Final:               true,
			AuthorityProvider:   ProviderName,
			Quality:             domain.QualityRecovered,
			Recovered:           true,
			CandleEngineVersion: "historical-monthly-v1",
		})
	}
	return result
}

func aggregateCalendarBars(
	bars []domain.Bar,
	timeframe string,
	now time.Time,
) []domain.Bar {
	location := time.FixedZone("IST", 5*60*60+30*60)
	type bucket struct {
		open  time.Time
		close time.Time
		bars  []domain.Bar
	}
	buckets := make(map[int64]*bucket)

	for _, bar := range bars {
		if !bar.Final {
			continue
		}
		local := bar.OpenTime.In(location)
		var open, close time.Time
		switch timeframe {
		case "1W":
			day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
			offset := (int(day.Weekday()) + 6) % 7
			open = day.AddDate(0, 0, -offset)
			close = open.AddDate(0, 0, 7)
		case "3M", "6M", "12M":
			months, _ := strconv.Atoi(strings.TrimSuffix(timeframe, "M"))
			startMonth := ((int(local.Month())-1)/months)*months + 1
			open = time.Date(local.Year(), time.Month(startMonth), 1, 0, 0, 0, 0, location)
			close = open.AddDate(0, months, 0)
		default:
			continue
		}
		key := open.UnixMilli()
		if buckets[key] == nil {
			buckets[key] = &bucket{open: open.UTC(), close: close.UTC()}
		}
		buckets[key].bars = append(buckets[key].bars, bar)
	}

	keys := make([]int64, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	result := make([]domain.Bar, 0, len(keys))
	for _, key := range keys {
		group := buckets[key]
		if group.close.After(now.UTC()) || len(group.bars) == 0 {
			continue
		}
		if strings.HasSuffix(timeframe, "M") {
			months, _ := strconv.Atoi(strings.TrimSuffix(timeframe, "M"))
			if len(group.bars) != months {
				continue
			}
		}
		sort.Slice(group.bars, func(i, j int) bool { return group.bars[i].OpenTime.Before(group.bars[j].OpenTime) })
		result = append(result, aggregateBarGroup(group.bars, timeframe, group.open, group.close))
	}
	return result
}

func aggregateBarGroup(
	group []domain.Bar,
	timeframe string,
	openTime time.Time,
	closeTime time.Time,
) domain.Bar {
	first := group[0]
	last := group[len(group)-1]
	high := first.High
	low := first.Low
	volume := float64(0)
	for _, bar := range group {
		if bar.High > high {
			high = bar.High
		}
		if bar.Low < low {
			low = bar.Low
		}
		volume += bar.Volume
	}
	return domain.Bar{
		InstrumentID:        first.InstrumentID,
		Timeframe:           timeframe,
		OpenTime:            openTime.UTC(),
		CloseTime:           closeTime.UTC(),
		Open:                first.Open,
		High:                high,
		Low:                 low,
		Close:               last.Close,
		Volume:              volume,
		Final:               true,
		AuthorityProvider:   ProviderName,
		Quality:             domain.QualityRecovered,
		Recovered:           true,
		CandleEngineVersion: "historical-rollup-v2",
	}
}

func sameHistoricalBar(a, b domain.Bar) bool {
	return closeFloat(a.Open, b.Open) &&
		closeFloat(a.High, b.High) &&
		closeFloat(a.Low, b.Low) &&
		closeFloat(a.Close, b.Close) &&
		closeFloat(a.Volume, b.Volume) &&
		a.Final == b.Final
}

func closeFloat(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}

func validRepairDays(days int) bool {
	switch days {
	case 3, 7, 15, 30:
		return true
	default:
		return false
	}
}

func (r *HistoricalRepairer) setRunning(result HistoricalRepairResult) {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	r.status = HistoricalRepairStatus{
		Running:     true,
		Days:        result.Days,
		Reason:      result.Reason,
		StartedAtMS: result.StartedAtMS,
	}
}

func (r *HistoricalRepairer) finish(result HistoricalRepairResult, err error) {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	resultCopy := result
	r.status.Running = false
	r.status.CompletedAtMS = result.CompletedAtMS
	if result.CompletedAtMS == 0 {
		r.status.CompletedAtMS = time.Now().UTC().UnixMilli()
	}
	r.status.LastError = ""
	if err != nil {
		r.status.LastError = err.Error()
	}
	r.status.LastResult = &resultCopy
}
