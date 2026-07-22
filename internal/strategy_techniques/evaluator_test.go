package strategy_techniques

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// makeSnapshot creates a MacroDataSnapshot with the given TAIEX,
// RetailMarginBalance, and USD_TWD values. All other fields are zero.
func makeSnapshot(taiex, margin, usdtwd float64) marketdata.MacroDataSnapshot {
	s := marketdata.MacroDataSnapshot{}
	if taiex != 0 {
		s.TAIEX = marketdata.MacroDataPoint{Symbol: "TAIEX", Value: taiex}
	}
	if margin != 0 {
		s.RetailMarginBalance = marketdata.MacroDataPoint{
			Symbol: "RetailMarginBalance",
			Value:  margin,
		}
	}
	if usdtwd != 0 {
		s.USD_TWD = marketdata.MacroDataPoint{Symbol: "USD_TWD", Value: usdtwd}
	}
	return s
}

// TestConditionEvaluator_AllConditionsMatch computes hit rate when
// every snapshot matches the strategy conditions and forward return
// aligns with Direction.
func TestConditionEvaluator_AllConditionsMatch(t *testing.T) {
	e := NewConditionEvaluator()

	frame := StrategyFrame{
		ID:        "test-up",
		Direction: DirectionUp,
		Conditions: []Condition{
			{Field: "RetailMarginBalance", Operator: "gt", Value: 3000},
		},
	}

	// 3 snapshots: TAIEX 10000 → 11000 → 10500
	// Condition matches all 3, forward lookback=1:
	//   s0: TAIEX 10000→11000 (+10%) → hit (up)
	//   s1: TAIEX 11000→10500 (-4.5%) → miss (up)
	snapshots := []marketdata.MacroDataSnapshot{
		makeSnapshot(10000, 3500, 0),
		makeSnapshot(11000, 3500, 0),
		makeSnapshot(10500, 3500, 0),
	}

	result := e.Evaluate(frame, snapshots, 1)

	if result.TotalTests != 2 {
		t.Errorf("TotalTests = %d, want 2", result.TotalTests)
	}
	if result.TotalHits != 1 {
		t.Errorf("TotalHits = %d, want 1", result.TotalHits)
	}
	if result.HitRate != 0.5 {
		t.Errorf("HitRate = %f, want 0.5", result.HitRate)
	}
}

// TestConditionEvaluator_NoMatch returns zero when no Snapshots meet
// the conditions.
func TestConditionEvaluator_NoMatch(t *testing.T) {
	e := NewConditionEvaluator()

	frame := StrategyFrame{
		ID:        "test-down",
		Direction: DirectionDown,
		Conditions: []Condition{
			{Field: "USD_TWD", Operator: "gt", Value: 33.0},
		},
	}

	snapshots := []marketdata.MacroDataSnapshot{
		makeSnapshot(10000, 0, 32.0),
		makeSnapshot(10000, 0, 32.5),
		makeSnapshot(10000, 0, 32.8),
	}

	result := e.Evaluate(frame, snapshots, 1)

	if result.TotalTests != 0 {
		t.Errorf("TotalTests = %d, want 0", result.TotalTests)
	}
	if result.TotalHits != 0 {
		t.Errorf("TotalHits = %d, want 0", result.TotalHits)
	}
	if result.HitRate != 0 {
		t.Errorf("HitRate = %f, want 0", result.HitRate)
	}
}

