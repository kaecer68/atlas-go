package monitoring

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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

func TestQuoteBackfillRunner_StopsWhenQuotaNearlyExhausted(t *testing.T) {
	dir := t.TempDir()
	// fundamentals.json with one PE>0 symbol.
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "fundamentals.json"),
		[]byte(`{"2330":{"PE":15}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-seed the quota state file so the client starts with 99/100 used
	// (1 remaining < backfillQuotaStopRemaining).
	stateDir := t.TempDir()
	state := map[string]any{
		"calls_today": 99,
		"last_reset":  time.Now().Truncate(24 * time.Hour),
	}
	raw, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(stateDir, "finmind_daily_quota.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	client := marketdata.NewFinMindClientWithStateDir("k", stateDir)
	client.SetQuotaLimit(100) // 99 used → 1 remaining

	runner := NewQuoteBackfillRunner(QuoteBackfillDeps{
		FinMindClient: client,
		QuoteStore:    &fakeQuoteStore{},
		WorkDir:       dir,
	})
	if err := runner(context.Background()); err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if got := client.QuotaRemaining(); got >= backfillQuotaStopRemaining {
		t.Fatalf("runner did not stop early: remaining=%d (gate floor=%d)", got, backfillQuotaStopRemaining)
	}
}

// fix/20260924-finmind-quota：保留水位必須足夠保護「配額日後半段」才啟動的
// live 消費者，但也不可把整日配額吃掉（backfill 仍要能推進）。
func TestQuoteBackfillQuotaFloor_IsMeaningfulAndBounded(t *testing.T) {
	// Use the single source of truth instead of a copied number: the ceiling
	// lived in three places as "14400" before fix/finmind-quota-honor-402-r,
	// which is exactly how the 2026-09-26 stale-limit incident happened
	// (the upstream started refusing at ~12,500 while every reserve
	// calculation still assumed 14,400).
	finmindDailyLimit := marketdata.FinMindDailyLimit()

	// 2026-09-23 實證：實測 live 消費者需求約 900–2,700 次/日，且它們在配額日
	// 後半段才執行 —— 200 的水位讓最後 200 次在 15:27Z 前就被用完。
	if backfillQuotaStopRemaining < 1000 {
		t.Fatalf("quota floor %d is too small to protect late-running live consumers (2026-09-23: 200 was exhausted by 15:27Z)",
			backfillQuotaStopRemaining)
	}
	// 不可過度保留：bulk backfill 仍需大部分配額才能推進（需求 ~44,000）。
	if maxReserve := finmindDailyLimit / 4; backfillQuotaStopRemaining > maxReserve {
		t.Fatalf("quota floor %d exceeds 25%% of the daily ceiling (%d): the backfill would stall",
			backfillQuotaStopRemaining, maxReserve)
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
