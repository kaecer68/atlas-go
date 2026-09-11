package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// TestEnsurePersistentStateLoaded_HydratesHistory covers issue #1888 option A:
// the per-process return/portfolio series must be seeded from the persisted
// simulation state so the VaR snapshot path (and the LLM forensics hook behind
// it) is reachable in production, where every entry point runs a fresh System.
func TestEnsurePersistentStateLoaded_HydratesHistory(t *testing.T) {
	dir := t.TempDir()
	returns := make([]float64, 0, 40)
	equity := make([]float64, 0, 41)
	value := 1_000_000.0
	equity = append(equity, value)
	for i := 0; i < 40; i++ {
		r := 0.002 * float64((i%7)-3)
		returns = append(returns, r)
		value *= 1 + r
		equity = append(equity, value)
	}
	state := domain.SimulationState{Cash: value, EquityCurve: equity, DailyReturns: returns}
	if err := sim.SavePersistentState(dir, &state); err != nil {
		t.Fatalf("SavePersistentState: %v", err)
	}

	sys := newTestSystem(t)
	sys.Sim().session.Mode = "daily"
	sys.Sim().cfg.LedgerDir = dir

	if err := sys.ensurePersistentStateLoaded(); err != nil {
		t.Fatalf("ensurePersistentStateLoaded: %v", err)
	}
	if got := len(sys.Sim().returnHistory); got != len(returns) {
		t.Errorf("len(returnHistory) = %d, want %d", got, len(returns))
	}
	if got := len(sys.Sim().portfolioHistory); got != len(equity) {
		t.Errorf("len(portfolioHistory) = %d, want %d", got, len(equity))
	}
	if len(sys.Sim().returnHistory) < RiskForensicsMinSamples {
		t.Fatalf("hydrated history (%d) is below RiskForensicsMinSamples (%d): the forensics gate would still be unreachable",
			len(sys.Sim().returnHistory), RiskForensicsMinSamples)
	}

	// Values must be copies: mutating the source must not change the system.
	returns[0] = 99
	if sys.Sim().returnHistory[0] == 99 {
		t.Error("returnHistory aliases the persisted slice; it must be a copy")
	}
}

// TestEnsurePersistentStateLoaded_DoesNotClobberAccumulatedHistory verifies the
// hydration is a no-op once the process has accumulated its own series.
func TestEnsurePersistentStateLoaded_DoesNotClobberAccumulatedHistory(t *testing.T) {
	dir := t.TempDir()
	state := domain.SimulationState{
		EquityCurve:  []float64{100, 101, 102},
		DailyReturns: []float64{0.01, 0.01},
	}
	if err := sim.SavePersistentState(dir, &state); err != nil {
		t.Fatalf("SavePersistentState: %v", err)
	}

	sys := newTestSystem(t)
	sys.Sim().session.Mode = "daily"
	sys.Sim().cfg.LedgerDir = dir
	sys.Sim().returnHistory = []float64{0.5}
	sys.Sim().portfolioHistory = []float64{123}

	if err := sys.ensurePersistentStateLoaded(); err != nil {
		t.Fatalf("ensurePersistentStateLoaded: %v", err)
	}
	if len(sys.Sim().returnHistory) != 1 || sys.Sim().returnHistory[0] != 0.5 {
		t.Errorf("returnHistory = %v, want the in-process series untouched", sys.Sim().returnHistory)
	}
	if len(sys.Sim().portfolioHistory) != 1 || sys.Sim().portfolioHistory[0] != 123 {
		t.Errorf("portfolioHistory = %v, want the in-process series untouched", sys.Sim().portfolioHistory)
	}
}

