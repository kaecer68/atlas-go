package spawning

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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

	// State
	mu            sync.RWMutex
	isRunning     bool
	lastCheck     time.Time
	checkInterval time.Duration
}

// SpawningConfig holds configuration for the spawning manager
type SpawningConfig struct {
	MaxActiveSpawns      int
	TrainingWindowDays   int
	ValidationMinSignals int
	AcceptanceThreshold  float64
	CheckInterval        time.Duration
}

// DefaultSpawningConfig returns recommended default configuration
func DefaultSpawningConfig() SpawningConfig {
	return SpawningConfig{
		MaxActiveSpawns:      3,              // Max 3 concurrent spawns
		TrainingWindowDays:   30,             // 30-day training period
		ValidationMinSignals: 20,             // Need 20 signals to evaluate
		AcceptanceThreshold:  0.5,            // Sharpe > 0.5 to accept
		CheckInterval:        24 * time.Hour, // Check daily
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
		lastCheck:            time.Time{},
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
	log.Println("[SpawningManager] Started automated agent spawning")
}

// Stop halts the spawning process
func (m *SpawningManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isRunning = false
	log.Println("[SpawningManager] Stopped")
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
	log.Println("[SpawningManager] Starting spawning cycle...")

	// 1. Detect gaps
	scorecards := m.collectScorecards()
	universe := m.getUniverse()
	gaps := m.gapDetector.DetectGaps(*m.registry, scorecards, universe)

	if len(gaps) == 0 {
		log.Println("[SpawningManager] No gaps detected")
		return
	}

	log.Printf("[SpawningManager] Detected %d knowledge gaps", len(gaps))

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
			log.Printf("[SpawningManager] Failed to spawn for gap %s: %v", gap.ID, err)
			continue
		}

		m.spawnedAgents[spawned.AgentID] = spawned
		m.gapDetector.UpdateGapStatus(gap.ID, GapStatusSpawning)
		spawnCount++

		log.Printf("[SpawningManager] Spawned agent %s for gap %s", spawned.AgentID, gap.ID)
	}

	// 4. Update training agents
	m.updateTrainingAgents(scorecards)

	// 5. Validate completed agents
	m.validateCompletedAgents()

	log.Printf("[SpawningManager] Cycle complete. Active spawns: %d", len(m.spawnedAgents))
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
	for i := 0; i < len(scored); i++ {
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

	// Save prompt file
	promptPath := filepath.Join("prompts/agents", filepath.Base(spec.PromptFile))
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

		log.Printf("[SpawningManager] Agent %s completed training, entering validation", agentID)

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
		log.Printf("[SpawningManager] Agent %s is now a candidate for acceptance", agentID)
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

	log.Printf("[SpawningManager] Accepted agent %s", agentID)
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

	log.Printf("[SpawningManager] Rejected agent %s: %s", agentID, reason)
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

				// Remove prompt file
				promptPath := filepath.Join("prompts/agents", agentID+".md")
				os.Remove(promptPath)

				removed++
			}
		}
	}

	return removed
}
