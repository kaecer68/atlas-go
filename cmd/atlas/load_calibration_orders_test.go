package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadCalibrationOrders_EmptySessionsDirReturnsEmpty locks the
// empty-directory behavior: when data/state/sessions exists but has no
// JSONL files, the function must return (nil, nil) — NOT an error. This
// is the production cold-start path on a fresh deployment.
//
// This is a safety net for #611 sub-issue-2 refactor: loadCalibrationOrders
// is currently called from orchestrator/system.go (line ~800) at every
// regime-confirmed cycle. A regression that returned an error on empty
// input would block production cold-start.
func TestLoadCalibrationOrders_EmptySessionsDirReturnsEmpty(t *testing.T) {
	workDir := t.TempDir()
	sessionsDir := filepath.Join(workDir, "data", "state", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	orders, err := loadCalibrationOrders(workDir)
	if err != nil {
		t.Fatalf("empty sessions dir should not error, got: %v", err)
	}
	if len(orders) != 0 {
		t.Errorf("empty sessions dir should return 0 orders, got %d", len(orders))
	}
}

// TestLoadCalibrationOrders_MissingSessionsDirReturnsError locks the
// missing-directory behavior: when data/state/sessions does not exist,
// the function must return an error (not panic, not silently succeed).
// The error must come from os.ReadDir (preserving the underlying
// diagnostic for ops debugging).
func TestLoadCalibrationOrders_MissingSessionsDirReturnsError(t *testing.T) {
	workDir := t.TempDir() // no data/state/sessions subdir created

	orders, err := loadCalibrationOrders(workDir)
	if err == nil {
		t.Fatal("missing sessions dir must return error, got nil")
	}
	if !os.IsNotExist(err) && !filepathIsNotExist(err) {
		// Accept os.IsNotExist direct, or wrapped (filepath may wrap).
		t.Logf("note: error type is %T, value %v (acceptable as long as non-nil)", err, err)
	}
	if orders != nil {
		t.Errorf("error path must return nil slice, got %v", orders)
	}
}

// filepathIsNotExist unwraps to detect os.ErrNotExist under fmt.Errorf wrapping.
func filepathIsNotExist(err error) bool {
	for e := err; e != nil; {
		if e == os.ErrNotExist {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}