// TestConditionEvaluator_DownDirection verifies that DirectionDown
// correctly counts negative forward returns as hits.
func TestConditionEvaluator_DownDirection(t *testing.T) {
	e := NewConditionEvaluator()

	frame := StrategyFrame{
		ID:        "test-down",
		Direction: DirectionDown,
		Conditions: []Condition{
			{Field: "RetailMarginBalance", Operator: "gte", Value: 3000},
		},
	}

	// All match, forward: 10000→9500 (-5%) → hit (down)
	snapshots := []marketdata.MacroDataSnapshot{
		makeSnapshot(10000, 3000, 0),
		makeSnapshot(9500, 3000, 0),
	}

	result := e.Evaluate(frame, snapshots, 1)

	if result.TotalTests != 1 {
		t.Errorf("TotalTests = %d, want 1", result.TotalTests)
	}
	if result.TotalHits != 1 {
		t.Errorf("TotalHits = %d, want 1 (down direction with -5%%)", result.TotalHits)
	}
	if result.HitRate != 1.0 {
		t.Errorf("HitRate = %f, want 1.0", result.HitRate)
	}
}

// TestConditionEvaluator_TooFewSnapshots returns zero when there are
// insufficient snapshots for forward lookback.
func TestConditionEvaluator_TooFewSnapshots(t *testing.T) {
	e := NewConditionEvaluator()

	frame := StrategyFrame{
		ID:         "short",
		Direction:  DirectionUp,
		Conditions: []Condition{{Field: "TAIEX", Operator: "gt", Value: 0}},
	}

	snapshots := []marketdata.MacroDataSnapshot{
		makeSnapshot(10000, 0, 0),
	}

	result := e.Evaluate(frame, snapshots, 1)

	if result.TotalTests != 0 {
		t.Errorf("TotalTests = %d, want 0 (not enough snapshots)", result.TotalTests)
	}
}

// TestConditionEvaluator_ZeroTAIEX skips snapshots where TAIEX is
// unset (value=0).
func TestConditionEvaluator_ZeroTAIEX(t *testing.T) {
	e := NewConditionEvaluator()

	frame := StrategyFrame{
		ID:         "zerotaiex",
		Direction:  DirectionUp,
		Conditions: []Condition{{Field: "RetailMarginBalance", Operator: "gt", Value: 3000}},
	}

	// TAIEX=0 at forward point → skip that match
	snapshots := []marketdata.MacroDataSnapshot{
		{RetailMarginBalance: marketdata.MacroDataPoint{Symbol: "RetailMarginBalance", Value: 3500}},
		{RetailMarginBalance: marketdata.MacroDataPoint{Symbol: "RetailMarginBalance", Value: 3500}},
	}

	result := e.Evaluate(frame, snapshots, 1)

	// Condition matched, TotalTests counts, but TAIEX=0 → not counted as hit
	if result.TotalTests != 1 {
		t.Errorf("TotalTests = %d, want 1", result.TotalTests)
	}
	if result.TotalHits != 0 {
		t.Errorf("TotalHits = %d, want 0 (forward TAIEX is zero)", result.TotalHits)
	}
}

// TestResolveField_KnownField returns the correct value for a known field.
func TestResolveField_KnownField(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		RetailMarginBalance: marketdata.MacroDataPoint{
			Symbol: "RetailMarginBalance", Value: 1234,
		},
	}

	val, ok := resolveField(snap, "RetailMarginBalance")
	if !ok {
		t.Error("expected RetailMarginBalance to be found")
	}
	if val != 1234 {
		t.Errorf("value = %f, want 1234", val)
	}
}

// TestResolveField_UnknownField returns (0, false).
func TestResolveField_UnknownField(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{}
	val, ok := resolveField(snap, "NonExistent")
	if ok {
		t.Error("expected NonExistent to not be found")
	}
	if val != 0 {
		t.Errorf("value = %f, want 0", val)
	}
}

// TestResolveField_ZeroSymbol returns (value, false) even when value is zero
// but symbol is empty — distinguishing "no data" from "value is zero".
func TestResolveField_ZeroSymbol(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		TAIEX: marketdata.MacroDataPoint{Symbol: "TAIEX", Value: 0},
	}
	val, ok := resolveField(snap, "TAIEX")
	if !ok {
		t.Error("TAIEX with symbol set should be found even if value=0")
	}
	if val != 0 {
		t.Errorf("value = %f, want 0", val)
	}
}
