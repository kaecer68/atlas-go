package orchestrator

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// fixtureResolver returns a TradingSessionResolver that always yields
// the provided date (or error). Used to control NextTradingSession
// without a full CSV parse.
func fixtureResolver(next time.Time, err error) TradingSessionResolver {
	return &fixtureTradingSessionResolver{next: next, err: err}
}

type fixtureTradingSessionResolver struct {
	next time.Time
	err  error
}

func (r *fixtureTradingSessionResolver) NextTradingSession(_ time.Time) (time.Time, error) {
	return r.next, r.err
}

func TestApplySectorRotation_ClosureNotWired(t *testing.T) {
	e := NewStrategyEvolver()
	plan := &portfolio.SectorRotationPlan{PrimaryFlow: "risk_on"}
	_, applied, reason := e.ApplySectorRotation(plan, time.Now(), nil)
	if applied {
		t.Fatalf("expected not applied (no closure store), got reason=%q", reason)
	}
}

func TestApplySectorRotation_SuspendedBlocks(t *testing.T) {
	dir := t.TempDir()
	store := sectorallocation.NewFileClosureStore(dir)
	resolver := fixtureResolver(time.Now().Add(24*time.Hour), nil)

	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(resolver)
	// Force suspended state.
	evolver.currentState = StrategySuspended

	plan := &portfolio.SectorRotationPlan{PrimaryFlow: "risk_on"}
	_, applied, reason := evolver.ApplySectorRotation(plan, time.Now(), nil)
	if applied {
		t.Fatalf("expected blocked by suspended state, got applied reason=%q", reason)
	}
}

func TestApplySectorRotation_PersistsSnapshot(t *testing.T) {
	dir := t.TempDir()
	store := sectorallocation.NewFileClosureStore(dir)
	nextDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	resolver := fixtureResolver(nextDate, nil)

	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(resolver)

	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	currentAllocs := map[string]float64{
		"semiconductor": 0.25,
		"financials":    0.15,
	}
	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow: "risk_on",
		Allocations: []portfolio.SectorAllocation{
			{Sector: "semiconductor", TargetPct: 0.30},
			{Sector: "financials", TargetPct: 0.20},
		},
	}

	receipt, applied, reason := evolver.ApplySectorRotation(plan, asOf, currentAllocs)
	if !applied {
		t.Fatalf("expected applied, got %q", reason)
	}
	if receipt == nil {
		t.Fatal("expected non-nil receipt")
	}
	if receipt.ReceiptID == "" {
		t.Error("expected non-empty receipt id")
	}

	// Verify snapshot was persisted.
	snap, err := store.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot after persist")
	}
	if snaPAsOf := snap.AsOfTradingDate; snaPAsOf != "2026-06-30" {
		t.Errorf("expected AsOfTradingDate=2026-06-30, got %q", snaPAsOf)
	}
	if snap.EffectiveFrom != "2026-07-01" {
		t.Errorf("expected EffectiveFrom=2026-07-01, got %q", snap.EffectiveFrom)
	}
	if snap.FallbackReason != "" && snap.FallbackReason != "no weight engine" {
		t.Errorf("unexpected fallback reason (want empty or 'no weight engine' when engine not wired): %q", snap.FallbackReason)
	}
	if len(snap.Target) == 0 {
		t.Error("expected non-empty target map")
	}
	if len(snap.Delta) == 0 {
		t.Error("expected non-empty delta map")
	}
	if !snap.Applied {
		t.Error("expected Applied=true on persisted snapshot")
	}
}

func TestApplySectorRotation_ResolverFailClosed(t *testing.T) {
	dir := t.TempDir()
	store := sectorallocation.NewFileClosureStore(dir)
	resolver := fixtureResolver(time.Time{}, ErrSessionUnavailable)

	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(resolver)

	plan := &portfolio.SectorRotationPlan{PrimaryFlow: "risk_on"}
	_, applied, reason := evolver.ApplySectorRotation(plan, time.Now(), nil)
	if applied {
		t.Fatalf("expected fail-closed on resolver error, got reason=%q", reason)
	}

	// Verify no snapshot was persisted.
	snap, err := store.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if snap != nil {
		t.Fatalf("expected nil snapshot after fail-closed resolver, got %+v", snap)
	}
}

func TestApplySectorRotation_DerivesDeltaCorrectly(t *testing.T) {
	dir := t.TempDir()
	store := sectorallocation.NewFileClosureStore(dir)
	nextDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	resolver := fixtureResolver(nextDate, nil)

	// No weight engine → uses plan allocations directly as target.
	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(resolver)

	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	currentAllocs := map[string]float64{
		"semiconductor": 0.25,
		"financials":    0.15,
	}
	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow: "risk_on",
		Allocations: []portfolio.SectorAllocation{
			{Sector: "semiconductor", TargetPct: 0.35},
			{Sector: "financials", TargetPct: 0.10},
		},
	}

	_, applied, _ := evolver.ApplySectorRotation(plan, asOf, currentAllocs)
	if !applied {
		t.Fatal("expected applied")
	}

	snap, err := store.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	// delta = target - current
	// semiconductor: 0.35 - 0.25 = +0.10
	// financials:    0.10 - 0.15 = -0.05
	if val := snap.Delta[industry.SectorID("semiconductor")]; absDiff(val, 0.10) > 1e-9 {
		t.Errorf("expected semiconductor delta 0.10, got %f", val)
	}
	if val := snap.Delta[industry.SectorID("financials")]; absDiff(val, -0.05) > 1e-9 {
		t.Errorf("expected financials delta -0.05, got %f", val)
	}
	// Verify target map matches plan directly (no engine → plan fallback).
	if val := snap.Target[industry.SectorID("semiconductor")]; absDiff(val, 0.35) > 1e-9 {
		t.Errorf("expected semiconductor target 0.35, got %f", val)
	}
	if val := snap.Target[industry.SectorID("financials")]; absDiff(val, 0.10) > 1e-9 {
		t.Errorf("expected financials target 0.10, got %f", val)
	}
}

func TestApplySectorRotation_RecordSessionBumpsCounter(t *testing.T) {
	dir := t.TempDir()
	store := sectorallocation.NewFileClosureStore(dir)
	nextDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	resolver := fixtureResolver(nextDate, nil)

	// Simulate the SAC state manager using the worktree layout.
	mgr := sectorallocation.NewSACClosureStateManager(filepath.Dir(dir))
	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(resolver).
		WithSACClosureStateManager(mgr)

	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow: "risk_on",
		Allocations: []portfolio.SectorAllocation{
			{Sector: "semiconductor", TargetPct: 0.30},
		},
	}

	_, applied, _ := evolver.ApplySectorRotation(plan, asOf, nil)
	if !applied {
		t.Fatal("expected applied")
	}

	st := mgr.Get()
	if st.SessionCount < 1 {
		t.Errorf("expected session count >= 1 after applied rotation, got %d", st.SessionCount)
	}
	if st.LastReceiptID == "" {
		t.Error("expected non-empty LastReceiptID after applied rotation")
	}
}

// absDiff helper: absolute difference between two float64s.
func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
