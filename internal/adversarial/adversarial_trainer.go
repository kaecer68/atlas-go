// Package adversarial implements red-team vs blue-team training for stress testing agents
// Red Team: Attacks, finds vulnerabilities, simulates extreme scenarios
// Blue Team: Defense, adapts, learns from attacks
package adversarial

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TeamType represents the adversarial team
type TeamType string

const (
	TeamRed  TeamType = "red"  // Attackers
	TeamBlue TeamType = "blue" // Defenders
)

// AdversarialScenario represents an attack or defense scenario
type AdversarialScenario struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Type            ScenarioType   `json:"type"`
	Team            TeamType       `json:"team"`
	Target          string         `json:"target"` // Agent or market target
	Severity        SeverityLevel  `json:"severity"`
	Parameters      map[string]any `json:"parameters"`
	ExpectedOutcome string         `json:"expected_outcome"`
	CreatedAt       time.Time      `json:"created_at"`
}

// ScenarioType defines different attack/defense patterns
type ScenarioType string

const (
	// Red Team scenarios
	ScenarioFlashCrash       ScenarioType = "flash_crash"
	ScenarioLiquidityCrisis  ScenarioType = "liquidity_crisis"
	ScenarioCorrelationSpike ScenarioType = "correlation_spike"
	ScenarioDisinformation   ScenarioType = "disinformation"
	ScenarioFlashRally       ScenarioType = "flash_rally"
	ScenarioSectorRotation   ScenarioType = "sector_rotation"

	// Blue Team scenarios
	ScenarioStabilityControl ScenarioType = "stability_control"
	ScenarioRecoveryMode     ScenarioType = "recovery_mode"
	ScenarioDiversification  ScenarioType = "diversification"
	ScenarioHedging          ScenarioType = "hedging"
)

// SeverityLevel indicates scenario intensity
type SeverityLevel int

const (
	SeverityLow      SeverityLevel = 1
	SeverityMedium   SeverityLevel = 2
	SeverityHigh     SeverityLevel = 3
	SeverityCritical SeverityLevel = 4
)

// AdversarialAgent represents a red or blue team participant
type AdversarialAgent struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Team          TeamType         `json:"team"`
	Skill         string           `json:"skill"`
	Effectiveness float64          `json:"effectiveness"`
	Adaptability  float64          `json:"adaptability"`
	WinCount      int              `json:"win_count"`
	LossCount     int              `json:"loss_count"`
	DrawCount     int              `json:"draw_count"`
	Strategies    []AttackStrategy `json:"strategies"`
	LastActive    time.Time        `json:"last_active"`
}

// AttackStrategy represents a specific attack/defense approach
type AttackStrategy struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Type        ScenarioType `json:"type"`
	SuccessRate float64      `json:"success_rate"`
	UsageCount  int          `json:"usage_count"`
}

// BattleResult captures outcome of a confrontation
type BattleResult struct {
	ID        string        `json:"id"`
	Timestamp time.Time     `json:"timestamp"`
	RedAgent  string        `json:"red_agent"`
	BlueAgent string        `json:"blue_agent"`
	Scenario  string        `json:"scenario"`
	Winner    TeamType      `json:"winner"`
	RedScore  float64       `json:"red_score"`
	BlueScore float64       `json:"blue_score"`
	Duration  float64       `json:"duration"`
	Rounds    int           `json:"rounds"`
	KeyEvents []BattleEvent `json:"key_events"`
}

// BattleEvent records significant moments in a battle
type BattleEvent struct {
	Round     int       `json:"round"`
	Timestamp time.Time `json:"timestamp"`
	Team      TeamType  `json:"team"`
	Event     string    `json:"event"`
	Impact    float64   `json:"impact"`
}

// AdversarialConfig configures the training system
type AdversarialConfig struct {
	RedTeamSize        int           `json:"red_team_size"`
	BlueTeamSize       int           `json:"blue_team_size"`
	BattleRounds       int           `json:"battle_rounds"`
	MatchDuration      time.Duration `json:"match_duration"`
	RestBetweenBattles time.Duration `json:"rest_between_battles"`
	TrainingCycles     int           `json:"training_cycles"`
	AdaptiveDifficulty bool          `json:"adaptive_difficulty"`
}

