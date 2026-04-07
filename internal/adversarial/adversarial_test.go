package adversarial

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestAdversarialTrainer(t *testing.T) {
	t.Run("NewAdversarialTrainer", func(t *testing.T) {
		config := DefaultAdversarialConfig()
		trainer := NewAdversarialTrainer(config)

		if trainer == nil {
			t.Fatal("Expected non-nil trainer")
		}

		if len(trainer.redTeam) != config.RedTeamSize {
			t.Errorf("Expected %d red agents, got %d", config.RedTeamSize, len(trainer.redTeam))
		}

		if len(trainer.blueTeam) != config.BlueTeamSize {
			t.Errorf("Expected %d blue agents, got %d", config.BlueTeamSize, len(trainer.blueTeam))
		}
	})

	t.Run("DefaultConfig", func(t *testing.T) {
		config := DefaultAdversarialConfig()

		if config.RedTeamSize != 5 {
			t.Errorf("Expected RedTeamSize 5, got %d", config.RedTeamSize)
		}

		if config.BlueTeamSize != 5 {
			t.Errorf("Expected BlueTeamSize 5, got %d", config.BlueTeamSize)
		}

		if config.TrainingCycles != 100 {
			t.Errorf("Expected TrainingCycles 100, got %d", config.TrainingCycles)
		}

		if !config.AdaptiveDifficulty {
			t.Error("Expected AdaptiveDifficulty to be true")
		}
	})

	t.Run("TeamTypes", func(t *testing.T) {
		if TeamRed != "red" {
			t.Errorf("Expected TeamRed to be 'red', got %s", TeamRed)
		}

		if TeamBlue != "blue" {
			t.Errorf("Expected TeamBlue to be 'blue', got %s", TeamBlue)
		}
	})

	t.Run("RunTraining", func(t *testing.T) {
		config := &AdversarialConfig{
			RedTeamSize:        3,
			BlueTeamSize:       3,
			BattleRounds:       5,
			TrainingCycles:     10,
			AdaptiveDifficulty: false,
		}
		trainer := NewAdversarialTrainer(config)

		summary := trainer.RunTraining()

		if summary == nil {
			t.Fatal("Expected non-nil summary")
		}

		if summary.Cycles != 10 {
			t.Errorf("Expected 10 cycles, got %d", summary.Cycles)
		}

		totalBattles := summary.RedWins + summary.BlueWins + summary.Draws
		if totalBattles == 0 {
			t.Error("Expected some battles to be recorded")
		}

		t.Logf("Training complete: Red=%d, Blue=%d, Draws=%d",
			summary.RedWins, summary.BlueWins, summary.Draws)
	})

	t.Run("StressTestAgent", func(t *testing.T) {
		config := DefaultAdversarialConfig()
		trainer := NewAdversarialTrainer(config)

		agent := domain.AgentSpec{
			ID:     "test_agent_001",
			Name:   "Test Agent",
			Layer:  domain.LayerSector,
			Skill:  "semiconductor",
			Enabled: true,
		}

		result := trainer.StressTestAgent("test_agent_001", agent)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}

		if result.AgentID != "test_agent_001" {
			t.Errorf("Expected AgentID test_agent_001, got %s", result.AgentID)
		}

		if len(result.Scenarios) == 0 {
			t.Error("Expected some scenario results")
		}

		t.Logf("Stress test complete: Score=%.2f, Passed=%v",
			result.OverallScore, result.Passed)
	})

	t.Run("GetVulnerabilities", func(t *testing.T) {
		config := DefaultAdversarialConfig()
		trainer := NewAdversarialTrainer(config)

		// Run some training to generate results
		trainer.RunTraining()

		vulns := trainer.GetVulnerabilities()

		// Vulnerabilities may or may not be detected depending on training outcomes
		t.Logf("Found %d vulnerabilities", len(vulns))

		for _, v := range vulns {
			t.Logf("Vulnerability: %s (severity: %d, occurrences: %d)",
				v.Type, v.Severity, v.Occurrences)
		}
	})

	t.Run("ScenarioTypes", func(t *testing.T) {
		scenarios := []ScenarioType{
			ScenarioFlashCrash,
			ScenarioLiquidityCrisis,
			ScenarioCorrelationSpike,
			ScenarioDisinformation,
			ScenarioFlashRally,
			ScenarioSectorRotation,
		}

		for _, s := range scenarios {
			if s == "" {
				t.Error("Scenario type should not be empty")
			}
		}
	})

	t.Run("GenerateReport", func(t *testing.T) {
		config := &AdversarialConfig{
			RedTeamSize:    2,
			BlueTeamSize:   2,
			TrainingCycles: 5,
		}
		trainer := NewAdversarialTrainer(config)
		trainer.RunTraining()

		report := trainer.GenerateReport()

		if report == nil {
			t.Fatal("Expected non-nil report")
		}

		if report.TotalBattles == 0 {
			t.Error("Expected some battles in report")
		}

		t.Logf("Report: %d battles, Red win rate: %.2f%%, Blue win rate: %.2f%%",
			report.TotalBattles, report.RedWinRate*100, report.BlueWinRate*100)
	})
}

