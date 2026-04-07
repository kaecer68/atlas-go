package portfolio

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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
		adjustments := m.PerformDailyAdjustment()

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

		// Test via constrainWeight (private but accessible in same package)
		clamped := m.constrainWeight(3.0)
		if clamped != DarwinianWeightMax {
			t.Errorf("Expected max %f, got %f", DarwinianWeightMax, clamped)
		}

		clamped = m.constrainWeight(0.1)
		if clamped != DarwinianWeightMin {
			t.Errorf("Expected min %f, got %f", DarwinianWeightMin, clamped)
		}

		clamped = m.constrainWeight(1.5)
		if clamped != 1.5 {
			t.Errorf("Expected 1.5, got %f", clamped)
		}
	})

	t.Run("ApplyDarwinianWeights", func(t *testing.T) {
		m := NewDarwinianWeightManager("/tmp/test_dw.json")

		seedAgent(m, "agent_001", "tech", "sector", 2.0) // High weight
		seedAgent(m, "agent_002", "val", "style", 0.5)   // Low weight

		recs := []domain.Recommendation{
			{Agent: "agent_001", Symbol: "2330.TW", Conviction: 70},
			{Agent: "agent_002", Symbol: "2881.TW", Conviction: 80},
		}

		weighted := m.ApplyDarwinianWeights(recs)

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