// DefaultAdversarialConfig returns standard configuration
func DefaultAdversarialConfig() *AdversarialConfig {
	return &AdversarialConfig{
		RedTeamSize:        5,
		BlueTeamSize:       5,
		BattleRounds:       10,
		MatchDuration:      30 * time.Minute,
		RestBetweenBattles: 5 * time.Minute,
		TrainingCycles:     100,
		AdaptiveDifficulty: true,
	}
}

// AdversarialTrainer manages red/blue team training
type AdversarialTrainer struct {
	redTeam      []*AdversarialAgent
	blueTeam     []*AdversarialAgent
	scenarios    []*AdversarialScenario
	results      []*BattleResult
	config       *AdversarialConfig
	currentCycle int
	stopChan     chan struct{}
	mu           sync.RWMutex
}

// NewAdversarialTrainer creates a new training system
func NewAdversarialTrainer(config *AdversarialConfig) *AdversarialTrainer {
	if config == nil {
		config = DefaultAdversarialConfig()
	}

	at := &AdversarialTrainer{
		redTeam:   make([]*AdversarialAgent, 0),
		blueTeam:  make([]*AdversarialAgent, 0),
		scenarios: make([]*AdversarialScenario, 0),
		results:   make([]*BattleResult, 0),
		config:    config,
		stopChan:  make(chan struct{}),
	}

	at.initializeTeams()
	at.initializeScenarios()

	return at
}

// initializeTeams creates red and blue team agents
func (at *AdversarialTrainer) initializeTeams() {
	// Red Team: Aggressive attackers
	redSkills := []string{"market_manipulation", "correlation_exploit", "liquidity_drain", "panic_induction"}
	for i := 0; i < at.config.RedTeamSize; i++ {
		agent := &AdversarialAgent{
			ID:            fmt.Sprintf("red_%03d", i+1),
			Name:          fmt.Sprintf("Red Attacker %d", i+1),
			Team:          TeamRed,
			Skill:         redSkills[i%len(redSkills)],
			Effectiveness: 0.6 + rand.Float64()*0.3,
			Adaptability:  0.5 + rand.Float64()*0.3,
			Strategies:    at.generateAttackStrategies(),
			LastActive:    time.Now(),
		}
		at.redTeam = append(at.redTeam, agent)
	}

	// Blue Team: Defensive adapters
	blueSkills := []string{"risk_mitigation", "stabilization", "diversification", "recovery"}
	for i := 0; i < at.config.BlueTeamSize; i++ {
		agent := &AdversarialAgent{
			ID:            fmt.Sprintf("blue_%03d", i+1),
			Name:          fmt.Sprintf("Blue Defender %d", i+1),
			Team:          TeamBlue,
			Skill:         blueSkills[i%len(blueSkills)],
			Effectiveness: 0.6 + rand.Float64()*0.3,
			Adaptability:  0.5 + rand.Float64()*0.3,
			Strategies:    at.generateDefenseStrategies(),
			LastActive:    time.Now(),
		}
		at.blueTeam = append(at.blueTeam, agent)
	}
}

// generateAttackStrategies creates red team strategies
func (at *AdversarialTrainer) generateAttackStrategies() []AttackStrategy {
	return []AttackStrategy{
		{ID: "str_flash_crash", Name: "Flash Crash Attack", Type: ScenarioFlashCrash, SuccessRate: 0.3},
		{ID: "str_liquidity", Name: "Liquidity Drain", Type: ScenarioLiquidityCrisis, SuccessRate: 0.25},
		{ID: "str_correlation", Name: "Correlation Spike", Type: ScenarioCorrelationSpike, SuccessRate: 0.35},
		{ID: "str_disinfo", Name: "Disinformation Campaign", Type: ScenarioDisinformation, SuccessRate: 0.2},
		{ID: "str_rotation", Name: "Forced Sector Rotation", Type: ScenarioSectorRotation, SuccessRate: 0.4},
	}
}

// generateDefenseStrategies creates blue team strategies
func (at *AdversarialTrainer) generateDefenseStrategies() []AttackStrategy {
	return []AttackStrategy{
		{ID: "def_stability", Name: "Stability Control", Type: ScenarioStabilityControl, SuccessRate: 0.5},
		{ID: "def_recovery", Name: "Recovery Mode", Type: ScenarioRecoveryMode, SuccessRate: 0.45},
		{ID: "def_diverse", Name: "Emergency Diversification", Type: ScenarioDiversification, SuccessRate: 0.4},
		{ID: "def_hedge", Name: "Dynamic Hedging", Type: ScenarioHedging, SuccessRate: 0.55},
	}
}

