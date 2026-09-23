package orchestrator

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
)

func sessionAlmostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// TestRecordSessionHistory_AppendPathUnchanged pins the behavior of a new
// trading session: one portfolio point, one return, derived from the in-process
// series exactly as before #1900.
func TestRecordSessionHistory_AppendPathUnchanged(t *testing.T) {
	sys := newTestSystem(t)
	sys.Sim().portfolioHistory = []float64{1_000_000}

	sys.recordSessionHistory(domain.SimulationResult{PortfolioValue: 1_010_000})

	if len(sys.Sim().portfolioHistory) != 2 || !sessionAlmostEqual(sys.Sim().portfolioHistory[1], 1_010_000) {
		t.Fatalf("portfolioHistory = %v, want the new close appended", sys.Sim().portfolioHistory)
	}
	if len(sys.Sim().returnHistory) != 1 || !sessionAlmostEqual(sys.Sim().returnHistory[0], 0.01) {
		t.Fatalf("returnHistory = %v, want [0.01]", sys.Sim().returnHistory)
	}
}

// TestRecordSessionHistory_SameSessionRerunReplacesEntry pins the in-process
// half of #1900: a same-session re-run rewrites the session's entry (and does
// not grow the series), and a re-run whose session has no return entry leaves
// the return series alone instead of writing a bogus value.
func TestRecordSessionHistory_SameSessionRerunReplacesEntry(t *testing.T) {
	sys := newTestSystem(t)
	sys.Sim().portfolioHistory = []float64{1_000_000, 1_010_000}
	sys.Sim().returnHistory = []float64{0.01}

	sys.recordSessionHistory(domain.SimulationResult{
		PortfolioValue:        1_021_000,
		SessionRerun:          true,
		SessionReturn:         0.021,
		SessionReturnRecorded: true,
	})

	if len(sys.Sim().portfolioHistory) != 2 || !sessionAlmostEqual(sys.Sim().portfolioHistory[1], 1_021_000) {
		t.Fatalf("portfolioHistory = %v, want the same-session close replaced in place", sys.Sim().portfolioHistory)
	}
	if len(sys.Sim().returnHistory) != 1 || !sessionAlmostEqual(sys.Sim().returnHistory[0], 0.021) {
		t.Fatalf("returnHistory = %v, want [0.021] (replaced, not appended)", sys.Sim().returnHistory)
	}

	// A re-run of the first session ever has no return entry to replace.
	sys.recordSessionHistory(domain.SimulationResult{PortfolioValue: 1_022_000, SessionRerun: true})
	if len(sys.Sim().returnHistory) != 1 || !sessionAlmostEqual(sys.Sim().returnHistory[0], 0.021) {
		t.Errorf("returnHistory = %v, want the recorded entry untouched when the session has no return", sys.Sim().returnHistory)
	}
	if !sessionAlmostEqual(sys.Sim().portfolioHistory[1], 1_022_000) {
		t.Errorf("portfolioHistory = %v, want the close still replaced", sys.Sim().portfolioHistory)
	}
}

// TestSameSessionRerun_DoesNotProduceZeroDominatedVaR is the #1900 acceptance
// test for the risk snapshot.
//
// It first reproduces the reported production failure with the pre-fix
// append-only behavior (14 same-day re-runs ⇒ var95 = cvar95 = 0) and then
// shows that the dated series keeps the snapshot intact: same length, same
// VaR/CVaR.
func TestSameSessionRerun_DoesNotProduceZeroDominatedVaR(t *testing.T) {
	const sessions = risk.MinObservationsForVaR // 252: below this VaR is gated to 0 anyway
	returns := make([]float64, 0, sessions)
	equity := make([]float64, 0, sessions+1)
	value := 1_000_000.0
	equity = append(equity, value)
	for i := range sessions {
		// All sessions positive, the shape the production snapshot showed:
		// that is why BOTH var95 and cvar95 collapsed to 0 once zeros arrived.
		r := 0.001 * float64(1+i%5)
		returns = append(returns, r)
		value *= 1 + r
		equity = append(equity, value)
	}

	before := risk.ComputeRiskSnapshot(returns, equity)
	if before.VaR95 == 0 || before.CVaR95 == 0 {
		t.Fatalf("baseline var95=%v cvar95=%v, want non-zero so the regression is observable", before.VaR95, before.CVaR95)
	}

	// Control: the old behavior — every same-day re-run appended one flat
	// return to the same series.
	naiveReturns := append([]float64(nil), returns...)
	naiveEquity := append([]float64(nil), equity...)
	for range 14 {
		naiveReturns = append(naiveReturns, 0)
		naiveEquity = append(naiveEquity, equity[len(equity)-1])
	}
	naive := risk.ComputeRiskSnapshot(naiveReturns, naiveEquity)
	if naive.VaR95 != 0 || naive.CVaR95 != 0 {
		t.Fatalf("control: 14 appended same-day zeros gave var95=%v cvar95=%v, want the reported 0/0 signature",
			naive.VaR95, naive.CVaR95)
	}

	// Fixed behavior: the same 14 re-runs replace the session's entry.
	sys := newTestSystem(t)
	sys.Sim().returnHistory = append([]float64(nil), returns...)
	sys.Sim().portfolioHistory = append([]float64(nil), equity...)
	for range 14 {
		sys.recordSessionHistory(domain.SimulationResult{
			PortfolioValue:        equity[len(equity)-1],
			SessionRerun:          true,
			SessionReturn:         0, // a flat same-day re-run
			SessionReturnRecorded: true,
		})
	}

	if len(sys.Sim().returnHistory) != sessions {
		t.Fatalf("returnHistory grew to %d entries over 14 same-day re-runs, want %d", len(sys.Sim().returnHistory), sessions)
	}
	if len(sys.Sim().portfolioHistory) != len(equity) {
		t.Fatalf("portfolioHistory grew to %d entries, want %d", len(sys.Sim().portfolioHistory), len(equity))
	}
	after := risk.ComputeRiskSnapshot(sys.Sim().returnHistory, sys.Sim().portfolioHistory)
	if after.VaR95 != before.VaR95 {
		t.Errorf("var95 = %v after same-day re-runs, want the pre-rerun %v", after.VaR95, before.VaR95)
	}
	if after.VaR95 == 0 || after.CVaR95 == 0 {
		t.Errorf("var95=%v cvar95=%v, want a non-zero (not same-day-zero dominated) snapshot", after.VaR95, after.CVaR95)
	}
}

