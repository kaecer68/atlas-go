package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEntry is one JSONL record written to the audit log per tools/call.
// Field tags are snake_case to align with the atlas-go domain convention.
type AuditEntry struct {
	TS         string                 `json:"ts"`
	Tool       string                 `json:"tool"`
	TenantID   string                 `json:"tenant_id,omitempty"`
	AgentID    string                 `json:"agent_id,omitempty"`
	CallerPID  int                    `json:"caller_pid,omitempty"`
	ArgKeys    []string               `json:"arg_keys,omitempty"`
	Status     string                 `json:"status"` // "ok" | "error" | "unauthorized"
	DurationMS int64                  `json:"duration_ms"`
	Error      string                 `json:"error,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
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

// Write serializes entry as one JSON line and flushes it to disk. Required
// fields (TS, Tool, Status, DurationMS) are populated from entry or
// automatically (TS, DurationMS).
func (w *AuditWriter) Write(entry AuditEntry) error {
	if entry.TS == "" {
		entry.TS = w.now().UTC().Format(time.RFC3339Nano)
	}
	if entry.DurationMS < 0 {
		entry.DurationMS = 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return fmt.Errorf("audit: writer closed")
	}
	if err := w.enc.Encode(&entry); err != nil {
		return fmt.Errorf("audit: encode: %w", err)
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
	w.f = nil

	rf, err := os.Open(w.path)
	if err != nil {
		return 0, fmt.Errorf("audit: reopen for cleanup: %w", err)
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
		return removed, fmt.Errorf("audit: scan during cleanup: %w", scanErr)
	}

	tmpPath := w.path + ".cleanup.tmp"
	tf, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return removed, fmt.Errorf("audit: open tmp: %w", err)
	}
	for _, line := range keep {
		if _, wErr := tf.Write(line); wErr != nil {
			_ = tf.Close()
			_ = os.Remove(tmpPath)
			return removed, fmt.Errorf("audit: write tmp: %w", wErr)
		}
		if _, wErr := tf.Write([]byte("\n")); wErr != nil {
			_ = tf.Close()
			_ = os.Remove(tmpPath)
			return removed, fmt.Errorf("audit: write tmp newline: %w", wErr)
		}
	}
	if sErr := tf.Sync(); sErr != nil {
		_ = tf.Close()
		_ = os.Remove(tmpPath)
		return removed, fmt.Errorf("audit: sync tmp: %w", sErr)
	}
	if cErr := tf.Close(); cErr != nil {
		_ = os.Remove(tmpPath)
		return removed, fmt.Errorf("audit: close tmp: %w", cErr)
	}
	if rErr := os.Rename(tmpPath, w.path); rErr != nil {
		return removed, fmt.Errorf("audit: rename tmp: %w", rErr)
	}

	// Reopen for append so subsequent Writes succeed.
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return removed, fmt.Errorf("audit: reopen after cleanup: %w", err)
	}
	w.f = f
	w.enc = json.NewEncoder(f)

	return removed, nil
}

// extractTS pulls the "ts":"..." value from a JSON line. Returns nil if not
// parseable. Best-effort: malformed lines are kept (audit log integrity
// takes priority over cleanup aggression).
func extractTS(line []byte) *time.Time {
	var e AuditEntry
	if err := json.Unmarshal(line, &e); err != nil {
		return nil
	}
	if e.TS == "" {
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
