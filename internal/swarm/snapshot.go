package swarm

import (
	"encoding/json"
	"os"
	"time"
)

// SwarmSnapshot is a serializable snapshot of the swarm's latest state.
type SwarmSnapshot struct {
	RecordedAt          time.Time                      `json:"recorded_at"`
	Scenarios           []ScenarioSnapshot             `json:"scenarios"`
	Consensus           map[string]ConsensusPrediction `json:"consensus"`
	Anomalies           []Anomaly                      `json:"anomalies"`
	TotalFish           int                            `json:"total_fish"`
	ConsensusConfidence float64                        `json:"consensus_confidence"`
	TopFishAccuracy     float64                        `json:"top_fish_accuracy"`
	GenerationsEvolved  int                            `json:"generations_evolved"`
}

// ScenarioSnapshot captures scenario parameters at snapshot time.
type ScenarioSnapshot struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Regime     string  `json:"regime"`
	Volatility float64 `json:"volatility"`
	Trend      float64 `json:"trend"`
}

// Snapshot builds a SwarmSnapshot from the swarm's current state.
func (sw *MiroFishSwarm) Snapshot() SwarmSnapshot {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	scenarios := make([]ScenarioSnapshot, len(sw.scenarios))
	for i, s := range sw.scenarios {
		scenarios[i] = ScenarioSnapshot{
			ID:         s.ID,
			Name:       s.Name,
			Regime:     s.Regime,
			Volatility: s.Volatility,
			Trend:      s.Trend,
		}
	}

	var result SimulationResult
	var hasResult bool
	if len(sw.results) > 0 {
		result = sw.results[len(sw.results)-1]
		hasResult = true
	}

	consensus := make(map[string]ConsensusPrediction)
	var anomalies []Anomaly
	confidence := 0.0
	if hasResult {
		for k, v := range result.Consensus {
			consensus[k] = v
		}
		anomalies = result.Anomalies
		if anomalies == nil {
			anomalies = []Anomaly{}
		}
		confidence = result.Confidence
	}

	// Top fish accuracy
	topAccuracy := 0.0
	if len(sw.fish) > 0 {
		topN := sw.getTopFishUnsafe(1)
		if len(topN) > 0 {
			topAccuracy = topN[0].Performance.Accuracy
		}
	}

	return SwarmSnapshot{
		RecordedAt:          time.Now(),
		Scenarios:           scenarios,
		Consensus:           consensus,
		Anomalies:           anomalies,
		TotalFish:           len(sw.fish),
		ConsensusConfidence: confidence,
		TopFishAccuracy:     topAccuracy,
		GenerationsEvolved:  0, // Updated externally by Phase3Controller
	}
}

// SaveSnapshot writes the current snapshot as JSON to the given path.
func (sw *MiroFishSwarm) SaveSnapshot(path string) error {
	snap := sw.Snapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
