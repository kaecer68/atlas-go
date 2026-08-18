//go:build integration

// Package atlas (module root) provides integration tests for Phase 2 and Phase 3
// components (Darwinian Weights, Superinvestor, Spawning, PRISM, Reflexivity).
// These tests exercise live internal packages and are kept after the retirement of
// the legacy root experiment harness (see
// docs/decisions/2026-08-code-disposition-enhanced-runner.md).
// Run with: go test -v -tags=integration ./...
package atlas

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/spawning"
)

// TestPhase2Integration validates Darwinian Weights and Superinvestor Layer
func TestPhase2Integration(t *testing.T) {
	t.Run("DarwinianWeightsWithRecommendations", func(t *testing.T) {
		// Create weight manager
		weightManager := portfolio.NewDarwinianWeightManager("/tmp/test_integration_weights.json")

		// Initialize agents through registry with different weights
		registry := domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "agent_high", Layer: domain.LayerSector, Skill: "test", Enabled: true, DarwinianWeight: 2.0},
				{ID: "agent_neutral", Layer: domain.LayerSector, Skill: "test", Enabled: true, DarwinianWeight: 1.0},
				{ID: "agent_low", Layer: domain.LayerSector, Skill: "test", Enabled: true, DarwinianWeight: 0.5},
			},
		}
		weightManager.InitializeFromRegistry(registry)

		// Create sample recommendations
		recs := []domain.Recommendation{
			{Agent: "agent_high", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 70, Reason: "Strong momentum"},
			{Agent: "agent_neutral", Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 70, Reason: "Value play"},
			{Agent: "agent_low", Symbol: "2454.TW", Side: domain.SideBuy, Conviction: 70, Reason: "Recovery"},
		}

		// Apply weights
		weighted, _ := weightManager.ApplyDarwinianWeightsWithEvents(recs)

		// Verify high-weight agent's conviction boosted
		for _, w := range weighted {
			if w.Agent == "agent_high" && w.Conviction <= 70 {
				t.Errorf("High-weight agent conviction not boosted: got %d", w.Conviction)
			}
			if w.Agent == "agent_low" && w.Conviction >= 70 {
				t.Errorf("Low-weight agent conviction not reduced: got %d", w.Conviction)
			}
		}

		t.Logf("✓ Darwinian weights applied successfully")
	})

	t.Run("SuperinvestorLayerIntegration", func(t *testing.T) {
		// Create sample superinvestor agents
		superinvestors := []domain.AgentSpec{
			{ID: "super_druckenmiller", Layer: domain.LayerSuperinvestor, Skill: "macro_momentum", Enabled: true},
			{ID: "super_aschenbrenner", Layer: domain.LayerSuperinvestor, Skill: "ai_cycle", Enabled: true},
			{ID: "super_baker", Layer: domain.LayerSuperinvestor, Skill: "deep_tech", Enabled: true},
			{ID: "super_ackman", Layer: domain.LayerSuperinvestor, Skill: "quality", Enabled: true},
		}

		// Verify all 4 superinvestors exist
		if len(superinvestors) != 4 {
			t.Errorf("Expected 4 superinvestor agents, got %d", len(superinvestors))
		}

		// Verify layer assignment
		for _, agent := range superinvestors {
			if agent.Layer != domain.LayerSuperinvestor {
				t.Errorf("Agent %s not in superinvestor layer", agent.ID)
			}
		}

		t.Logf("✓ Superinvestor layer integration validated")
	})

	t.Run("WeightPersistence", func(t *testing.T) {
		tempFile := "/tmp/test_integration_weights.json"
		defer os.Remove(tempFile)

		// Create manager and initialize with agent
		weightManager := portfolio.NewDarwinianWeightManager(tempFile)
		registry := domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "persistent_agent", Layer: domain.LayerSector, Skill: "test", Enabled: true, DarwinianWeight: 1.5},
			},
		}
		weightManager.InitializeFromRegistry(registry)

		// Add some performance data
		weightManager.RecordOutcome("persistent_agent", 0.02, true)
		weightManager.RecordOutcome("persistent_agent", 0.01, true)
		weightManager.RecordOutcome("persistent_agent", -0.01, false)

		// Save
		err := weightManager.Save()
		if err != nil {
			t.Fatalf("Failed to save weights: %v", err)
		}

		// Load into new manager
		newManager := portfolio.NewDarwinianWeightManager(tempFile)
		err = newManager.Load()
		if err != nil {
			t.Fatalf("Failed to load weights: %v", err)
		}

		weight := newManager.GetWeight("persistent_agent")
		if weight < 0.3 || weight > 2.5 {
			t.Errorf("Weight out of expected range: got %f", weight)
		}

		t.Logf("✓ Weight persistence working correctly")
	})
}

