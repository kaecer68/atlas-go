package llm_annotator

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// AnnotationStore persists AnnotationRecord for offline analysis. A nil
// store is valid and means "in-memory ring buffer only".
type AnnotationStore interface {
	Write(rec AnnotationRecord) error
	Close() error
}

// jsonlStore writes one JSON-encoded AnnotationRecord per line to a
// file. File rotation is optional: when the current file exceeds
// maxBytes, it is renamed with a numeric suffix and a fresh file is
// opened on the next write. At most keepFiles rotated copies are kept.
type jsonlStore struct {
	mu       sync.Mutex
	path     string
	f        *os.File
	size     int64
	maxBytes int64
	keep     int
}

const defaultJSONLMaxBytes int64 = 50 * 1024 * 1024
const defaultJSONLKeep int = 3

// NewJSONLStore creates a JSONL store with default rotation (50MB per
// file, 3 rotated copies).
func NewJSONLStore(path string) (AnnotationStore, error) {
	return NewJSONLStoreWithRotation(path, defaultJSONLMaxBytes, defaultJSONLKeep)
}

// NewJSONLStoreWithRotation creates a JSONL store with explicit rotation
// parameters. maxBytes <= 0 disables rotation.
func NewJSONLStoreWithRotation(path string, maxBytes int64, keep int) (AnnotationStore, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	return &jsonlStore{
		path:     path,
		f:        f,
		size:     st.Size(),
		maxBytes: maxBytes,
		keep:     keep,
	}, nil
}

func (s *jsonlStore) Write(rec AnnotationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.maxBytes > 0 && s.size >= s.maxBytes {
		if err := s.rotateLocked(); err != nil {
			return err
		}
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')
	n, err := s.f.Write(data)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	s.size += int64(n)
	return nil
}

func (s *jsonlStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

func (s *jsonlStore) rotateLocked() error {
	if err := s.f.Close(); err != nil {
		return fmt.Errorf("close before rotate: %w", err)
	}
	for i := s.keep - 1; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", s.path, i)
		next := fmt.Sprintf("%s.%d", s.path, i+1)
		if _, err := os.Stat(old); err == nil {
			_ = os.Rename(old, next)
		}
	}
	if err := os.Rename(s.path, s.path+".1"); err != nil {
		return fmt.Errorf("rotate %s: %w", s.path, err)
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open new %s: %w", s.path, err)
	}
	s.f = f
	s.size = 0
	return nil
}
