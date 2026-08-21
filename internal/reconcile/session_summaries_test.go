package reconcile_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/reconcile"
)

func summary(id string, at time.Time, pv float64) domain.SessionSummary {
	return domain.SessionSummary{
		SessionID:      id,
		Regime:         domain.RegimeRiskOn,
		OutcomeCount:   3,
		PortfolioValue: pv,
		RecordedAt:     at,
	}
}

// fakePGRepo is an in-memory PGRepo recording backfill writes.
type fakePGRepo struct {
	summaries map[string]domain.SessionSummary
	saved     []string // session IDs written via SaveSessionSummary
	err       error    // optional load error
	saveErr   error    // optional save error
}

func (f *fakePGRepo) LoadAllSessionSummaries(context.Context) ([]domain.SessionSummary, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]domain.SessionSummary, 0, len(f.summaries))
	for _, s := range f.summaries {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakePGRepo) SaveSessionSummary(_ context.Context, s domain.SessionSummary) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, s.SessionID)
	if f.summaries == nil {
		f.summaries = map[string]domain.SessionSummary{}
	}
	f.summaries[s.SessionID] = s
	return nil
}

// C1 — Compare detects one-sided gaps and content conflicts (dry-run diff).
func TestCompare_DetectsSymmetricDifference(t *testing.T) {
	t0 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	pg := []domain.SessionSummary{
		summary("s-only-pg", t0, 100),
		summary("s-conflict", t0, 100), // PG copy
		summary("s-identical", t0, 100),
	}
	jsonl := []domain.SessionSummary{
		summary("s-only-jsonl", t0.Add(24*time.Hour), 200),
		summary("s-conflict", t0, 999), // drifted JSONL copy
		summary("s-identical", t0, 100),
	}

	d := reconcile.Compare(pg, jsonl)
	if len(d.OnlyPG) != 1 || d.OnlyPG[0] != "s-only-pg" {
		t.Errorf("OnlyPG = %v, want [s-only-pg]", d.OnlyPG)
	}
	if len(d.OnlyJSONL) != 1 || d.OnlyJSONL[0] != "s-only-jsonl" {
		t.Errorf("OnlyJSONL = %v, want [s-only-jsonl]", d.OnlyJSONL)
	}
	if len(d.Conflicts) != 1 || d.Conflicts[0].SessionID != "s-conflict" {
		t.Errorf("Conflicts = %+v, want [s-conflict]", d.Conflicts)
	}
	if d.PGCount != 3 || d.JSONLCount != 3 {
		t.Errorf("counts = (PG %d, JSONL %d), want (3, 3)", d.PGCount, d.JSONLCount)
	}
	if d.Empty() {
		t.Error("Empty() = true for a divergent diff, want false")
	}
}

// C2 — identical sides produce an empty diff.
func TestCompare_IdenticalSides_Empty(t *testing.T) {
	t0 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	pg := []domain.SessionSummary{summary("s-a", t0, 100), summary("s-b", t0.Add(time.Hour), 200)}
	jsonl := []domain.SessionSummary{summary("s-b", t0.Add(time.Hour), 200), summary("s-a", t0, 100)}

	d := reconcile.Compare(pg, jsonl)
	if !d.Empty() {
		t.Errorf("Empty() = false for identical sides: %+v", d)
	}
}

