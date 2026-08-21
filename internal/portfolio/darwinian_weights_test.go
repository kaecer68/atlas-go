package portfolio

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// helper to add an agent directly into the manager's internal map
func seedAgent(m *DarwinianWeightManager, id, skill, layer string, weight float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.weights[id] = &DarwinianAgentWeight{
		AgentID:        id,
		Skill:          skill,
		Layer:          layer,
		Weight:         weight,
		DailyReturns:   make([]float64, 0, m.lookbackDays),
		LastAdjustedAt: time.Now().Add(-24 * time.Hour), // allow immediate adjustment
		LastUpdatedAt:  time.Now(),
	}
}

func TestDarwinianWeightManager(t *testing.T) {
	t.Run("NewDarwinianWeightManager", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		if m == nil {
			t.Fatal("Expected non-nil manager")
		}

		if m.weights == nil {
			t.Error("Expected weights map to be initialized")
		}

		if m.lookbackDays != 30 {
			t.Errorf("Expected lookbackDays=30, got %d", m.lookbackDays)
		}
	})

	t.Run("InitializeFromRegistry", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		registry := domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "agent_001", Skill: "tech", Layer: domain.LayerSector, Enabled: true},
				{ID: "agent_002", Skill: "growth", Layer: domain.LayerStyle, Enabled: true},
				{ID: "agent_003", Skill: "risk", Layer: domain.LayerControl, Enabled: true},   // should be skipped
				{ID: "agent_004", Skill: "macro", Layer: domain.LayerContext, Enabled: false}, // disabled
				{ID: "agent_005", Skill: "macro_momentum", Layer: domain.LayerSuperinvestor, Enabled: true},
			},
		}

		m.InitializeFromRegistry(registry)

		// Sector, style, and superinvestor agents should be initialized
		w1 := m.GetWeight("agent_001")
		if w1 != DarwinianNeutralWeight {
			t.Errorf("Expected neutral weight for agent_001, got %f", w1)
		}

		w2 := m.GetWeight("agent_002")
		if w2 != DarwinianNeutralWeight {
			t.Errorf("Expected neutral weight for agent_002, got %f", w2)
		}

		// Superinvestor layer should be tracked
		w5 := m.GetWeight("agent_005")
		if w5 != DarwinianNeutralWeight {
			t.Errorf("Expected neutral weight for superinvestor agent_005, got %f", w5)
		}

		data, ok := m.GetAgentWeightData("agent_005")
		if !ok {
			t.Error("Expected superinvestor agent_005 to have weight data after InitializeFromRegistry")
		} else if data.Layer != "superinvestor" {
			t.Errorf("Expected layer=superinvestor for agent_005, got %s", data.Layer)
		}

		// Control layer should not be tracked
		w3 := m.GetWeight("agent_003")
		if w3 != DarwinianNeutralWeight {
			// GetWeight returns neutral for unknown agents, which is correct
		}
	})

	t.Run("RecordOutcome", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")
		seedAgent(m, "agent_001", "tech", "sector", 1.0)

		// Record positive outcome
		m.RecordOutcome("agent_001", 0.05, true)

		data, ok := m.GetAgentWeightData("agent_001")
		if !ok {
			t.Fatal("Expected agent data")
		}

		if data.TotalSignals != 1 {
			t.Errorf("Expected 1 signal, got %d", data.TotalSignals)
		}

		if data.WinCount != 1 {
			t.Errorf("Expected 1 win, got %d", data.WinCount)
		}

		// Record negative outcome
		m.RecordOutcome("agent_001", -0.03, false)

		data, ok = m.GetAgentWeightData("agent_001")
		if !ok {
			t.Fatal("Expected agent data after second record")
		}
		if data.TotalSignals != 2 {
			t.Errorf("Expected 2 signals, got %d", data.TotalSignals)
		}

		if data.LossCount != 1 {
			t.Errorf("Expected 1 loss, got %d", data.LossCount)
		}
	})

	t.Run("RollingSharpeCalculation", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")
		seedAgent(m, "agent_001", "tech", "sector", 1.0)

		// Add 60 returns with known positive mean, realistic variance, and
		// >=8 unique values (the degenerate-window guard zeroes Sharpe when
		// the window holds fewer than minUniqueReturnsForSharpe distinct values).
		for i := 0; i < 60; i++ {
			r := 0.01 + []float64{0.02, -0.01, 0.015, -0.005, 0.025, -0.02, 0.01, -0.015, 0.005, -0.025}[i%10]
			m.RecordOutcome("agent_001", r, r > 0)
		}

		data, ok := m.GetAgentWeightData("agent_001")
		if !ok {
			t.Fatal("Expected agent data")
		}

		// With >=30 returns (MinSamples=30), Sharpe should be calculated, positive, and sane
		if data.RollingSharpe <= 0 {
			t.Errorf("Expected positive Sharpe for positive-mean returns, got %f", data.RollingSharpe)
		}
		if math.Abs(data.RollingSharpe) > maxSharpeMagnitude {
			t.Errorf("Expected Sharpe within ±%v (non-annualized, no degenerate window), got %f", maxSharpeMagnitude, data.RollingSharpe)
		}
	})

	t.Run("RollingSharpeCalculationNegative", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")
		seedAgent(m, "agent_neg", "financials", "sector", 1.0)

		// Add 60 returns with known negative mean, realistic variance, and
		// >=8 unique values (degenerate-window guard).
		for i := 0; i < 60; i++ {
			r := -0.01 + []float64{-0.02, 0.01, -0.015, 0.005, -0.025, 0.02, -0.01, 0.015, -0.005, 0.025}[i%10]
			m.RecordOutcome("agent_neg", r, r > 0)
		}

		data, ok := m.GetAgentWeightData("agent_neg")
		if !ok {
			t.Fatal("Expected agent data")
		}

		// With 60 negative-mean returns, Sharpe should be negative (not zero)
		if data.RollingSharpe >= 0 {
			t.Errorf("Expected negative Sharpe for negative-mean returns, got %f", data.RollingSharpe)
		}
		t.Logf("Negative Sharpe: %.4f (avg_return=%.4f)", data.RollingSharpe, data.AvgReturn)
	})

	t.Run("PerformDailyAdjustment", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		// Create 8 agents with different performance
		agents := []string{"top_001", "top_002", "mid_001", "mid_002", "bot_001", "bot_002", "bot_003", "bot_004"}
		for _, agent := range agents {
			seedAgent(m, agent, "test", "sector", 1.0)
		}

		// Simulate performance: top performers get positive returns
		for range 20 {
			m.RecordOutcome("top_001", 0.03, true)
			m.RecordOutcome("top_002", 0.025, true)
			m.RecordOutcome("mid_001", 0.01, true)
			m.RecordOutcome("mid_002", 0.005, true)
			m.RecordOutcome("bot_001", -0.02, false)
			m.RecordOutcome("bot_002", -0.015, false)
			m.RecordOutcome("bot_003", -0.025, false)
			m.RecordOutcome("bot_004", -0.01, false)
		}

		// Perform daily adjustment
		adjustments, _ := m.PerformDailyAdjustment()

		if len(adjustments) == 0 {
			t.Log("No adjustments returned (may be due to cooldown)")
		}

		// Verify weights stayed within bounds
		allWeights := m.GetAllWeights()
		for id, w := range allWeights {
			if w < DarwinianWeightMin {
				t.Errorf("Weight for %s below min: %f", id, w)
			}
			if w > DarwinianWeightMax {
				t.Errorf("Weight for %s above max: %f", id, w)
			}
		}
	})

	t.Run("ConstrainWeight", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		clamped, _ := m.constrainWeight("test-agent", 3.0)
		if clamped != DarwinianWeightMax {
			t.Errorf("Expected max %f, got %f", DarwinianWeightMax, clamped)
		}

		clamped, _ = m.constrainWeight("test-agent", 0.1)
		if clamped != DarwinianWeightMin {
			t.Errorf("Expected min %f, got %f", DarwinianWeightMin, clamped)
		}

		clamped, _ = m.constrainWeight("test-agent", 1.5)
		if clamped != 1.5 {
			t.Errorf("Expected 1.5, got %f", clamped)
		}
	})

	t.Run("ClampingEvent", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		// Case 1: clamped to MAX
		clamped, event := m.constrainWeight("agent-max", 3.0)
		if clamped != DarwinianWeightMax {
			t.Errorf("Expected clamped to max %f, got %f", DarwinianWeightMax, clamped)
		}
		if event == nil {
			t.Fatal("Expected non-nil ClampingEvent when clamping to max")
		}
		if event.Boundary != "max" {
			t.Errorf("Expected boundary 'max', got '%s'", event.Boundary)
		}
		if event.RawWeight != 3.0 {
			t.Errorf("Expected raw weight 3.0, got %f", event.RawWeight)
		}
		if event.FinalWeight != DarwinianWeightMax {
			t.Errorf("Expected final weight %f, got %f", DarwinianWeightMax, event.FinalWeight)
		}
		if event.AgentID != "agent-max" {
			t.Errorf("Expected agent ID 'agent-max', got '%s'", event.AgentID)
		}
		if event.Timestamp.IsZero() {
			t.Error("Expected non-zero timestamp")
		}

		// Case 2: clamped to MIN
		clamped, event = m.constrainWeight("agent-min", 0.1)
		if clamped != DarwinianWeightMin {
			t.Errorf("Expected clamped to min %f, got %f", DarwinianWeightMin, clamped)
		}
		if event == nil {
			t.Fatal("Expected non-nil ClampingEvent when clamping to min")
		}
		if event.Boundary != "min" {
			t.Errorf("Expected boundary 'min', got '%s'", event.Boundary)
		}
		if event.RawWeight != 0.1 {
			t.Errorf("Expected raw weight 0.1, got %f", event.RawWeight)
		}
		if event.FinalWeight != DarwinianWeightMin {
			t.Errorf("Expected final weight %f, got %f", DarwinianWeightMin, event.FinalWeight)
		}

		// Case 3: no clamping (within bounds)
		clamped, event = m.constrainWeight("agent-ok", 1.5)
		if clamped != 1.5 {
			t.Errorf("Expected unchanged 1.5, got %f", clamped)
		}
		if event != nil {
			t.Errorf("Expected nil ClampingEvent when no clamping, got %+v", event)
		}
	})

	t.Run("PerformDailyAdjustment_ClampingEvents", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_clamp.json")
		defer os.Remove("/tmp/test_clamp.json")

		// Seed agents: one will get huge multiplier → clamped to max
		seedAgent(m, "top-performer", "tech", "sector", 2.4)
		seedAgent(m, "bottom-performer", "val", "style", 0.35)

		// Record enough outcomes to trigger cooldown bypass (20+ outcomes)
		for range 25 {
			m.RecordOutcome("top-performer", 0.02, true)
			m.RecordOutcome("bottom-performer", -0.02, false)
		}

		adjustments, events := m.PerformDailyAdjustment()
		if len(adjustments) == 0 {
			t.Fatal("Expected some adjustments")
		}

		// Verify events are populated when clamping occurs
		for _, e := range events {
			if e.AgentID == "" {
				t.Error("ClampingEvent has empty AgentID")
			}
			if e.Boundary != "min" && e.Boundary != "max" {
				t.Errorf("Invalid boundary '%s', expected 'min' or 'max'", e.Boundary)
			}
			if e.RawWeight == e.FinalWeight {
				t.Error("ClampingEvent raw_weight equals final_weight, no clamping occurred")
			}
			if e.Timestamp.IsZero() {
				t.Error("ClampingEvent has zero timestamp")
			}
			t.Logf("ClampingEvent: agent=%s raw=%.3f final=%.3f boundary=%s",
				e.AgentID, e.RawWeight, e.FinalWeight, e.Boundary)
		}

		// All weights must stay in bounds after adjustment
		for id, w := range m.GetAllWeights() {
			if w < DarwinianWeightMin || w > DarwinianWeightMax {
				t.Errorf("Agent %s weight %.3f outside bounds [%.1f, %.1f]",
					id, w, DarwinianWeightMin, DarwinianWeightMax)
			}
		}
	})

	t.Run("ApplyDarwinianWeightsWithEvents", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		seedAgent(m, "agent_001", "tech", "sector", 2.0) // High weight
		seedAgent(m, "agent_002", "val", "style", 0.5)   // Low weight

		recs := []domain.Recommendation{
			{Agent: "agent_001", Symbol: "2330.TW", Conviction: 70},
			{Agent: "agent_002", Symbol: "2881.TW", Conviction: 80},
		}

		weighted, _ := m.ApplyDarwinianWeightsWithEvents(recs)

		// agent_001 has high weight (2.0) so should boost conviction
		// agent_002 has low weight (0.5) so should reduce conviction
		for _, w := range weighted {
			if w.Agent == "agent_001" && w.Conviction <= 70 {
				t.Errorf("Expected boosted conviction for high-weight agent, got %d", w.Conviction)
			}
			if w.Agent == "agent_002" && w.Conviction >= 80 {
				t.Errorf("Expected reduced conviction for low-weight agent, got %d", w.Conviction)
			}
		}
	})

	t.Run("Persistence", func(t *testing.T) {
		testFile := "/tmp/test_darwinian_weights.json"
		defer os.Remove(testFile)

		m := NewDarwinianWeightManager(testFile)
		seedAgent(m, "agent_001", "tech", "sector", 1.5)

		// Add some returns
		for range 10 {
			m.RecordOutcome("agent_001", 0.01, true)
		}

		// Save
		err := m.Save()
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Create new manager and load
		m2 := NewDarwinianWeightManager(testFile)
		err = m2.Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		// Verify data loaded
		weight := m2.GetWeight("agent_001")
		if math.Abs(weight-1.5) > 0.001 {
			t.Errorf("Expected weight 1.5, got %f", weight)
		}

		data, ok := m2.GetAgentWeightData("agent_001")
		if !ok {
			t.Fatal("Expected agent data after load")
		}
		if data.TotalSignals != 10 {
			t.Errorf("Expected 10 signals after load, got %d", data.TotalSignals)
		}
	})

	t.Run("TopPerformers", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		// Create agents with varying performance
		for i := 1; i <= 10; i++ {
			agentID := fmt.Sprintf("agent_%03d", i)
			seedAgent(m, agentID, "test", "sector", 1.0)

			// Different performance levels
			ret := float64(i) * 0.1 / 30
			for range 30 {
				m.RecordOutcome(agentID, ret, true)
			}
		}

		top := m.GetTopPerformers(3)

		if len(top) != 3 {
			t.Errorf("Expected 3 top performers, got %d", len(top))
		}
	})

	t.Run("ResetAndRemove", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")
		seedAgent(m, "agent_001", "tech", "sector", 1.8)

		m.Reset()

		w := m.GetWeight("agent_001")
		if w != DarwinianNeutralWeight {
			t.Errorf("Expected neutral weight after reset, got %f", w)
		}

		m.RemoveAgent("agent_001")
		allWeights := m.GetAllWeights()
		if _, exists := allWeights["agent_001"]; exists {
			t.Error("Agent should be removed")
		}
	})
}