// TestRunDailySimulation_SameSessionDateDoesNotGrowPersistedSeries is the
// end-to-end wiring test: three production-shaped entry points (each one a
// fresh System, as auto_daily_simulation / stress_test_daily /
// POST /admin/trigger-simulation are) run the same trading day and the
// persisted series keeps exactly one entry for it. The next trading day then
// appends exactly one entry.
func TestRunDailySimulation_SameSessionDateDoesNotGrowPersistedSeries(t *testing.T) {
	dir := t.TempDir()
	day := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	sessionKey := day.Format(domain.SessionDateLayout)

	const sessions = RiskForensicsMinSamples
	returns := make([]float64, 0, sessions)
	equity := make([]float64, 0, sessions+1)
	value := 1_000_000.0
	equity = append(equity, value)
	for i := range sessions {
		r := 0.002 * float64((i%7)-3)
		returns = append(returns, r)
		value *= 1 + r
		equity = append(equity, value)
	}
	seed := domain.SimulationState{
		Cash:             value,
		EquityCurve:      equity,
		DailyReturns:     returns,
		PreviousValues:   map[string]float64{"_portfolio_": value},
		LastSessionDate:  sessionKey,
		SessionBaseValue: equity[len(equity)-2],
	}
	if err := sim.SavePersistentState(dir, &seed); err != nil {
		t.Fatalf("SavePersistentState: %v", err)
	}

	newDailySystem := func() *System {
		sys := newTestSystem(t)
		sys.Sim().session.Mode = "daily"
		sys.Sim().cfg.LedgerDir = dir
		return sys
	}

	for i := range 3 {
		sys := newDailySystem()
		result, err := sys.RunDailySimulation(day)
		if err != nil {
			t.Fatalf("run %d: RunDailySimulation: %v", i, err)
		}
		if !result.SessionRerun {
			t.Errorf("run %d: SessionRerun = false, want true: the persisted state already holds %s", i, sessionKey)
		}
		saved, err := sim.LoadPersistentState(dir)
		if err != nil {
			t.Fatalf("run %d: LoadPersistentState: %v", i, err)
		}
		if saved == nil {
			t.Fatalf("run %d: simulation_state.json disappeared", i)
		}
		if len(saved.DailyReturns) != sessions {
			t.Fatalf("run %d: persisted daily returns = %d, want %d (same-day re-runs must not append)",
				i, len(saved.DailyReturns), sessions)
		}
		if len(saved.EquityCurve) != len(equity) {
			t.Fatalf("run %d: persisted equity curve = %d, want %d", i, len(saved.EquityCurve), len(equity))
		}
		if saved.LastSessionDate != sessionKey {
			t.Errorf("run %d: LastSessionDate = %q, want %q", i, saved.LastSessionDate, sessionKey)
		}
		if result.RiskSnapshot == nil {
			t.Errorf("run %d: RiskSnapshot is nil, want the snapshot built from the dated history", i)
		}
	}

	// The next trading session still appends exactly one entry.
	nextDay := day.AddDate(0, 0, 1)
	sys := newDailySystem()
	result, err := sys.RunDailySimulation(nextDay)
	if err != nil {
		t.Fatalf("next day: RunDailySimulation: %v", err)
	}
	if result.SessionRerun {
		t.Error("next day: SessionRerun = true, want false")
	}
	saved, err := sim.LoadPersistentState(dir)
	if err != nil {
		t.Fatalf("next day: LoadPersistentState: %v", err)
	}
	if len(saved.DailyReturns) != sessions+1 {
		t.Errorf("next day: persisted daily returns = %d, want %d", len(saved.DailyReturns), sessions+1)
	}
	if saved.LastSessionDate != nextDay.Format(domain.SessionDateLayout) {
		t.Errorf("next day: LastSessionDate = %q, want %q", saved.LastSessionDate, nextDay.Format(domain.SessionDateLayout))
	}
}