// initializeScenarios creates training scenarios
func (at *AdversarialTrainer) initializeScenarios() {
	// Red team attack scenarios
	redScenarios := []*AdversarialScenario{
		{
			ID:       "red_flash_crash_001",
			Name:     "Sudden 20% Market Drop",
			Type:     ScenarioFlashCrash,
			Team:     TeamRed,
			Severity: SeverityCritical,
			Parameters: map[string]any{
				"drop_percent":     0.20,
				"duration_minutes": 5,
				"recovery_time":    60,
			},
			ExpectedOutcome: "Test agent panic response and stop-loss discipline",
		},
		{
			ID:       "red_correlation_001",
			Name:     "Cross-Asset Correlation Spike",
			Type:     ScenarioCorrelationSpike,
			Team:     TeamRed,
			Severity: SeverityHigh,
			Parameters: map[string]any{
				"correlation_target": 0.95,
				"affected_sectors":   []string{"semiconductor", "hardware", "software"},
			},
			ExpectedOutcome: "Test diversification effectiveness",
		},
		{
			ID:       "red_liquidity_001",
			Name:     "Liquidity Evaporation",
			Type:     ScenarioLiquidityCrisis,
			Team:     TeamRed,
			Severity: SeverityHigh,
			Parameters: map[string]any{
				"spread_widening":  5.0,
				"volume_reduction": 0.8,
				"duration_hours":   2,
			},
			ExpectedOutcome: "Test position sizing and exit strategies",
		},
		{
			ID:       "red_rotation_001",
			Name:     "Forced Sector Rotation",
			Type:     ScenarioSectorRotation,
			Team:     TeamRed,
			Severity: SeverityMedium,
			Parameters: map[string]any{
				"from_sectors": []string{"tech"},
				"to_sectors":   []string{"utilities", "consumer_staples"},
				"speed":        "rapid",
			},
			ExpectedOutcome: "Test sector rotation adaptation",
		},
	}

	// Blue team defense scenarios
	blueScenarios := []*AdversarialScenario{
		{
			ID:       "blue_stability_001",
			Name:     "Market Stabilization Protocol",
			Type:     ScenarioStabilityControl,
			Team:     TeamBlue,
			Severity: SeverityMedium,
			Parameters: map[string]any{
				"volatility_target": 0.15,
				"hedge_ratio":       0.3,
			},
			ExpectedOutcome: "Reduce portfolio volatility",
		},
		{
			ID:       "blue_recovery_001",
			Name:     "Post-Crash Recovery",
			Type:     ScenarioRecoveryMode,
			Team:     TeamBlue,
			Severity: SeverityHigh,
			Parameters: map[string]any{
				"recovery_target": 0.95, // 95% of pre-crash value
				"max_drawdown":    0.15,
			},
			ExpectedOutcome: "Recover portfolio value efficiently",
		},
	}

	at.scenarios = append(redScenarios, blueScenarios...)

	// Set timestamps
	for _, s := range at.scenarios {
		s.CreatedAt = time.Now()
	}
}

// RunTraining executes full adversarial training cycle
func (at *AdversarialTrainer) RunTraining() *TrainingSummary {
	summary := &TrainingSummary{
		StartTime: time.Now(),
		Cycles:    at.config.TrainingCycles,
	}

	for cycle := 0; cycle < at.config.TrainingCycles; cycle++ {
		at.currentCycle = cycle

		// Match red vs blue agents
		for _, red := range at.redTeam {
			for _, blue := range at.blueTeam {
				result := at.executeBattle(red, blue)
				at.recordResult(result)

				// Adapt strategies based on outcome
				at.adaptStrategies(red, blue, result)
			}
		}

		// Periodic team rebalancing
		if cycle%10 == 0 && at.config.AdaptiveDifficulty {
			at.rebalanceTeams()
		}
	}

	summary.EndTime = time.Now()
	summary.Duration = summary.EndTime.Sub(summary.StartTime)
	summary.RedWins = at.countTeamWins(TeamRed)
	summary.BlueWins = at.countTeamWins(TeamBlue)
	summary.Draws = len(at.results) - summary.RedWins - summary.BlueWins
	summary.RedImprovement = at.calculateTeamImprovement(TeamRed)
	summary.BlueImprovement = at.calculateTeamImprovement(TeamBlue)

	return summary
}