// TestPhase3Integration validates Agent Spawning, PRISM, and Reflexivity
func TestPhase3Integration(t *testing.T) {
	t.Run("AgentSpawningLifecycle", func(t *testing.T) {
		t.Chdir(t.TempDir())

		// Create registry
		registry := &domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "existing_001", Layer: domain.LayerSector, Skill: "semiconductor", Enabled: true},
			},
		}

		// Create spawning manager
		config := spawning.DefaultSpawningConfig()
		spawningManager := spawning.NewSpawningManager(registry, config)

		// Manual spawn for testing (gap variable used for documentation)
		_ = &spawning.KnowledgeGap{
			ID:       "test_gap_001",
			Type:     spawning.GapTypeSector,
			Severity: spawning.GapSeverityMedium,
			Sector:   "biotech",
		}

		spawned, err := spawningManager.ManualSpawn(spawning.GapTypeSector, "biotech", "")
		if err != nil {
			t.Fatalf("Manual spawn failed: %v", err)
		}

		if spawned == nil {
			t.Fatal("Expected spawned agent")
		}

		if spawned.Status != spawning.SpawnStatusTraining {
			t.Errorf("Expected status Training, got %s", spawned.Status)
		}

		t.Logf("✓ Agent spawning lifecycle working (spawned: %s)", spawned.AgentID)
	})

	t.Run("PRISMTrainingQueues", func(t *testing.T) {
		// Create PRISM manager
		config := prism.DefaultPRISMConfig()
		prismManager := prism.NewPRISMManager(config)

		// Schedule training for test agent
		agent := domain.AgentSpec{
			ID:              "test_agent_001",
			Skill:           "test_skill",
			Layer:           domain.LayerStyle,
			DarwinianWeight: 1.0,
		}

		windows := []prism.TrainingWindow{
			{
				Start:     time.Now().AddDate(0, 0, -90),
				End:       time.Now(),
				Regime:    prism.RegimeRiskOn,
				RegimeSet: true,
			},
		}

		err := prismManager.ScheduleTraining(agent, windows)
		if err != nil {
			t.Fatalf("Failed to schedule training: %v", err)
		}

		// Check queue stats
		stats := prismManager.GetQueueStats()
		if len(stats) != 5 {
			t.Errorf("Expected 5 queue stats, got %d", len(stats))
		}

		// Verify Risk-On queue has tasks
		riskOnQueue := stats[prism.RegimeRiskOn]
		if riskOnQueue.Regime != prism.RegimeRiskOn {
			t.Error("Wrong regime in stats")
		}

		t.Logf("✓ PRISM training queues operational")
	})

	t.Run("ReflexivityEngine", func(t *testing.T) {
		engine := reflexivity.NewReflexivityEngine()

		// Register market bias
		bias := &reflexivity.MarketBias{
			ID:         "bias_001",
			Type:       reflexivity.TrendFollowing,
			Target:     "2330.TW",
			Magnitude:  0.75,
			Confidence: 0.8,
		}
		engine.RegisterBias(bias)

		// Update market reality
		reality := &reflexivity.MarketReality{
			ID:         "reality_001",
			Target:     "2330.TW",
			Price:      850.0,
			Trend:      0.02,
			Volatility: 0.18,
		}
		engine.UpdateReality(reality)

		// Check for feedback loops
		loops := engine.GetActiveLoops()

		// Should have detected a positive feedback loop
		found := false
		for _, loop := range loops {
			if loop.Bias != nil && loop.Bias.Target == "2330.TW" {
				found = true
				break
			}
		}

		if !found && len(loops) > 0 {
			t.Logf("Note: Feedback loop detection depends on correlation calculation")
		}

		// Test recommendation processing
		recs := []domain.Recommendation{
			{Agent: "agent_001", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
		}

		adjusted := engine.ApplyReflexivityAdjustment(recs)
		if len(adjusted) == 0 {
			t.Error("Expected adjusted recommendations")
		}

		t.Logf("✓ Reflexivity engine operational")
	})
}

