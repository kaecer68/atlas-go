package sectorallocation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// MockWeightEngine implements WeightEngine for testing.
type MockWeightEngine struct {
	weights []SectorWeight
	err     error
}

func (m *MockWeightEngine) ComputeWeights(ctx context.Context, now time.Time) ([]SectorWeight, error) {
	return m.weights, m.err
}

func (m *MockWeightEngine) ComputeWeight(ctx context.Context, industryID string, now time.Time) (*SectorWeight, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, w := range m.weights {
		if w.ID == industryID {
			return &w, nil
		}
	}
	return nil, nil
}

func (m *MockWeightEngine) ComputeProjectedTarget(ctx context.Context, drivers DriverInputs) (ProjectedTarget, error) {
	if m.err != nil {
		return ProjectedTarget{}, m.err
	}
	// 測試用：mock 不跑 Projector，直接把現有 weights 轉成 ProjectedTarget.Target。
	// 既有 mock 行為不變；新增 method 是 SA04 介面相容要求。
	out := ProjectedTarget{
		AsOfTradingDate: drivers.AsOfTradingDate,
		ModelVersion:    "mock",
		Target:          make(map[industry.SectorID]float64, len(m.weights)),
	}
	for _, w := range m.weights {
		out.Target[industry.SectorID(w.ID)] = w.AdjustedWeight
	}
	return out, nil
}

// MockCycleInputProvider implements CycleInputProvider for testing.
type MockCycleInputProvider struct{}

func (m *MockCycleInputProvider) GetCycleMultiplier(ctx context.Context, industryID string) (float64, error) {
	return 1.1, nil
}

// MockSeasonalInputProvider implements SeasonalInputProvider for testing.
type MockSeasonalInputProvider struct{}

func (m *MockSeasonalInputProvider) GetSeasonalMultiplier(ctx context.Context, industryID string, now time.Time) (float64, error) {
	return 1.05, nil
}

// MockLinkageInputProvider implements LinkageInputProvider for testing.
type MockLinkageInputProvider struct{}

func (m *MockLinkageInputProvider) GetLinkageMultiplier(ctx context.Context, industryID string) (float64, error) {
	return 1.15, nil
}

// MockNarrativeInputProvider implements NarrativeInputProvider for testing.
type MockNarrativeInputProvider struct{}

func (m *MockNarrativeInputProvider) GetNarrativeMultiplier(ctx context.Context, industryID string) (float64, error) {
	return 1.1, nil
}

// MockMacroInputProvider implements MacroInputProvider for testing.
type MockMacroInputProvider struct{}

func (m *MockMacroInputProvider) GetMacroTilt(ctx context.Context, industryID, macroLevel, primaryFlow string) (float64, error) {
	return -0.15, nil
}

// MockFactorInputProvider implements FactorInputProvider for testing.
type MockFactorInputProvider struct{}

func (m *MockFactorInputProvider) GetFactorTilt(ctx context.Context, industryID string) (float64, error) {
	return 0.05, nil
}