func TestDarwinianWeightManagerWithParameters(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw.json")
	params := DefaultRuntimeParameters()
	params.Darwinian.LookbackDays = 30
	result := m.WithParameters(params)
	if result != m {
		t.Error("expected WithParameters to return the same manager")
	}
	if m.lookbackDays != 30 {
		t.Errorf("expected lookbackDays 30, got %d", m.lookbackDays)
	}
}

func TestDarwinianWeightManagerGetBottomPerformers(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw.json")
	seedAgent(m, "agent_001", "tech", "sector", 1.5)
	seedAgent(m, "agent_002", "val", "style", 0.8)

	for range 10 {
		m.RecordOutcome("agent_001", 0.02, true)
		m.RecordOutcome("agent_002", -0.01, false)
	}

	bottom := m.GetBottomPerformers(1)
	if len(bottom) != 1 {
		t.Fatalf("expected 1 bottom performer, got %d", len(bottom))
	}
	if bottom[0].AgentID == "" {
		t.Error("bottom performer AgentID should not be empty")
	}
}

func TestDarwinianWeightManagerGetAllAgentWeightData(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw.json")
	seedAgent(m, "agent_001", "tech", "sector", 1.5)
	seedAgent(m, "agent_002", "val", "style", 0.8)

	data := m.GetAllAgentWeightData()
	if len(data) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(data))
	}
	found := false
	for _, d := range data {
		if d.AgentID == "agent_001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected agent_001 in data")
	}
}

