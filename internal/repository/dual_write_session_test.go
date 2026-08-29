package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// stubSessionSummaryStore is a configurable mock that lets tests control
// what LoadSessionSummaries returns and whether RecordSessionSummary fails.
// It satisfies SessionSummaryStore.
type stubSessionSummaryStore struct {
	loadedSummaries []domain.SessionSummary
	loadErr         error
	recordErr       error
}

func (s *stubSessionSummaryStore) RecordSessionSummary(_ domain.ReplaySession, summary domain.SessionSummary) error {
	return s.recordErr
}

func (s *stubSessionSummaryStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return s.loadedSummaries, s.loadErr
}

func (s *stubSessionSummaryStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}

// makeStubSessionSummaries builds n synthetic session summaries for tests.
func makeStubSessionSummaries(n int) []domain.SessionSummary {
	out := make([]domain.SessionSummary, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		out[i] = domain.SessionSummary{
			SessionID:      "sess-test-" + string(rune('A'+i)),
			Regime:         domain.RegimeRiskOn,
			OrderCount:     i,
			OutcomeCount:   i * 2,
			PortfolioValue: 1000000 + float64(i)*1000,
			RecordedAt:     base.Add(time.Duration(i) * 24 * time.Hour),
		}
	}
	return out
}

// S1 — LoadAllSessionSummaries MUST fall back to JSONL when PG is unavailable.
// REGRESSION for empty Evolution page: production server had r.pg == nil
// (no PostgreSQL env vars), buggy code returned (nil, nil), JSONL fallback
// was never invoked, evolution page rendered "system has not accumulated
// enough session data" despite 109 sessions in data/state/sessions/.
func TestDualWriteRepository_LoadAllSessionSummaries_PGNil_FallsBackToJSONL(t *testing.T) {
	want := makeStubSessionSummaries(3)
	jsonl := &JSONLRepository{
		sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: want},
	}
	repo := &DualWriteRepository{jsonl: jsonl} // pg == nil — production case

	got, err := repo.LoadAllSessionSummaries(context.Background())
	if err != nil {
		t.Fatalf("LoadAllSessionSummaries returned error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d summaries from JSONL fallback, got %d (nil=%v)", len(want), len(got), got == nil)
	}
	for i := range got {
		if got[i].SessionID != want[i].SessionID {
			t.Errorf("summary[%d].SessionID = %q, want %q", i, got[i].SessionID, want[i].SessionID)
		}
	}
}

// S1b — Every JSONL fallback in LoadAllSessionSummaries increments the
// exported atomic fallback counter.
func TestDualWriteRepository_LoadAllSessionSummaries_FallbackCounter(t *testing.T) {
	want := makeStubSessionSummaries(2)
	jsonl := &JSONLRepository{
		sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: want},
	}
	repo := &DualWriteRepository{jsonl: jsonl} // pg == nil

	before := DualWriteFallbackTotal()
	if _, err := repo.LoadAllSessionSummaries(context.Background()); err != nil {
		t.Fatalf("LoadAllSessionSummaries returned error: %v", err)
	}
	if got := DualWriteFallbackTotal(); got != before+1 {
		t.Errorf("fallback counter = %d, want %d", got, before+1)
	}

	// Second call keeps incrementing.
	if _, err := repo.LoadAllSessionSummaries(context.Background()); err != nil {
		t.Fatalf("LoadAllSessionSummaries returned error: %v", err)
	}
	if got := DualWriteFallbackTotal(); got != before+2 {
		t.Errorf("fallback counter after second call = %d, want %d", got, before+2)
	}
}