// TestSectorWeightJSONRoundtrip verifies that SectorWeight marshals and unmarshals
// with snake_case JSON tags correctly.
func TestSectorWeightJSONRoundtrip(t *testing.T) {
	sw := SectorWeight{
		ID:             "semiconductor",
		Name:           "半導體",
		BaseWeight:     0.22,
		AdjustedWeight: 0.25,
		DerivationFactors: []WeightFactor{
			{Factor: "出口比重", Contribution: 0.35, Source: "TWSE", Evidence: "台灣出口占比"},
			{Factor: "景氣循環", Contribution: 0.25, Source: "CIA", Evidence: "全球GDP成長率"},
		},
		AdjustmentLog: []string{"base_weight=0.2200"},
	}

	// Marshal to JSON
	data, err := json.Marshal(sw)
	if err != nil {
		t.Fatalf("failed to marshal SectorWeight: %v", err)
	}

	// Verify snake_case keys are present
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	expectedKeys := []string{"id", "name", "base_weight", "adjusted_weight", "derivation_factors", "adjustment_log"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected snake_case key %q not found in JSON output", key)
		}
	}

	// Unmarshal back
	var sw2 SectorWeight
	if err := json.Unmarshal(data, &sw2); err != nil {
		t.Fatalf("failed to unmarshal SectorWeight: %v", err)
	}

	// Verify values match
	if sw2.ID != sw.ID {
		t.Errorf("ID mismatch: got %q, want %q", sw2.ID, sw.ID)
	}
	if sw2.Name != sw.Name {
		t.Errorf("Name mismatch: got %q, want %q", sw2.Name, sw.Name)
	}
	if sw2.BaseWeight != sw.BaseWeight {
		t.Errorf("BaseWeight mismatch: got %f, want %f", sw2.BaseWeight, sw.BaseWeight)
	}
	if sw2.AdjustedWeight != sw.AdjustedWeight {
		t.Errorf("AdjustedWeight mismatch: got %f, want %f", sw2.AdjustedWeight, sw.AdjustedWeight)
	}
	if len(sw2.DerivationFactors) != len(sw.DerivationFactors) {
		t.Errorf("DerivationFactors length mismatch: got %d, want %d", len(sw2.DerivationFactors), len(sw.DerivationFactors))
	}
	if len(sw2.AdjustmentLog) != len(sw.AdjustmentLog) {
		t.Errorf("AdjustmentLog length mismatch: got %d, want %d", len(sw2.AdjustmentLog), len(sw.AdjustmentLog))
	}
}

// TestSectorAllocationPlanJSON verifies SectorAllocationPlan JSON marshaling.
func TestSectorAllocationPlanJSON(t *testing.T) {
	plan := SectorAllocationPlan{
		Allocations: []SectorWeight{
			{ID: "semiconductor", Name: "半導體", BaseWeight: 0.22, AdjustedWeight: 0.25},
			{ID: "financials", Name: "金融", BaseWeight: 0.14, AdjustedWeight: 0.15},
		},
		PrimaryFlow:  "risk_on",
		Rationale:    "Macro conditions favor risk assets",
		Timestamp:    time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
		ConfigSource: "parameters.json",
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("failed to marshal SectorAllocationPlan: %v", err)
	}

	var plan2 SectorAllocationPlan
	if err := json.Unmarshal(data, &plan2); err != nil {
		t.Fatalf("failed to unmarshal SectorAllocationPlan: %v", err)
	}

	if len(plan2.Allocations) != len(plan.Allocations) {
		t.Errorf("Allocations length mismatch: got %d, want %d", len(plan2.Allocations), len(plan.Allocations))
	}
	if plan2.PrimaryFlow != plan.PrimaryFlow {
		t.Errorf("PrimaryFlow mismatch: got %q, want %q", plan2.PrimaryFlow, plan.PrimaryFlow)
	}
	if plan2.ConfigSource != plan.ConfigSource {
		t.Errorf("ConfigSource mismatch: got %q, want %q", plan2.ConfigSource, plan.ConfigSource)
	}
}

// TestWeightDerivationJSON verifies WeightDerivation struct.
func TestWeightDerivationJSON(t *testing.T) {
	wd := WeightDerivation{
		BaseWeight: 0.22,
		DerivationFactors: []WeightFactor{
			{Factor: "出口比重", Contribution: 0.35, Source: "TWSE", Evidence: "台灣出口占比"},
		},
		Interpretation: "Strong export dependency",
		RiskFactors:    []string{"晶片法案", "景氣放緩"},
		Opportunities:  []string{"AI需求", "先進製程"},
	}

	data, err := json.Marshal(wd)
	if err != nil {
		t.Fatalf("failed to marshal WeightDerivation: %v", err)
	}

	var wd2 WeightDerivation
	if err := json.Unmarshal(data, &wd2); err != nil {
		t.Fatalf("failed to unmarshal WeightDerivation: %v", err)
	}

	if wd2.BaseWeight != wd.BaseWeight {
		t.Errorf("BaseWeight mismatch: got %f, want %f", wd2.BaseWeight, wd.BaseWeight)
	}
	if len(wd2.RiskFactors) != len(wd.RiskFactors) {
		t.Errorf("RiskFactors length mismatch: got %d, want %d", len(wd2.RiskFactors), len(wd.RiskFactors))
	}
}

