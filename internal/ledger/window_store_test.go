package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// Q2 batch 2, T2 — the window family (window summary + mutation brief).
//
// Before this unit the family was file-only on BOTH sides (the JSONL *Store
// wrote <LedgerDir>/windows/*.json and every reader scanned that directory), so
// a PostgreSQL deployment wrote its windows where no reader looked. These tests
// pin the store half of the fix; the reader seams are pinned in
// internal/monitoring/service (report + system health) and the live PostgreSQL
// half in window_store_pg_integration_test.go.

func windowFixture(id string, generatedAt time.Time) domain.BacktestWindowSummary {
	return domain.BacktestWindowSummary{
		WindowID:        id,
		StartDate:       generatedAt.AddDate(0, 0, -20),
		EndDate:         generatedAt,
		SessionCount:    3,
		OutcomeCount:    7,
		GeneratedAt:     generatedAt,
		WorstAgentID:    "agent-" + id,
		WorstAgentSkill: "skill-" + id,
	}
}

func TestJSONLWindowStore_RoundTripAndBriefSkip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	first := windowFixture("window-20260101-20260121", time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC))
	second := windowFixture("window-20260122-20260211", time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC))
	for _, summary := range []domain.BacktestWindowSummary{first, second} {
		if err := store.RecordWindowSummary(summary); err != nil {
			t.Fatalf("RecordWindowSummary(%s): %v", summary.WindowID, err)
		}
	}
	// The mutation-brief half lives in the same directory and must never be
	// unmarshalled as a window summary (the pre-migration reader filtered it by
	// filename suffix).
	if err := store.RecordMutationBrief(first.WindowID, domain.MutationBrief{
		WindowID:      first.WindowID,
		TargetAgentID: "agent-1",
		MutationType:  "prompt_tightening",
		GeneratedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("RecordMutationBrief: %v", err)
	}

	summaries, err := store.LoadWindowSummaries()
	if err != nil {
		t.Fatalf("LoadWindowSummaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("LoadWindowSummaries returned %d summaries, want 2 (brief file must be skipped)", len(summaries))
	}
	if summaries[0].WindowID != first.WindowID || summaries[1].WindowID != second.WindowID {
		t.Errorf("window ids = %s, %s; want %s, %s (directory order)",
			summaries[0].WindowID, summaries[1].WindowID, first.WindowID, second.WindowID)
	}
	if summaries[0].WorstAgentID != first.WorstAgentID || summaries[1].SessionCount != second.SessionCount {
		t.Errorf("payload mismatch: got %+v / %+v", summaries[0], summaries[1])
	}

	got, found, err := store.LoadWindowSummary(first.WindowID)
	if err != nil || !found {
		t.Fatalf("LoadWindowSummary(%s) = found %v, err %v; want found true", first.WindowID, found, err)
	}
	if got.WindowID != first.WindowID || got.OutcomeCount != first.OutcomeCount {
		t.Errorf("LoadWindowSummary payload = %+v, want %+v", got, first)
	}
}

func TestJSONLWindowStore_MissingDirAndWindow(t *testing.T) {
	store := NewStore(t.TempDir()).(*Store)

	summaries, err := store.LoadWindowSummaries()
	if err != nil {
		t.Fatalf("LoadWindowSummaries on a fresh ledger: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("LoadWindowSummaries on a fresh ledger = %d summaries, want 0", len(summaries))
	}

	if _, found, err := store.LoadWindowSummary("window-20260101-20260121"); err != nil || found {
		t.Fatalf("LoadWindowSummary(absent) = found %v, err %v; want found false, nil error", found, err)
	}
	// A caller-supplied id must never escape the window directory.
	for _, id := range []string{"", "../secrets", "windows/../../etc/passwd", `a\b`} {
		if _, found, err := store.LoadWindowSummary(id); err != nil || found {
			t.Errorf("LoadWindowSummary(%q) = found %v, err %v; want found false, nil error", id, found, err)
		}
	}
}

func TestJSONLWindowStore_CorruptedWindowIsSkipped(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	good := windowFixture("window-20260101-20260121", time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC))
	if err := store.RecordWindowSummary(good); err != nil {
		t.Fatalf("RecordWindowSummary: %v", err)
	}
	// A truncated write (process killed mid-rename is possible on old data):
	// one bad file must not blank the whole report.
	if err := os.WriteFile(filepath.Join(dir, "windows", "window-broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupted window: %v", err)
	}

	summaries, err := store.LoadWindowSummaries()
	if err != nil {
		t.Fatalf("LoadWindowSummaries with one corrupted file: %v", err)
	}
	if len(summaries) != 1 || summaries[0].WindowID != good.WindowID {
		t.Fatalf("LoadWindowSummaries = %+v, want exactly the readable window %s", summaries, good.WindowID)
	}
}

func TestSQLiteWindowStore_RoundTripAndUpsert(t *testing.T) {
	store, err := newSQLiteFullStore(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("newSQLiteFullStore: %v", err)
	}

	summary := windowFixture("window-20260101-20260121", time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC))
	if err := store.RecordWindowSummary(summary); err != nil {
		t.Fatalf("RecordWindowSummary: %v", err)
	}
	// The JSONL store overwrites <windowID>.json; the relational stores carry a
	// UNIQUE window_id, so re-writing one window must refresh, not fail.
	summary.SessionCount = 4
	if err := store.RecordWindowSummary(summary); err != nil {
		t.Fatalf("RecordWindowSummary (re-run of the same window): %v", err)
	}

	summaries, err := store.LoadWindowSummaries()
	if err != nil {
		t.Fatalf("LoadWindowSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("LoadWindowSummaries = %d rows, want 1 (upsert, not duplicate)", len(summaries))
	}
	if summaries[0].SessionCount != 4 {
		t.Errorf("SessionCount = %d, want 4 (the re-run must win)", summaries[0].SessionCount)
	}

	got, found, err := store.LoadWindowSummary(summary.WindowID)
	if err != nil || !found || got.SessionCount != 4 {
		t.Fatalf("LoadWindowSummary = %+v (found %v, err %v), want the re-written row", got, found, err)
	}
	if _, found, err := store.LoadWindowSummary("window-absent"); err != nil || found {
		t.Fatalf("LoadWindowSummary(absent) = found %v, err %v; want found false, nil error", found, err)
	}
}

// stubWindowStore is an OutcomeStore that also carries the window surface, so
// the PG-first wrapper's window methods can be exercised without PostgreSQL.
type stubWindowStore struct {
	stubOutcomeStore
	summaries    []domain.BacktestWindowSummary
	summariesErr error
	singleErr    error
	written      []domain.BacktestWindowSummary
	briefs       []string
	writeErr     error
}

func (s *stubWindowStore) LoadWindowSummaries() ([]domain.BacktestWindowSummary, error) {
	return s.summaries, s.summariesErr
}

func (s *stubWindowStore) LoadWindowSummary(windowID string) (domain.BacktestWindowSummary, bool, error) {
	if s.singleErr != nil {
		return domain.BacktestWindowSummary{}, false, s.singleErr
	}
	for _, summary := range s.summaries {
		if summary.WindowID == windowID {
			return summary, true, nil
		}
	}
	return domain.BacktestWindowSummary{}, false, nil
}

func (s *stubWindowStore) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.written = append(s.written, summary)
	return nil
}

