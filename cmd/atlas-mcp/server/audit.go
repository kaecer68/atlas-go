package server

import (
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
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
	now func() time.Time
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
		f:   f,
		enc: json.NewEncoder(f),
		now: time.Now,
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