func TestDarwinianWeightReport(t *testing.T) {
	t.Run("GenerateReport", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		seedAgent(m, "agent_001", "tech", "sector", 1.5)
		seedAgent(m, "agent_002", "val", "style", 0.8)
		seedAgent(m, "agent_003", "growth", "sector", 2.2)

		for range 10 {
			m.RecordOutcome("agent_001", 0.02, true)
			m.RecordOutcome("agent_002", -0.01, false)
			m.RecordOutcome("agent_003", 0.03, true)
		}

		report := m.GenerateReport()

		if report.TotalAgents != 3 {
			t.Errorf("Expected 3 agents in report, got %d", report.TotalAgents)
		}

		if report.Summary == "" {
			t.Error("Expected non-empty summary")
		}

		avgWeight := m.GetAverageWeight()
		if avgWeight <= 0 {
			t.Error("Expected positive average weight")
		}
	})
}

func TestApplyDarwinianWeightsWithEvents_PreservesAllFields(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw_preserve.json")

	input := domain.Recommendation{
		Agent: "test_agent", Skill: "momentum", Layer: domain.LayerStyle, Symbol: "2330.TW",
		Side: domain.SideBuy, Conviction: 80, TargetPrice: 650, StopLossPrice: 580,
		Reason: "strong signal", ReasoningChain: []string{"step1"}, SupportingEvents: []string{"ev1"},
		FactorScores:        domain.FactorScores{Momentum: 0.85, Total: 0.72},
		ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 60, Final: 80},
	}
	seedAgent(m, "test_agent", "momentum", "style", 1.0)

	weighted, events := m.ApplyDarwinianWeightsWithEvents([]domain.Recommendation{input})
	if len(weighted) != 1 {
		t.Fatalf("expected 1 result, got %d", len(weighted))
	}
	out := weighted[0]
	if out.TargetPrice != input.TargetPrice {
		t.Errorf("TargetPrice: got %f, want %f", out.TargetPrice, input.TargetPrice)
	}
	if out.StopLossPrice != input.StopLossPrice {
		t.Errorf("StopLossPrice: got %f, want %f", out.StopLossPrice, input.StopLossPrice)
	}
	if out.ConvictionBreakdown == nil || out.ConvictionBreakdown.Final != input.ConvictionBreakdown.Final {
		t.Error("ConvictionBreakdown not preserved")
	}
	if out.FactorScores.Total != input.FactorScores.Total {
		t.Errorf("FactorScores.Total: got %f, want %f", out.FactorScores.Total, input.FactorScores.Total)
	}
	if out.Layer != input.Layer {
		t.Errorf("Layer: got %q, want %q", out.Layer, input.Layer)
	}
	if out.Conviction <= 0 {
		t.Error("Conviction should be positive")
	}
	if len(events) > 0 {
		t.Errorf("expected 0 clamping events with weight 1.0, got %d", len(events))
	}
}

