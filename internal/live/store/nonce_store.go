package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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

type redisNonceReplayStore struct {
	client    *redis.Client
	keyPrefix string
}

type NonceReplayStoreOptions struct {
	FilePath       string
	RedisURL       string
	RedisKeyPrefix string
	RedisClient    *redis.Client
	RedisOpTimeout time.Duration
}

func NewFileNonceReplayStore(path string) NonceReplayStore {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return &fileNonceReplayStore{path: ""}
	}
	return &fileNonceReplayStore{path: filepath.Clean(trimmed)}
}

func BuildNonceReplayStore(storeType string, storePath string) (NonceReplayStore, error) {
	return BuildNonceReplayStoreWithOptions(storeType, NonceReplayStoreOptions{FilePath: storePath})
}

func BuildNonceReplayStoreWithOptions(storeType string, opts NonceReplayStoreOptions) (NonceReplayStore, error) {
	kind := strings.TrimSpace(strings.ToLower(storeType))
	if kind == "" {
		kind = "memory"
	}

	switch kind {
	case "memory":
		return NewInMemoryNonceReplayStore(), nil
	case "file":
		if strings.TrimSpace(opts.FilePath) == "" {
			return nil, fmt.Errorf("nonce replay store path is required for file store")
		}
		return NewFileNonceReplayStore(opts.FilePath), nil
	case "redis":
		if opts.RedisClient != nil {
			return NewRedisNonceReplayStore(opts.RedisClient, opts.RedisKeyPrefix), nil
		}
		if strings.TrimSpace(opts.RedisURL) == "" {
			return nil, fmt.Errorf("nonce replay redis url is required for redis store")
		}
		redisOpts, err := redis.ParseURL(strings.TrimSpace(opts.RedisURL))
		if err != nil {
			return nil, fmt.Errorf("parse redis url: %w", err)
		}
		client := redis.NewClient(redisOpts)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := client.Ping(ctx).Err(); err != nil {
			return nil, fmt.Errorf("ping redis: %w", err)
		}
		return NewRedisNonceReplayStore(client, opts.RedisKeyPrefix), nil
	default:
		return nil, fmt.Errorf("unsupported nonce replay store type %q (allowed: memory, file, redis)", kind)
	}
}

func NewRedisNonceReplayStore(client *redis.Client, keyPrefix string) NonceReplayStore {
	prefix := strings.TrimSpace(keyPrefix)
	if prefix == "" {
		prefix = "atlas:nonce:"
	}
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return &redisNonceReplayStore{client: client, keyPrefix: prefix}
}

func (s *redisNonceReplayStore) Register(nonce string, requestTime time.Time, ttl time.Duration) error {
	if s.client == nil {
		return fmt.Errorf("redis client is required")
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := s.keyPrefix + nonce
	ok, err := s.client.SetArgs(ctx, key, requestTime.UTC().Format(time.RFC3339Nano), redis.SetArgs{
		Mode: "NX",
		TTL:  ttl,
	}).Result()
	if errors.Is(err, redis.Nil) || ok == "" {
		return fmt.Errorf("%w: nonce=%s", ErrNonceReplayDetected, nonce)
	}
	if err != nil {
		return fmt.Errorf("redis setnx failed: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("read nonce store: %w", err)
	}
	if len(bytes) == 0 {
		return items, nil
	}
	if err := json.Unmarshal(bytes, &items); err != nil {
		return nil, fmt.Errorf("unmarshal nonce store: %w", err)
	}
	return items, nil
}

func (s *fileNonceReplayStore) save(items map[string]time.Time) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	bytes, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("marshal nonce store: %w", err)
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, bytes, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	return os.Rename(tmpPath, s.path)
}