// S2 — LoadSessionSummary (singular) MUST fall back to JSONL when PG is
// unavailable. Same regression as S1; the pipeline calls both methods
// (singular for chart anchors, plural for the full history list).
func TestDualWriteRepository_LoadSessionSummary_PGNil_FallsBackToJSONL(t *testing.T) {
	want := makeStubSessionSummaries(3)
	jsonl := &JSONLRepository{
		sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: want},
	}
	repo := &DualWriteRepository{jsonl: jsonl}

	got, err := repo.LoadSessionSummary(context.Background(), "sess-test-B")
	if err != nil {
		t.Fatalf("LoadSessionSummary returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("LoadSessionSummary returned nil; expected fallback to JSONL summary sess-test-B")
	}
	if got.SessionID != "sess-test-B" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "sess-test-B")
	}
	if got.OrderCount != 1 {
		t.Errorf("OrderCount = %d, want 1 (the B-summary)", got.OrderCount)
	}
}

// Regression guard — when JSONL is also empty (cold start), must return
// empty slice with nil error, not a panic or a wrapped error.
func TestDualWriteRepository_LoadAllSessionSummaries_BothEmpty_ReturnsEmpty(t *testing.T) {
	jsonl := &JSONLRepository{
		sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: nil},
	}
	repo := &DualWriteRepository{jsonl: jsonl}

	got, err := repo.LoadAllSessionSummaries(context.Background())
	if err != nil {
		t.Fatalf("expected nil error on cold start, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 summaries on cold start, got %d", len(got))
	}
}

// T1 — REGRESSION: empty Evolution page bug, production scenario.
// PG is wired (PostgresRepository non-nil) but pool is nil (e.g..
// constructor called with nil pool, or production server had r.pg.pool
// stale-nil after pool teardown). JSONL has 109+ sessions on disk.
// Must NOT panic; must fall back to JSONL.
func TestDualWriteRepository_LoadAllSessionSummaries_PGPoolNil_FallsBackToJSONL(t *testing.T) {
	want := makeStubSessionSummaries(3)
	jsonl := &JSONLRepository{
		sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: want},
	}
	// r.pg is non-nil but r.pg.pool is nil — simulates production nil-pool edge case
	repo := &DualWriteRepository{
		pg:    &PostgresRepository{pool: nil},
		jsonl: jsonl,
	}

	// Must NOT panic at r.pg.pool.Query deref
	got, err := repo.LoadAllSessionSummaries(context.Background())
	if err != nil {
		t.Fatalf("LoadAllSessionSummaries returned error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d summaries from JSONL fallback, got %d (nil=%v)", len(want), len(got), got == nil)
	}
	for i := range got {
		if got[i].SessionID != want[i].SessionID {
			t.Errorf("summary[%d].SessionID = %q, want %q", i, got[i].SessionID, want[i].SessionID)
		}
	}
}

// ============================================
// B6 — session summary dual-write consistency
// ============================================

// pgUsableFakePool returns a fakePGPool whose SELECT 1 probe succeeds so that
// pgUsable() reports PostgreSQL healthy (the probe is TTL-cached per repo).
func pgUsableFakePool() *fakePGPool {
	return &fakePGPool{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "SELECT 1") {
				return fakeRow{values: []any{1}}
			}
			return fakeRow{}
		},
	}
}

// B6-1 — RecordSessionSummary with JSONL failing while PG is unavailable
// (nothing persisted anywhere) MUST NOT be silent: the error is returned (so
// recordSummaryWithRetry can retry) and the JSONL-error / reconcile-pending
// counters increment (WARN is logged).
func TestDualWriteRepository_RecordSessionSummary_JSONLError_NotSilent(t *testing.T) {
	beforeJSONL := SessionSummaryJSONLErrorTotal()
	beforePending := SessionSummaryReconcilePendingTotal()
	beforePG := SessionSummaryPGErrorTotal()

	repo := &DualWriteRepository{
		jsonl: &JSONLRepository{sessionSummaryStore: &stubSessionSummaryStore{recordErr: errors.New("disk full")}},
	} // pg == nil

	err := repo.RecordSessionSummary(context.Background(), domain.ReplaySession{ID: "sess-jsonl-err"}, validTestSummary("sess-jsonl-err"))
	if err == nil {
		t.Fatal("JSONL-only total write failure must return an error (no silent loss)")
	}
	if got := SessionSummaryJSONLErrorTotal(); got != beforeJSONL+1 {
		t.Errorf("JSONL error counter = %d, want %d", got, beforeJSONL+1)
	}
	if got := SessionSummaryReconcilePendingTotal(); got != beforePending+1 {
		t.Errorf("reconcile-pending counter = %d, want %d", got, beforePending+1)
	}
	if got := SessionSummaryPGErrorTotal(); got != beforePG {
		t.Errorf("PG error counter = %d, want unchanged %d", got, beforePG)
	}
}

