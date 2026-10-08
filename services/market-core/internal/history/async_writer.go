package history

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/observability"
)

type persistRequest struct {
	bar      domain.Bar
	queuedAt time.Time
}

type AsyncWriter struct {
	store    *Store
	queue    chan persistRequest
	stop     chan struct{}
	done     chan struct{}
	writeFn  func(domain.Bar) error
	submitMu sync.Mutex
	stopOnce sync.Once
	closed   bool

	enqueued       atomic.Uint64
	writes         atomic.Uint64
	errors         atomic.Uint64
	lastQueuedAtMS atomic.Int64
	lastFlushAtMS  atomic.Int64

	errMu     sync.RWMutex
	lastError string
}

type PersistenceStats struct {
	Queued         int    `json:"queued"`
	Capacity       int    `json:"capacity"`
	Pending        uint64 `json:"pending"`
	Enqueued       uint64 `json:"enqueued"`
	Writes         uint64 `json:"writes"`
	Errors         uint64 `json:"errors"`
	LastQueuedAtMS int64  `json:"last_queued_at_ms,omitempty"`
	LastFlushAtMS  int64  `json:"last_flush_at_ms,omitempty"`
	FlushLagMS     int64  `json:"flush_lag_ms"`
	LastError      string `json:"last_error,omitempty"`
}

var asyncRegistry = struct {
	sync.Mutex
	writers map[*Store]*AsyncWriter
}{writers: make(map[*Store]*AsyncWriter)}

