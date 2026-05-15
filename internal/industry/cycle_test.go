package industry

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestNewCycleTracker(t *testing.T) {
	ct := NewCycleTracker()
	if ct == nil {
		t.Fatal("expected non-nil CycleTracker")
	}
	if ct.positions == nil {
		t.Error("expected positions map to be initialized")
	}
	if ct.history == nil {
		t.Error("expected history map to be initialized")
	}
}

func TestUpdateAndGetPosition(t *testing.T) {
	ct := NewCycleTracker()

	metrics := IndustryMetrics{
		IndustryID:          "semiconductor",
		RevenueGrowthYoY:    0.25,
		ProfitGrowthYoY:     0.30,
		InventoryTurnover:   7.0,
		CapacityUtilization: 0.85,
	}

	position := ct.UpdatePosition("semiconductor", metrics)
	if position == nil {
		t.Fatal("expected non-nil position")
	}

	if position.IndustryID != "semiconductor" {
		t.Errorf("expected industry semiconductor, got %s", position.IndustryID)
	}

	if position.BusinessCycle != CycleExpansion {
		t.Errorf("expected expansion phase, got %s", position.BusinessCycle)
	}

	if position.InventoryCycle != InvRestockingActive {
		t.Errorf("expected active restocking, got %s", position.InventoryCycle)
	}

	if position.CapexCycle != CapexMaintenance {
		t.Errorf("expected capex maintenance, got %s", position.CapexCycle)
	}

	// Test retrieval
	retrieved, ok := ct.GetPosition("semiconductor")
	if !ok {
		t.Fatal("expected to find position")
	}
	if retrieved.BusinessCycle != CycleExpansion {
		t.Errorf("expected expansion in retrieved position, got %s", retrieved.BusinessCycle)
	}
}

func TestDetectBusinessCycle(t *testing.T) {
	ct := NewCycleTracker()

	tests := []struct {
		name          string
		revenueGrowth float64
		profitGrowth  float64
		expectedPhase CyclePhase
	}{
		{"expansion", 0.25, 0.30, CycleExpansion},
		{"recovery", 0.10, 0.08, CycleRecovery},
		{"mature", 0.02, 0.01, CycleMature},
		{"recession", -0.10, -0.15, CycleRecession},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := IndustryMetrics{
				RevenueGrowthYoY: tt.revenueGrowth,
				ProfitGrowthYoY:  tt.profitGrowth,
			}
			phase := ct.detectBusinessCycle(metrics)
			if phase != tt.expectedPhase {
				t.Errorf("expected %s, got %s", tt.expectedPhase, phase)
			}
		})
	}
}

func TestDetectInventoryCycle(t *testing.T) {
	ct := NewCycleTracker()

	tests := []struct {
		name                string
		inventoryTurnover   float64
		capacityUtilization float64
		expectedCycle       InventoryCycle
	}{
		{"active_restocking", 7.0, 0.85, InvRestockingActive},
		{"passive_restocking", 5.0, 0.75, InvRestockingPassive},
		{"active_destocking", 2.0, 0.50, InvDestockingActive},
		{"passive_destocking", 3.5, 0.65, InvDestockingPassive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := IndustryMetrics{
				InventoryTurnover:   tt.inventoryTurnover,
				CapacityUtilization: tt.capacityUtilization,
			}
			cycle := ct.detectInventoryCycle(metrics)
			if cycle != tt.expectedCycle {
				t.Errorf("expected %s, got %s", tt.expectedCycle, cycle)
			}
		})
	}
}

func TestDetectCapexCycle(t *testing.T) {
	ct := NewCycleTracker()

	tests := []struct {
		name                string
		capacityUtilization float64
		revenueGrowth       float64
		expectedCycle       CapexCycle
	}{
		{"expansion", 0.90, 0.20, CapexExpansion},
		{"maintenance", 0.75, 0.08, CapexMaintenance},
		{"contraction", 0.60, -0.05, CapexContraction},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := IndustryMetrics{
				CapacityUtilization: tt.capacityUtilization,
				RevenueGrowthYoY:    tt.revenueGrowth,
			}
			cycle := ct.detectCapexCycle(metrics)
			if cycle != tt.expectedCycle {
				t.Errorf("expected %s, got %s", tt.expectedCycle, cycle)
			}
		})
	}
}

func TestCalculateConfidence(t *testing.T) {
	ct := NewCycleTracker()

	// Full metrics
	fullMetrics := IndustryMetrics{
		RevenueGrowthYoY:    0.10,
		ProfitGrowthYoY:     0.08,
		InventoryTurnover:   5.0,
		CapacityUtilization: 0.75,
	}
	confidence := ct.calculateConfidence("test", fullMetrics)
	if confidence < 0.5 || confidence > 0.7 {
		t.Errorf("expected moderate confidence (~0.55) with moderate metrics, got %f", confidence)
	}

	// Empty metrics — should return configured signal base (was ConfidenceFloor before fix)
	emptyMetrics := IndustryMetrics{}
	sig := config.GetParametersConfig().Industry.ConfidenceSignal.Value
	cfgBase := sig.SignalBase
	confidence = ct.calculateConfidence("test", emptyMetrics)
	if math.Abs(confidence-cfgBase) > 0.001 {
		t.Errorf("expected base confidence %f (SignalBase), got %f", cfgBase, confidence)
	}
}