// executeBattle runs a single confrontation
func (at *AdversarialTrainer) executeBattle(red, blue *AdversarialAgent) *BattleResult {
	result := &BattleResult{
		ID:        fmt.Sprintf("battle_%s_%s_%d", red.ID, blue.ID, time.Now().Unix()),
		Timestamp: time.Now(),
		RedAgent:  red.ID,
		BlueAgent: blue.ID,
		Rounds:    at.config.BattleRounds,
		KeyEvents: make([]BattleEvent, 0),
	}

	startTime := time.Now()

	// Simulate battle rounds
	redScore := 0.0
	blueScore := 0.0

	for round := 1; round <= at.config.BattleRounds; round++ {
		// Select strategies
		redStrategy := at.selectStrategy(red)
		blueStrategy := at.selectStrategy(blue)

		// Calculate round outcome
		redEffectiveness := at.calculateEffectiveness(red, redStrategy, round)
		blueEffectiveness := at.calculateEffectiveness(blue, blueStrategy, round)

		// Mutual adaptation over rounds
		adaptationFactor := float64(round) / float64(at.config.BattleRounds)
		redEffectiveness *= (1 - adaptationFactor*0.3)  // Red becomes less effective as blue adapts
		blueEffectiveness *= (1 + adaptationFactor*0.2) // Blue gets stronger as they learn

		redScore += redEffectiveness
		blueScore += blueEffectiveness

		// Record key events
		if round == 1 || round == at.config.BattleRounds || math.Abs(redEffectiveness-blueEffectiveness) > 0.5 {
			event := BattleEvent{
				Round:     round,
				Timestamp: time.Now(),
				Team:      TeamRed,
				Event:     fmt.Sprintf("%s effectiveness: %.2f", redStrategy.Name, redEffectiveness),
				Impact:    redEffectiveness,
			}
			if blueEffectiveness > redEffectiveness {
				event.Team = TeamBlue
				event.Event = fmt.Sprintf("%s effectiveness: %.2f", blueStrategy.Name, blueEffectiveness)
				event.Impact = blueEffectiveness
			}
			result.KeyEvents = append(result.KeyEvents, event)
		}

		// Update strategy usage
		redStrategy.UsageCount++
		blueStrategy.UsageCount++
	}

	result.Duration = time.Since(startTime).Seconds()
	result.RedScore = redScore
	result.BlueScore = blueScore

	// Determine winner
	if redScore > blueScore*1.1 {
		result.Winner = TeamRed
		red.WinCount++
		blue.LossCount++
	} else if blueScore > redScore*1.1 {
		result.Winner = TeamBlue
		blue.WinCount++
		red.LossCount++
	} else {
		// Draw
		red.DrawCount++
		blue.DrawCount++
	}

	red.LastActive = time.Now()
	blue.LastActive = time.Now()

	return result
}

// selectStrategy chooses a strategy for an agent
func (at *AdversarialTrainer) selectStrategy(agent *AdversarialAgent) *AttackStrategy {
	if len(agent.Strategies) == 0 {
		return nil
	}

	// Weight by success rate and exploration
	totalWeight := 0.0
	weights := make([]float64, len(agent.Strategies))

	for i, s := range agent.Strategies {
		// Success rate weight
		weight := s.SuccessRate

		// Exploration bonus for underused strategies
		if s.UsageCount < 5 {
			weight *= 1.5
		}

		weights[i] = weight
		totalWeight += weight
	}

	// Random selection weighted by effectiveness
	r := rand.Float64() * totalWeight
	cumulative := 0.0

	for i, w := range weights {
		cumulative += w
		if r <= cumulative {
			return &agent.Strategies[i]
		}
	}

	return &agent.Strategies[len(agent.Strategies)-1]
}

// calculateEffectiveness determines how effective a strategy is
func (at *AdversarialTrainer) calculateEffectiveness(agent *AdversarialAgent, strategy *AttackStrategy, round int) float64 {
	baseEffectiveness := agent.Effectiveness * strategy.SuccessRate

	// Skill bonus
	skillBonus := 0.1 * float64(round) * agent.Adaptability

	// Random variation
	noise := (rand.Float64() - 0.5) * 0.2

	return baseEffectiveness + skillBonus + noise
}

