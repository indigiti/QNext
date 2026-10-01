package history

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type diskFingerprint struct {
	exists    bool
	size      int64
	modTimeNS int64
}

func (f diskFingerprint) equal(other diskFingerprint) bool {
	return f.exists == other.exists && f.size == other.size && f.modTimeNS == other.modTimeNS
}

type cachedSeries struct {
	loaded bool
	bars   map[string]domain.Bar
	disk   diskFingerprint
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
	externalRefreshes  atomic.Uint64
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
	ExternalRefreshes      uint64 `json:"external_refreshes"`
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
	bars, loaded, fresh, err := s.cachedBarsFresh(dayKey, seriesKey, instrumentID, timeframe, day)
	if err != nil {
		return nil, err
	}
	if loaded && fresh {
		s.cache.hits.Add(1)
		return bars, nil
	}
	s.cache.misses.Add(1)

	// Only one current-day request hydrates or refreshes a series at a time.
	s.cache.loadMu.Lock()
	defer s.cache.loadMu.Unlock()
	bars, loaded, fresh, err = s.cachedBarsFresh(dayKey, seriesKey, instrumentID, timeframe, day)
	if err != nil {
		return nil, err
	}
	if loaded && fresh {
		return bars, nil
	}
	if loaded && externallyWrittenSeries(instrumentID) {
		// NIFTY-SYN+ is persisted by qnext-data-collector while Market Core
		// serves history from a different process. Count only actual refreshes
		// after acquiring the single-loader lock, not all concurrent waiters.
		s.cache.externalRefreshes.Add(1)
	}

	started := time.Now()
	// Capture the file fingerprint before scanning. If another process appends
	// while the scan is in progress, the next request will see a newer size or
	// mtime and refresh again rather than incorrectly marking the cache fresh.
	fingerprint, err := s.dayFingerprint(instrumentID, timeframe, day)
	if err != nil {
		return nil, err
	}
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
	series.disk = fingerprint
	bars = sortedBarsCopy(series.bars)
	s.cache.mu.Unlock()

	s.cache.diskLoads.Add(1)
	s.cache.lastDiskLoadAtMS.Store(loadedAt.UnixMilli())
	s.cache.lastDiskLoadTimeMS.Store(time.Since(started).Milliseconds())
	return bars, nil
}

func (s *Store) cachedBarsFresh(
	day string,
	key string,
	instrumentID string,
	timeframe string,
	at time.Time,
) ([]domain.Bar, bool, bool, error) {
	s.cache.mu.Lock()
	s.cache.ensureDayLocked(day)
	series := s.cache.series[key]
	if series == nil || !series.loaded {
		s.cache.mu.Unlock()
		return nil, false, false, nil
	}
	knownDisk := series.disk
	s.cache.mu.Unlock()

	// Index and standard INDEX-SYN candles are produced by this Market Core
	// process; observeBar keeps those RAM series authoritative and avoids an
	// fs stat/JSONL reload on every live final. SYN+ is the intentional
	// exception because its chart history is appended by qnext-data-collector.
	if !externallyWrittenSeries(instrumentID) {
		bars, ok := s.cachedBars(day, key)
		return bars, ok, ok, nil
	}

	currentDisk, err := s.dayFingerprint(instrumentID, timeframe, at)
	if err != nil {
		return nil, true, false, err
	}
	if !knownDisk.equal(currentDisk) {
		return nil, true, false, nil
	}

	bars, ok := s.cachedBars(day, key)
	return bars, ok, ok, nil
}

func externallyWrittenSeries(instrumentID string) bool {
	return strings.HasSuffix(strings.ToUpper(strings.TrimSpace(instrumentID)), "-SYN+")
}

func (s *Store) dayFingerprint(instrumentID, timeframe string, at time.Time) (diskFingerprint, error) {
	info, err := os.Stat(s.dayPath(instrumentID, timeframe, at))
	if os.IsNotExist(err) {
		return diskFingerprint{}, nil
	}
	if err != nil {
		return diskFingerprint{}, err
	}
	return diskFingerprint{
		exists:    true,
		size:      info.Size(),
		modTimeNS: info.ModTime().UnixNano(),
	}, nil
}

// refreshDiskFingerprint is called after this Store's asynchronous writer has
// durably appended a final. It primarily benefits SYN+ when the collector ever
// reads its own chart cache; ordinary Market Core Index/SYN series stay RAM-led.
func (s *Store) refreshDiskFingerprint(bar domain.Bar) {
	if s == nil || s.cache == nil || !bar.Final || !s.isCurrentDay(bar.OpenTime) {
		return
	}
	fingerprint, err := s.dayFingerprint(bar.InstrumentID, bar.Timeframe, bar.OpenTime)
	if err != nil {
		return
	}
	day := bar.OpenTime.UTC().Format("2006-01-02")
	key := cacheSeriesKey(bar.InstrumentID, bar.Timeframe)
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	s.cache.ensureDayLocked(day)
	if series := s.cache.series[key]; series != nil {
		series.disk = fingerprint
	}
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
	stats.ExternalRefreshes = s.cache.externalRefreshes.Load()
	stats.LastDiskLoadAtMS = s.cache.lastDiskLoadAtMS.Load()
	stats.LastDiskLoadDurationMS = s.cache.lastDiskLoadTimeMS.Load()
	return stats
}