func TestCyclePositionIsFavorable(t *testing.T) {
	// Expansion/Recovery + high confidence = favorable
	expHigh := &CyclePosition{BusinessCycle: CycleExpansion, Confidence: 0.80}
	if !expHigh.IsFavorable() {
		t.Errorf("expected favorable=true for expansion with high confidence")
	}
	recHigh := &CyclePosition{BusinessCycle: CycleRecovery, Confidence: 0.50}
	if !recHigh.IsFavorable() {
		t.Errorf("expected favorable=true for recovery with high confidence")
	}
	// Expansion + low confidence = not favorable
	expLow := &CyclePosition{BusinessCycle: CycleExpansion, Confidence: 0.20}
	if expLow.IsFavorable() {
		t.Errorf("expected favorable=false for expansion with low confidence")
	}
	// Recession = never favorable regardless of confidence
	recHighConf := &CyclePosition{BusinessCycle: CycleRecession, Confidence: 0.90}
	if recHighConf.IsFavorable() {
		t.Errorf("expected favorable=false for recession with high confidence")
	}
	mature := &CyclePosition{BusinessCycle: CycleMature, Confidence: 0.50}
	if mature.IsFavorable() {
		t.Errorf("expected favorable=false for mature")
	}
}

func TestGetPhaseScore(t *testing.T) {
	tests := []struct {
		phase    CyclePhase
		expected float64
	}{
		{CycleExpansion, 1.0},
		{CycleRecovery, 0.5},
		{CycleMature, 0.0},
		{CycleRecession, -1.0},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			pos := &CyclePosition{BusinessCycle: tt.phase}
			score := pos.GetPhaseScore()
			if score != tt.expected {
				t.Errorf("expected score %f for %s, got %f", tt.expected, tt.phase, score)
			}
		})
	}
}

func TestGetTrend(t *testing.T) {
	ct := NewCycleTracker()

	tests := []struct {
		value     float64
		threshold float64
		expected  string
	}{
		{12.0, 10.0, "up"},
		{8.0, 10.0, "down"},
		{10.5, 10.0, "stable"},
		{9.5, 10.0, "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := ct.getTrend(tt.value, tt.threshold)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetAllPositions(t *testing.T) {
	ct := NewCycleTracker()

	ct.UpdatePosition("semiconductor", IndustryMetrics{RevenueGrowthYoY: 0.25})
	ct.UpdatePosition("financials", IndustryMetrics{RevenueGrowthYoY: 0.05})

	all := ct.GetAllPositions()
	if len(all) < 2 {
		t.Errorf("expected at least 2 positions, got %d", len(all))
	}

	if _, ok := all["semiconductor"]; !ok {
		t.Error("expected semiconductor in positions")
	}
	if _, ok := all["financials"]; !ok {
		t.Error("expected financials in positions")
	}
}

func TestGetHistory(t *testing.T) {
	ct := NewCycleTracker()

	// Update same industry multiple times
	ct.UpdatePosition("semiconductor", IndustryMetrics{RevenueGrowthYoY: 0.10})
	ct.UpdatePosition("semiconductor", IndustryMetrics{RevenueGrowthYoY: 0.15})
	ct.UpdatePosition("semiconductor", IndustryMetrics{RevenueGrowthYoY: 0.20})

	history := ct.GetHistory("semiconductor")
	if len(history) != 4 {
		t.Errorf("expected 4 historical positions, got %d", len(history))
	}
}

func TestGetTypicalTransitions(t *testing.T) {
	transitions := GetTypicalTransitions()
	if len(transitions) != 4 {
		t.Errorf("expected 4 typical transitions, got %d", len(transitions))
	}

	// Check recession to recovery transition
	var found bool
	for _, tr := range transitions {
		if tr.FromPhase == CycleRecession && tr.ToPhase == CycleRecovery {
			found = true
			if tr.Probability <= 0 {
				t.Error("expected positive probability")
			}
			if len(tr.Triggers) == 0 {
				t.Error("expected triggers")
			}
		}
	}
	if !found {
		t.Error("expected recession to recovery transition")
	}
}

func TestCyclePositionString(t *testing.T) {
	pos := &CyclePosition{
		IndustryID:     "semiconductor",
		BusinessCycle:  CycleExpansion,
		InventoryCycle: InvRestockingActive,
		CapexCycle:     CapexExpansion,
		Confidence:     0.85,
	}

	s := pos.String()
	expected := "semiconductor: Business=expansion, Inventory=active_restocking, Capex=expansion, Confidence=85%"
	if s != expected {
		t.Errorf("expected '%s', got '%s'", expected, s)
	}
}