// recordResult stores battle outcome
func (at *AdversarialTrainer) recordResult(result *BattleResult) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.results = append(at.results, result)
}

// adaptStrategies updates strategies based on battle outcome
func (at *AdversarialTrainer) adaptStrategies(red, blue *AdversarialAgent, result *BattleResult) {
	learningRate := 0.1

	if result.Winner == TeamRed {
		// Red won - boost successful strategies
		for i := range red.Strategies {
			red.Strategies[i].SuccessRate = math.Min(1.0,
				red.Strategies[i].SuccessRate*(1+learningRate))
		}
		// Blue lost - reduce failed strategies slightly
		for i := range blue.Strategies {
			blue.Strategies[i].SuccessRate = math.Max(0.1,
				blue.Strategies[i].SuccessRate*(1-learningRate*0.5))
		}
		red.Effectiveness = math.Min(1.0, red.Effectiveness+0.02)
		blue.Adaptability = math.Min(1.0, blue.Adaptability+0.03) // Blue learns from loss
	} else if result.Winner == TeamBlue {
		// Blue won - boost successful strategies
		for i := range blue.Strategies {
			blue.Strategies[i].SuccessRate = math.Min(1.0,
				blue.Strategies[i].SuccessRate*(1+learningRate))
		}
		// Red lost - reduce failed strategies
		for i := range red.Strategies {
			red.Strategies[i].SuccessRate = math.Max(0.1,
				red.Strategies[i].SuccessRate*(1-learningRate*0.5))
		}
		blue.Effectiveness = math.Min(1.0, blue.Effectiveness+0.02)
		red.Adaptability = math.Min(1.0, red.Adaptability+0.03)
	}
	// Draws - both learn slightly
	if result.Winner != TeamRed && result.Winner != TeamBlue {
		red.Adaptability = math.Min(1.0, red.Adaptability+0.01)
		blue.Adaptability = math.Min(1.0, blue.Adaptability+0.01)
	}
}

// rebalanceTeams adjusts team composition based on performance
func (at *AdversarialTrainer) rebalanceTeams() {
	// Sort red team by effectiveness
	sortAgents(at.redTeam)

	// Sort blue team by effectiveness
	sortAgents(at.blueTeam)

	// Replace bottom performers with new agents
	if len(at.redTeam) >= 3 {
		// Keep top 80%, replace bottom 20%
		keepCount := int(float64(len(at.redTeam)) * 0.8)
		at.redTeam = at.redTeam[:keepCount]

		// Add new red agents
		for i := keepCount; i < at.config.RedTeamSize; i++ {
			agent := &AdversarialAgent{
				ID:            fmt.Sprintf("red_%03d_new_%d", i, at.currentCycle),
				Name:          fmt.Sprintf("Red Attacker %d (New)", i+1),
				Team:          TeamRed,
				Skill:         at.redTeam[i%len(at.redTeam)].Skill,
				Effectiveness: 0.5 + rand.Float64()*0.3,
				Adaptability:  0.6 + rand.Float64()*0.3,
				Strategies:    at.generateAttackStrategies(),
				LastActive:    time.Now(),
			}
			at.redTeam = append(at.redTeam, agent)
		}
	}

	// Similar for blue team
	if len(at.blueTeam) >= 3 {
		keepCount := int(float64(len(at.blueTeam)) * 0.8)
		at.blueTeam = at.blueTeam[:keepCount]

		for i := keepCount; i < at.config.BlueTeamSize; i++ {
			agent := &AdversarialAgent{
				ID:            fmt.Sprintf("blue_%03d_new_%d", i, at.currentCycle),
				Name:          fmt.Sprintf("Blue Defender %d (New)", i+1),
				Team:          TeamBlue,
				Skill:         at.blueTeam[i%len(at.blueTeam)].Skill,
				Effectiveness: 0.5 + rand.Float64()*0.3,
				Adaptability:  0.6 + rand.Float64()*0.3,
				Strategies:    at.generateDefenseStrategies(),
				LastActive:    time.Now(),
			}
			at.blueTeam = append(at.blueTeam, agent)
		}
	}
}