func (s *stubWindowStore) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.briefs = append(s.briefs, windowID+"|"+brief.MutationType)
	return nil
}

var _ WindowStore = (*stubWindowStore)(nil)

func TestPGFirstWindowStore_PGAnswersAndWrites(t *testing.T) {
	pg := &stubWindowStore{summaries: []domain.BacktestWindowSummary{windowFixture("window-pg", time.Now())}}
	jsonl := &stubWindowStore{summaries: []domain.BacktestWindowSummary{windowFixture("window-jsonl", time.Now())}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	summaries, err := store.LoadWindowSummaries()
	if err != nil {
		t.Fatalf("LoadWindowSummaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].WindowID != "window-pg" {
		t.Fatalf("LoadWindowSummaries = %+v, want the postgres row only", summaries)
	}
	if store.Degraded() {
		t.Error("Degraded() = true after a successful postgres read")
	}
	if store.SourceBackend() != "postgres" {
		t.Errorf("SourceBackend() = %q, want postgres", store.SourceBackend())
	}

	// A window that postgres answers "absent" for must NOT be masked by a
	// pre-migration JSONL file.
	if _, found, err := store.LoadWindowSummary("window-pg"); err != nil || !found {
		t.Fatalf("LoadWindowSummary(window-pg) = found %v, err %v; want found true", found, err)
	}
	if _, found, err := store.LoadWindowSummary("window-jsonl"); err != nil || found {
		t.Fatalf("LoadWindowSummary(window-jsonl) = found %v, err %v; a usable-but-empty postgres is authoritative",
			found, err)
	}

	summary := windowFixture("window-written", time.Now())
	if err := store.RecordWindowSummary(summary); err != nil {
		t.Fatalf("RecordWindowSummary: %v", err)
	}
	if err := store.RecordMutationBrief(summary.WindowID, domain.MutationBrief{MutationType: "prompt_tightening"}); err != nil {
		t.Fatalf("RecordMutationBrief: %v", err)
	}
	if len(pg.written) != 1 || pg.written[0].WindowID != "window-written" {
		t.Fatalf("postgres writes = %+v, want the window written to postgres", pg.written)
	}
	if len(jsonl.written) != 0 {
		t.Fatalf("jsonl writes = %+v; window writes must never be redirected to the JSONL ledger", jsonl.written)
	}
}

func TestPGFirstWindowStore_PGUnavailableFallsBackDegraded(t *testing.T) {
	pg := &stubWindowStore{summariesErr: errors.New("connection refused")}
	jsonl := &stubWindowStore{summaries: []domain.BacktestWindowSummary{windowFixture("window-jsonl", time.Now())}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	summaries, err := store.LoadWindowSummaries()
	if err != nil {
		t.Fatalf("LoadWindowSummaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].WindowID != "window-jsonl" {
		t.Fatalf("LoadWindowSummaries = %+v, want the JSONL fallback", summaries)
	}
	if !store.Degraded() {
		t.Error("Degraded() = false after the JSONL fallback")
	}
	if store.SourceBackend() != "jsonl" {
		t.Errorf("SourceBackend() = %q, want jsonl", store.SourceBackend())
	}
	if store.FallbackCount() == 0 {
		t.Error("FallbackCount() = 0 after the JSONL fallback")
	}

	// Both sides failing must surface the postgres error, not a silent empty
	// window list.
	jsonl.summariesErr = errors.New("jsonl gone")
	if _, err := store.LoadWindowSummaries(); err == nil {
		t.Error("LoadWindowSummaries with both backends failing returned nil error")
	}
}

func TestNewWindowStore_DeclaredBackendDecidesTheStore(t *testing.T) {
	jsonl, err := NewWindowStore(config.Config{StoreBackend: "jsonl", LedgerDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewWindowStore(jsonl): %v", err)
	}
	if _, ok := jsonl.(*Store); !ok {
		t.Fatalf("NewWindowStore(jsonl) = %T, want the JSONL *Store", jsonl)
	}

	sqlite, err := NewWindowStore(config.Config{StoreBackend: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("NewWindowStore(sqlite): %v", err)
	}
	if _, ok := sqlite.(*SQLiteStore); !ok {
		t.Fatalf("NewWindowStore(sqlite) = %T, want the SQLite store", sqlite)
	}

	// Declared postgres without an injected pool must fail loudly rather than
	// fall back to the JSONL directory (the T1 lesson).
	SetPostgresPool(nil)
	t.Cleanup(func() { SetPostgresPool(nil) })
	if _, err := NewWindowStore(config.Config{StoreBackend: "postgres", LedgerDir: t.TempDir()}); err == nil {
		t.Fatal("NewWindowStore(postgres) without a pool returned nil error; it must not fall back to JSONL")
	}

	if _, err := NewWindowStore(config.Config{StoreBackend: "mysql"}); err == nil {
		t.Fatal("NewWindowStore(unknown backend) returned nil error")
	}
}