// B6-2 — JSONL write fails but PG write succeeds: still nil error, JSONL side
// counted, PG write MUST still be attempted (conditional write preserved).
func TestDualWriteRepository_RecordSessionSummary_JSONLError_PGStillWrites(t *testing.T) {
	beforeJSONL := SessionSummaryJSONLErrorTotal()
	beforePending := SessionSummaryReconcilePendingTotal()
	beforePG := SessionSummaryPGErrorTotal()

	var pgWrites int
	pool := pgUsableFakePool()
	pool.execFunc = func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
		if strings.Contains(sql, "INSERT INTO session_summaries") {
			pgWrites++
		}
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	}
	repo := &DualWriteRepository{
		pg:    &PostgresRepository{pool: pool},
		jsonl: &JSONLRepository{sessionSummaryStore: &stubSessionSummaryStore{recordErr: errors.New("disk full")}},
	}

	err := repo.RecordSessionSummary(context.Background(), domain.ReplaySession{ID: "sess-jsonl-err"}, validTestSummary("sess-jsonl-err"))
	if err != nil {
		t.Fatalf("JSONL failure must not abort the PG write: %v", err)
	}
	if pgWrites != 1 {
		t.Errorf("PG SaveSessionSummary calls = %d, want 1", pgWrites)
	}
	if got := SessionSummaryJSONLErrorTotal(); got != beforeJSONL+1 {
		t.Errorf("JSONL error counter = %d, want %d", got, beforeJSONL+1)
	}
	if got := SessionSummaryReconcilePendingTotal(); got != beforePending+1 {
		t.Errorf("reconcile-pending counter = %d, want %d", got, beforePending+1)
	}
	if got := SessionSummaryPGErrorTotal(); got != beforePG {
		t.Errorf("PG error counter = %d, want unchanged %d", got, beforePG)
	}
}

// B6-3 — PG write fails while JSONL succeeded: error MUST be returned (unchanged
// contract) and both the PG-error and reconcile-pending counters increment.
func TestDualWriteRepository_RecordSessionSummary_PGError_ReturnsError(t *testing.T) {
	beforeJSONL := SessionSummaryJSONLErrorTotal()
	beforePending := SessionSummaryReconcilePendingTotal()
	beforePG := SessionSummaryPGErrorTotal()

	pool := pgUsableFakePool()
	pool.execFunc = func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, errors.New("connection refused")
	}
	repo := &DualWriteRepository{
		pg:    &PostgresRepository{pool: pool},
		jsonl: &JSONLRepository{sessionSummaryStore: &stubSessionSummaryStore{}}, // JSONL ok
	}

	err := repo.RecordSessionSummary(context.Background(), domain.ReplaySession{ID: "sess-pg-err"}, validTestSummary("sess-pg-err"))
	if err == nil {
		t.Fatal("PG write failure must return an error")
	}
	if got := SessionSummaryPGErrorTotal(); got != beforePG+1 {
		t.Errorf("PG error counter = %d, want %d", got, beforePG+1)
	}
	if got := SessionSummaryReconcilePendingTotal(); got != beforePending+1 {
		t.Errorf("reconcile-pending counter = %d, want %d", got, beforePending+1)
	}
	if got := SessionSummaryJSONLErrorTotal(); got != beforeJSONL {
		t.Errorf("JSONL error counter = %d, want unchanged %d", got, beforeJSONL)
	}
}

