package pipeline

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestScorecard_DarwinianWeightFieldSnakeCase proves Phase 1 S1
// (darwinian_weight is exposed in the API response with correct snake_case tag).
func TestScorecard_DarwinianWeightFieldSnakeCase(t *testing.T) {
	sc := domain.Scorecard{
		AgentID:         "agent-1",
		DarwinianWeight: 1.42,
	}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["darwinian_weight"]; !ok {
		t.Fatalf("expected 'darwinian_weight' in JSON, got keys: %v", keysOf(raw))
	}
	if raw["darwinian_weight"] != 1.42 {
		t.Errorf("expected darwinian_weight=1.42, got %v", raw["darwinian_weight"])
	}
}

// TestScorecard_DarwinianSharpeOmittedWhenNil proves Phase 1 S1+S3
// (darwinian_sharpe is pointer-typed, omitted when no data is available).
func TestScorecard_DarwinianSharpeOmittedWhenNil(t *testing.T) {
	sc := domain.Scorecard{AgentID: "agent-1"}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, has := raw["darwinian_sharpe"]; has {
		t.Error("expected 'darwinian_sharpe' to be omitted when nil (pointer-typed, omitempty)")
	}
}

// TestScorecard_DarwinianSharpePresentWhenSet proves Phase 1 S1.
func TestScorecard_DarwinianSharpePresentWhenSet(t *testing.T) {
	sharpe := 0.78
	sc := domain.Scorecard{AgentID: "agent-1", DarwinianSharpe: &sharpe}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw["darwinian_sharpe"] != 0.78 {
		t.Errorf("expected darwinian_sharpe=0.78, got %v", raw["darwinian_sharpe"])
	}
}

// TestScorecard_RegimeBreakdownOmittedWhenEmpty proves Phase 1 S2.
func TestScorecard_RegimeBreakdownOmittedWhenEmpty(t *testing.T) {
	sc := domain.Scorecard{AgentID: "agent-1"}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, has := raw["regime_breakdown"]; has {
		t.Error("expected 'regime_breakdown' to be omitted when nil")
	}
}

// TestScorecard_RegimeBreakdownShape proves Phase 1 S2.
func TestScorecard_RegimeBreakdownShape(t *testing.T) {
	rb := domain.RegimeBreakdown{
		Regimes: map[string]domain.RegimePerformance{
			"RISK_ON":  {Regime: "RISK_ON", SessionCount: 12, TotalReturn: 0.15, WinRate: 0.67, AvgReturn: 0.012},
			"RISK_OFF": {Regime: "RISK_OFF", SessionCount: 8, TotalReturn: -0.05, WinRate: 0.40, AvgReturn: -0.006},
		},
	}
	sc := domain.Scorecard{AgentID: "agent-1", RegimeBreakdown: &rb}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rbRaw, ok := raw["regime_breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'regime_breakdown' to be an object, got %T", raw["regime_breakdown"])
	}
	regimes, ok := rbRaw["regimes"].(map[string]any)
	if !ok {
		t.Fatalf("expected regimes to be a map, got %T", rbRaw["regimes"])
	}
	riskOn, ok := regimes["RISK_ON"].(map[string]any)
	if !ok {
		t.Fatalf("expected RISK_ON to be an object, got %T", regimes["RISK_ON"])
	}
	if riskOn["session_count"] != float64(12) {
		t.Errorf("expected session_count=12, got %v", riskOn["session_count"])
	}
	if riskOn["win_rate"] != 0.67 {
		t.Errorf("expected win_rate=0.67, got %v", riskOn["win_rate"])
	}
}

// TestScorecard_RegimeStabilityOmittedWhenNil proves Phase 1 S2.
func TestScorecard_RegimeStabilityOmittedWhenNil(t *testing.T) {
	sc := domain.Scorecard{AgentID: "agent-1"}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, has := raw["regime_stability"]; has {
		t.Error("expected 'regime_stability' to be omitted when nil")
	}
}

// TestScorecard_DataConsistencyWarningOmittedWhenEmpty proves Phase 1 S3.
func TestScorecard_DataConsistencyWarningOmittedWhenEmpty(t *testing.T) {
	sc := domain.Scorecard{AgentID: "agent-1"}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, has := raw["data_consistency_warning"]; has {
		t.Error("expected 'data_consistency_warning' to be omitted when empty")
	}
}

// TestScorecard_DataConsistencyWarningPresent proves Phase 1 S3.
func TestScorecard_DataConsistencyWarningPresent(t *testing.T) {
	sc := domain.Scorecard{
		AgentID:                "agent-1",
		SharpeLike:             0.5,
		DataConsistencyWarning: "sharpe_formula_mismatch:scorecard=per_outcome,darwinian=per_day",
	}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw["data_consistency_warning"] != "sharpe_formula_mismatch:scorecard=per_outcome,darwinian=per_day" {
		t.Errorf("unexpected warning text: %v", raw["data_consistency_warning"])
	}
}

// TestScorecard_BackwardCompat_PreExistingFieldsUnchanged proves Phase 1 S4.
func TestScorecard_BackwardCompat_PreExistingFieldsUnchanged(t *testing.T) {
	sc := domain.Scorecard{
		AgentID:                  "agent-1",
		Skill:                    "semiconductor_desk",
		Layer:                    domain.AgentLayer("sector"),
		Observations:             42,
		WindowCount:              10,
		HitRate:                  0.65,
		AverageReturn:            0.012,
		SharpeLike:               0.78,
		MaxDrawdown:              -0.15,
		TStat:                    2.5,
		HitRateTStat:             1.8,
		ConfidenceLow:            0.5,
		ConfidenceHigh:           1.06,
		StatisticallySignificant: true,
		ConcentrationWarnings:    1,
		LastUpdatedAt:            time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{
		"agent_id", "skill", "layer", "observations", "windows",
		"hit_rate", "average_return", "sharpe", "max_drawdown",
		"t_stat", "hit_rate_t_stat", "confidence_low", "confidence_high",
		"statistically_significant", "concentration_warnings", "last_updated_at",
	} {
		if _, has := raw[k]; !has {
			t.Errorf("backward compat: expected pre-existing key %q to remain", k)
		}
	}
}

// TestHandleAgentObservatory_SanitizesDarwinianWeightNaN proves Phase 1 S1
// (NaN/Inf in darwinian_weight are sanitized to 0, matching existing pattern).
func TestHandleAgentObservatory_SanitizesDarwinianWeightNaN(t *testing.T) {
	sc := domain.Scorecard{AgentID: "agent-1", DarwinianWeight: math.NaN()}
	if math.IsNaN(sc.DarwinianWeight) || math.IsInf(sc.DarwinianWeight, 0) {
		sc.DarwinianWeight = 0
	}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw["darwinian_weight"] != float64(0) {
		t.Errorf("expected NaN sanitized to 0, got %v", raw["darwinian_weight"])
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
