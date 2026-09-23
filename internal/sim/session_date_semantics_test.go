package sim

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// sessionTestEngine is a frictionless engine so portfolio values are exact.
func sessionTestEngine() *Engine {
	return NewEngine(domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           1.0,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
	})
}

// sessionTestState holds 1000 shares of 2330.TW bought at 100 plus 900k cash,
// so the portfolio value follows the quote price directly:
// value = 900_000 + 1000*price.
func sessionTestState() domain.SimulationState {
	state := domain.NewSimulationState(1_000_000)
	state.Cash = 900_000
	state.Positions = []domain.Position{{
		Symbol:       "2330.TW",
		Quantity:     1000,
		AverageCost:  100,
		CurrentPrice: 100,
		MarketValue:  100_000,
	}}
	return state
}

func sessionQuotes(day time.Time, price float64) []domain.Quote {
	return []domain.Quote{{
		Symbol:     "2330.TW",
		Last:       price,
		Volume:     1_000_000,
		IsTradable: true,
		AsOf:       day,
	}}
}

// TestRunDay_SameSessionDateReplacesLastReturn is the core #1900 regression
// test: re-running an already recorded trading session must keep
// daily_returns at the same length and rewrite its entry with a true
// one-session return (measured against the previous session's close), not with
// the flat delta of the same-day re-run.
func TestRunDay_SameSessionDateReplacesLastReturn(t *testing.T) {
	engine := sessionTestEngine()
	state := sessionTestState()
	day1 := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	// Session 1: the first run has no previous close, so it seeds the baseline
	// and records no return.
	first := engine.RunDay(&state, day1, domain.RegimeRiskOn, sessionQuotes(day1, 100), nil)
	if first.SessionRerun {
		t.Error("first session must not be reported as a re-run")
	}
	if len(state.EquityCurve) != 1 || len(state.DailyReturns) != 0 {
		t.Fatalf("after first session: equity=%v returns=%v, want 1 equity point and no return",
			state.EquityCurve, state.DailyReturns)
	}
	if state.LastSessionDate != "2026-09-22" {
		t.Errorf("LastSessionDate = %q, want 2026-09-22", state.LastSessionDate)
	}

	// Session 2: a new date appends one return (value 1.01M vs base 1.0M).
	second := engine.RunDay(&state, day2, domain.RegimeRiskOn, sessionQuotes(day2, 110), nil)
	if len(state.DailyReturns) != 1 || !almostEqual(state.DailyReturns[0], 0.01) {
		t.Fatalf("after second session: returns=%v, want [0.01]", state.DailyReturns)
	}
	if !second.SessionReturnRecorded || !almostEqual(second.SessionReturn, 0.01) {
		t.Errorf("second session result = %+v, want a recorded 0.01 return", second)
	}
	if state.LastSessionDate != "2026-09-23" {
		t.Errorf("LastSessionDate = %q, want 2026-09-23", state.LastSessionDate)
	}

	// Same session, again, dearer quotes: value 1.021M. The stored return must
	// become (1.021M - 1.0M)/1.0M = 0.021 — measured against the previous
	// session's close — and the series must NOT grow.
	rerun := engine.RunDay(&state, day2, domain.RegimeRiskOn, sessionQuotes(day2, 121), nil)
	if len(state.DailyReturns) != 1 {
		t.Fatalf("re-running the same session grew daily_returns to %d entries: %v",
			len(state.DailyReturns), state.DailyReturns)
	}
	if !almostEqual(state.DailyReturns[0], 0.021) {
		t.Errorf("daily_returns[0] = %v, want 0.021 (return vs the previous session's close, not the same-day delta 0.0108)",
			state.DailyReturns[0])
	}
	if len(state.EquityCurve) != 2 || !almostEqual(state.EquityCurve[1], 1_021_000) {
		t.Errorf("EquityCurve = %v, want one point per session with the latest close", state.EquityCurve)
	}
	if !rerun.SessionRerun || !rerun.SessionReturnRecorded || !almostEqual(rerun.SessionReturn, 0.021) {
		t.Errorf("re-run result = %+v, want SessionRerun with the recomputed 0.021 return", rerun)
	}
	if state.LastSessionDate != "2026-09-23" {
		t.Errorf("LastSessionDate = %q, want it to stay 2026-09-23", state.LastSessionDate)
	}
	if got := state.PreviousValues["_portfolio_"]; !almostEqual(got, 1_021_000) {
		t.Errorf("PreviousValues[_portfolio_] = %v, want the latest close 1021000", got)
	}
}

