package capture

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/observability"
)

const (
	rawFrameSchema       = "QNEXT.RAW.FRAME/1"
	defaultCaptureBuffer = 1024
)

type FrameStore struct {
	root     string
	provider string
	now      func() time.Time
	queue    chan []byte

	enqueued atomic.Uint64
	dropped  atomic.Uint64
	errors   atomic.Uint64
}

type FrameStoreStats struct {
	Enqueued uint64 `json:"enqueued"`
	Dropped  uint64 `json:"dropped"`
	Errors   uint64 `json:"errors"`
	Queued   int    `json:"queued"`
	Capacity int    `json:"capacity"`
}

type frameRecord struct {
	Schema        string `json:"schema"`
	Provider      string `json:"provider"`
	CapturedAtMS  int64  `json:"captured_at_ms"`
	PayloadBase64 string `json:"payload_base64"`
}

func NewFrameStore(root, provider string) *FrameStore {
	store := &FrameStore{
		root:     root,
		provider: strings.ToLower(strings.TrimSpace(provider)),
		now:      time.Now,
		queue:    make(chan []byte, defaultCaptureBuffer),
	}
	observability.SetCaptureSource(func() any { return store.Stats() })
	go store.run()
	return store
}

// Append is intentionally non-blocking. Raw capture is diagnostic and must
// never stall the live market-data path. If storage cannot keep up, frames are
// dropped from capture while live decoding/normalization continues.
func (s *FrameStore) Append(payload []byte) error {
	if s.root == "" || s.provider == "" {
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

func (s *FrameStore) Stats() FrameStoreStats {
	if s == nil {
		return FrameStoreStats{}
	}
	return FrameStoreStats{
		Enqueued: s.enqueued.Load(),
		Dropped:  s.dropped.Load(),
		Errors:   s.errors.Load(),
		Queued:   len(s.queue),
		Capacity: cap(s.queue),
	}
}

func (s *FrameStore) run() {
	for payload := range s.queue {
		if err := s.persist(payload); err != nil {
			s.errors.Add(1)
		}
	}
}

func (s *FrameStore) persist(payload []byte) error {
	now := s.now().UTC()
	record := frameRecord{
		Schema:        rawFrameSchema,
		Provider:      s.provider,
		CapturedAtMS:  now.UnixMilli(),
		PayloadBase64: base64.StdEncoding.EncodeToString(payload),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(
		s.root,
		"raw",
		s.provider,
		now.Format("2006"),
		now.Format("01"),
		now.Format("2006-01-02")+".jsonl",
	)

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