func TestDarwinianWeightManager_WithEventBus(t *testing.T) {
	bus := &eventbus.ChannelEventBus{}
	m := NewDarwinianWeightManager("/tmp/test_dw_eb.json").WithEventBus(bus)
	if m == nil {
		t.Fatal("WithEventBus returned nil")
	}
	report := m.GenerateReport()
	if report.TotalAgents != 0 {
		t.Errorf("expected 0 agents, got %d", report.TotalAgents)
	}
}

func TestDarwinianWeightManager_SaveReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	m := NewDarwinianWeightManager("/tmp/test_dw_sr.json")
	_ = m.SaveReport(path)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected report file at %s: %v", path, err)
	}
}

func TestDarwinianWeightManager_ResetAgent(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw_ra.json")
	seedAgent(m, "agent_001", "tech", "sector", 2.0)
	m.ResetAgent("agent_001")
	w := m.GetWeight("agent_001")
	if w != DarwinianNeutralWeight {
		t.Errorf("expected neutral weight after reset, got %f", w)
	}
}

// TestApplyDarwinianWeightsWithEvents_ConvictionSteps verifies that
// ApplyDarwinianWeightsWithEvents creates ConvictionSteps for weight_adjust
// and clamping when weight differs from 1.0.
func TestApplyDarwinianWeightsWithEvents_ConvictionSteps(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw_cs.json")

	input := domain.Recommendation{
		Agent: "test_agent", Skill: "momentum", Layer: domain.LayerStyle, Symbol: "2330.TW",
		Side: domain.SideBuy, Conviction: 80,
		ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 60, Final: 80},
	}
	seedAgent(m, "test_agent", "momentum", "style", 0.5)

	weighted, events := m.ApplyDarwinianWeightsWithEvents([]domain.Recommendation{input})
	if len(weighted) == 0 {
		t.Fatal("expected weighted recommendations")
	}
	out := weighted[0]
	if out.ConvictionBreakdown == nil {
		t.Fatal("expected ConvictionBreakdown to be preserved")
	}

	var hasWeightAdj bool
	for _, step := range out.ConvictionBreakdown.Steps {
		if step.Rule == "modulator:darwinian:weight_adjust" {
			hasWeightAdj = true
			if step.Source != "config" {
				t.Errorf("expected weight_adjust source=config, got %q", step.Source)
			}
			if step.ParamValue == "" {
				t.Error("expected weight_adjust ParamValue to be non-empty")
			}
		}
	}
	if !hasWeightAdj {
		t.Error("expected at least one 'modulator:darwinian:weight_adjust' ConvictionStep with weight != 1.0")
	}
	if len(events) > 0 && len(out.ConvictionBreakdown.Steps) > 0 {
		t.Logf("created %d clamping events and %d ConvictionSteps with weight=0.5", len(events), len(out.ConvictionBreakdown.Steps))
	}
}

