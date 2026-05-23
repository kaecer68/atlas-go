package spawning

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// SpawningManager orchestrates the entire agent spawning lifecycle
type SpawningManager struct {
	gapDetector   *GapDetector
	agentFactory  *AgentFactory
	spawnedAgents map[string]*SpawnedAgent
	registry      *domain.AgentRegistry

	// Configuration
	maxActiveSpawns      int
	trainingWindowDays   int
	validationMinSignals int
	acceptanceThreshold  float64
	minWeightDays        int
	promptsDir           string // absolute path for prompt file I/O

	// State
	mu            sync.RWMutex
	isRunning     bool
	lastCheck     time.Time
	checkInterval time.Duration
	weightHistory map[string]int // agentID -> consecutive days at min weight
}

// SpawningConfig holds configuration for the spawning manager
type SpawningConfig struct {
	MaxActiveSpawns      int
	TrainingWindowDays   int
	ValidationMinSignals int
	AcceptanceThreshold  float64
	CheckInterval        time.Duration
	MinWeightDays        int    // days at DarwinianWeightMin before extinction
	PromptsDir           string // absolute path to prompts/ directory (default "prompts")
}

// DefaultSpawningConfig returns recommended default configuration
func DefaultSpawningConfig() SpawningConfig {
	return SpawningConfig{
		MaxActiveSpawns:      3,              // Max 3 concurrent spawns
		TrainingWindowDays:   30,             // 30-day training period
		ValidationMinSignals: 20,             // Need 20 signals to evaluate
		AcceptanceThreshold:  0.5,            // Sharpe > 0.5 to accept
		CheckInterval:        24 * time.Hour, // Check daily
		MinWeightDays:        20,             // 20 days at min weight -> extinct
		PromptsDir:           "prompts",      // relative to CWD by default
	}
}

// NewSpawningManager creates a new spawning manager
func NewSpawningManager(registry *domain.AgentRegistry, config SpawningConfig) *SpawningManager {
	return &SpawningManager{
		gapDetector:          NewGapDetector(),
		agentFactory:         NewAgentFactory(),
		spawnedAgents:        make(map[string]*SpawnedAgent),
		registry:             registry,
		maxActiveSpawns:      config.MaxActiveSpawns,
		trainingWindowDays:   config.TrainingWindowDays,
		validationMinSignals: config.ValidationMinSignals,
		acceptanceThreshold:  config.AcceptanceThreshold,
		checkInterval:        config.CheckInterval,
		minWeightDays:        config.MinWeightDays,
		promptsDir:           config.PromptsDir,
		lastCheck:            time.Time{},
		weightHistory:        make(map[string]int),
	}
}

// Start begins the automated spawning process
func (m *SpawningManager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return
	}

	m.isRunning = true
	go m.runLoop()
	logging.Info("spawning_manager", "started")
}

// Stop halts the spawning process
func (m *SpawningManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isRunning = false
	logging.Info("spawning_manager", "stopped")
}

// runLoop is the main background loop
func (m *SpawningManager) runLoop() {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	// Run initial check
	m.PerformSpawningCycle()

	for range ticker.C {
		m.mu.RLock()
		running := m.isRunning
		m.mu.RUnlock()
		if !running {
			return
		}
		m.PerformSpawningCycle()
	}
}

