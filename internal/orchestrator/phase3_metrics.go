package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/spawning"
)

// Phase3Metrics exposes a serializable snapshot of the 5-track controller state.
type Phase3Metrics struct {
	SwarmRunning               bool              `json:"swarm_running"`
	SwarmConsensusSymbols      int               `json:"swarm_consensus_symbols"`
	PRISMQueuedTasks           int               `json:"prism_queued_tasks"`
	PRISMCompletedResults      int               `json:"prism_completed_results"`
	PRISMTopAgentID            string            `json:"prism_top_agent_id"`
	PRISMTopAgentSharpe        float64           `json:"prism_top_agent_sharpe"`
	SpawningActive             int               `json:"spawning_active"`
	SpawningCandidates         int               `json:"spawning_candidates"`
	ReflexivityActiveLoops     int               `json:"reflexivity_active_loops"`
	AdversarialLastScore       float64           `json:"adversarial_last_score"`
	AdversarialVulnerabilities []string          `json:"adversarial_vulnerabilities"`
	RecordedAt                 time.Time         `json:"recorded_at"`
}

// defaultMetricsPath writes to a well-known location inside the project.
var defaultMetricsPath = "data/state/phase3_metrics.json"

// CollectMetrics gathers the current state of all 5 optimization tracks.
func (c *Phase3Controller) CollectMetrics() Phase3Metrics {
	m := Phase3Metrics{RecordedAt: time.Now()}

	// Track A: Swarm
	c.mu.RLock()
	m.SwarmRunning = c.swarmRunning
	c.mu.RUnlock()
	if c.swarm != nil {
		if result, ok := c.swarm.GetLatestResult(); ok {
			m.SwarmConsensusSymbols = len(result.Consensus)
		}
	}

	// Track B: PRISM
	if c.prismManager != nil {
		stats := c.prismManager.GetOverallStats()
		m.PRISMQueuedTasks = stats.TotalTasks - stats.CompletedTasks - stats.FailedTasks
		m.PRISMCompletedResults = stats.CompletedTasks
		c.mu.RLock()
		bestAgent := ""
		bestSharpe := -999.0
		for agentID, sharpe := range c.prismWeightCache {
			if sharpe > bestSharpe {
				bestSharpe = sharpe
				bestAgent = agentID
			}
		}
		c.mu.RUnlock()
		m.PRISMTopAgentID = bestAgent
		m.PRISMTopAgentSharpe = bestSharpe
	}

	// Track C: Spawning
	if c.spawningManager != nil {
		agents := c.spawningManager.GetSpawnedAgents()
		for _, a := range agents {
			m.SpawningActive++
			if a.Status == spawning.SpawnStatusCandidate {
				m.SpawningCandidates++
			}
		}
	}

	// Track D: Reflexivity
	if c.reflexEngine != nil {
		m.ReflexivityActiveLoops = len(c.reflexEngine.GetActiveLoops())
	}

	// Track E: Adversarial
	if last := c.GetLastAdversarialResult(); last != nil {
		m.AdversarialLastScore = last.OverallScore
		m.AdversarialVulnerabilities = append([]string(nil), last.Vulnerabilities...)
	}

	return m
}

// SaveMetrics persists metrics to disk.
func (c *Phase3Controller) SaveMetrics(path string) error {
	if path == "" {
		path = defaultMetricsPath
	}
	m := c.CollectMetrics()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal phase3 metrics: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadPhase3Metrics reads the latest persisted metrics.
func LoadPhase3Metrics(path string) (Phase3Metrics, error) {
	if path == "" {
		path = defaultMetricsPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Phase3Metrics{}, err
	}
	var m Phase3Metrics
	if err := json.Unmarshal(data, &m); err != nil {
		return Phase3Metrics{}, fmt.Errorf("unmarshal phase3 metrics: %w", err)
	}
	return m, nil
}