func TestLoadThenInitializeFromRegistry(t *testing.T) {
	// Setup: Create a saved weights file with agent_saved_01 (weight 1.5) and agent_saved_02 (weight 2.0)
	testFile := "/tmp/test_darwinian_load_init.json"
	defer os.Remove(testFile)

	m1 := NewDarwinianWeightManager(testFile)
	m1.WithParameters(DefaultRuntimeParameters())
	seedAgent(m1, "agent_saved_01", "tech", "sector", 1.5)
	seedAgent(m1, "agent_saved_02", "growth", "style", 2.0)
	if err := m1.Save(); err != nil {
		t.Fatalf("Setup Save failed: %v", err)
	}

	// Test: Create new manager, Load() first, then InitializeFromRegistry()
	m2 := NewDarwinianWeightManager(testFile)
	m2.WithParameters(DefaultRuntimeParameters())
	if err := m2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Registry has agent_saved_02 (same as saved) and agent_new_03 (brand new)
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent_saved_02", Skill: "growth", Layer: domain.LayerStyle, Enabled: true},
			{ID: "agent_new_03", Skill: "value", Layer: domain.LayerSector, Enabled: true},
		},
	}
	m2.InitializeFromRegistry(registry)

	// agent_saved_01: from file only, not in registry — should retain saved weight
	w1 := m2.GetWeight("agent_saved_01")
	if math.Abs(w1-1.5) > 0.001 {
		t.Errorf("agent_saved_01: expected weight 1.5 (preserved from file), got %f", w1)
	}

	// agent_saved_02: in both file and registry — should retain saved weight, not be re-initialized to neutral
	w2 := m2.GetWeight("agent_saved_02")
	if math.Abs(w2-2.0) > 0.001 {
		t.Errorf("agent_saved_02: expected weight 2.0 (preserved from file, NOT overwritten), got %f", w2)
	}

	// agent_new_03: only in registry — should be initialized with neutral weight
	w3 := m2.GetWeight("agent_new_03")
	if math.Abs(w3-DarwinianNeutralWeight) > 0.001 {
		t.Errorf("agent_new_03: expected neutral weight %f (newly initialized), got %f", DarwinianNeutralWeight, w3)
	}

	// agent_new_03 should be retrievable with full data
	data, ok := m2.GetAgentWeightData("agent_new_03")
	if !ok {
		t.Fatal("agent_new_03: expected to exist after InitializeFromRegistry")
	}
	if data.TotalSignals != 0 || data.WinCount != 0 || data.LossCount != 0 {
		t.Error("agent_new_03: expected zero signals for newly initialized agent")
	}
}

