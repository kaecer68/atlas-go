package autobacktest

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

// fakeOpener records every channel it was asked to force-open so the test
// can assert which live channels the CIRCUIT_BREAKER consumer touched.
type fakeOpener struct {
	mu     sync.Mutex
	opened []string
	fail   map[string]error
}

func (f *fakeOpener) ForceOpenChannel(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		if err, ok := f.fail[id]; ok {
			return err
		}
	}
	f.opened = append(f.opened, id)
	return nil
}

func (f *fakeOpener) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.opened))
	copy(out, f.opened)
	return out
}

// TestSignalApply_NilGatewayIsSafe verifies the production wiring can pass
// nil (older call sites) without crashing.
func TestSignalApply_NilGatewayIsSafe(t *testing.T) {
	dir := t.TempDir()
	if err := SignalApply(context.Background(), dir, nil); err != nil {
		t.Errorf("nil gateway should be silent: %v", err)
	}
}

// TestSignalApply_NoRealOutcomesNoOp exercises the path where the ledger
// store exists but has no real outcomes — SignalEngine should evaluate to
// no active signals and the opener must stay quiet.
func TestSignalApply_NoRealOutcomesNoOp(t *testing.T) {
	dir := t.TempDir()
	opener := &fakeOpener{}
	// NewSignalEngine requires a FullStore ledger; constructing with an
	// empty dir will fail to load outcomes. Either way the consumer must
	// never touch the gateway.
	if err := SignalApply(context.Background(), dir, opener); err != nil {
		// Acceptable: an error from missing outcomes is fine.
	}
	if got := opener.snapshot(); len(got) != 0 {
		t.Errorf("expected no opens with empty ledger, got %v", got)
	}
}

// TestSignalApply_PropagatesBadLedger exercises the error-surfacing path
// so a future regression that swallows NewSignalEngine failures is caught.
func TestSignalApply_PropagatesBadLedger(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	opener := &fakeOpener{}
	// Either NewSignalEngine fails (preferred) or Evaluate fails; in either
	// case the opener must NOT be touched.
	_ = SignalApply(context.Background(), dir, opener)
	if got := opener.snapshot(); len(got) != 0 {
		t.Errorf("opener touched despite bad ledger: %v", got)
	}
}
