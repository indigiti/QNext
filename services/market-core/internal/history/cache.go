package history

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type cachedSeries struct {
	loaded bool
	bars   map[string]domain.Bar
}

type currentDayCache struct {
	mu        sync.Mutex
	loadMu    sync.Mutex
	now       func() time.Time
	activeDay string
	series    map[string]*cachedSeries

	hits               atomic.Uint64
	misses             atomic.Uint64
	diskLoads          atomic.Uint64
	lastDiskLoadAtMS   atomic.Int64
	lastDiskLoadTimeMS atomic.Int64
}

type CacheStats struct {
	Day                    string `json:"day,omitempty"`
	Streams                int    `json:"streams"`
	Bars                   int    `json:"bars"`
	Hits                   uint64 `json:"hits"`
	Misses                 uint64 `json:"misses"`
	DiskLoads              uint64 `json:"disk_loads"`
	LastDiskLoadAtMS       int64  `json:"last_disk_load_at_ms,omitempty"`
	LastDiskLoadDurationMS int64  `json:"last_disk_load_duration_ms,omitempty"`
}

func newCurrentDayCache() *currentDayCache {
	return &currentDayCache{
		now:    func() time.Time { return time.Now().UTC() },
		series: make(map[string]*cachedSeries),
	}
}

func (s *Store) isCurrentDay(day time.Time) bool {
	if s == nil || s.cache == nil || day.IsZero() {
		return false
	}
	now := s.cache.now().UTC()
	day = day.UTC()
	return now.Year() == day.Year() && now.YearDay() == day.YearDay()
}

func (s *Store) observeBar(bar domain.Bar) {
	if s == nil || s.cache == nil || !bar.Final || !s.isCurrentDay(bar.OpenTime) {
		return
	}
	day := bar.OpenTime.UTC().Format("2006-01-02")
	key := cacheSeriesKey(bar.InstrumentID, bar.Timeframe)

	s.cache.mu.Lock()
	s.cache.ensureDayLocked(day)
	series := s.cache.series[key]
	if series == nil {
		series = &cachedSeries{bars: make(map[string]domain.Bar)}
		s.cache.series[key] = series
	}
	mergeCachedBar(series.bars, bar)
	s.cache.mu.Unlock()
}

func (s *Store) loadCurrentDay(instrumentID, timeframe string, day time.Time) ([]domain.Bar, error) {
	dayKey := day.UTC().Format("2006-01-02")
	seriesKey := cacheSeriesKey(instrumentID, timeframe)
	if bars, ok := s.cachedBars(dayKey, seriesKey); ok {
		s.cache.hits.Add(1)
		return bars, nil
	}
	s.cache.misses.Add(1)

	// Only the first current-day request for a series scans JSONL. Other
	// readers wait for hydration and then reuse the revision-aware RAM view.
	s.cache.loadMu.Lock()
	defer s.cache.loadMu.Unlock()
	if bars, ok := s.cachedBars(dayKey, seriesKey); ok {
		return bars, nil
	}

	started := time.Now()
	diskBars, err := s.loadDayDisk(instrumentID, timeframe, day)
	if err != nil {
		return nil, err
	}
	loadedAt := time.Now().UTC()

	s.cache.mu.Lock()
	s.cache.ensureDayLocked(dayKey)
	series := s.cache.series[seriesKey]
	if series == nil {
		series = &cachedSeries{bars: make(map[string]domain.Bar)}
		s.cache.series[seriesKey] = series
	}
	for _, bar := range diskBars {
		mergeCachedBar(series.bars, bar)
	}
	series.loaded = true
	bars := sortedBarsCopy(series.bars)
	s.cache.mu.Unlock()

	s.cache.diskLoads.Add(1)
	s.cache.lastDiskLoadAtMS.Store(loadedAt.UnixMilli())
	s.cache.lastDiskLoadTimeMS.Store(time.Since(started).Milliseconds())
	return bars, nil
}

func (s *Store) cachedBars(day, key string) ([]domain.Bar, bool) {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	s.cache.ensureDayLocked(day)
	series := s.cache.series[key]
	if series == nil || !series.loaded {
		return nil, false
	}
	return sortedBarsCopy(series.bars), true
}

func (c *currentDayCache) ensureDayLocked(day string) {
	if c.activeDay == day {
		return
	}
	c.activeDay = day
	c.series = make(map[string]*cachedSeries)
}

func mergeCachedBar(dst map[string]domain.Bar, bar domain.Bar) {
	key := bar.Key()
	if previous, ok := dst[key]; !ok || bar.Revision >= previous.Revision {
		dst[key] = bar
	}
}

func sortedBarsCopy(latest map[string]domain.Bar) []domain.Bar {
	copyMap := make(map[string]domain.Bar, len(latest))
	for key, bar := range latest {
		copyMap[key] = bar
	}
	return sortedBars(copyMap)
}

func cacheSeriesKey(instrumentID, timeframe string) string {
	return instrumentID + "\x00" + timeframe
}

func (s *Store) CacheStats() CacheStats {
	if s == nil || s.cache == nil {
		return CacheStats{}
	}
	s.cache.mu.Lock()
	stats := CacheStats{Day: s.cache.activeDay, Streams: len(s.cache.series)}
	for _, series := range s.cache.series {
		stats.Bars += len(series.bars)
	}
	s.cache.mu.Unlock()
	stats.Hits = s.cache.hits.Load()
	stats.Misses = s.cache.misses.Load()
	stats.DiskLoads = s.cache.diskLoads.Load()
	stats.LastDiskLoadAtMS = s.cache.lastDiskLoadAtMS.Load()
	stats.LastDiskLoadDurationMS = s.cache.lastDiskLoadTimeMS.Load()
	return stats
}