// TestRecordOutcomeAt_PerDayAggregation verifies Fix1: RecordOutcomeAt
// aggregates all outcomes of the same calendar day into a single daily mean
// entry, so a multi-recommendation day contributes exactly one window entry.
func TestRecordOutcomeAt_PerDayAggregation(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw_agg.json")
	seedAgent(m, "agent_001", "tech", "sector", 1.0)

	day1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)

	// Day 1: 4 outcomes (mean = (0.02+0.04+0.00+0.02)/4 = 0.02)
	m.RecordOutcomeAt("agent_001", 0.02, true, day1)
	m.RecordOutcomeAt("agent_001", 0.04, true, day1)
	m.RecordOutcomeAt("agent_001", 0.00, false, day1)
	m.RecordOutcomeAt("agent_001", 0.02, true, day1)

	data, _ := m.GetAgentWeightData("agent_001")
	if len(data.DailyReturns) != 0 {
		t.Fatalf("day 1 still in progress: expected 0 completed daily entries, got %d", len(data.DailyReturns))
	}
	if data.TotalSignals != 4 {
		t.Errorf("expected 4 signals, got %d", data.TotalSignals)
	}

	// Day 2: first outcome flushes day 1's mean (0.02) into the window.
	m.RecordOutcomeAt("agent_001", 0.03, true, day2)

	data, _ = m.GetAgentWeightData("agent_001")
	if len(data.DailyReturns) != 1 {
		t.Fatalf("expected 1 completed daily entry after day 2 starts, got %d: %v", len(data.DailyReturns), data.DailyReturns)
	}
	if math.Abs(data.DailyReturns[0]-0.02) > 1e-12 {
		t.Errorf("expected day-1 mean 0.02 in window, got %f", data.DailyReturns[0])
	}

	// Day 3: flushes day 2's single outcome (0.03).
	m.RecordOutcomeAt("agent_001", -0.01, false, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))

	data, _ = m.GetAgentWeightData("agent_001")
	if len(data.DailyReturns) != 2 {
		t.Fatalf("expected 2 completed daily entries, got %d: %v", len(data.DailyReturns), data.DailyReturns)
	}
	if math.Abs(data.DailyReturns[1]-0.03) > 1e-12 {
		t.Errorf("expected day-2 mean 0.03 in window, got %f", data.DailyReturns[1])
	}
}

// TestRecordOutcomeAt_MultiRecPerDaySharpeSanity verifies the A4 acceptance
// criteria: a realistic multi-recommendation/day agent gets a Sharpe in a
// sane range (|sharpe| <= maxSharpeMagnitude), never a 20+ / 500+ outlier.
func TestRecordOutcomeAt_MultiRecPerDaySharpeSanity(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw_sane.json")
	seedAgent(m, "agent_001", "tech", "sector", 1.0)

	// 40 trading days x 50 recommendations/day. Daily means vary around a
	// positive base (12 distinct day levels) so the daily window is not
	// degenerate; per-rec noise keeps intra-day dispersion realistic.
	dayBases := []float64{0.008, 0.004, 0.012, -0.002, 0.006, 0.010, 0.002, 0.014, -0.004, 0.005, 0.009, 0.007}
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for d := 0; d < 40; d++ {
		for r := 0; r < 50; r++ {
			ret := dayBases[d%len(dayBases)] + 0.002*float64(r%5-2)
			m.RecordOutcomeAt("agent_001", ret, ret > 0, day)
		}
		day = day.Add(24 * time.Hour)
	}

	data, _ := m.GetAgentWeightData("agent_001")
	if len(data.DailyReturns) != m.lookbackDays {
		t.Fatalf("expected %d daily entries in window (one per day, not per rec), got %d", m.lookbackDays, len(data.DailyReturns))
	}
	if math.Abs(data.RollingSharpe) > maxSharpeMagnitude {
		t.Errorf("expected Sharpe within ±%v for daily-aggregated series, got %f", maxSharpeMagnitude, data.RollingSharpe)
	}
	// Non-annualized per-day Sharpe for this series should be modest (0..3).
	if data.RollingSharpe <= 0 || data.RollingSharpe > 3 {
		t.Errorf("expected sane positive Sharpe (0,3] for positive-mean daily series, got %f", data.RollingSharpe)
	}
}

// TestUpdateRollingMetrics_DegenerateWindowGuard verifies Fix3: a window with
// fewer than minUniqueReturnsForSharpe unique values yields Sharpe=0 instead
// of an exploding mean/std ratio.
func TestUpdateRollingMetrics_DegenerateWindowGuard(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw_guard.json")
	seedAgent(m, "agent_001", "tech", "sector", 1.0)

	// 30 entries but only 2 unique values — the historical degenerate case
	// (e.g. -0.0765 / -0.0723 alternating produced Sharpe -544 before the fix).
	for i := 0; i < 30; i++ {
		if i%2 == 0 {
			m.RecordOutcome("agent_001", -0.0765, false)
		} else {
			m.RecordOutcome("agent_001", -0.0723, false)
		}
	}

	data, _ := m.GetAgentWeightData("agent_001")
	if data.RollingSharpe != 0 {
		t.Errorf("expected Sharpe=0 for degenerate window (2 unique values), got %f", data.RollingSharpe)
	}
	if data.RollingVolatility != 0 {
		t.Errorf("expected Volatility=0 for degenerate window, got %f", data.RollingVolatility)
	}
}

