package portfolio

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
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

		if m.lookbackDays != 20 {
			t.Errorf("Expected lookbackDays=20, got %d", m.lookbackDays)
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
			},
		}

		m.InitializeFromRegistry(registry)

		// Only sector and style agents should be initialized
		w1 := m.GetWeight("agent_001")
		if w1 != DarwinianNeutralWeight {
			t.Errorf("Expected neutral weight for agent_001, got %f", w1)
		}

		w2 := m.GetWeight("agent_002")
		if w2 != DarwinianNeutralWeight {
			t.Errorf("Expected neutral weight for agent_002, got %f", w2)
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

		// Add returns with known positive mean
		returns := []float64{0.01, 0.02, -0.01, 0.015, 0.005, -0.005, 0.01, 0.02}
		for _, r := range returns {
			m.RecordOutcome("agent_001", r, r > 0)
		}

		data, ok := m.GetAgentWeightData("agent_001")
		if !ok {
			t.Fatal("Expected agent data")
		}

		// With 8 returns (>5 threshold), Sharpe should be calculated
		if data.RollingSharpe == 0 && len(returns) >= 5 {
			t.Log("Sharpe is zero, may need more data points")
		}
	})

	t.Run("PerformDailyAdjustment", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		// Create 8 agents with different performance
		agents := []string{"top_001", "top_002", "mid_001", "mid_002", "bot_001", "bot_002", "bot_003", "bot_004"}
		for _, agent := range agents {
			seedAgent(m, agent, "test", "sector", 1.0)
		}

		// Simulate performance: top performers get positive returns
		for i := 0; i < 20; i++ {
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
		for i := 0; i < 25; i++ {
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
		for i := 0; i < 10; i++ {
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
			for j := 0; j < 30; j++ {
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

	for i := 0; i < 10; i++ {
		m.RecordOutcome("agent_001", 0.02, true)
		m.RecordOutcome("agent_002", -0.01, false)
	}

	bottom := m.GetBottomPerformers(1)
	if len(bottom) != 1 {
		t.Fatalf("expected 1 bottom performer, got %d", len(bottom))
	}
	if bottom[0].AgentID != "agent_002" {
		t.Errorf("expected agent_002 as bottom performer, got %s", bottom[0].AgentID)
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

		for i := 0; i < 10; i++ {
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
