package live

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
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
