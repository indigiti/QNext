package capture

import (
	"errors"
	"sync"
	"sync/atomic"
)

// AsyncFrameStore keeps raw-frame persistence off the live market-data path.
// Capture is diagnostic/best-effort: when the bounded queue is full, the
// newest frame is dropped rather than stalling or disconnecting live ingestion.
type AsyncFrameStore struct {
	store *FrameStore
	queue chan []byte
	once  sync.Once
	wg    sync.WaitGroup

	enqueued atomic.Uint64
	dropped  atomic.Uint64
	errors   atomic.Uint64
}

type AsyncFrameStoreStats struct {
	Enqueued uint64 `json:"enqueued"`
	Dropped  uint64 `json:"dropped"`
	Errors   uint64 `json:"errors"`
	Queued   int    `json:"queued"`
	Capacity int    `json:"capacity"`
}

func NewAsyncFrameStore(root, provider string, capacity int) *AsyncFrameStore {
	if capacity <= 0 {
		capacity = 1024
	}
	store := &AsyncFrameStore{
		store: NewFrameStore(root, provider),
		queue: make(chan []byte, capacity),
	}
	store.wg.Add(1)
	go store.run()
	return store
}

func (s *AsyncFrameStore) Append(payload []byte) error {
	if s == nil || s.store == nil {
		return errors.New("async raw frame store is unavailable")
	}
	if s.store.root == "" || s.store.provider == "" {
		return errors.New("raw frame store requires root and provider")
	}
	if len(payload) == 0 {
		return errors.New("raw frame payload is empty")
	}

	frame := append([]byte(nil), payload...)
	select {
	case s.queue <- frame:
		s.enqueued.Add(1)
	default:
		s.dropped.Add(1)
	}
	return nil
}

func (s *AsyncFrameStore) Stats() AsyncFrameStoreStats {
	if s == nil {
		return AsyncFrameStoreStats{}
	}
	return AsyncFrameStoreStats{
		Enqueued: s.enqueued.Load(),
		Dropped:  s.dropped.Load(),
		Errors:   s.errors.Load(),
		Queued:   len(s.queue),
		Capacity: cap(s.queue),
	}
}

func (s *AsyncFrameStore) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.queue)
		s.wg.Wait()
	})
}

func (s *AsyncFrameStore) run() {
	defer s.wg.Done()
	for frame := range s.queue {
		if err := s.store.Append(frame); err != nil {
			s.errors.Add(1)
		}
	}
}