// sortAgents orders agents by win rate
func sortAgents(agents []*AdversarialAgent) {
	// Simple bubble sort for small arrays
	n := len(agents)
	for i := range n {
		for j := 0; j < n-i-1; j++ {
			winRate1 := float64(agents[j].WinCount) / float64(agents[j].WinCount+agents[j].LossCount+1)
			winRate2 := float64(agents[j+1].WinCount) / float64(agents[j+1].WinCount+agents[j+1].LossCount+1)
			if winRate1 < winRate2 {
				agents[j], agents[j+1] = agents[j+1], agents[j]
			}
		}
	}
}

// countTeamWins returns win count for a team
func (at *AdversarialTrainer) countTeamWins(team TeamType) int {
	count := 0
	for _, r := range at.results {
		if r.Winner == team {
			count++
		}
	}
	return count
}

// calculateTeamImprovement computes effectiveness improvement
func (at *AdversarialTrainer) calculateTeamImprovement(team TeamType) float64 {
	var agents []*AdversarialAgent
	if team == TeamRed {
		agents = at.redTeam
	} else {
		agents = at.blueTeam
	}

	if len(agents) == 0 {
		return 0.0
	}

	totalImprovement := 0.0
	for _, a := range agents {
		// Calculate based on win rate vs baseline
		total := a.WinCount + a.LossCount + a.DrawCount
		if total > 0 {
			winRate := float64(a.WinCount) / float64(total)
			totalImprovement += winRate - 0.5 // 0.5 is baseline
		}
	}

	return totalImprovement / float64(len(agents))
}

// GetVulnerabilities analyzes red team success patterns
func (at *AdversarialTrainer) GetVulnerabilities() []Vulnerability {
	at.mu.RLock()
	defer at.mu.RUnlock()

	vulns := make(map[ScenarioType]int)

	// Count red wins by scenario type
	for _, result := range at.results {
		if result.Winner == TeamRed {
			// Find scenario
			for _, s := range at.scenarios {
				// Simplified matching
				_ = s
				// In real implementation, would track which scenario was used
			}
		}
	}

	// Convert to vulnerability list
	var list []Vulnerability
	for scenarioType, count := range vulns {
		list = append(list, Vulnerability{
			Type:           scenarioType,
			Occurrences:    count,
			Severity:       at.calculateSeverity(count),
			Recommendation: at.generateRecommendation(scenarioType),
		})
	}

	return list
}

// Vulnerability represents a system weakness
type Vulnerability struct {
	Type           ScenarioType  `json:"type"`
	Occurrences    int           `json:"occurrences"`
	Severity       SeverityLevel `json:"severity"`
	Recommendation string        `json:"recommendation"`
}

func (at *AdversarialTrainer) calculateSeverity(count int) SeverityLevel {
	switch {
	case count >= 20:
		return SeverityCritical
	case count >= 10:
		return SeverityHigh
	case count >= 5:
		return SeverityMedium
	default:
		return SeverityLow
	}
}

func (at *AdversarialTrainer) generateRecommendation(scenarioType ScenarioType) string {
	recommendations := map[ScenarioType]string{
		ScenarioFlashCrash:       "Implement circuit breakers and gradual position reduction",
		ScenarioLiquidityCrisis:  "Maintain higher cash reserves and reduce position sizes",
		ScenarioCorrelationSpike: "Increase cross-asset diversification",
		ScenarioDisinformation:   "Implement multi-source verification and sentiment filtering",
		ScenarioSectorRotation:   "Add sector rotation detection and faster rebalancing",
	}

	if rec, ok := recommendations[scenarioType]; ok {
		return rec
	}
	return "Review and strengthen defense mechanisms"
}

// StressTestAgent runs a specific agent through adversarial scenarios
func (at *AdversarialTrainer) StressTestAgent(agentID string, targetAgent domain.AgentSpec) *StressTestResult {
	result := &StressTestResult{
		AgentID:         agentID,
		Timestamp:       time.Now(),
		Scenarios:       make([]ScenarioResult, 0),
		Vulnerabilities: make([]string, 0),
	}

	// Run each scenario type
	for _, scenario := range at.scenarios {
		scenarioResult := at.simulateScenario(scenario, targetAgent)
		result.Scenarios = append(result.Scenarios, scenarioResult)

		if !scenarioResult.Passed {
			result.Vulnerabilities = append(result.Vulnerabilities, string(scenario.Type))
		}
	}

	// Calculate overall score
	totalScore := 0.0
	for _, sr := range result.Scenarios {
		totalScore += sr.Score
	}
	result.OverallScore = totalScore / float64(len(result.Scenarios))
	result.Passed = result.OverallScore >= 0.6

	return result
}

