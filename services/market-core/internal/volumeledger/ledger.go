package volumeledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

const schema = "QNEXT.FUTURES_VOLUME/1"

// Entry is an independently persisted traded-volume delta. No synthetic or
// cash-index quote is authoritative for this record.
type Entry struct {
	Schema           string  `json:"schema"`
	TargetID         string  `json:"target_instrument_id"`
	SourceID         string  `json:"source_instrument_id"`
	Provider         string  `json:"provider"`
	EventTimeMS      int64   `json:"event_time_ms"`
	Quantity         float64 `json:"quantity"`
	CumulativeVolume float64 `json:"cumulative_volume"`
	Sequence         uint64  `json:"sequence"`
}

type Ledger struct {
	root string
	mu   sync.Mutex
}

func New(root string) *Ledger {
	return &Ledger{root: filepath.Join(root, "futures-volume-ledger")}
}

func validRecord(source, routed domain.Tick) bool {
	return source.InstrumentID != "" && routed.InstrumentID != "" &&
		source.InstrumentID != routed.InstrumentID &&
		!source.EventTime.IsZero() && routed.EventTime.Equal(source.EventTime) &&
		routed.Quantity > 0 && !math.IsNaN(routed.Quantity) && !math.IsInf(routed.Quantity, 0) &&
		routed.Provider == "upstox-futures-proxy"
}

// Record is deliberately on the futures-proxy path only. Append plus fsync
// completes before the delta is applied to canonical candles; an I/O failure
// fails closed instead of silently publishing unjournaled volume.
func (l *Ledger) Record(source, routed domain.Tick) error {
	if l == nil || l.root == "" {
		return errors.New("futures volume ledger is not configured")
	}
	if !validRecord(source, routed) {
		return errors.New("invalid futures-proxy volume record")
	}
	entry := Entry{
		Schema: schema, TargetID: routed.InstrumentID,
		SourceID: source.InstrumentID, Provider: source.Provider,
		EventTimeMS: source.EventTime.UTC().UnixMilli(),
		Quantity:    routed.Quantity, CumulativeVolume: source.CumulativeVolume,
		Sequence: source.Sequence,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := l.path(entry.TargetID, source.EventTime)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	n, writeErr := file.Write(data)
	syncErr := error(nil)
	if writeErr == nil && n == len(data) {
		syncErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return errors.Join(syncErr, closeErr)
}

// Sum distinguishes a real source observation from an unknown zero.
func (l *Ledger) Sum(targetID string, from, to time.Time) (float64, bool, error) {
	entries, err := l.Read(targetID, from, to)
	if err != nil {
		return 0, false, err
	}
	var sum float64
	for _, entry := range entries {
		sum += entry.Quantity
	}
	return sum, len(entries) > 0, nil
}

// Read returns only verified recorded deltas, scanning each day's file once.
// Deduplication keys survive process restarts and source replay.
func (l *Ledger) Read(targetID string, from, to time.Time) ([]Entry, error) {
	if l == nil || l.root == "" || targetID == "" || from.IsZero() ||
		!from.Before(to) || to.Sub(from) > 32*24*time.Hour {
		return nil, errors.New("invalid futures volume query")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	from, to = from.UTC(), to.UTC()
	day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	var result []Entry
	seen := make(map[string]bool)
	for day.Before(to) {
		file, err := os.Open(l.path(targetID, day))
		if errors.Is(err, os.ErrNotExist) {
			day = day.AddDate(0, 0, 1)
			continue
		}
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			var entry Entry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("corrupted volume ledger: %w", err)
			}
			if entry.Schema != schema || entry.TargetID != targetID {
				_ = file.Close()
				return nil, errors.New("invalid volume ledger provenance")
			}
			at := time.UnixMilli(entry.EventTimeMS)
			if at.Before(from) || !at.Before(to) {
				continue
			}
			if entry.Quantity <= 0 || math.IsNaN(entry.Quantity) || math.IsInf(entry.Quantity, 0) {
				_ = file.Close()
				return nil, errors.New("invalid stored futures quantity")
			}
			key := fmt.Sprintf("%s|%d|%.9f|%.9f", entry.SourceID, entry.EventTimeMS, entry.CumulativeVolume, entry.Quantity)
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, entry)
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil || closeErr != nil {
			return nil, errors.Join(scanErr, closeErr)
		}
		day = day.AddDate(0, 0, 1)
	}
	return result, nil
}

func (l *Ledger) path(targetID string, at time.Time) string {
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", "..", "_")
	name := replacer.Replace(targetID)
	return filepath.Join(l.root, name, at.UTC().Format("2006-01-02")+".jsonl")
}