// C3 — dry-run reports the diff and writes nothing.
func TestRun_DryRun_WritesNothing(t *testing.T) {
	t0 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	pgRepo := &fakePGRepo{summaries: map[string]domain.SessionSummary{
		"s-only-pg": summary("s-only-pg", t0, 100),
	}}
	dir := t.TempDir()
	jsonlStore, err := ledger.NewSessionStore(config.Config{LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	if err := jsonlStore.RecordSessionSummary(domain.ReplaySession{ID: "s-only-jsonl"}, summary("s-only-jsonl", t0.Add(time.Hour), 200)); err != nil {
		t.Fatalf("seed JSONL: %v", err)
	}

	res, err := reconcile.Run(context.Background(), pgRepo, jsonlStore, reconcile.Options{Apply: false})
	if err != nil {
		t.Fatalf("dry-run Run: %v", err)
	}
	if res.BackfilledToPG != 0 || res.BackfilledToJSONL != 0 || len(res.Errors) != 0 {
		t.Errorf("dry-run must not write: backfill=(PG %d, JSONL %d) errors=%v",
			res.BackfilledToPG, res.BackfilledToJSONL, res.Errors)
	}
	if len(res.Diff.OnlyPG) != 1 || res.Diff.OnlyPG[0] != "s-only-pg" ||
		len(res.Diff.OnlyJSONL) != 1 || res.Diff.OnlyJSONL[0] != "s-only-jsonl" {
		t.Errorf("dry-run diff = (onlyPG %v onlyJSONL %v), want ([s-only-pg] [s-only-jsonl])",
			res.Diff.OnlyPG, res.Diff.OnlyJSONL)
	}
	if len(pgRepo.saved) != 0 {
		t.Errorf("dry-run wrote to PG: %v", pgRepo.saved)
	}
}

// C4 — Apply backfills both one-sided gaps and converges the sides.
func TestRun_Apply_BackfillsBothDirections(t *testing.T) {
	t0 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	pgRepo := &fakePGRepo{summaries: map[string]domain.SessionSummary{
		"s-only-pg": summary("s-only-pg", t0, 100),
	}}
	dir := t.TempDir()
	jsonlStore, err := ledger.NewSessionStore(config.Config{LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	if err := jsonlStore.RecordSessionSummary(domain.ReplaySession{ID: "s-only-jsonl"}, summary("s-only-jsonl", t0.Add(time.Hour), 200)); err != nil {
		t.Fatalf("seed JSONL: %v", err)
	}

	res, err := reconcile.Run(context.Background(), pgRepo, jsonlStore, reconcile.Options{Apply: true})
	if err != nil {
		t.Fatalf("apply Run: %v", err)
	}
	if res.BackfilledToPG != 1 || res.BackfilledToJSONL != 1 {
		t.Errorf("backfill = (PG %d, JSONL %d), want (1, 1)", res.BackfilledToPG, res.BackfilledToJSONL)
	}
	if len(res.Errors) != 0 {
		t.Errorf("apply errors: %v", res.Errors)
	}

	// PG side now has the JSONL-only session.
	if _, ok := pgRepo.summaries["s-only-jsonl"]; !ok {
		t.Error("PG missing backfilled s-only-jsonl")
	}
	// JSONL side now has the PG-only session (fixture: read back from disk).
	jsonlSummaries, err := jsonlStore.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("reload JSONL: %v", err)
	}
	found := false
	for _, s := range jsonlSummaries {
		if s.SessionID == "s-only-pg" && s.PortfolioValue == 100 {
			found = true
		}
	}
	if !found {
		t.Errorf("JSONL missing backfilled s-only-pg: %+v", jsonlSummaries)
	}

	// A second apply run must be a no-op (sides converged).
	res2, err := reconcile.Run(context.Background(), pgRepo, jsonlStore, reconcile.Options{Apply: true})
	if err != nil {
		t.Fatalf("second apply Run: %v", err)
	}
	if !res2.Diff.Empty() {
		t.Errorf("sides not converged after apply: %+v", res2.Diff)
	}
	if res2.BackfilledToPG != 0 || res2.BackfilledToJSONL != 0 {
		t.Errorf("second apply rewrote data: (PG %d, JSONL %d)", res2.BackfilledToPG, res2.BackfilledToJSONL)
	}
}

// C5 — Apply continues past a failing side and reports per-backfill errors.
func TestRun_Apply_CollectsErrors(t *testing.T) {
	t0 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	failingPG := &fakePGRepo{
		summaries: map[string]domain.SessionSummary{"s-only-pg": summary("s-only-pg", t0, 100)},
	}
	dir := t.TempDir()
	jsonlStore, err := ledger.NewSessionStore(config.Config{LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	// JSONL has one session whose PG backfill fails...
	jsonlStore.RecordSessionSummary(domain.ReplaySession{ID: "s-only-jsonl"}, summary("s-only-jsonl", t0.Add(time.Hour), 200))
	failingPG.saveErr = errors.New("connection refused")

	res, err := reconcile.Run(context.Background(), failingPG, jsonlStore, reconcile.Options{Apply: true})
	if err != nil {
		t.Fatalf("apply Run: %v", err)
	}
	if res.BackfilledToPG != 0 {
		t.Errorf("BackfilledToPG = %d, want 0 (PG save fails)", res.BackfilledToPG)
	}
	if len(res.Errors) != 1 {
		t.Errorf("Errors = %v, want 1 entry", res.Errors)
	}
	// JSONL-side backfill of the PG-only session still succeeded.
	jsonlSummaries, _ := jsonlStore.LoadSessionSummaries()
	found := false
	for _, s := range jsonlSummaries {
		if s.SessionID == "s-only-pg" {
			found = true
		}
	}
	if !found {
		t.Error("JSONL backfill should have succeeded despite PG failure")
	}
}

// C6 — a load failure on either side aborts the run with an error.
func TestRun_LoadError_Aborts(t *testing.T) {
	pgRepo := &fakePGRepo{err: errors.New("pg down")}
	dir := t.TempDir()
	jsonlStore, _ := ledger.NewSessionStore(config.Config{LedgerDir: dir})

	if _, err := reconcile.Run(context.Background(), pgRepo, jsonlStore, reconcile.Options{}); err == nil {
		t.Fatal("expected error when PG load fails")
	}
}
