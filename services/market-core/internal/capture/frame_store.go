package capture

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const rawFrameSchema = "QNEXT.RAW.FRAME/1"

type FrameStore struct {
	root     string
	provider string
	mu       sync.Mutex
	now      func() time.Time
}

type frameRecord struct {
	Schema       string `json:"schema"`
	Provider     string `json:"provider"`
	CapturedAtMS int64  `json:"captured_at_ms"`
	PayloadBase64 string `json:"payload_base64"`
}

func NewFrameStore(root, provider string) *FrameStore {
	return &FrameStore{
		root:     root,
		provider: strings.ToLower(strings.TrimSpace(provider)),
		now:      time.Now,
	}
}

func (s *FrameStore) Append(payload []byte) error {
	if s.root == "" || s.provider == "" {
		return errors.New("raw frame store requires root and provider")
	}
	if len(payload) == 0 {
		return errors.New("raw frame payload is empty")
	}
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

	s.mu.Lock()
	defer s.mu.Unlock()
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
	return file.Sync()
}