// B6-4 — both sides succeed: no error and no counter movement.
func TestDualWriteRepository_RecordSessionSummary_BothSucceed(t *testing.T) {
	beforeJSONL := SessionSummaryJSONLErrorTotal()
	beforePending := SessionSummaryReconcilePendingTotal()
	beforePG := SessionSummaryPGErrorTotal()

	pool := pgUsableFakePool() // default Exec succeeds
	repo := &DualWriteRepository{
		pg:    &PostgresRepository{pool: pool},
		jsonl: &JSONLRepository{sessionSummaryStore: &stubSessionSummaryStore{}},
	}

	err := repo.RecordSessionSummary(context.Background(), domain.ReplaySession{ID: "sess-ok"}, validTestSummary("sess-ok"))
	if err != nil {
		t.Fatalf("dual success returned error: %v", err)
	}
	if got := SessionSummaryJSONLErrorTotal(); got != beforeJSONL {
		t.Errorf("JSONL error counter moved: %d -> %d", beforeJSONL, got)
	}
	if got := SessionSummaryPGErrorTotal(); got != beforePG {
		t.Errorf("PG error counter moved: %d -> %d", beforePG, got)
	}
	if got := SessionSummaryReconcilePendingTotal(); got != beforePending {
		t.Errorf("reconcile-pending counter moved: %d -> %d", beforePending, got)
	}
}

// B6-5 — SaveSessionSummary (direct backfill entry point) shares the same
// non-silent JSONL error handling: a total write failure (JSONL fails, PG
// unavailable) returns an error and increments the counters.
func TestDualWriteRepository_SaveSessionSummary_JSONLError_NotSilent(t *testing.T) {
	beforeJSONL := SessionSummaryJSONLErrorTotal()
	beforePending := SessionSummaryReconcilePendingTotal()

	repo := &DualWriteRepository{
		jsonl: &JSONLRepository{sessionSummaryStore: &stubSessionSummaryStore{recordErr: errors.New("disk full")}},
	}

	err := repo.SaveSessionSummary(context.Background(), domain.SessionSummary{SessionID: "sess-save-jsonl-err"})
	if err == nil {
		t.Fatal("JSONL-only total write failure must return an error (no silent loss)")
	}
	if got := SessionSummaryJSONLErrorTotal(); got != beforeJSONL+1 {
		t.Errorf("JSONL error counter = %d, want %d", got, beforeJSONL+1)
	}
	if got := SessionSummaryReconcilePendingTotal(); got != beforePending+1 {
		t.Errorf("reconcile-pending counter = %d, want %d", got, beforePending+1)
	}
}

// ============================================
// B6 — LoadAllSessionSummaries merge semantics
// ============================================

// validTestSummary returns a session summary that satisfies the strict SSoT
// write validation (SessionSummary.Validate) so tests can focus on the
// error-handling path under test rather than validation itself.
func validTestSummary(id string) domain.SessionSummary {
	return domain.SessionSummary{
		SessionID:      id,
		Regime:         domain.RegimeRiskOn,
		EndingCash:     100_000,
		PortfolioValue: 1_000_000,
		OutcomeCount:   1,
		RecordedAt:     time.Now(),
	}
}

func sessionSummary(id string, at time.Time, pv float64) domain.SessionSummary {
	return domain.SessionSummary{
		SessionID:      id,
		Regime:         domain.RegimeRiskOn,
		OutcomeCount:   3,
		PortfolioValue: pv,
		RecordedAt:     at,
	}
}

func summaryIDs(ss []domain.SessionSummary) []string {
	out := make([]string, len(ss))
	for i := range ss {
		out[i] = ss[i].SessionID
	}
	return out
}

func containsAll(got []string, want ...string) bool {
	m := make(map[string]bool, len(got))
	for _, g := range got {
		m[g] = true
	}
	for _, w := range want {
		if !m[w] {
			return false
		}
	}
	return true
}

