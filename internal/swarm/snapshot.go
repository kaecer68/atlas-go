package swarm

import "time"

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
