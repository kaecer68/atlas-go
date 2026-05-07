package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestBuildNonceReplayStoreDefaultsToMemory(t *testing.T) {
	store, err := BuildNonceReplayStore("", "")
	if err != nil {
		t.Fatalf("BuildNonceReplayStore error: %v", err)
	}
	if store == nil {
		t.Fatalf("expected non-nil store")
	}
}

func TestBuildNonceReplayStoreFileRequiresPath(t *testing.T) {
	_, err := BuildNonceReplayStore("file", "")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestBuildNonceReplayStoreRedisRequiresURLWhenNoClient(t *testing.T) {
	_, err := BuildNonceReplayStoreWithOptions("redis", NonceReplayStoreOptions{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestInMemoryNonceReplayStoreRejectsReplayWithinTTL(t *testing.T) {
	store := NewInMemoryNonceReplayStore()
	now := time.Date(2026, time.April, 11, 22, 0, 0, 0, time.UTC)

	if err := store.Register("nonce-1", now, 5*time.Minute); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := store.Register("nonce-1", now.Add(1*time.Minute), 5*time.Minute)
	if err == nil {
		t.Fatalf("expected replay error, got nil")
	}
	if !errors.Is(err, ErrNonceReplayDetected) {
		t.Fatalf("expected ErrNonceReplayDetected, got %v", err)
	}
}

func TestInMemoryNonceReplayStoreAllowsReuseAfterTTL(t *testing.T) {
	store := NewInMemoryNonceReplayStore()
	now := time.Date(2026, time.April, 11, 22, 1, 0, 0, time.UTC)

	if err := store.Register("nonce-2", now, 2*time.Minute); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := store.Register("nonce-2", now.Add(3*time.Minute), 2*time.Minute); err != nil {
		t.Fatalf("second register failed: %v", err)
	}
}

func TestFileNonceReplayStorePersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonce-store.json")
	now := time.Date(2026, time.April, 11, 22, 2, 0, 0, time.UTC)

	storeA := NewFileNonceReplayStore(path)
	if err := storeA.Register("nonce-3", now, 10*time.Minute); err != nil {
		t.Fatalf("storeA register failed: %v", err)
	}

	storeB := NewFileNonceReplayStore(path)
	err := storeB.Register("nonce-3", now.Add(1*time.Minute), 10*time.Minute)
	if err == nil {
		t.Fatalf("expected replay error, got nil")
	}
	if !errors.Is(err, ErrNonceReplayDetected) {
		t.Fatalf("expected ErrNonceReplayDetected, got %v", err)
	}
}

func TestFileNonceReplayStoreAllowsReuseAfterTTL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonce-store.json")
	now := time.Date(2026, time.April, 11, 22, 3, 0, 0, time.UTC)

	store := NewFileNonceReplayStore(path)
	if err := store.Register("nonce-4", now, 2*time.Minute); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := store.Register("nonce-4", now.Add(3*time.Minute), 2*time.Minute); err != nil {
		t.Fatalf("second register failed: %v", err)
	}
}

func TestRedisNonceReplayStoreRejectsReplayAcrossInstances(t *testing.T) {
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer srv.Close()

	clientA := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer clientA.Close()
	clientB := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer clientB.Close()

	storeA := NewRedisNonceReplayStore(clientA, "atlas:test")
	storeB := NewRedisNonceReplayStore(clientB, "atlas:test")
	now := time.Date(2026, time.April, 11, 23, 0, 0, 0, time.UTC)

	if err := storeA.Register("nonce-r1", now, 5*time.Minute); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err = storeB.Register("nonce-r1", now.Add(1*time.Minute), 5*time.Minute)
	if err == nil {
		t.Fatalf("expected replay error, got nil")
	}
	if !errors.Is(err, ErrNonceReplayDetected) {
		t.Fatalf("expected ErrNonceReplayDetected, got %v", err)
	}
}

func TestRedisNonceReplayStoreAllowsReuseAfterTTL(t *testing.T) {
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer srv.Close()

	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer client.Close()
	store := NewRedisNonceReplayStore(client, "atlas:test")
	now := time.Date(2026, time.April, 11, 23, 10, 0, 0, time.UTC)

	if err := store.Register("nonce-r2", now, 2*time.Second); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	srv.FastForward(3 * time.Second)
	if err := store.Register("nonce-r2", now.Add(4*time.Second), 2*time.Second); err != nil {
		t.Fatalf("second register failed: %v", err)
	}
}
