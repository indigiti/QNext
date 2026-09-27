package research

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const researchStoreMaxOpenFiles = 8

// JSONLStore is the shared durable append sink for research datasets. It keeps
// daily files open between snapshots, serializes writes, and fsyncs every
// append so the storage optimization does not weaken durability semantics.
type JSONLStore struct {
	root string

	mu     sync.Mutex
	files  map[string]*os.File
	closed bool
}

func NewJSONLStore(root string) (*JSONLStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("research storage root is required")
	}
	return &JSONLStore{
		root:  root,
		files: make(map[string]*os.File),
	}, nil
}

func (s *JSONLStore) Append(relativePath string, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	clean := filepath.Clean(relativePath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return errors.New("research storage path must stay below storage root")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("research storage is closed")
	}

	path := filepath.Join(s.root, clean)
	file := s.files[path]
	if file == nil {
		if len(s.files) >= researchStoreMaxOpenFiles {
			if err := s.closeFilesLocked(); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		opened, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return err
		}
		file = opened
		s.files[path] = file
	}

	written, err := file.Write(payload)
	if err != nil {
		return err
	}
	if written != len(payload) {
		return io.ErrShortWrite
	}
	return file.Sync()
}

func (s *JSONLStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.closeFilesLocked()
}

func (s *JSONLStore) closeFilesLocked() error {
	var result error
	for path, file := range s.files {
		if err := file.Sync(); err != nil {
			result = errors.Join(result, err)
		}
		if err := file.Close(); err != nil {
			result = errors.Join(result, err)
		}
		delete(s.files, path)
	}
	return result
}