func NewAsyncWriter(store *Store, capacity int) *AsyncWriter {
	if capacity <= 0 {
		capacity = 4096
	}
	writer := &AsyncWriter{
		store: store,
		queue: make(chan persistRequest, capacity),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	if store != nil {
		writer.writeFn = store.appendBarDisk
	}
	go writer.run()
	return writer
}

func (s *Store) AppendBarAsync(bar domain.Bar) error {
	if s == nil {
		return errors.New("history store is unavailable")
	}
	writer := asyncWriterFor(s)
	return writer.AppendBar(bar)
}

func (s *Store) FlushAsync(ctx context.Context) error {
	if s == nil {
		return nil
	}
	asyncRegistry.Lock()
	writer := asyncRegistry.writers[s]
	asyncRegistry.Unlock()
	if writer == nil {
		return nil
	}
	return writer.Flush(ctx)
}

// PersistenceStats reports an existing writer without starting a new one.
// A newly started, idle Market Core is ready even before its first final bar.
func (s *Store) PersistenceStats() (PersistenceStats, bool) {
	if s == nil {
		return PersistenceStats{}, false
	}
	asyncRegistry.Lock()
	writer := asyncRegistry.writers[s]
	asyncRegistry.Unlock()
	if writer == nil {
		return PersistenceStats{}, false
	}
	return writer.Stats(), true
}

// PersistenceReadiness never drops a finalized candle. Instead, it returns
// machine-readable failure reasons when disk I/O or queue pressure threatens
// the canonical ingestion path. The HTTP readiness probe can then fail early.
func (s *Store) PersistenceReadiness() (bool, []string) {
	if s == nil || s.root == "" {
		return false, []string{"history_storage_unavailable"}
	}
	stats, started := s.PersistenceStats()
	if !started {
		return true, nil
	}
	var reasons []string
	if stats.Pending > 0 && stats.LastError != "" {
		reasons = append(reasons, "history_persistence_error")
	}
	if stats.Pending > 0 && stats.FlushLagMS >= 30_000 {
		reasons = append(reasons, "history_flush_lag")
	}
	if stats.Capacity > 0 && stats.Queued >= (stats.Capacity*9+9)/10 {
		reasons = append(reasons, "history_queue_pressure")
	}
	return len(reasons) == 0, reasons
}

func asyncWriterFor(store *Store) *AsyncWriter {
	asyncRegistry.Lock()
	defer asyncRegistry.Unlock()
	if writer := asyncRegistry.writers[store]; writer != nil {
		return writer
	}
	writer := NewAsyncWriter(store, 4096)
	asyncRegistry.writers[store] = writer
	observability.SetHistorySource(func() any {
		return map[string]any{
			"cache":       store.CacheStats(),
			"persistence": writer.Stats(),
		}
	})
	return writer
}

func snapshotAsyncWriters() []*AsyncWriter {
	asyncRegistry.Lock()
	defer asyncRegistry.Unlock()
	writers := make([]*AsyncWriter, 0, len(asyncRegistry.writers))
	for _, writer := range asyncRegistry.writers {
		writers = append(writers, writer)
	}
	return writers
}

func FlushAsyncWriters(ctx context.Context) error {
	for _, writer := range snapshotAsyncWriters() {
		if err := writer.Flush(ctx); err != nil {
			return err
		}
	}
	return nil
}

func CloseAsyncWriters(ctx context.Context) error {
	for _, writer := range snapshotAsyncWriters() {
		if err := writer.Close(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (w *AsyncWriter) AppendBar(bar domain.Bar) error {
	if w == nil || w.store == nil || w.writeFn == nil {
		return errors.New("history async writer is unavailable")
	}
	if err := validateBar(bar); err != nil {
		return err
	}

	request := persistRequest{bar: bar, queuedAt: time.Now().UTC()}
	w.submitMu.Lock()
	defer w.submitMu.Unlock()
	if w.closed {
		return errors.New("history async writer is closed")
	}

	// Make the finalized candle immediately visible to current-day history.
	// Persistence remains ordered behind the queue and never drops finals.
	w.store.observeBar(bar)
	w.enqueued.Add(1)
	w.lastQueuedAtMS.Store(request.queuedAt.UnixMilli())
	w.queue <- request
	return nil
}

func (w *AsyncWriter) run() {
	defer close(w.done)
	for {
		select {
		case request := <-w.queue:
			w.persist(request)
		case <-w.stop:
			for {
				select {
				case request := <-w.queue:
					w.persist(request)
				default:
					return
				}
			}
		}
	}
}

func (w *AsyncWriter) persist(request persistRequest) {
	for {
		err := w.writeFn(request.bar)
		if err == nil {
			// This process already placed the final in its RAM cache before
			// queuing persistence. Record the resulting file fingerprint so the
			// current-day reader stays on the fast RAM path. A different process
			// appending later changes the fingerprint and triggers a refresh.
			w.store.refreshDiskFingerprint(request.bar)
			w.writes.Add(1)
			w.lastFlushAtMS.Store(time.Now().UTC().UnixMilli())
			w.errMu.Lock()
			w.lastError = ""
			w.errMu.Unlock()
			return
		}

		// Canonical finalized bars are never silently dropped. A storage error
		// is retried; if the queue eventually fills, producers backpressure
		// rather than losing history.
		w.errors.Add(1)
		w.errMu.Lock()
		w.lastError = err.Error()
		w.errMu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
}

func (w *AsyncWriter) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		stats := w.Stats()
		if stats.Pending == 0 && stats.Queued == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *AsyncWriter) Close(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.submitMu.Lock()
	w.closed = true
	w.submitMu.Unlock()
	w.stopOnce.Do(func() { close(w.stop) })

	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *AsyncWriter) Stats() PersistenceStats {
	if w == nil {
		return PersistenceStats{}
	}
	enqueued := w.enqueued.Load()
	writes := w.writes.Load()
	pending := uint64(0)
	if enqueued > writes {
		pending = enqueued - writes
	}
	lastQueued := w.lastQueuedAtMS.Load()
	lastFlush := w.lastFlushAtMS.Load()
	flushLag := int64(0)
	if pending > 0 {
		nowMS := time.Now().UTC().UnixMilli()
		if lastFlush > 0 {
			flushLag = nowMS - lastFlush
		} else if lastQueued > 0 {
			flushLag = nowMS - lastQueued
		}
		if flushLag < 0 {
			flushLag = 0
		}
	}
	w.errMu.RLock()
	lastError := w.lastError
	w.errMu.RUnlock()
	return PersistenceStats{
		Queued:         len(w.queue),
		Capacity:       cap(w.queue),
		Pending:        pending,
		Enqueued:       enqueued,
		Writes:         writes,
		Errors:         w.errors.Load(),
		LastQueuedAtMS: lastQueued,
		LastFlushAtMS:  lastFlush,
		FlushLagMS:     flushLag,
		LastError:      lastError,
	}
}
