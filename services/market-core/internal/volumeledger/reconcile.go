package volumeledger

import (
	"errors"
	"fmt"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

// BarHistory is a minimal append-only revision interface. Reconciliation never
// manufactures OHLC bars when the canonical price history is missing.
type BarHistory interface {
	LoadRange(string, string, time.Time, time.Time) ([]domain.Bar, error)
	AppendBar(domain.Bar) error
}

type ResyncPublisher interface {
	PublishResync(instrumentID, timeframe, reason string)
}

type ReconcileStats struct {
	Scanned   int `json:"scanned"`
	Corrected int `json:"corrected"`
	Unknown   int `json:"unknown"`
}

// Reconcile corrects already finalized index-price candles using the
// independent futures ledger. It never guesses how a minute's volume should
// be distributed to missing 5s/15s buckets.
func (l *Ledger) Reconcile(history BarHistory, resync ResyncPublisher,
	targetID string, timeframes []string, from, to time.Time) (ReconcileStats, error) {
	var stats ReconcileStats
	if l == nil || history == nil || targetID == "" {
		return stats, errors.New("volume reconciliation requires ledger, history and market")
	}
	entries, err := l.Read(targetID, from, to)
	if err != nil {
		return stats, err
	}
	for _, timeframe := range timeframes {
		volumeByOpen := make(map[int64]float64)
		for _, entry := range entries {
			open, _, err := candle.Bucket(time.UnixMilli(entry.EventTimeMS), timeframe)
			if err != nil {
				continue
			}
			volumeByOpen[open.UnixMilli()] += entry.Quantity
		}
		bars, err := history.LoadRange(targetID, timeframe, from, to)
		if err != nil {
			return stats, fmt.Errorf("load %s: %w", timeframe, err)
		}
		changed := false
		for _, bar := range bars {
			if !bar.Final || bar.CloseTime.IsZero() || bar.CloseTime.After(to) {
				continue
			}
			stats.Scanned++
			quantity, observed := volumeByOpen[bar.OpenTime.UnixMilli()]
			if !observed {
				stats.Unknown++
				continue
			}
			// Never downgrade an already-observed canonical proxy on a
			// partial journal (e.g. records made before ledger activation).
			if quantity <= bar.Volume {
				continue
			}
			bar.Volume = quantity
			bar.Revision++
			bar.Corrected = true
			bar.CorrectedAt = time.Now().UTC()
			bar.VolumeSource = "FUTURES_PROXY"
			bar.VolumeStatus = "OBSERVED"
			if err := history.AppendBar(bar); err != nil {
				return stats, err
			}
			stats.Corrected++
			changed = true
		}
		if changed && resync != nil {
			resync.PublishResync(targetID, timeframe, "futures_volume_reconciled")
		}
	}
	return stats, nil
}
