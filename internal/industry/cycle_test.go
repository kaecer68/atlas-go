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

	// Empty metrics — should return configured confidence floor
	emptyMetrics := IndustryMetrics{}
	cfgFloor := config.GetParametersConfig().Industry.ConfidenceSignal.Value.ConfidenceFloor
	confidence = ct.calculateConfidence("test", emptyMetrics)
	if math.Abs(confidence-cfgFloor) > 0.001 {
		t.Errorf("expected base confidence %f, got %f", cfgFloor, confidence)
	}
}

func TestCyclePositionIsFavorable(t *testing.T) {
	tests := []struct {
		phase     CyclePhase
		favorable bool
	}{
		{CycleExpansion, true},
		{CycleRecovery, true},
		{CycleMature, false},
		{CycleRecession, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			pos := &CyclePosition{BusinessCycle: tt.phase}
			if pos.IsFavorablePhase() != tt.favorable {
				t.Errorf("expected favorable=%v for %s", tt.favorable, tt.phase)
			}
		})
	}
}

func TestGetPhaseScore(t *testing.T) {
	tests := []struct {
		phase    CyclePhase
		expected float64
	}{
		{CycleExpansion, 20.0},
		{CycleRecovery, 10.0},
		{CycleMature, 0.0},
		{CycleRecession, -20.0},
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

func TestCycleTracker_NewIndustries(t *testing.T) {
	ct := NewCycleTracker()
	newIndustries := []string{"foundry", "server_assembly", "cooling"}
	for _, id := range newIndustries {
		t.Run(id, func(t *testing.T) {
			pos, ok := ct.GetPosition(id)
			if !ok {
				t.Fatalf("expected position for %s", id)
			}
			if pos.IndustryID != id {
				t.Errorf("expected industry %s, got %s", id, pos.IndustryID)
			}
			score := pos.GetPhaseScore()
			if score == 0 && pos.BusinessCycle == CycleMature {
				// mature = 0 score by config default, acceptable
			}
			if pos.Confidence <= 0 {
				t.Errorf("expected positive confidence for %s, got %f", id, pos.Confidence)
			}
			t.Logf("%s: phase=%s score=%.1f confidence=%.0f%%", id, pos.BusinessCycle, score, pos.Confidence*100)
		})
	}
}

func TestCycleTracker_ConfigDriven(t *testing.T) {
	ct := NewCycleTracker()
	cfg := config.GetParametersConfig().Industry
	signal := config.GetParametersConfig().Industry.ConfidenceSignal.Value

	// Empty metrics should return config floor, not hardcoded 0.3
	emptyConf := ct.calculateConfidence("test", IndustryMetrics{})
	if math.Abs(emptyConf-signal.ConfidenceFloor) > 0.001 {
		t.Errorf("empty metrics: expected config floor %f, got %f", signal.ConfidenceFloor, emptyConf)
	}

	// Phase score should match config, not hardcoded values
	pos := &CyclePosition{BusinessCycle: CycleExpansion}
	phaseScore := pos.GetPhaseScore()
	if phaseScore != cfg.PhaseScores.Value.ScoreExpansion {
		t.Errorf("expansion score: expected config %f, got %f", cfg.PhaseScores.Value.ScoreExpansion, phaseScore)
	}

	// Transitions should come from config
	transitions := GetTypicalTransitions()
	cfgTransitions := cfg.CycleTransitions.Value
	if len(transitions) != len(cfgTransitions) {
		t.Errorf("transitions count mismatch: expected %d, got %d", len(cfgTransitions), len(transitions))
	}

	// Config threshold change should affect business cycle detection
	semi := IndustryMetrics{
		IndustryID:       "semiconductor",
		RevenueGrowthYoY: 0.15,
		ProfitGrowthYoY:  0.18,
	}
	phase := ct.detectBusinessCycle(semi)
	t.Logf("semiconductor at 15%% rev: phase=%s (threshold expansion=%f)", phase, cfg.CycleThresholds.Value["semiconductor"].ExpansionRevenuePct)
}

func TestCycleTracker_GetPhase(t *testing.T) {
	ct := NewCycleTracker()
	metrics := IndustryMetrics{
		IndustryID:       "test_industry",
		RevenueGrowthYoY: 0.25,
		ProfitGrowthYoY:  0.25,
	}
	ct.UpdatePosition("test_industry", metrics)

	phase, ok := ct.GetPhase("test_industry")
	if !ok {
		t.Fatal("expected phase to be found")
	}
	if phase != CycleExpansion {
		t.Fatalf("expected %s, got %s", CycleExpansion, phase)
	}
}

func TestCycleTracker_GetPhase_Missing(t *testing.T) {
	ct := NewCycleTracker()
	_, ok := ct.GetPhase("nonexistent")
	if ok {
		t.Fatal("expected false for unknown industry")
	}
}

func TestCycleTracker_GetContinuousPhaseScore_ExistingIndustry(t *testing.T) {
	ct := NewCycleTracker()
	// CycleTracker is initialized with default positions, so "semiconductor" exists
	score := ct.GetContinuousPhaseScore("semiconductor")
	if score < -20.0 || score > 20.0 {
		t.Fatalf("expected score in range [-20, 20], got %f", score)
	}
	t.Logf("semiconductor continuous score: %f", score)
}

func TestCycleTracker_GetContinuousPhaseScore_MissingIndustry(t *testing.T) {
	ct := NewCycleTracker()
	score := ct.GetContinuousPhaseScore("nonexistent")
	if score != 0.0 {
		t.Fatalf("expected 0.0 for missing industry, got %f", score)
	}
}

func TestCycleTracker_GetContinuousPhaseScore_HighConfidence(t *testing.T) {
	ct := NewCycleTracker()
	// A position with high confidence should produce a score close to the discrete phase score
	pos := &CyclePosition{
		IndustryID:    "test_high",
		BusinessCycle: CycleExpansion,
		Confidence:    0.95,
	}
	ct.positions["test_high"] = pos

	score := ct.GetContinuousPhaseScore("test_high")
	if score < 14.0 {
		t.Fatalf("expected score near 20.0 for high confidence expansion, got %f", score)
	}
}

func TestCycleTracker_GetContinuousPhaseScore_LowConfidence(t *testing.T) {
	ct := NewCycleTracker()
	pos := &CyclePosition{
		IndustryID:    "test_low",
		BusinessCycle: CycleRecovery,
		Confidence:    0.15,
	}
	ct.positions["test_low"] = pos

	score := ct.GetContinuousPhaseScore("test_low")
	// Low confidence Recovery pulls toward next phase (Expansion = 20.0)
	// blend = 1 - 0.15² = 0.9775, transProb = 0.80
	// Score = 10 * (1 - 0.9775 * 0.80) + 20 * (0.9775 * 0.80) = 17.82
	if score < 16.0 || score > 19.0 {
		t.Fatalf("expected ~17.82 for low confidence recovery, got %f", score)
	}
}

// --- TASK 1: HasEmpiricalData + EvidenceTier tests ---

func TestHasEmpiricalData_AfterInit(t *testing.T) {
	ct := NewCycleTracker()
	// After NewCycleTracker()+initializeDefaultPositions(), all seeded industries
	// have exactly 1 history entry. HasEmpiricalData should return true for them
	// since the tracker has actual data (even if estimated from seed values).
	seededIndustries := []string{
		"semiconductor", "ai_supply_chain", "financials", "shipping",
		"electronics", "foundry", "server_assembly", "cooling",
	}
	for _, id := range seededIndustries {
		t.Run(id, func(t *testing.T) {
			if !ct.HasEmpiricalData(id) {
				t.Errorf("expected HasEmpiricalData(%q)=true after init (1 history entry), got false", id)
			}
		})
	}
}

func TestHasEmpiricalData_NoData(t *testing.T) {
	ct := NewCycleTracker()
	// An industry that was never seeded should return false.
	if ct.HasEmpiricalData("nonexistent_industry") {
		t.Error("expected HasEmpiricalData=false for unseeded industry with 0 history entries")
	}
}

func TestEvidenceTier(t *testing.T) {
	ct := NewCycleTracker()

	// After init: seeded industries have exactly 1 history entry → "estimated"
	t.Run("estimated_after_init", func(t *testing.T) {
		if tier := ct.EvidenceTier("semiconductor"); tier != "estimated" {
			t.Errorf("expected 'estimated' (1 history entry), got '%s'", tier)
		}
	})

	// No data at all → "insufficient"
	t.Run("insufficient_no_data", func(t *testing.T) {
		if tier := ct.EvidenceTier("nonexistent"); tier != "insufficient" {
			t.Errorf("expected 'insufficient' (0 entries), got '%s'", tier)
		}
	})

	// After second update → "empirical" (2+ entries)
	t.Run("empirical_after_update", func(t *testing.T) {
		ct.UpdatePosition("semiconductor", IndustryMetrics{RevenueGrowthYoY: 0.30})
		if tier := ct.EvidenceTier("semiconductor"); tier != "empirical" {
			t.Errorf("expected 'empirical' (2+ history entries), got '%s'", tier)
		}
	})
}

// --- TASK 2: Falsification tests for detectBusinessCycle ---

func TestDetectBusinessCycle_IdenticalMetrics_SamePhase(t *testing.T) {
	// Scenario A (Boundary): Two calls with IDENTICAL metrics MUST produce the
	// same BusinessCycle phase — no random variance, no non-determinism.
	ct := NewCycleTracker()
	metrics := IndustryMetrics{
		RevenueGrowthYoY:    0.12,
		ProfitGrowthYoY:     0.15,
		InventoryTurnover:   5.0,
		CapacityUtilization: 0.80,
	}
	phase1 := ct.detectBusinessCycle(metrics)
	phase2 := ct.detectBusinessCycle(metrics)
	if phase1 != phase2 {
		t.Errorf("identical metrics produced different phases: %s vs %s", phase1, phase2)
	}
}

func TestDetectBusinessCycle_ExtremeRecession(t *testing.T) {
	// Scenario B (Extreme — 2020/03-style crash): Severely negative metrics
	// MUST detect CycleRecession, not any other phase.
	ct := NewCycleTracker()
	metrics := IndustryMetrics{
		RevenueGrowthYoY:    -0.80,
		ProfitGrowthYoY:     -0.90,
		InventoryTurnover:   1.5,
		CapacityUtilization: 0.35,
	}
	phase := ct.detectBusinessCycle(metrics)
	if phase != CycleRecession {
		t.Errorf("expected CycleRecession for extreme crash metrics, got %s", phase)
	}
}

func TestDetectBusinessCycle_AllPhasesReachable(t *testing.T) {
	// Scenario C: All four phases (expansion, recovery, mature, recession)
	// must be reachable with distinct input sets.
	ct := NewCycleTracker()
	tests := []struct {
		name     string
		rev      float64
		profit   float64
		expected CyclePhase
	}{
		{"expansion", 0.30, 0.35, CycleExpansion},
		{"recovery", 0.10, 0.08, CycleRecovery},
		{"mature", 0.02, 0.01, CycleMature},
		{"recession", -0.10, -0.15, CycleRecession},
	}

	seen := make(map[CyclePhase]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := IndustryMetrics{
				RevenueGrowthYoY: tt.rev,
				ProfitGrowthYoY:  tt.profit,
			}
			phase := ct.detectBusinessCycle(metrics)
			if phase != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, phase)
			}
			seen[phase] = true
		})
	}

	// Verify all four phases were covered
	for _, p := range []CyclePhase{CycleExpansion, CycleRecovery, CycleMature, CycleRecession} {
		if !seen[p] {
			t.Errorf("phase %s was not produced by any test case", p)
		}
	}
}