// TestRunWithState_DifferentSessionDatesAppend pins the happy path: distinct
// sessions keep appending exactly one entry each.
func TestRunWithState_DifferentSessionDatesAppend(t *testing.T) {
	engine := sessionTestEngine()
	state := sessionTestState()
	base := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

	for i, price := range []float64{100, 110, 121} {
		day := base.AddDate(0, 0, i)
		result := engine.RunWithState(&state, domain.RegimeRiskOn, sessionQuotes(day, price), nil)
		if result.SessionRerun {
			t.Errorf("day %d: unexpected SessionRerun", i)
		}
		wantLen := i // session 0 has no previous close
		if len(state.DailyReturns) != wantLen {
			t.Fatalf("day %d: daily_returns = %v, want %d entries", i, state.DailyReturns, wantLen)
		}
		if len(state.EquityCurve) != i+1 {
			t.Fatalf("day %d: equity curve = %v, want %d entries", i, state.EquityCurve, i+1)
		}
	}
	// 1.0M -> 1.01M -> 1.021M
	if !almostEqual(state.DailyReturns[0], 0.01) || !almostEqual(state.DailyReturns[1], 11_000.0/1_010_000.0) {
		t.Errorf("daily_returns = %v, want [0.01, 0.01089...]", state.DailyReturns)
	}
}

// TestRunDay_LegacyStateAdoptsDateSemantics covers the migration path: a state
// file written before the date fields existed must load and keep its history,
// append once (the date is unknown, so the run cannot be attributed), and then
// adopt the semantics — the very next same-session run replaces.
func TestRunDay_LegacyStateAdoptsDateSemantics(t *testing.T) {
	engine := sessionTestEngine()
	state := sessionTestState()
	// Shape of a legacy file: series present, no LastSessionDate.
	state.EquityCurve = []float64{950_000, 1_000_000}
	state.DailyReturns = []float64{0.0526}
	state.PreviousValues["_portfolio_"] = 1_000_000

	day := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	adopt := engine.RunDay(&state, day, domain.RegimeRiskOn, sessionQuotes(day, 110), nil)
	if adopt.SessionRerun {
		t.Error("a legacy state has no session date, so the run cannot be a re-run")
	}
	if len(state.DailyReturns) != 2 {
		t.Fatalf("legacy state: daily_returns = %v, want the existing entry kept and one appended", state.DailyReturns)
	}
	if state.LastSessionDate != "2026-09-23" {
		t.Fatalf("LastSessionDate = %q, want the semantics adopted on the first dated run", state.LastSessionDate)
	}

	rerun := engine.RunDay(&state, day, domain.RegimeRiskOn, sessionQuotes(day, 121), nil)
	if !rerun.SessionRerun {
		t.Error("the run after adoption re-runs the same session date and must be a re-run")
	}
	if len(state.DailyReturns) != 2 {
		t.Fatalf("daily_returns = %v, want 2 entries (no growth after adoption)", state.DailyReturns)
	}
	if !almostEqual(state.DailyReturns[1], 0.021) {
		t.Errorf("daily_returns[1] = %v, want 0.021", state.DailyReturns[1])
	}
}

// TestRunDay_UnknownSessionDateKeepsLegacyAppend documents the fallback for
// quotes without a session date (deriveSimDay returns the zero time): there is
// nothing to compare, so the pre-#1900 append behavior is preserved.
func TestRunDay_UnknownSessionDateKeepsLegacyAppend(t *testing.T) {
	engine := sessionTestEngine()
	state := sessionTestState()
	quotes := sessionQuotes(time.Time{}, 100)
	quotes[0].AsOf = time.Time{}

	for i := range 2 {
		result := engine.RunDay(&state, time.Time{}, domain.RegimeRiskOn, quotes, nil)
		if result.SessionRerun {
			t.Errorf("run %d: an unknown session date cannot be a re-run", i)
		}
	}
	if len(state.DailyReturns) != 1 {
		t.Fatalf("daily_returns = %v, want the legacy append behavior (1 entry)", state.DailyReturns)
	}
	if state.LastSessionDate != "" {
		t.Errorf("LastSessionDate = %q, want it to stay empty for undated runs", state.LastSessionDate)
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