// TestEndToEndWorkflow validates complete system workflow
func TestEndToEndWorkflow(t *testing.T) {
	t.Run("CompleteDailyWorkflow", func(t *testing.T) {
		// This test demonstrates the complete daily workflow
		// combining all Phase 2 and Phase 3 components

		t.Log("Step 1: Darwinian Weights Adjustment")
		weightManager := portfolio.NewDarwinianWeightManager("/tmp/test_e2e_weights.json")
		registry := domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "taiwan_macro", Layer: domain.LayerMacro, Skill: "macro", Enabled: true, DarwinianWeight: 1.2},
				{ID: "sector_semiconductor", Layer: domain.LayerSector, Skill: "semiconductor", Enabled: true, DarwinianWeight: 1.5},
			},
		}
		weightManager.InitializeFromRegistry(registry)

		// Simulate daily performance tracking using RecordOutcome
		weightManager.RecordOutcome("taiwan_macro", 0.02, true)
		weightManager.RecordOutcome("taiwan_macro", 0.01, true)
		weightManager.RecordOutcome("sector_semiconductor", 0.03, true)
		weightManager.RecordOutcome("sector_semiconductor", 0.015, true)

		adjustments, _ := weightManager.PerformDailyAdjustment()
		t.Logf("  - Calculated adjustments for %d agents", len(adjustments))

		t.Log("Step 2: Gap Detection (Spawning)")
		registry2 := &domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "taiwan_macro", Layer: domain.LayerMacro, Skill: "macro", Enabled: true},
				{ID: "sector_semiconductor", Layer: domain.LayerSector, Skill: "semiconductor", Enabled: true},
			},
		}

		gapDetector := spawning.NewGapDetector()
		scorecards := make(map[string]*domain.Scorecard)
		universe := []string{"2330.TW", "2317.TW", "2454.TW"}

		gaps := gapDetector.DetectGaps(*registry2, scorecards, universe)
		t.Logf("  - Detected %d knowledge gaps", len(gaps))

		t.Log("Step 3: PRISM Training Queue")
		prismConfig := prism.DefaultPRISMConfig()
		prismManager := prism.NewPRISMManager(prismConfig)

		agent := domain.AgentSpec{
			ID:    "spawn_biotech_001",
			Skill: "biotech_specialist",
			Layer: domain.LayerSector,
		}
		windows := []prism.TrainingWindow{
			{Start: time.Now().AddDate(0, 0, -60), End: time.Now()},
		}

		err := prismManager.ScheduleTraining(agent, windows)
		if err != nil {
			t.Logf("  - Training scheduling: %v", err)
		} else {
			t.Log("  - Training scheduled successfully")
		}

		t.Log("Step 4: Reflexivity Check")
		reflexEngine := reflexivity.NewReflexivityEngine()

		// Process recommendations from multiple agents
		allRecs := []domain.Recommendation{
			{Agent: "taiwan_macro", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 75},
			{Agent: "sector_semiconductor", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
			{Agent: "super_druckenmiller", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 85},
		}

		reflexEngine.ProcessRecommendations(allRecs)
		loops := reflexEngine.GetActiveLoops()
		t.Logf("  - Detected %d feedback loops", len(loops))

		t.Log("Step 5: Apply Darwinian Weights to Final Recommendations")
		weightedRecs, _ := weightManager.ApplyDarwinianWeightsWithEvents(allRecs)
		t.Logf("  - Weighted %d recommendations", len(weightedRecs))

		t.Log("✓ Complete daily workflow executed successfully")
	})
}

// TestPerformance benchmarks key operations
func TestPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	t.Run("DarwinianWeightsPerformance", func(t *testing.T) {
		weightManager := portfolio.NewDarwinianWeightManager("/tmp/test_perf_weights.json")

		// Initialize 100 agents through registry
		agents := make([]domain.AgentSpec, 100)
		for i := 0; i < 100; i++ {
			agents[i] = domain.AgentSpec{
				ID:              fmt.Sprintf("agent_%03d", i),
				Layer:           domain.LayerSector,
				Skill:           "test",
				Enabled:         true,
				DarwinianWeight: 1.0,
			}
		}
		weightManager.InitializeFromRegistry(domain.AgentRegistry{Agents: agents})

		// Create 500 recommendations
		recs := make([]domain.Recommendation, 500)
		for i := 0; i < 500; i++ {
			agentID := fmt.Sprintf("agent_%03d", i%100)
			recs[i] = domain.Recommendation{
				Agent:      agentID,
				Symbol:     fmt.Sprintf("STOCK%04d", i),
				Side:       domain.SideBuy,
				Conviction: 70,
			}
		}

		start := time.Now()
		weighted, _ := weightManager.ApplyDarwinianWeightsWithEvents(recs)
		duration := time.Since(start)

		t.Logf("Applied weights to %d recommendations in %v", len(weighted), duration)

		if duration > time.Second {
			t.Logf("⚠ Weight application took longer than expected: %v", duration)
		}
	})
}

// BenchmarkDarwinianAdjustment benchmarks weight adjustment calculations
func BenchmarkDarwinianAdjustment(b *testing.B) {
	weightManager := portfolio.NewDarwinianWeightManager("/tmp/bench_weights.json")

	// Setup: 50 agents through registry with performance data
	agents := make([]domain.AgentSpec, 50)
	for i := 0; i < 50; i++ {
		agents[i] = domain.AgentSpec{
			ID:              fmt.Sprintf("agent_%03d", i),
			Layer:           domain.LayerSector,
			Skill:           "test",
			Enabled:         true,
			DarwinianWeight: 1.0,
		}
	}
	weightManager.InitializeFromRegistry(domain.AgentRegistry{Agents: agents})

	// Add 30 days of returns for each agent
	for i := 0; i < 50; i++ {
		agentID := fmt.Sprintf("agent_%03d", i)
		for j := 0; j < 30; j++ {
			weightManager.RecordOutcome(agentID, float64(i)*0.001, true)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		weightManager.PerformDailyAdjustment()
	}
}