// TestUpdateRollingMetrics_SharpeClip verifies Fix5: pathological-but-passable
// windows (>=8 unique values, tiny std) are clipped to ±maxSharpeMagnitude.
func TestUpdateRollingMetrics_SharpeClip(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_dw_clip.json")
	seedAgent(m, "agent_001", "tech", "sector", 1.0)

	// 8 unique values but nearly identical magnitudes -> tiny std -> huge mean/std.
	rets := []float64{0.0100, 0.0101, 0.0102, 0.0103, 0.0104, 0.0105, 0.0106, 0.0107, 0.0100, 0.0101}
	for i := 0; i < 30; i++ {
		r := rets[i%len(rets)]
		m.RecordOutcome("agent_001", r, true)
	}

	data, _ := m.GetAgentWeightData("agent_001")
	if math.Abs(data.RollingSharpe) > maxSharpeMagnitude {
		t.Errorf("expected Sharpe clipped to ±%v, got %f", maxSharpeMagnitude, data.RollingSharpe)
	}
}

// TestPerformDailyAdjustment_ZeroSignalPenalty covers B3 penalty 1: an agent
// with zero signals for ZeroSignalPenaltyAfterDays (14d default) is forced
// into the bottom tier AND receives the extra ZeroSignalPenaltyMultiplier,
// while a fresh zero-signal agent (recent LastUpdatedAt) is not extra-penalized.
func TestPerformDailyAdjustment_ZeroSignalPenalty(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_zero_signal.json")

	// 3 agents, n=3 -> top=1, middle=1, bottom=1:
	//   performer: positive Sharpe (top)
	//   silent_fresh: zero signals, updated now (middle, Sharpe 0)
	//   silent_stale: zero signals, updated 20 days ago (forced bottom)
	seedAgent(m, "performer", "tech", "sector", 1.0)
	seedAgent(m, "silent_fresh", "value", "style", 1.0)
	seedAgent(m, "silent_stale", "macro", "sector", 1.0)
	for i := 0; i < 60; i++ {
		r := 0.01 + []float64{0.02, -0.01, 0.015, -0.005, 0.025, -0.02, 0.01, -0.015, 0.005, -0.025}[i%10]
		m.RecordOutcome("performer", r, r > 0)
	}
	m.mu.Lock()
	m.weights["silent_stale"].LastUpdatedAt = time.Now().Add(-20 * 24 * time.Hour)
	m.mu.Unlock()

	_, _ = m.PerformDailyAdjustment()

	// silent_stale: 1.0 * BottomQuartileMultiplier(0.95) * ZeroSignalPenaltyMultiplier(0.9) = 0.855
	wStale, ok := m.GetAgentWeightData("silent_stale")
	if !ok {
		t.Fatal("silent_stale missing")
	}
	if math.Abs(wStale.Weight-0.855) > 0.001 {
		t.Errorf("expected silent_stale weight 0.855 (bottom + zero-signal penalty), got %f", wStale.Weight)
	}
	// silent_fresh: middle tier with HitRate 0 -> pre-existing
	// MiddleTierCutMultiplier (0.98) applies, but NOT the zero-signal penalty
	// (0.855): the stale gate (ZeroSignalPenaltyAfterDays) is what matters.
	wFresh, ok := m.GetAgentWeightData("silent_fresh")
	if !ok {
		t.Fatal("silent_fresh missing")
	}
	if math.Abs(wFresh.Weight-0.98) > 0.001 {
		t.Errorf("expected silent_fresh weight 0.98 (middle cut only, no zero-signal penalty), got %f", wFresh.Weight)
	}
	if wFresh.Weight <= wStale.Weight {
		t.Errorf("fresh zero-signal agent (%f) must be penalized less than stale zero-signal agent (%f)", wFresh.Weight, wStale.Weight)
	}
	// DefaultNeutralWeight must expose the configured neutral reference (1.0).
	if neutral := m.DefaultNeutralWeight(); math.Abs(neutral-1.0) > 0.001 {
		t.Errorf("expected DefaultNeutralWeight 1.0, got %f", neutral)
	}
}

