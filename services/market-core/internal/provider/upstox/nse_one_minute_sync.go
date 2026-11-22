package upstox

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
)

// NSEMinuteFetcher is intentionally limited to an instrument allowlist
// established in the authenticated market configuration at process startup.
type NSEMinuteFetcher interface {
	FetchRange(context.Context, string, string, time.Time, time.Time) ([]ProviderCandle, error)
}

type NSEMinuteSymbolStatus struct {
	Symbol        string `json:"symbol"`
	InstrumentID  string `json:"instrument_id"`
	AssetClass    string `json:"asset_class"`
	Source        string `json:"source"`
	LastAttemptMS int64  `json:"last_attempt_ms,omitempty"`
	LastSuccessMS int64  `json:"last_success_ms,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	Scanned       uint64 `json:"scanned"`
	Missing       uint64 `json:"missing"`
	Corrected     uint64 `json:"corrected"`
	Unchanged     uint64 `json:"unchanged"`
}

type NSEMinuteStatus struct {
	Enabled                bool                    `json:"enabled"`
	Mode                   string                  `json:"mode"`
	Timeframe              string                  `json:"timeframe"`
	RefreshIntervalSeconds int                     `json:"refresh_interval_seconds"`
	Running                bool                    `json:"running"`
	Symbols                []NSEMinuteSymbolStatus `json:"symbols"`
}

// NSEMinuteSync periodically fetches *completed provider 1-minute candles*.
// It does not create a second Upstox websocket, invent 5s candles, or treat
// cash-index historical volume as exchange-traded index volume.
type NSEMinuteSync struct {
	Client      NSEMinuteFetcher
	History     HistoricalRepairStore
	Resync      HistoricalRepairResync
	AccessToken string
	Symbols     []marketconfig.NSEMinuteSymbol
	Now         func() time.Time
	Interval    time.Duration

	mu      sync.RWMutex
	running bool
	results map[string]NSEMinuteSymbolStatus
}

func (s *NSEMinuteSync) Snapshot() NSEMinuteStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	interval := s.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	result := NSEMinuteStatus{
		Enabled: true, Mode: "historical_polling_not_live_ticks",
		Timeframe: "1m", RefreshIntervalSeconds: int(interval.Seconds()),
		Running: s.running,
		Symbols: make([]NSEMinuteSymbolStatus, 0, len(s.Symbols)),
	}
	for _, item := range s.Symbols {
		entry, ok := s.results[item.InstrumentID()]
		if !ok {
			class := "equity"
			if item.IsIndex() {
				class = "index"
			}
			entry = NSEMinuteSymbolStatus{
				Symbol:       strings.ToUpper(item.Symbol),
				InstrumentID: item.InstrumentID(),
				AssetClass:   class, Source: "upstox_historical_1m",
			}
		}
		result.Symbols = append(result.Symbols, entry)
	}
	return result
}

func (s *NSEMinuteSync) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := s.SyncOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("selected NSE 1m history refresh: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SyncOnce is sequential and has a per-service guard so refresh cycles cannot
// overlap, preserving candle revisions and the provider's request budget.
func (s *NSEMinuteSync) SyncOnce(ctx context.Context) error {
	if s.Client == nil || s.History == nil || strings.TrimSpace(s.AccessToken) == "" {
		return errors.New("NSE minute sync requires provider client, history and token")
	}
	if len(s.Symbols) == 0 || len(s.Symbols) > 12 {
		return errors.New("NSE minute sync requires 1 to 12 selected symbols")
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("NSE minute sync already running")
	}
	s.running = true
	if s.results == nil {
		s.results = make(map[string]NSEMinuteSymbolStatus)
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	current := now().UTC()
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return err
	}
	local := current.In(ist)
	from := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, ist).AddDate(0, 0, -2).UTC()
	// Upstox intraday data may include the still-forming candle. Allow a
	// 30-second publication buffer and accept only fully closed 1m bars.
	to := current.Add(-30 * time.Second)
	if !from.Before(to) {
		return nil
	}
	repairer := &HistoricalRepairer{History: s.History}
	var failures []error

	for _, item := range s.Symbols {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		candles, fetchErr := s.Client.FetchRange(ctx, s.AccessToken, item.ProviderKey, from, to)
		status := NSEMinuteSymbolStatus{
			Symbol:       strings.ToUpper(strings.TrimSpace(item.Symbol)),
			InstrumentID: item.InstrumentID(),
			Source:       "upstox_historical_1m", LastAttemptMS: current.UnixMilli(),
			AssetClass: "equity",
		}
		if item.IsIndex() {
			status.AssetClass = "index"
		}
		if fetchErr == nil {
			bars := make([]domain.Bar, 0, len(candles))
			for _, candle := range candles {
				if candle.OpenTime.Before(from) || candle.OpenTime.Add(time.Minute).After(to) {
					continue
				}
				bar := providerBar(item.InstrumentID(), "1m", candle, time.Minute)
				if item.IsIndex() {
					bar.Volume = 0
					bar.VolumeStatus = "UNKNOWN"
				} else {
					bar.VolumeSource = "UPSTOX_EQUITY_HISTORICAL"
					bar.VolumeStatus = "OBSERVED"
				}
				bars = append(bars, bar)
			}
			var counts HistoricalRepairCounts
			var changed bool
			if item.IsIndex() {
				counts, changed, fetchErr = repairer.repairIndexSourceBars(from, to, bars)
			} else {
				counts, changed, fetchErr = repairer.repairBars(from, to, bars)
			}
			status.Scanned = counts.Scanned
			status.Missing = counts.Missing
			status.Corrected = counts.Corrected
			status.Unchanged = counts.Unchanged
			if fetchErr == nil {
				status.LastSuccessMS = current.UnixMilli()
				if changed && s.Resync != nil {
					s.Resync.PublishResync(item.InstrumentID(), "1m", "nse_1m_history_sync")
				}
			}
		}
		s.mu.Lock()
		if prior, ok := s.results[item.InstrumentID()]; ok {
			if status.LastSuccessMS == 0 {
				status.LastSuccessMS = prior.LastSuccessMS
			}
		}
		if fetchErr != nil {
			status.LastError = fetchErr.Error()
		}
		s.results[item.InstrumentID()] = status
		s.mu.Unlock()
		if fetchErr != nil {
			failures = append(failures, fmt.Errorf("%s: %w", item.Symbol, fetchErr))
		}
		// Pace sequential requests even if the provider responds immediately.
		// Never sleep past cancellation.
		if len(s.Symbols) > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	return errors.Join(failures...)
}