// PerformSpawningCycle executes one full spawning cycle
func (m *SpawningManager) PerformSpawningCycle() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastCheck = time.Now()
	logging.Info("spawning_manager", "spawning_cycle_started")

	// 1. Detect gaps
	scorecards := m.collectScorecards()
	universe := m.getUniverse()
	gaps := m.gapDetector.DetectGaps(*m.registry, scorecards, universe)

	if len(gaps) == 0 {
		logging.Info("spawning_manager", "no_gaps_detected")
		return
	}

	logging.Info("spawning_manager", "knowledge_gaps_detected", "count", len(gaps))

	// 2. Prioritize gaps
	prioritized := m.prioritizeGaps(gaps)

	// 3. Spawn agents for top gaps (up to maxActiveSpawns)
	spawnCount := 0
	for _, gap := range prioritized {
		if spawnCount >= m.maxActiveSpawns {
			break
		}

		// Check if gap is already being addressed
		if m.isGapBeingAddressed(gap.ID) {
			continue
		}

		// Spawn agent
		spawned, err := m.spawnAgentForGap(gap)
		if err != nil {
			logging.Error("spawning_manager", "spawn_failed", logging.FStr("gap_id", gap.ID), logging.Err(err))
			continue
		}

		m.spawnedAgents[spawned.AgentID] = spawned
		m.gapDetector.UpdateGapStatus(gap.ID, GapStatusSpawning)
		spawnCount++

		logging.Info("spawning_manager", "agent_spawned", logging.AgentID(spawned.AgentID), logging.FStr("gap_id", gap.ID))
	}

	// 4. Update training agents
	m.updateTrainingAgents(scorecards)

	// 5. Validate completed agents
	m.validateCompletedAgents()

	logging.Info("spawning_manager", "cycle_complete", "active_spawns", len(m.spawnedAgents))
}