// TestPerformDailyAdjustment_LossPenaltyDeepensBottomCut covers B3 penalty 2:
// bottom-tier agents with negative Sharpe and >=30 signals receive the extra
// LossPenaltyMultiplier; agents with fewer than 30 signals do not (their
// negative trend could be noise, and their Sharpe is gated to 0 anyway).
func TestPerformDailyAdjustment_LossPenaltyDeepensBottomCut(t *testing.T) {
	m := NewDarwinianWeightManager("/tmp/test_loss_penalty.json")

	// Sort (desc Sharpe): performer(>0) > losing_poor(Sharpe 0, <30 samples) > losing_rich(<0)
	// n=3 -> top=1 (performer), middle=1 (losing_poor, multiplier 1.0), bottom=1 (losing_rich).
	seedAgent(m, "performer", "tech", "sector", 1.0)
	seedAgent(m, "losing_rich", "value", "style", 1.0)
	seedAgent(m, "losing_poor", "macro", "sector", 1.0)
	for i := 0; i < 60; i++ {
		pr := 0.01 + []float64{0.02, -0.01, 0.015, -0.005, 0.025, -0.02, 0.01, -0.015, 0.005, -0.025}[i%10]
		nr := -0.01 + []float64{-0.02, 0.01, -0.015, 0.005, -0.025, 0.02, -0.01, 0.015, -0.005, 0.025}[i%10]
		m.RecordOutcome("performer", pr, pr > 0)
		m.RecordOutcome("losing_rich", nr, false)
	}
	for i := 0; i < 20; i++ {
		nr := -0.01 + []float64{-0.02, 0.01, -0.015, 0.005, -0.025, 0.02, -0.01, 0.015, -0.005, 0.025}[i%10]
		m.RecordOutcome("losing_poor", nr, false)
	}

	_, _ = m.PerformDailyAdjustment()

	// losing_rich: 1.0 * BottomQuartileMultiplier(0.95) * LossPenaltyMultiplier(0.9) = 0.855
	wRich, ok := m.GetAgentWeightData("losing_rich")
	if !ok {
		t.Fatal("losing_rich missing")
	}
	if math.Abs(wRich.Weight-0.855) > 0.001 {
		t.Errorf("expected losing_rich weight 0.855 (bottom + loss penalty), got %f", wRich.Weight)
	}
	// losing_poor: 20 signals < 30 -> no loss penalty. It sits in the middle
	// tier with HitRate 0, so only the pre-existing MiddleTierCutMultiplier
	// (0.98) applies — the extra 0.855 loss-penalty cut is reserved for
	// signal-rich (>=30) negative-Sharpe agents.
	wPoor, ok := m.GetAgentWeightData("losing_poor")
	if !ok {
		t.Fatal("losing_poor missing")
	}
	if math.Abs(wPoor.Weight-0.98) > 0.001 {
		t.Errorf("expected losing_poor weight 0.98 (middle cut only, no loss penalty), got %f", wPoor.Weight)
	}
	if wPoor.Weight <= wRich.Weight {
		t.Errorf("low-signal loser (%f) must be penalized less than signal-rich loser (%f)", wPoor.Weight, wRich.Weight)
	}
}

// TestPerformDailyAdjustment_WeightChangeAlert covers B3 penalty 3: any agent
// whose |daily weight change| exceeds WeightChangeAlertThreshold triggers a
// warning health alert on the event bus (and a Warn log).
func TestPerformDailyAdjustment_WeightChangeAlert(t *testing.T) {
	bus := eventbus.NewChannelEventBus(64)
	defer bus.Close()

	params := DefaultRuntimeParameters()
	params.Darwinian.WeightChangeAlertThreshold = 0.01 // any meaningful move alerts

	m := NewDarwinianWeightManager("/tmp/test_alert.json").WithParameters(params).WithEventBus(bus)

	seedAgent(m, "top", "tech", "sector", 1.5)
	seedAgent(m, "bottom", "value", "style", 1.0)
	for i := 0; i < 60; i++ {
		pr := 0.01 + []float64{0.02, -0.01, 0.015, -0.005, 0.025, -0.02, 0.01, -0.015, 0.005, -0.025}[i%10]
		nr := -0.01 + []float64{-0.02, 0.01, -0.015, 0.005, -0.025, 0.02, -0.01, 0.015, -0.005, 0.025}[i%10]
		m.RecordOutcome("top", pr, pr > 0)
		m.RecordOutcome("bottom", nr, false)
	}

	var alerts []eventbus.HealthAlertPayload
	var mu sync.Mutex
	bus.SubscribeAll(func(ctx context.Context, event eventbus.BusEvent) error {
		if event.Type == eventbus.EventHealthAlert {
			if p, ok := event.Payload.(eventbus.HealthAlertPayload); ok {
				mu.Lock()
				alerts = append(alerts, p)
				mu.Unlock()
			}
		}
		return nil
	})

	_, _ = m.PerformDailyAdjustment()

	// ChannelEventBus delivers asynchronously; give the worker a moment.
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(alerts) == 0 {
		t.Fatal("expected at least one weight-change health alert on the event bus")
	}
	for _, a := range alerts {
		if a.Category != "darwinian_weights" {
			t.Errorf("expected category darwinian_weights, got %s", a.Category)
		}
		if a.Severity != "warning" {
			t.Errorf("expected severity warning, got %s", a.Severity)
		}
		if math.Abs(a.Value) <= a.Threshold {
			t.Errorf("expected |value| %.4f > threshold %.4f", a.Value, a.Threshold)
		}
	}
}