func TestAdversarialAgent(t *testing.T) {
	t.Run("AgentCreation", func(t *testing.T) {
		agent := &AdversarialAgent{
			ID:            "red_001",
			Name:          "Red Attacker 1",
			Team:          TeamRed,
			Skill:         "market_manipulation",
			Effectiveness: 0.75,
			Adaptability:  0.6,
			Strategies:    []AttackStrategy{},
			LastActive:    time.Now(),
		}

		if agent.ID != "red_001" {
			t.Errorf("Expected ID red_001, got %s", agent.ID)
		}

		if agent.Team != TeamRed {
			t.Errorf("Expected team red, got %s", agent.Team)
		}
	})

	t.Run("AgentStrategies", func(t *testing.T) {
		strategies := []AttackStrategy{
			{ID: "str_1", Name: "Flash Crash", Type: ScenarioFlashCrash, SuccessRate: 0.3},
			{ID: "str_2", Name: "Liquidity Drain", Type: ScenarioLiquidityCrisis, SuccessRate: 0.25},
		}

		agent := &AdversarialAgent{
			ID:         "blue_001",
			Name:       "Blue Defender 1",
			Team:       TeamBlue,
			Strategies: strategies,
		}

		if len(agent.Strategies) != 2 {
			t.Errorf("Expected 2 strategies, got %d", len(agent.Strategies))
		}
	})
}

func TestBattleResult(t *testing.T) {
	t.Run("BattleResultCreation", func(t *testing.T) {
		result := &BattleResult{
			ID:        "battle_001",
			Timestamp: time.Now(),
			RedAgent:  "red_001",
			BlueAgent: "blue_001",
			Scenario:  "flash_crash",
			Winner:    TeamRed,
			RedScore:  7.5,
			BlueScore: 5.2,
			Duration:  120.5,
			Rounds:    10,
			KeyEvents: []BattleEvent{},
		}

		if result.Winner != TeamRed {
			t.Errorf("Expected winner to be red, got %s", result.Winner)
		}

		if result.RedScore <= result.BlueScore {
			t.Error("Red score should be higher than blue score when red wins")
		}
	})
}

func TestSeverityLevels(t *testing.T) {
	t.Run("SeverityValues", func(t *testing.T) {
		if SeverityLow != 1 {
			t.Errorf("Expected SeverityLow to be 1, got %d", SeverityLow)
		}

		if SeverityMedium != 2 {
			t.Errorf("Expected SeverityMedium to be 2, got %d", SeverityMedium)
		}

		if SeverityHigh != 3 {
			t.Errorf("Expected SeverityHigh to be 3, got %d", SeverityHigh)
		}

		if SeverityCritical != 4 {
			t.Errorf("Expected SeverityCritical to be 4, got %d", SeverityCritical)
		}
	})
}