// prioritizeGaps sorts gaps by priority score
func (m *SpawningManager) prioritizeGaps(gaps []*KnowledgeGap) []*KnowledgeGap {
	// Sort by priority score (descending)
	type scoredGap struct {
		gap   *KnowledgeGap
		score float64
	}

	scored := make([]scoredGap, len(gaps))
	for i, gap := range gaps {
		scored[i] = scoredGap{
			gap:   gap,
			score: CalculateGapPriorityScore(gap),
		}
	}

	// Simple bubble sort (small list, not performance critical)
	for i := range scored {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	result := make([]*KnowledgeGap, len(scored))
	for i, s := range scored {
		result[i] = s.gap
	}

	return result
}

// isGapBeingAddressed checks if a gap already has an active spawn
func (m *SpawningManager) isGapBeingAddressed(gapID string) bool {
	for _, spawned := range m.spawnedAgents {
		if spawned.GapID == gapID && spawned.Status != SpawnStatusRejected && spawned.Status != SpawnStatusDisabled {
			return true
		}
	}
	return false
}

// spawnAgentForGap creates and registers a new agent for a gap
func (m *SpawningManager) spawnAgentForGap(gap *KnowledgeGap) (*SpawnedAgent, error) {
	// Generate agent spec and prompt
	spec, promptContent := m.agentFactory.CreateAgentForGap(gap, "")

	// Save prompt file — anchored to PromptsDir for CWD-independence
	promptPath := filepath.Join(m.promptsDir, "agents", filepath.Base(spec.PromptFile))
	if err := os.MkdirAll(filepath.Dir(promptPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create prompts directory: %w", err)
	}

	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write prompt file: %w", err)
	}

	// Add to registry
	m.registry.Agents = append(m.registry.Agents, *spec)

	// Create spawn tracking record
	spawned := &SpawnedAgent{
		AgentID:        spec.ID,
		ParentAgentID:  "", // No parent for fresh spawns
		GapID:          gap.ID,
		CreatedAt:      time.Now(),
		TrainingStart:  time.Now(),
		Status:         SpawnStatusTraining,
		CurrentWeight:  1.0,
		PromptTemplate: promptContent,
	}

	return spawned, nil
}

// updateTrainingAgents updates agents in training phase
func (m *SpawningManager) updateTrainingAgents(scorecards map[string]*domain.Scorecard) {
	for agentID, spawned := range m.spawnedAgents {
		if spawned.Status != SpawnStatusTraining {
			continue
		}

		// Check if training period complete
		trainingDuration := time.Since(spawned.TrainingStart)
		if trainingDuration < time.Duration(m.trainingWindowDays)*24*time.Hour {
			continue // Still in training
		}

		// Move to validation
		spawned.Status = SpawnStatusValidating
		spawned.TrainingEnd = time.Now()

		logging.Info("spawning_manager", "training_completed", logging.AgentID(agentID))

		// Update gap status
		m.gapDetector.UpdateGapStatus(spawned.GapID, GapStatusTesting)
	}
}

// validateCompletedAgents evaluates agents that have completed validation
func (m *SpawningManager) validateCompletedAgents() {
	for agentID, spawned := range m.spawnedAgents {
		if spawned.Status != SpawnStatusValidating {
			continue
		}

		// Get agent scorecard
		// In real implementation, this would come from the ledger/scorecard system
		// For now, simulate the validation process

		// Check if we have enough signals
		validationDuration := time.Since(spawned.TrainingEnd)
		minValidationDuration := time.Duration(m.validationMinSignals) * 24 * time.Hour / 30 // Rough estimate

		if validationDuration < minValidationDuration {
			continue // Need more validation time
		}

		// Validate performance (simulated - in real system would use actual scorecard)
		// This is where we'd check:
		// - Sharpe ratio vs threshold
		// - Hit rate
		// - Max drawdown
		// - Correlation with existing agents

		// For now, mark as candidate for manual review
		spawned.Status = SpawnStatusCandidate
		logging.Info("spawning_manager", "candidate_ready", logging.AgentID(agentID))
	}
}

// AcceptAgent promotes a candidate agent to full acceptance
func (m *SpawningManager) AcceptAgent(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	spawned, ok := m.spawnedAgents[agentID]
	if !ok {
		return fmt.Errorf("agent %s not found in spawned agents", agentID)
	}

	if spawned.Status != SpawnStatusCandidate {
		return fmt.Errorf("agent %s is not in candidate status (current: %s)", agentID, spawned.Status)
	}

	// Find and enable the agent in registry
	for i := range m.registry.Agents {
		if m.registry.Agents[i].ID == agentID {
			m.registry.Agents[i].Enabled = true
			break
		}
	}

	spawned.Status = SpawnStatusAccepted
	m.gapDetector.UpdateGapStatus(spawned.GapID, GapStatusResolved)

	logging.Info("spawning_manager", "agent_accepted", logging.AgentID(agentID))
	return nil
}

// RejectAgent dismisses a candidate agent
func (m *SpawningManager) RejectAgent(agentID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	spawned, ok := m.spawnedAgents[agentID]
	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	spawned.Status = SpawnStatusRejected

	// Mark gap as open again for retry
	m.gapDetector.UpdateGapStatus(spawned.GapID, GapStatusOpen)

	logging.Warn("spawning_manager", "agent_rejected", logging.AgentID(agentID), logging.FStr("reason", reason))
	return nil
}

// GetSpawnedAgents returns all spawned agents
func (m *SpawningManager) GetSpawnedAgents() []*SpawnedAgent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*SpawnedAgent, 0, len(m.spawnedAgents))
	for _, spawned := range m.spawnedAgents {
		result = append(result, spawned)
	}

	return result
}

// GetSpawnedAgentByID returns a specific spawned agent
func (m *SpawningManager) GetSpawnedAgentByID(agentID string) (*SpawnedAgent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	spawned, ok := m.spawnedAgents[agentID]
	return spawned, ok
}

// CheckExtinction marks spawned agents as extinct if their Darwinian weight
// has stayed at the minimum threshold for minWeightDays or more.
// It returns the IDs of agents that were marked extinct in this call.
func (m *SpawningManager) CheckExtinction(weights map[string]float64) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.minWeightDays <= 0 {
		m.minWeightDays = 20
	}
	minWeight := 0.3 // DarwinianWeightMin

	var extinct []string
	for agentID, spawned := range m.spawnedAgents {
		if spawned.Status == SpawnStatusExtinct {
			continue
		}
		w, ok := weights[agentID]
		if !ok {
			// No weight data; reset streak
			m.weightHistory[agentID] = 0
			continue
		}
		if w <= minWeight {
			m.weightHistory[agentID]++
		} else {
			m.weightHistory[agentID] = 0
		}
		if m.weightHistory[agentID] >= m.minWeightDays {
			spawned.Status = SpawnStatusExtinct
			m.weightHistory[agentID] = 0
			// Disable in registry without deleting
			for i := range m.registry.Agents {
				if m.registry.Agents[i].ID == agentID {
					m.registry.Agents[i].Enabled = false
					break
				}
			}
			extinct = append(extinct, agentID)
			logging.Warn("spawning_manager", "agent_extinct", logging.AgentID(agentID), logging.FInt("days_at_min_weight", m.minWeightDays))
		}
	}
	return extinct
}

