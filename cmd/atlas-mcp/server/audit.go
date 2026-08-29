package server

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEntry is one JSONL record written to the audit log per tools/call.
// Field tags are snake_case to align with the atlas-go domain convention.
//
// Schema version 2 adds fields: SchemaVersion, ArgsHash, SessionID, Transport.
// When reading, entries without SchemaVersion are treated as v1 and backfilled
// with safe defaults (empty ArgsHash/SessionID, Transport="unknown").
type AuditEntry struct {
	SchemaVersion int                    `json:"schema_version"`
	TS            string                 `json:"ts"`
	SessionID     string                 `json:"session_id,omitempty"`
	Tool          string                 `json:"tool"`
	TenantID      string                 `json:"tenant_id,omitempty"`
	AgentID       string                 `json:"agent_id,omitempty"`
	CallerPID     int                    `json:"caller_pid,omitempty"`
	ArgKeys       []string               `json:"arg_keys,omitempty"`
	ArgsHash      string                 `json:"args_hash,omitempty"`
	Status        string                 `json:"status"` // "ok" | "error" | "unauthorized" | "ratelimited"
	LatencyMS     int64                  `json:"latency_ms"`
	DurationMS    int64                  `json:"duration_ms"` // kept for v1 backward compat; v2 prefers latency_ms
	Transport     string                 `json:"transport,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Extra         map[string]interface{} `json:"extra,omitempty"`
}

// CanonicalizeArgsHash computes SHA-256 hex of the canonical JSON form of
// argKeys. This is deterministic: same keys → same hash, regardless of order
// (the caller is responsible for sorting if needed).
func CanonicalizeArgsHash(argKeys []string) string {
	if len(argKeys) == 0 {
		return ""
	}
	raw, err := json.Marshal(argKeys)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// AuditWriter serializes AuditEntry values as JSON lines to a file. It is safe
// for concurrent use.
type AuditWriter struct {
	mu   sync.Mutex
	f    *os.File
	enc  *json.Encoder
	path string
	now  func() time.Time
}

// NewAuditWriter opens (creating if missing) the audit log at path. The parent
// directory is created with mode 0700 if it does not exist. The returned writer
// should be closed via Close.
func NewAuditWriter(path string) (*AuditWriter, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("audit: mkdir %s: %w", dir, err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	return &AuditWriter{
		f:    f,
		enc:  json.NewEncoder(f),
		path: path,
		now:  time.Now,
	}, nil
}

// Close closes the underlying file. Safe to call multiple times.
func (w *AuditWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// Write serializes entry as one JSON line and flushes it to disk. It
// auto-populates TS, SchemaVersion, Transport, and ArgsHash when empty.
// DurationMS and LatencyMS are synchronized for backward compatibility.
func (w *AuditWriter) Write(entry AuditEntry) error {
	if entry.TS == "" {
		entry.TS = w.now().UTC().Format(time.RFC3339Nano)
	}
	if entry.SchemaVersion == 0 {
		entry.SchemaVersion = 2
	}
	if entry.Transport == "" {
		entry.Transport = "stdio"
	}
	if entry.ArgsHash == "" && len(entry.ArgKeys) > 0 {
		entry.ArgsHash = CanonicalizeArgsHash(entry.ArgKeys)
	}
	if entry.DurationMS < 0 {
		entry.DurationMS = 0
	}
	if entry.LatencyMS == 0 && entry.DurationMS != 0 {
		entry.LatencyMS = entry.DurationMS
	}
	if entry.DurationMS == 0 && entry.LatencyMS != 0 {
		entry.DurationMS = entry.LatencyMS
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return fmt.Errorf("audit: writer closed")
	}
	if err := w.enc.Encode(&entry); err != nil {
		return fmt.Errorf("audit: encode: %w", err)
	}
	if entry.Status == "error" || entry.Status == "unauthorized" {
		if err := w.f.Sync(); err != nil {
			return fmt.Errorf("audit: sync after %s: %w", entry.Status, err)
		}
	}
	return nil
}

// Cleanup removes entries older than retentionDays from the audit log. It
// reads all entries, filters by timestamp, and atomically rewrites the file
// (tmp + rename) to preserve durability. Returns the number of entries
// removed. retentionDays <= 0 disables cleanup (returns 0, nil).
//
// Each entry's TS is RFC3339Nano. Lines that fail to parse are kept (audit
// log integrity takes priority over cleanup aggression — corruption visible
// is better than corruption hidden). The writer remains usable after
// Cleanup; the underlying file is closed and reopened for append.
//
// Cleanup is safe to call concurrently with Write; it acquires the same
// mutex and the file swap is atomic (rename(2)).
func (w *AuditWriter) Cleanup(retentionDays int, now time.Time) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return 0, fmt.Errorf("audit: writer closed")
	}

	// Flush any pending writes to disk before reading.
	if err := w.f.Sync(); err != nil {
		return 0, fmt.Errorf("audit: sync before cleanup: %w", err)
	}
	if err := w.f.Close(); err != nil {
		return 0, fmt.Errorf("audit: close before cleanup: %w", err)
	}
	// Do NOT set w.f = nil here — the mutex already serializes Write access.
	// If the reopen below fails, we want to retry rather than leave the
	// writer permanently poisoned (#1267).

	rf, err := os.Open(w.path)
	if err != nil {
		if re := w.reopenForAppend(); re != nil {
			return 0, fmt.Errorf("audit: open for read failed (%w) and reopen also failed (%w)", err, re)
		}
		return 0, fmt.Errorf("audit: reopen for cleanup (read failed, but writer recovered): %w", err)
	}

	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	var keep [][]byte
	removed := 0
	scanner := bufio.NewScanner(rf)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		ts := extractTS(line)
		if ts == nil || ts.After(cutoff) {
			// copy because scanner reuses the buffer on next Scan()
			keep = append(keep, append([]byte(nil), line...))
		} else {
			removed++
		}
	}
	scanErr := scanner.Err()
	_ = rf.Close()
	if scanErr != nil {
		_ = w.reopenForAppend()
		return removed, fmt.Errorf("audit: scan during cleanup: %w", scanErr)
	}

	tmpPath := w.path + ".cleanup.tmp"
	tf, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		_ = w.reopenForAppend()
		return removed, fmt.Errorf("audit: open tmp: %w", err)
	}
	for _, line := range keep {
		if _, wErr := tf.Write(line); wErr != nil {
			_ = tf.Close()
			_ = os.Remove(tmpPath)
			_ = w.reopenForAppend()
			return removed, fmt.Errorf("audit: write tmp: %w", wErr)
		}
		if _, wErr := tf.Write([]byte("\n")); wErr != nil {
			_ = tf.Close()
			_ = os.Remove(tmpPath)
			_ = w.reopenForAppend()
			return removed, fmt.Errorf("audit: write tmp newline: %w", wErr)
		}
	}
	if sErr := tf.Sync(); sErr != nil {
		_ = tf.Close()
		_ = os.Remove(tmpPath)
		_ = w.reopenForAppend()
		return removed, fmt.Errorf("audit: sync tmp: %w", sErr)
	}
	if cErr := tf.Close(); cErr != nil {
		_ = os.Remove(tmpPath)
		_ = w.reopenForAppend()
		return removed, fmt.Errorf("audit: close tmp: %w", cErr)
	}
	if rErr := os.Rename(tmpPath, w.path); rErr != nil {
		_ = w.reopenForAppend()
		return removed, fmt.Errorf("audit: rename tmp: %w", rErr)
	}

	// Reopen for append so subsequent Writes succeed.
	if err := w.reopenForAppend(); err != nil {
		return removed, fmt.Errorf("audit: reopen after cleanup: %w", err)
	}

	return removed, nil
}

// reopenForAppend reopens the audit log file for append. It retries up to 3
// times with exponential backoff. The caller MUST hold w.mu. After a
// successful call, w.f and w.enc are restored.
func (w *AuditWriter) reopenForAppend() error {
	const maxAttempts = 3
	for attempt := range maxAttempts {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 50 * time.Millisecond
			time.Sleep(backoff)
		}
		f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			w.f = f
			w.enc = json.NewEncoder(f)
			return nil
		}
	}
	return fmt.Errorf("audit: reopen failed after %d attempts: %w", maxAttempts, os.ErrNotExist)
}

// Healthy returns whether the writer's underlying file handle is open and
// usable. A false return means the writer is in a poisoned state that may
// self-heal on the next Write or Cleanup.
func (w *AuditWriter) Healthy() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f != nil
}

// extractTS pulls the "ts":"..." value from a JSON line. Returns nil if not
// parseable. Best-effort: malformed lines are kept (audit log integrity
func extractTS(line []byte) *time.Time {
	var e AuditEntry
	if err := json.Unmarshal(line, &e); err != nil {
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, e.TS); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
		return &t
	}
	return nil
}

// ReadAuditEntries reads the audit log at path and returns all entries within
// the retention window (now - retentionDays). retentionDays <= 0 disables
// filtering and returns all entries.
//
// Backward compatibility: entries without schema_version are treated as v1
// and backfilled with SchemaVersion=1. LatencyMS is derived from DurationMS
// when missing.
//
// Malformed JSON lines are silently skipped. Missing file returns an error
// (fail-closed). An empty file returns an empty slice with no error.
func ReadAuditEntries(path string, retentionDays int, now time.Time) ([]AuditEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var entries []AuditEntry
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines
		}

		// Backfill v1 entries (no schema_version).
		if e.SchemaVersion == 0 {
			e.SchemaVersion = 1
		}

		// v1 entries don't have LatencyMS — use DurationMS.
		if e.LatencyMS == 0 && e.DurationMS != 0 {
			e.LatencyMS = e.DurationMS
		}

		// Retention filter.
		if retentionDays > 0 {
			t, err := time.Parse(time.RFC3339Nano, e.TS)
			if err != nil {
				t, err = time.Parse(time.RFC3339, e.TS)
			}
			if err == nil && t.Before(cutoff) {
				continue
			}
		}

		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("audit: scan %s: %w", path, err)
	}
	return entries, nil
}

// AuditEntry implements anomaly.Entry so the anomaly detector can consume
// freshly-written audit records without an extra translation layer.
func (e *AuditEntry) Version() int { return e.SchemaVersion }

// ObservedAt parses the audit timestamp. A zero value means the detector will
// fall back to its own clock.
func (e *AuditEntry) ObservedAt() time.Time {
	if e.TS == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, e.TS); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
		return t
	}
	return time.Time{}
}

func (e *AuditEntry) GetTool() string { return e.Tool }

func (e *AuditEntry) GetTenantID() string {
	if e.TenantID == "" {
		return "anonymous"
	}
	return e.TenantID
}

func (e *AuditEntry) GetStatus() string { return e.Status }

func (e *AuditEntry) GetError() string { return e.Error }

// NewV2Entry is the canonical constructor for AuditEntry v2 records.
// It sets SchemaVersion=2, Transport="stdio", Status="ok", and computes
// ArgsHash from argKeys via CanonicalizeArgsHash.
func NewV2Entry(agentID, tool string, argKeys []string, latencyMS int64) *AuditEntry {
	return &AuditEntry{
		SchemaVersion: 2,
		AgentID:       agentID,
		Tool:          tool,
		ArgKeys:       argKeys,
		ArgsHash:      CanonicalizeArgsHash(argKeys),
		LatencyMS:     latencyMS,
		DurationMS:    latencyMS,
		Transport:     "stdio",
		Status:        "ok",
	}
}