// B6-M1 — disjoint sides (the audited split: PG has older days, JSONL has
// newer days) must merge into the union with no losses.
func TestMergeSessionSummaries_DisjointUnion(t *testing.T) {
	t0 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	pg := []domain.SessionSummary{
		sessionSummary("s-0702", t0, 100),
		sessionSummary("s-0703", t0.Add(24*time.Hour), 101),
	}
	jsonl := []domain.SessionSummary{
		sessionSummary("s-0724", t0.Add(22*24*time.Hour), 200),
		sessionSummary("s-0725", t0.Add(23*24*time.Hour), 201),
	}

	merged, onlyJSONL, onlyPG, diverged := mergeSessionSummaries(pg, jsonl)
	if len(merged) != 4 {
		t.Fatalf("merged len = %d, want 4 (union)", len(merged))
	}
	ids := summaryIDs(merged)
	if !containsAll(ids, "s-0702", "s-0703", "s-0724", "s-0725") {
		t.Errorf("merged IDs = %v, want all four", ids)
	}
	// Newest-first order (PG contract).
	if merged[0].SessionID != "s-0725" || merged[3].SessionID != "s-0702" {
		t.Errorf("merged not newest-first: %v", ids)
	}
	if onlyJSONL != 2 || onlyPG != 2 || diverged != 0 {
		t.Errorf("counters = (onlyJSONL=%d onlyPG=%d diverged=%d), want (2,2,0)", onlyJSONL, onlyPG, diverged)
	}
}

// B6-M2 — same session on both sides: the newer RecordedAt wins.
func TestMergeSessionSummaries_NewerWins(t *testing.T) {
	t0 := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	pg := []domain.SessionSummary{sessionSummary("s-x", t0, 100)}                   // older
	jsonl := []domain.SessionSummary{sessionSummary("s-x", t0.Add(time.Hour), 999)} // newer

	merged, _, _, diverged := mergeSessionSummaries(pg, jsonl)
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1", len(merged))
	}
	if merged[0].PortfolioValue != 999 {
		t.Errorf("newer JSONL copy not used: PortfolioValue = %v, want 999", merged[0].PortfolioValue)
	}
	if diverged != 1 {
		t.Errorf("diverged = %d, want 1", diverged)
	}

	// Reverse: PG newer -> PG copy kept.
	pg2 := []domain.SessionSummary{sessionSummary("s-x", t0.Add(2*time.Hour), 1111)}
	merged2, _, _, diverged2 := mergeSessionSummaries(pg2, jsonl)
	if len(merged2) != 1 || merged2[0].PortfolioValue != 1111 {
		t.Errorf("PG-newer copy not kept: %+v", merged2)
	}
	if diverged2 != 1 {
		t.Errorf("diverged (reverse) = %d, want 1", diverged2)
	}
}

// B6-M3 — same timestamp but different content: PG wins the tiebreak.
func TestMergeSessionSummaries_TieKeepsPG(t *testing.T) {
	t0 := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	pg := []domain.SessionSummary{sessionSummary("s-x", t0, 100)}
	jsonl := []domain.SessionSummary{sessionSummary("s-x", t0, 200)} // same ts, different pv

	merged, _, _, diverged := mergeSessionSummaries(pg, jsonl)
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1", len(merged))
	}
	if merged[0].PortfolioValue != 100 {
		t.Errorf("tie must keep PG copy, got PortfolioValue = %v", merged[0].PortfolioValue)
	}
	if diverged != 1 {
		t.Errorf("diverged = %d, want 1 (content drift detected)", diverged)
	}
}

// B6-M4 — identical copies on both sides: no divergence counted.
func TestMergeSessionSummaries_IdenticalNoDivergence(t *testing.T) {
	t0 := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	s := sessionSummary("s-x", t0, 100)
	merged, _, _, diverged := mergeSessionSummaries([]domain.SessionSummary{s}, []domain.SessionSummary{s})
	if len(merged) != 1 || diverged != 0 {
		t.Errorf("identical copies: merged len = %d, diverged = %d, want (1, 0)", len(merged), diverged)
	}
}

