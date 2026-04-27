package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// WriteError wraps ledger write operations with context.
type WriteError struct {
	Op   string // "mkdir" | "write_outcomes" | "write_rejects" | "write_summary" | "rename"
	Path string
	Err  error
}

func (e *WriteError) Error() string { return fmt.Sprintf("ledger %s: path=%s: %v", e.Op, e.Path, e.Err) }
func (e *WriteError) Unwrap() error { return e.Err }

// SessionWriteRequest contains all data for a session write operation.
type SessionWriteRequest struct {
	Session  domain.ReplaySession
	Outcomes []domain.RecommendationOutcome
	Rejects  []domain.ScreeningReject
	Summary  *domain.SessionSummary // nil means skip
}

// SessionWriter provides atomic session directory writes.
type SessionWriter struct {
	store *Store
	mu    sync.Mutex
}

func NewSessionWriter(store *Store) *SessionWriter {
	return &SessionWriter{store: store}
}

// WriteSession atomically writes all session data to a temp dir then renames.
func (w *SessionWriter) WriteSession(ctx context.Context, req SessionWriteRequest) error {
	if err := ctx.Err(); err != nil {
		return &WriteError{Op: "context", Path: "", Err: err}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	sessionDir := w.store.sessionDir(req.Session.ID)
	tmpDir := sessionDir + ".tmp"

	// Cleanup function for failures
	cleanup := func() { os.RemoveAll(tmpDir) }

	// Create directories
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return &WriteError{Op: "mkdir_tmp", Path: tmpDir, Err: err}
	}

	// Write outcomes
	if len(req.Outcomes) > 0 {
		path := filepath.Join(tmpDir, "recommendation_outcomes.jsonl")
		if err := writeOutcomesToFile(path, req.Outcomes); err != nil {
			cleanup()
			return &WriteError{Op: "write_outcomes", Path: path, Err: err}
		}
	}

	// Write rejects
	if len(req.Rejects) > 0 {
		path := filepath.Join(tmpDir, "screened_symbols.jsonl")
		if err := writeRejectsToFile(path, req.Rejects); err != nil {
			cleanup()
			return &WriteError{Op: "write_rejects", Path: path, Err: err}
		}
	}

	// Write summary if provided
	if req.Summary != nil {
		path := filepath.Join(tmpDir, "summary.json")
		if err := writeSummaryToFile(path, req.Summary); err != nil {
			cleanup()
			return &WriteError{Op: "write_summary", Path: path, Err: err}
		}
	}

	// Atomic rename
	if err := os.Rename(tmpDir, sessionDir); err != nil {
		cleanup()
		return &WriteError{Op: "rename_tmp", Path: sessionDir, Err: err}
	}

	return nil
}

func writeOutcomesToFile(path string, outcomes []domain.RecommendationOutcome) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, o := range outcomes {
		if err := enc.Encode(o); err != nil {
			return err
		}
	}
	return nil
}

func writeRejectsToFile(path string, rejects []domain.ScreeningReject) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rejects {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func writeSummaryToFile(path string, summary *domain.SessionSummary) error {
	bytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