// TestWeightEngineInterface verifies that WeightEngine interface can be implemented.
func TestWeightEngineInterface(t *testing.T) {
	// Compile-time check: MockWeightEngine must implement WeightEngine
	var _ WeightEngine = (*MockWeightEngine)(nil)

	mock := &MockWeightEngine{
		weights: []SectorWeight{
			{ID: "semiconductor", Name: "半導體", BaseWeight: 0.22, AdjustedWeight: 0.25},
		},
	}

	ctx := context.Background()
	now := time.Now()

	// Test ComputeWeights
	weights, err := mock.ComputeWeights(ctx, now)
	if err != nil {
		t.Errorf("unexpected error from ComputeWeights: %v", err)
	}
	if len(weights) != 1 {
		t.Errorf("expected 1 weight, got %d", len(weights))
	}

	// Test ComputeWeight
	weight, err := mock.ComputeWeight(ctx, "semiconductor", now)
	if err != nil {
		t.Errorf("unexpected error from ComputeWeight: %v", err)
	}
	if weight == nil {
		t.Fatal("expected non-nil weight")
	}
	if weight.ID != "semiconductor" {
		t.Errorf("expected ID 'semiconductor', got %q", weight.ID)
	}
}

// TestCycleInputProviderInterface verifies CycleInputProvider can be implemented.
func TestCycleInputProviderInterface(t *testing.T) {
	var _ CycleInputProvider = (*MockCycleInputProvider)(nil)

	mock := &MockCycleInputProvider{}
	ctx := context.Background()

	multiplier, err := mock.GetCycleMultiplier(ctx, "semiconductor")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if multiplier == 0 {
		t.Error("expected non-zero multiplier")
	}
}

// TestSeasonalInputProviderInterface verifies SeasonalInputProvider can be implemented.
func TestSeasonalInputProviderInterface(t *testing.T) {
	var _ SeasonalInputProvider = (*MockSeasonalInputProvider)(nil)

	mock := &MockSeasonalInputProvider{}
	ctx := context.Background()
	now := time.Now()

	multiplier, err := mock.GetSeasonalMultiplier(ctx, "semiconductor", now)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if multiplier == 0 {
		t.Error("expected non-zero multiplier")
	}
}

// TestLinkageInputProviderInterface verifies LinkageInputProvider can be implemented.
func TestLinkageInputProviderInterface(t *testing.T) {
	var _ LinkageInputProvider = (*MockLinkageInputProvider)(nil)

	mock := &MockLinkageInputProvider{}
	ctx := context.Background()

	multiplier, err := mock.GetLinkageMultiplier(ctx, "semiconductor")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if multiplier == 0 {
		t.Error("expected non-zero multiplier")
	}
}

// TestNarrativeInputProviderInterface verifies NarrativeInputProvider can be implemented.
func TestNarrativeInputProviderInterface(t *testing.T) {
	var _ NarrativeInputProvider = (*MockNarrativeInputProvider)(nil)

	mock := &MockNarrativeInputProvider{}
	ctx := context.Background()

	multiplier, err := mock.GetNarrativeMultiplier(ctx, "semiconductor")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if multiplier == 0 {
		t.Error("expected non-zero multiplier")
	}
}

// TestMacroInputProviderInterface verifies MacroInputProvider can be implemented.
func TestMacroInputProviderInterface(t *testing.T) {
	var _ MacroInputProvider = (*MockMacroInputProvider)(nil)

	mock := &MockMacroInputProvider{}
	ctx := context.Background()

	tilt, err := mock.GetMacroTilt(ctx, "semiconductor", "red", "risk_off")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Tilt can be negative or positive, just not zero in meaningful scenarios
	_ = tilt
}

// TestFactorInputProviderInterface verifies FactorInputProvider can be implemented.
func TestFactorInputProviderInterface(t *testing.T) {
	var _ FactorInputProvider = (*MockFactorInputProvider)(nil)

	mock := &MockFactorInputProvider{}
	ctx := context.Background()

	tilt, err := mock.GetFactorTilt(ctx, "semiconductor")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	_ = tilt
}
