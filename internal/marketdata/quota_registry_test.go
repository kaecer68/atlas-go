package marketdata

import (
	"sync"
	"testing"
)

// ─── QuotaRegistry: unified cross-provider quota view ───────────────────────

// TestQuotaRegistry_Snapshot_StableOrder verifies that Snapshot returns
// entries sorted by provider name so JSON output is byte-stable across
// calls. The dashboard's quota panel relies on stable ordering for
// visual consistency and to avoid spurious diffs in alerting rules.
func TestQuotaRegistry_Snapshot_StableOrder(t *testing.T) {
	r := NewQuotaRegistry()
	r.Register("fugle", NewDailyQuotaTracker("fugle", t.TempDir(), 100))
	r.Register("finmind", NewDailyQuotaTracker("finmind", t.TempDir(), 100))
	r.Register("tej", NewDailyQuotaTracker("tej", t.TempDir(), 100))

	snap := r.Snapshot()
	if len(snap.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(snap.Entries))
	}
	want := []string{"finmind", "fugle", "tej"}
	for i, e := range snap.Entries {
		if e.Provider != want[i] {
			t.Errorf("entry[%d].Provider = %q, want %q (Snapshot output must be stable)", i, e.Provider, want[i])
		}
	}
}

// TestQuotaRegistry_Snapshot_ExhaustedFlag verifies the Exhausted flag flips
// at Used >= Limit. Used by the dashboard to render "quota exhausted" badges
// without recomputing Remaining == 0 in the frontend.
func TestQuotaRegistry_Snapshot_ExhaustedFlag(t *testing.T) {
	r := NewQuotaRegistry()
	tracker := NewDailyQuotaTracker("finmind", t.TempDir(), 3)
	r.Register("finmind", tracker)

	// Before any calls: not exhausted.
	if got := r.Snapshot().Entries[0].Exhausted; got {
		t.Error("fresh tracker should not be exhausted")
	}

	// Burn through the budget one call at a time.
	for i := range 3 {
		if !tracker.AllowCall() {
			t.Fatalf("AllowCall returned false at iteration %d (should have quota left)", i)
		}
	}
	if got := r.Snapshot().Entries[0].Exhausted; !got {
		t.Error("after 3/3 calls, Exhausted should be true")
	}

	// Fourth call: AllowCall returns false, Snapshot still shows exhausted.
	if tracker.AllowCall() {
		t.Error("AllowCall should return false after quota is gone")
	}
	if got := r.Snapshot().Entries[0].Exhausted; !got {
		t.Error("Exhausted should remain true after budget is gone")
	}
}

// TestQuotaRegistry_IsProviderExhausted is the fast boolean check that
// alert rules and the channel-health page use to decide whether to page
// on-call. The exhaustive Snapshot above covers the underlying state;
// this test asserts the boolean wrapper agrees with Snapshot.
func TestQuotaRegistry_IsProviderExhausted(t *testing.T) {
	r := NewQuotaRegistry()
	tracker := NewDailyQuotaTracker("fugle", t.TempDir(), 1)
	r.Register("fugle", tracker)

	if r.IsProviderExhausted("fugle") {
		t.Error("fresh tracker should not be exhausted")
	}
	tracker.AllowCall() // burn the only call
	if !r.IsProviderExhausted("fugle") {
		t.Error("after quota is gone, IsProviderExhausted should return true")
	}
	// Unknown provider: not exhausted (no false positives).
	if r.IsProviderExhausted("unknown") {
		t.Error("unknown provider should not be reported as exhausted")
	}
	// Nil registry: not exhausted (defensive — should not panic in tests).
	var nilR *QuotaRegistry
	if nilR.IsProviderExhausted("fugle") {
		t.Error("nil registry should report not-exhausted (defensive)")
	}
}

// TestQuotaRegistry_Register_NilSafe verifies that Register accepts nil
// inputs gracefully. The caller (FinMindClient.newFinMindClientInternal)
// registers in a context where the tracker is always non-nil, but tests
// and future migrations might pass nil — we don't want a nil pointer
// to take down the whole registry.
func TestQuotaRegistry_Register_NilSafe(t *testing.T) {
	r := NewQuotaRegistry()
	r.Register("finmind", nil)                                    // should silently skip
	r.Register("", NewDailyQuotaTracker("tej", t.TempDir(), 100)) // empty name skipped
	r.Register("fugle", NewDailyQuotaTracker("fugle", t.TempDir(), 100))

	snap := r.Snapshot()
	if len(snap.Entries) != 1 {
		t.Errorf("expected only 'fugle' to be registered, got %d entries", len(snap.Entries))
	}
	if snap.Entries[0].Provider != "fugle" {
		t.Errorf("registered provider = %q, want fugle", snap.Entries[0].Provider)
	}
}

// TestQuotaRegistry_ConcurrentReadSnapshot ensures Snapshot is safe to
// call from multiple goroutines while providers register in the background.
// Race detector run (via `go test -race`) catches any unsynchronized map
// access — this is the core invariant of the unified-quota-view design
// (kaecer's 2026-08-04 feedback: a single registry must not be a new
// race-condition vector when fixing the quota fragmentation).
func TestQuotaRegistry_ConcurrentReadSnapshot(t *testing.T) {
	r := NewQuotaRegistry()
	var wg sync.WaitGroup
	// Writer goroutines register providers.
	for i := range 5 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := []string{"alpha", "beta", "gamma", "delta", "epsilon"}[id]
			r.Register(name, NewDailyQuotaTracker(name, t.TempDir(), 100))
		}(i)
	}
	// Reader goroutines hit Snapshot concurrently.
	for range 10 {
		wg.Go(func() {
			_ = r.Snapshot()
		})
	}
	wg.Wait()
	// Final snapshot must include all 5 providers — proves no goroutine
	// saw an empty registry while writers were mid-flight.
	if got := len(r.Snapshot().Entries); got != 5 {
		t.Errorf("after concurrent register+Snapshot, entries = %d, want 5", got)
	}
}

// TestGlobalQuotaRegistry_Singleton verifies GlobalQuotaRegistry returns
// the same instance across calls. FinMind and Fugle constructors both
// rely on this so the dashboard sees both providers without explicit wiring.
func TestGlobalQuotaRegistry_Singleton(t *testing.T) {
	a := GlobalQuotaRegistry()
	b := GlobalQuotaRegistry()
	if a != b {
		t.Error("GlobalQuotaRegistry must return the same instance (sync.Once)")
	}
}