// TestRunDailySimulation_ForensicsHookFiresFromPersistedHistory is the
// end-to-end proof for issue #1888 option A: with history restored from disk,
// one production-shaped run (fresh System, single RunDailySimulation) is enough
// to cross the gate and invoke the LLM performance-forensics hook.
func TestRunDailySimulation_ForensicsHookFiresFromPersistedHistory(t *testing.T) {
	dir := t.TempDir()
	returns := make([]float64, 0, RiskForensicsMinSamples+5)
	equity := make([]float64, 0, RiskForensicsMinSamples+6)
	value := 1_000_000.0
	equity = append(equity, value)
	for i := 0; i < RiskForensicsMinSamples+5; i++ {
		r := 0.003 * float64((i%9)-4)
		returns = append(returns, r)
		value *= 1 + r
		equity = append(equity, value)
	}
	state := domain.SimulationState{Cash: value, EquityCurve: equity, DailyReturns: returns}
	if err := sim.SavePersistentState(dir, &state); err != nil {
		t.Fatalf("SavePersistentState: %v", err)
	}

	sys := newTestSystem(t)
	sys.Sim().session.Mode = "daily"
	sys.Sim().cfg.LedgerDir = dir

	origHook := risk.PerformanceForensics
	t.Cleanup(func() { risk.PerformanceForensics = origHook })
	calls := 0
	risk.PerformanceForensics = func(_ context.Context, snapshot any) (string, error) {
		calls++
		if _, ok := snapshot.(domain.RiskSnapshot); !ok {
			t.Errorf("hook snapshot type = %T, want domain.RiskSnapshot", snapshot)
		}
		return "風險敘事", nil
	}

	result, err := sys.RunDailySimulation(time.Now())
	if err != nil {
		t.Fatalf("RunDailySimulation: %v", err)
	}
	if calls == 0 {
		t.Fatal("performance-forensics hook was not invoked although the hydrated history exceeds the gate")
	}
	if result.RiskSnapshot == nil {
		t.Error("result.RiskSnapshot is nil, want a snapshot built from the hydrated history")
	}
	if result.RiskCommentary != "風險敘事" {
		t.Errorf("result.RiskCommentary = %q, want the hook output", result.RiskCommentary)
	}
}

// TestFinalizeRiskForensics_SharedByBothRunPaths pins the #1888 root cause: the
// replay/dispatcher path (the one production uses, because a replay session is
// always resolved) must produce the risk snapshot and invoke the LLM
// performance-forensics hook exactly like the non-replay path.
func TestFinalizeRiskForensics_SharedByBothRunPaths(t *testing.T) {
	dir := t.TempDir()
	returns := make([]float64, 0, RiskForensicsMinSamples+5)
	equity := make([]float64, 0, RiskForensicsMinSamples+6)
	value := 1_000_000.0
	equity = append(equity, value)
	for i := 0; i < RiskForensicsMinSamples+5; i++ {
		r := 0.003 * float64((i%9)-4)
		returns = append(returns, r)
		value *= 1 + r
		equity = append(equity, value)
	}
	state := domain.SimulationState{Cash: value, EquityCurve: equity, DailyReturns: returns}
	if err := sim.SavePersistentState(dir, &state); err != nil {
		t.Fatalf("SavePersistentState: %v", err)
	}

	sys := newTestSystem(t)
	sys.Sim().session.Mode = "daily"
	sys.Sim().cfg.LedgerDir = dir
	// Force the replay path: with a replay dataset and a session date,
	// RunDailySimulation early-returns into runReplaySimulation.
	sys.Sim().cfg.ReplaySessionDate = "2026-06-15"
	sys.Sim().replay = &replay.Dataset{}

	origHook := risk.PerformanceForensics
	t.Cleanup(func() { risk.PerformanceForensics = origHook })
	calls := 0
	risk.PerformanceForensics = func(_ context.Context, _ any) (string, error) {
		calls++
		return "風險敘事（replay path）", nil
	}

	if _, err := sys.RunDailySimulation(time.Now()); err != nil {
		t.Fatalf("RunDailySimulation (replay path): %v", err)
	}
	if calls == 0 {
		t.Fatal("replay path did not invoke the performance-forensics hook: it must share finalizeRiskForensics with the non-replay path")
	}
	if sys.Sim().returnHistory == nil || len(sys.Sim().returnHistory) < RiskForensicsMinSamples {
		t.Errorf("returnHistory len = %d, want >= %d", len(sys.Sim().returnHistory), RiskForensicsMinSamples)
	}
}

// TestFinalizeRiskForensics_PendingBelowThreshold covers the observability
// branch: below the threshold no snapshot is built and no hook runs.
func TestFinalizeRiskForensics_PendingBelowThreshold(t *testing.T) {
	sys := newTestSystem(t)
	sys.Sim().returnHistory = []float64{0.01, 0.02}

	origHook := risk.PerformanceForensics
	t.Cleanup(func() { risk.PerformanceForensics = origHook })
	calls := 0
	risk.PerformanceForensics = func(_ context.Context, _ any) (string, error) {
		calls++
		return "should not run", nil
	}

	result := domain.SimulationResult{}
	sys.finalizeRiskForensics(&result)

	if calls != 0 {
		t.Errorf("hook called %d times below the threshold, want 0", calls)
	}
	if result.RiskSnapshot != nil {
		t.Error("RiskSnapshot must stay nil below the threshold")
	}
}
