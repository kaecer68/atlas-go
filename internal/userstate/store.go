package userstate

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ErrSignalStateNotFound is returned by LoadByUserAndSignal when no record
// matches the (UserID, SignalKey) tuple.
var ErrSignalStateNotFound = errors.New("userstate: signal state not found")

// defaultSignalStateCap bounds the JSONL file size. The store keeps the
// last maxRecords entries (FIFO eviction). At 1 record per (user, signal)
// state-change this scales well for a single retail investor plus a few
// power-users; production scaling would move to a relational store.
const defaultSignalStateCap = 1000

// SignalStateStore persists per-user per-signal read-state so the
// dashboard can render a "new / acknowledged / dismissed" badge. The
// primary key is the (UserID, SignalKey) tuple — Upsert replaces the
// existing record; there is at most one record per tuple at any time.
//
// JSONL chosen over SQLite to keep read-modify-write semantics aligned with
// event_flow_prediction_store (R #1484) and to avoid a second SQL
// connection. The store reads the entire file on every write (FIFO 1000
// records ≈ a few hundred KB at typical scale — sub-millisecond parse).
type SignalStateStore interface {
	// Upsert inserts or updates the (UserID, SignalKey) record with the given
	// payload. AcknowledgedAt and Dismissed are merged from the new state;
	// other fields are stored verbatim.
	Upsert(state UserSignalState) error
	// LoadByUser returns all records for the given user, ordered by
	// UpdatedAt DESC. Used by the dashboard to render per-signal badges.
	LoadByUser(userID int64) ([]UserSignalState, error)
	// LoadByUserAndSignal returns the single record for the (userID,
	// signalKey) tuple. Returns ErrSignalStateNotFound when absent.
	LoadByUserAndSignal(userID int64, signalKey string) (UserSignalState, error)
}

// JSONLStore is the JSONL-backed implementation of SignalStateStore. The
// path defaults to <baseDir>/user_signals.jsonl. Concurrent access is
// serialized by a sync.Mutex on the read-modify-write path.
type JSONLStore struct {
	baseDir    string
	maxRecords int
	mu         sync.Mutex
}

// NewJSONLStore creates a store rooted at dir (will be created on first
// write). cap of 0 disables FIFO trimming (only recommended for tests).
func NewJSONLStore(dir string) *JSONLStore {
	return &JSONLStore{baseDir: dir, maxRecords: defaultSignalStateCap}
}

// NewJSONLStoreWithCap is the explicit-cap variant (used by tests that
// exercise the FIFO eviction path).
func NewJSONLStoreWithCap(dir string, cap int) *JSONLStore {
	return &JSONLStore{baseDir: dir, maxRecords: cap}
}

func (s *JSONLStore) path() string {
	return filepath.Join(s.baseDir, "user_signals.jsonl")
}

func (s *JSONLStore) Upsert(state UserSignalState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}
	records, err := readSignalStateJSONL(s.path())
	if err != nil {
		return fmt.Errorf("upsert read: %w", err)
	}
	// UpdatedAt is set by the store — callers do not control it. This keeps
	// the per-record timestamp authoritative and avoids drift between
	// caller-supplied and store-observed clocks.
	now := time.Now().UTC()
	state.UpdatedAt = now
	// Find and replace the existing (UserID, SignalKey) record.
	replaced := false
	for i := range records {
		if records[i].UserID == state.UserID && records[i].SignalKey == state.SignalKey {
			records[i] = state
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, state)
	}
	// Newest first: sort by UpdatedAt DESC so LoadByUser returns the most
	// recent state per (user, signal) without scanning all records. Ties
	// are broken by SignalKey for determinism.
	sort.Slice(records, func(i, j int) bool {
		if !records[i].UpdatedAt.Equal(records[j].UpdatedAt) {
			return records[i].UpdatedAt.After(records[j].UpdatedAt)
		}
		return records[i].SignalKey < records[j].SignalKey
	})
	if s.maxRecords > 0 && len(records) > s.maxRecords {
		records = records[:s.maxRecords]
	}
	return writeSignalStateJSONL(s.path(), records)
}

func (s *JSONLStore) LoadByUser(userID int64) ([]UserSignalState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := readSignalStateJSONL(s.path())
	if err != nil {
		return nil, fmt.Errorf("load by user: %w", err)
	}
	out := make([]UserSignalState, 0, len(records))
	for _, r := range records {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *JSONLStore) LoadByUserAndSignal(userID int64, signalKey string) (UserSignalState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := readSignalStateJSONL(s.path())
	if err != nil {
		return UserSignalState{}, fmt.Errorf("load by tuple: %w", err)
	}
	for _, r := range records {
		if r.UserID == userID && r.SignalKey == signalKey {
			return r, nil
		}
	}
	return UserSignalState{}, ErrSignalStateNotFound
}

// --- JSONL I/O (private; mirrors event_flow_prediction_store style) ----

func readSignalStateJSONL(path string) ([]UserSignalState, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	// Buffer up to 1 MiB per line so a future field (e.g. long user Notes
	// in UserJournal) cannot silently break the store by exceeding
	// bufio.Scanner's 64 KiB default.
	var out []UserSignalState
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		var r UserSignalState
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("decode signal state: %w", err)
		}
		out = append(out, r)
	}
	return out, scanner.Err()
}

func writeSignalStateJSONL(path string, records []UserSignalState) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("encode signal state: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close file: %w", err)
	}
	return os.Rename(tmp, path)
}