// B6-M5 — full read path: PG usable + non-empty merges with JSONL instead of
// returning PG only (the B6 regression fix for the audited split).
func TestDualWriteRepository_LoadAllSessionSummaries_MergesBothSides(t *testing.T) {
	beforeDiverged := SessionSummaryMergeDivergenceTotal()
	beforeJSONLOnly := SessionSummaryJSONLOnlyTotal()
	beforePGOnly := SessionSummaryPGOnlyTotal()

	t0 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	pgRow := func(id string, at time.Time, pv float64) []any {
		return []any{at, id, "RISK_ON", 1, 1, 900000.0, pv, 3, []byte(`{}`), "", "", "", "", []byte(`[]`), "", []byte(`[]`), nil, nil, ""}
	}
	pool := pgUsableFakePool()
	pool.queryFunc = func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
		if strings.Contains(sql, "FROM session_summaries") {
			return &fakeRows{rows: [][]any{
				pgRow("s-0702", t0, 100),
				pgRow("s-0703", t0.Add(24*time.Hour), 101),
			}}, nil
		}
		return &fakeRows{}, nil
	}
	jsonl := []domain.SessionSummary{
		sessionSummary("s-0724", t0.Add(22*24*time.Hour), 200),
		sessionSummary("s-0725", t0.Add(23*24*time.Hour), 201),
	}
	repo := &DualWriteRepository{
		pg:    &PostgresRepository{pool: pool},
		jsonl: &JSONLRepository{sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: jsonl}},
	}

	got, err := repo.LoadAllSessionSummaries(context.Background())
	if err != nil {
		t.Fatalf("LoadAllSessionSummaries returned error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("merged len = %d, want 4 (PG 2 + JSONL 2)", len(got))
	}
	ids := summaryIDs(got)
	if !containsAll(ids, "s-0702", "s-0703", "s-0724", "s-0725") {
		t.Errorf("merged IDs = %v, want all four", ids)
	}
	if got[0].SessionID != "s-0725" {
		t.Errorf("merged[0] = %q, want newest s-0725", got[0].SessionID)
	}
	if v := SessionSummaryJSONLOnlyTotal(); v != beforeJSONLOnly+2 {
		t.Errorf("JSONL-only counter = %d, want %d", v, beforeJSONLOnly+2)
	}
	if v := SessionSummaryPGOnlyTotal(); v != beforePGOnly+2 {
		t.Errorf("PG-only counter = %d, want %d", v, beforePGOnly+2)
	}
	if v := SessionSummaryMergeDivergenceTotal(); v != beforeDiverged {
		t.Errorf("divergence counter = %d, want unchanged %d", v, beforeDiverged)
	}
}

// B6-M6 — full read path: same session on both sides with newer JSONL copy is
// served as the newer version and counted as divergence.
func TestDualWriteRepository_LoadAllSessionSummaries_NewerJSONLWins(t *testing.T) {
	beforeDiverged := SessionSummaryMergeDivergenceTotal()

	t0 := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	pool := pgUsableFakePool()
	pool.queryFunc = func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
		if strings.Contains(sql, "FROM session_summaries") {
			return &fakeRows{rows: [][]any{
				{t0, "s-x", "RISK_ON", 1, 1, 900000.0, 100.0, 3, []byte(`{}`), "", "", "", "", []byte(`[]`), "", []byte(`[]`), nil, nil, ""},
			}}, nil
		}
		return &fakeRows{}, nil
	}
	repo := &DualWriteRepository{
		pg: &PostgresRepository{pool: pool},
		jsonl: &JSONLRepository{sessionSummaryStore: &stubSessionSummaryStore{
			loadedSummaries: []domain.SessionSummary{sessionSummary("s-x", t0.Add(time.Hour), 999)},
		}},
	}

	got, err := repo.LoadAllSessionSummaries(context.Background())
	if err != nil {
		t.Fatalf("LoadAllSessionSummaries returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("merged len = %d, want 1", len(got))
	}
	if got[0].PortfolioValue != 999 {
		t.Errorf("PortfolioValue = %v, want 999 (newer JSONL copy)", got[0].PortfolioValue)
	}
	if v := SessionSummaryMergeDivergenceTotal(); v != beforeDiverged+1 {
		t.Errorf("divergence counter = %d, want %d", v, beforeDiverged+1)
	}
}
