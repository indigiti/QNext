package capture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// Keep the current UTC date and the preceding 29 UTC dates by default.
	DefaultRawRetentionDays = 30
	// Each pass is deliberately capped and runs outside the market tick hot path.
	rawRetentionMaxDeletes = 256
	rawRetentionMaxEntries = 10000
	rawRetentionMaxDuration = 500 * time.Millisecond
	rawRetentionMinIdle = 24 * time.Hour
	rawRetentionInterval = time.Hour
)

type RawRetentionResult struct {
	CutoffDate      string `json:"cutoff_date"`
	ScannedFiles    int    `json:"scanned_files"`
	DeletedFiles    int    `json:"deleted_files"`
	ReclaimedBytes  int64  `json:"reclaimed_bytes"`
	SkippedRecent   int    `json:"skipped_recent"`
	Incomplete      bool   `json:"incomplete"`
}

// PruneRawFrames removes only YYYY-MM-DD.jsonl files in
// <storage-root>/raw/<provider>/<year>/<month>/ with days older than the
// retained UTC calendar-day window. No market history, futures ledger, replay,
// research, strategy or intelligence data is considered.
//
// This is metadata-only: no raw payload is read. Symlinks and irregular paths
// are never followed. Today's writer is outside the retention window.
func PruneRawFrames(storageRoot string, now time.Time, keepDays int) (RawRetentionResult, error) {
	result := RawRetentionResult{}
	if keepDays < 1 || keepDays > 3650 {
		return result, errors.New("raw retention days must be between 1 and 3650")
	}
	if storageRoot == "" {
		return result, errors.New("raw retention storage root is required")
	}
	day := now.UTC()
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := day.AddDate(0, 0, -(keepDays - 1))
	result.CutoffDate = cutoff.Format("2006-01-02")

	// Fail closed if the named storage root or raw subtree is a symlink.
	if info, err := os.Lstat(storageRoot); err != nil {
		return result, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("raw retention storage root must be a real directory")
	}
	rawRoot := filepath.Join(storageRoot, "raw")
	if info, err := os.Lstat(rawRoot); errors.Is(err, os.ErrNotExist) {
		return result, nil
	} else if err != nil {
		return result, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("raw capture directory must be a real directory")
	}

	started := time.Now()
	entries := 0
	var failures []error
	// os.ReadDir sorts entries, making older year/month/date files first.
	bounded := func() bool {
		if entries >= rawRetentionMaxEntries || result.DeletedFiles >= rawRetentionMaxDeletes || time.Since(started) >= rawRetentionMaxDuration {
			result.Incomplete = true
			return false
		}
		entries++
		return true
	}
	visit := func(parent string, valid func(string) bool, process func(path, name string) bool) bool {
		list, err := os.ReadDir(parent)
		if err != nil {
			failures = append(failures, fmt.Errorf("scan %s: %w", parent, err))
			return true // Continue other provider folders.
		}
		for _, entry := range list {
			if !bounded() {
				return false
			}
			if entry.Type()&os.ModeSymlink != 0 || !valid(entry.Name()) {
				continue
			}
			if !process(filepath.Join(parent, entry.Name()), entry.Name()) {
				return false
			}
		}
		return true
	}
	providerOK := func(value string) bool {
		if len(value) == 0 || len(value) > 80 {
			return false
		}
		for _, c := range value {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
		return true
	}
	yearOK := func(value string) bool {
		if len(value) != 4 {
			return false
		}
		n, err := strconv.Atoi(value)
		return err == nil && n >= 2000 && n <= 2100
	}
	monthOK := func(value string) bool {
		if len(value) != 2 {
			return false
		}
		n, err := strconv.Atoi(value)
		return err == nil && n >= 1 && n <= 12 && fmt.Sprintf("%02d", n) == value
	}
	fileOK := func(value string) bool {
		return strings.HasSuffix(value, ".jsonl") && len(value) == len("2006-01-02.jsonl")
	}

	visit(rawRoot, providerOK, func(providerDir, _ string) bool {
		if info, err := os.Lstat(providerDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return true
		}
		return visit(providerDir, yearOK, func(yearDir, year string) bool {
			if info, err := os.Lstat(yearDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return true
			}
			return visit(yearDir, monthOK, func(monthDir, month string) bool {
				if info, err := os.Lstat(monthDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return true
				}
				return visit(monthDir, fileOK, func(path, name string) bool {
					date := strings.TrimSuffix(name, ".jsonl")
					parsed, err := time.Parse("2006-01-02", date)
					if err != nil || parsed.Format("2006-01-02") != date ||
						parsed.Format("2006") != year || parsed.Format("01") != month {
						return true
					}
					result.ScannedFiles++
					if !parsed.Before(cutoff) {
						return true
					}
					info, err := os.Lstat(path)
					if err != nil {
						if !errors.Is(err, os.ErrNotExist) {
							failures = append(failures, err)
						}
						return true
					}
					if !info.Mode().IsRegular() {
						return true
					}
					// Never delete a recently touched file even if its day name is old.
					if now.Sub(info.ModTime()) < rawRetentionMinIdle {
						result.SkippedRecent++
						return true
					}
					if err := os.Remove(path); err != nil {
						if !errors.Is(err, os.ErrNotExist) {
							failures = append(failures, fmt.Errorf("delete raw capture %s: %w", path, err))
						}
					} else {
						result.DeletedFiles++
						result.ReclaimedBytes += info.Size()
					}
					return true
				})
			})
		})
	})
	return result, errors.Join(failures...)
}

type RawRetentionSnapshot struct {
	Enabled       bool                `json:"enabled"`
	KeepDays      int                 `json:"keep_days"`
	LastRunAt     string              `json:"last_run_at,omitempty"`
	LastError     string              `json:"last_error,omitempty"`
	LastResult    *RawRetentionResult `json:"last_result,omitempty"`
}

// RawRetention runs asynchronously and never blocks market ingestion. A zero
// keepDays disables cleanup. A bounded pass runs on startup and then hourly.
type RawRetention struct {
	root     string
	keepDays int
	mu       sync.RWMutex
	last     RawRetentionSnapshot
}

func NewRawRetention(root string, keepDays int) *RawRetention {
	return &RawRetention{
		root: root, keepDays: keepDays,
		last: RawRetentionSnapshot{Enabled: keepDays > 0, KeepDays: keepDays},
	}
}

func (r *RawRetention) Snapshot() RawRetentionSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := r.last
	if result.LastResult != nil {
		copyResult := *result.LastResult
		result.LastResult = &copyResult
	}
	return result
}

func (r *RawRetention) Run(ctx context.Context, report func(RawRetentionResult, error)) {
	if r.keepDays <= 0 {
		return
	}
	sweep := func() {
		if ctx.Err() != nil {
			return
		}
		stats, err := PruneRawFrames(r.root, time.Now(), r.keepDays)
		r.mu.Lock()
		r.last.LastRunAt = time.Now().UTC().Format(time.RFC3339)
		r.last.LastResult = &stats
		r.last.LastError = ""
		if err != nil {
			r.last.LastError = err.Error()
		}
		r.mu.Unlock()
		if report != nil {
			report(stats, err)
		}
	}
	sweep()
	ticker := time.NewTicker(rawRetentionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