// GetStatistics returns spawning system statistics
func (m *SpawningManager) GetStatistics() SpawningStatistics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := SpawningStatistics{
		TotalSpawned:   len(m.spawnedAgents),
		ActiveTraining: 0,
		InValidation:   0,
		Candidates:     0,
		Accepted:       0,
		Rejected:       0,
		Extinct:        0,
	}

	for _, spawned := range m.spawnedAgents {
		switch spawned.Status {
		case SpawnStatusTraining:
			stats.ActiveTraining++
		case SpawnStatusValidating:
			stats.InValidation++
		case SpawnStatusCandidate:
			stats.Candidates++
		case SpawnStatusAccepted:
			stats.Accepted++
		case SpawnStatusRejected:
			stats.Rejected++
		case SpawnStatusExtinct:
			stats.Extinct++
		default:
			// Unknown status ignored in stats aggregation.
		}
	}

	return stats
}

// SpawningStatistics provides metrics on the spawning system
type SpawningStatistics struct {
	TotalSpawned   int
	ActiveTraining int
	InValidation   int
	Candidates     int
	Accepted       int
	Rejected       int
	Extinct        int
}

// collectScorecards gathers scorecards from the system
// In real implementation, this would query the ledger/scorecard system
func (m *SpawningManager) collectScorecards() map[string]*domain.Scorecard {
	// Placeholder - would integrate with actual scorecard system
	return make(map[string]*domain.Scorecard)
}

// getUniverse returns the stock universe
func (m *SpawningManager) getUniverse() []string {
	// Taiwan stock universe - could be loaded from config
	return []string{
		"2330.TW", "2317.TW", "2454.TW", "2881.TW", "2303.TW",
		"2382.TW", "2882.TW", "2002.TW", "2891.TW", "3711.TW",
		"2885.TW", "1216.TW", "2308.TW", "2886.TW", "1101.TW",
		"2327.TW", "3008.TW", "2890.TW", "3045.TW", "1102.TW",
	}
}

// ManualSpawn allows manual spawning for a specific gap
func (m *SpawningManager) ManualSpawn(gapType GapType, sector, style string) (*SpawnedAgent, error) {
	gap := &KnowledgeGap{
		ID:         fmt.Sprintf("manual-%s-%d", gapType, time.Now().Unix()),
		Type:       gapType,
		Severity:   GapSeverityMedium,
		Sector:     sector,
		Style:      style,
		DetectedAt: time.Now(),
		Status:     GapStatusOpen,
	}

	return m.spawnAgentForGap(gap)
}

// Cleanup removes rejected/disabled agents older than retention period
func (m *SpawningManager) Cleanup(retentionPeriod time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	cutoff := time.Now().Add(-retentionPeriod)

	for agentID, spawned := range m.spawnedAgents {
		if spawned.Status == SpawnStatusRejected || spawned.Status == SpawnStatusDisabled {
			if spawned.CreatedAt.Before(cutoff) {
				// Remove from spawned agents
				delete(m.spawnedAgents, agentID)

				// Remove from registry
				for i, agent := range m.registry.Agents {
					if agent.ID == agentID {
						m.registry.Agents = append(m.registry.Agents[:i], m.registry.Agents[i+1:]...)
						break
					}
				}

				// Remove prompt file — anchored to PromptsDir
				promptPath := filepath.Join(m.promptsDir, "agents", agentID+".md")
				os.Remove(promptPath)

				removed++
			}
		}
	}

	return removed
}