// ScenarioResult captures outcome of one scenario test
type ScenarioResult struct {
	ScenarioType string  `json:"scenario_type"`
	Score        float64 `json:"score"`
	Passed       bool    `json:"passed"`
	Details      string  `json:"details"`
}

// StressTestResult summarizes agent stress testing
type StressTestResult struct {
	AgentID         string           `json:"agent_id"`
	Timestamp       time.Time        `json:"timestamp"`
	OverallScore    float64          `json:"overall_score"`
	Passed          bool             `json:"passed"`
	Scenarios       []ScenarioResult `json:"scenarios"`
	Vulnerabilities []string         `json:"vulnerabilities"`
}

func (at *AdversarialTrainer) simulateScenario(scenario *AdversarialScenario, agent domain.AgentSpec) ScenarioResult {
	// Simplified simulation
	// In real implementation, would actually run agent through scenario

	baseScore := 0.5

	// Adjust based on agent layer
	switch agent.Layer {
	case domain.LayerSuperinvestor:
		baseScore += 0.2
	case domain.LayerMacro:
		baseScore += 0.1
	default:
		// No layer-specific adjustment.
	}

	// Random variation
	baseScore += (rand.Float64() - 0.5) * 0.3

	return ScenarioResult{
		ScenarioType: string(scenario.Type),
		Score:        math.Max(0, math.Min(1, baseScore)),
		Passed:       baseScore >= 0.6,
		Details:      fmt.Sprintf("Simulated response to %s scenario", scenario.Type),
	}
}

// GenerateReport creates comprehensive training analysis
func (at *AdversarialTrainer) GenerateReport() *AdversarialReport {
	at.mu.RLock()
	defer at.mu.RUnlock()

	report := &AdversarialReport{
		GeneratedAt:  time.Now(),
		RedTeamSize:  len(at.redTeam),
		BlueTeamSize: len(at.blueTeam),
		TotalBattles: len(at.results),
		RedWinRate:   at.calculateWinRate(TeamRed),
		BlueWinRate:  at.calculateWinRate(TeamBlue),
		DrawRate:     at.calculateWinRate("draw"),
	}

	// Top performers
	for _, a := range at.redTeam {
		if a.WinCount > 10 {
			report.TopRedAgents = append(report.TopRedAgents, a)
		}
	}

	for _, a := range at.blueTeam {
		if a.WinCount > 10 {
			report.TopBlueAgents = append(report.TopBlueAgents, a)
		}
	}

	// Vulnerabilities
	report.Vulnerabilities = at.GetVulnerabilities()

	return report
}

// AdversarialReport summarizes training outcomes
type AdversarialReport struct {
	GeneratedAt     time.Time           `json:"generated_at"`
	RedTeamSize     int                 `json:"red_team_size"`
	BlueTeamSize    int                 `json:"blue_team_size"`
	TotalBattles    int                 `json:"total_battles"`
	RedWinRate      float64             `json:"red_win_rate"`
	BlueWinRate     float64             `json:"blue_win_rate"`
	DrawRate        float64             `json:"draw_rate"`
	TopRedAgents    []*AdversarialAgent `json:"top_red_agents"`
	TopBlueAgents   []*AdversarialAgent `json:"top_blue_agents"`
	Vulnerabilities []Vulnerability     `json:"vulnerabilities"`
}

// TrainingSummary captures overall training results
type TrainingSummary struct {
	StartTime       time.Time     `json:"start_time"`
	EndTime         time.Time     `json:"end_time"`
	Duration        time.Duration `json:"duration"`
	Cycles          int           `json:"cycles"`
	RedWins         int           `json:"red_wins"`
	BlueWins        int           `json:"blue_wins"`
	Draws           int           `json:"draws"`
	RedImprovement  float64       `json:"red_improvement"`
	BlueImprovement float64       `json:"blue_improvement"`
}

func (at *AdversarialTrainer) calculateWinRate(team any) float64 {
	total := len(at.results)
	if total == 0 {
		return 0.0
	}

	if team == "draw" {
		draws := total - at.countTeamWins(TeamRed) - at.countTeamWins(TeamBlue)
		return float64(draws) / float64(total)
	}

	if t, ok := team.(TeamType); ok {
		wins := at.countTeamWins(t)
		return float64(wins) / float64(total)
	}

	return 0.0
}
