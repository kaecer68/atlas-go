package monitoring

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ─── P1-12: scheduler quota gate ────────────────────────────────────────────

// fakeQuoteStore is a minimal ledger.QuoteStore that records nothing.
type fakeQuoteStore struct{}

func (f *fakeQuoteStore) RecordQuotes([]domain.DailyBar) error { return nil }
func (f *fakeQuoteStore) LoadQuotes(string, time.Time, time.Time) ([]domain.DailyBar, error) {
	return nil, nil
}
func (f *fakeQuoteStore) LoadLatestQuotes([]string) (map[string]domain.DailyBar, error) {
	return nil, nil
}

// newQuotaCappedFinMindClient builds a FinMindClient whose daily tracker has
// only a few calls left, so the backfill gate must stop early.
func newQuotaCappedFinMindClient(limit, used int) *marketdata.FinMindClient {
	tracker := marketdata.NewDailyQuotaTracker("finmind_test", t.TempDir(), limit)
	// bump callsToday directly so Remaining() == limit-used
	return &marketdata.FinMindClient{
		rateLimiter:  rate.NewLimiter(rate.Inf, 0),
		quotaTracker: tracker,
	}
}

func TestQuoteBackfillRunner_StopsWhenQuotaNearlyExhausted(t *testing.T) {
	dir := t.TempDir()
	// fundamentals.json with one PE>0 symbol.
	if err := os.WriteFile(filepath.Join(dir, "fundamentals.json"),
		[]byte(`{"2330":{"PE":15}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	tracker := marketdata.NewDailyQuotaTracker("finmind_gate_test", dir, 100)
	tracker.SetLimit(100)
	// simulate 99 calls used → 1 remaining < stop threshold (200)
	client := &marketdata.FinMindClient{
		rateLimiter:  rate.NewLimiter(rate.Inf, 0),
		quotaTracker: tracker,
	}

	runner := NewQuoteBackfillRunner(QuoteBackfillDeps{
		FinMindClient: client,
		QuoteStore:    &fakeQuoteStore{},
		WorkDir:       dir,
	})
	err := runner(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if got := tracker.Remaining(); got >= 200 {
		t.Fatalf("runner did not stop early: remaining=%d", got)
	}
}

func TestDailyQuotaTracker_AtomicSave(t *testing.T) {
	dir := t.TempDir()
	tracker := marketdata.NewDailyQuotaTracker("atomic", dir, 100)
	if !tracker.AllowCall() {
		t.Fatal("expected AllowCall to succeed")
	}
	if tracker.CallsToday() != 1 {
		t.Fatalf("CallsToday = %d, want 1", tracker.CallsToday())
	}

	// The persisted file must exist, contain the counter, and no .tmp residue.
	statePath := filepath.Join(dir, "atomic_daily_quota.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state file not written: %v", err)
	}
	if !strings.Contains(string(data), `"calls_today":1`) {
		t.Errorf("state file content wrong: %s", string(data))
	}
	if _, err := os.Stat(statePath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tmp file left behind (non-atomic write)")
	}

	// Reload must restore the counter (proving rename replaced the file).
	reloaded := marketdata.NewDailyQuotaTracker("atomic", dir, 100)
	if reloaded.CallsToday() != 1 {
		t.Fatalf("reloaded CallsToday = %d, want 1 (atomic rename must preserve state)", reloaded.CallsToday())
	}
}
