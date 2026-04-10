package live

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrNonceReplayDetected = errors.New("request nonce replay detected")

type NonceReplayStore interface {
	Register(nonce string, requestTime time.Time, ttl time.Duration) error
}

type inMemoryNonceReplayStore struct {
	mu    sync.Mutex
	items map[string]time.Time
}

func NewInMemoryNonceReplayStore() NonceReplayStore {
	return &inMemoryNonceReplayStore{items: make(map[string]time.Time)}
}

func (s *inMemoryNonceReplayStore) Register(nonce string, requestTime time.Time, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for n, ts := range s.items {
		if requestTime.Sub(ts) > ttl {
			delete(s.items, n)
		}
	}

	if _, exists := s.items[nonce]; exists {
		return fmt.Errorf("%w: nonce=%s", ErrNonceReplayDetected, nonce)
	}

	s.items[nonce] = requestTime.UTC()
	return nil
}

type fileNonceReplayStore struct {
	path string
	mu   sync.Mutex
}

func NewFileNonceReplayStore(path string) NonceReplayStore {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return &fileNonceReplayStore{path: ""}
	}
	return &fileNonceReplayStore{path: filepath.Clean(trimmed)}
}

func BuildNonceReplayStore(storeType string, storePath string) (NonceReplayStore, error) {
	kind := strings.TrimSpace(strings.ToLower(storeType))
	if kind == "" {
		kind = "memory"
	}

	switch kind {
	case "memory":
		return NewInMemoryNonceReplayStore(), nil
	case "file":
		if strings.TrimSpace(storePath) == "" {
			return nil, fmt.Errorf("nonce replay store path is required for file store")
		}
		return NewFileNonceReplayStore(storePath), nil
	default:
		return nil, fmt.Errorf("unsupported nonce replay store type %q (allowed: memory, file)", kind)
	}
}

func (s *fileNonceReplayStore) Register(nonce string, requestTime time.Time, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("nonce replay store path is required")
	}
	items, err := s.load()
	if err != nil {
		return fmt.Errorf("load nonce replay store: %w", err)
	}

	for n, ts := range items {
		if requestTime.Sub(ts) > ttl {
			delete(items, n)
		}
	}

	if _, exists := items[nonce]; exists {
		return fmt.Errorf("%w: nonce=%s", ErrNonceReplayDetected, nonce)
	}

	items[nonce] = requestTime.UTC()
	if err := s.save(items); err != nil {
		return fmt.Errorf("save nonce replay store: %w", err)
	}
	return nil
}

func (s *fileNonceReplayStore) load() (map[string]time.Time, error) {
	items := make(map[string]time.Time)
	bytes, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, err
	}
	if len(bytes) == 0 {
		return items, nil
	}
	if err := json.Unmarshal(bytes, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *fileNonceReplayStore) save(items map[string]time.Time) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	bytes, err := json.Marshal(items)
	if err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, bytes, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
