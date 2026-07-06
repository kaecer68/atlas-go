// Package domain provides shared domain types for the atlas-go investment research system.
//
// swarm_types.go contains types originally defined in internal/swarm/ and migrated
// to domain as part of the swarm module demotion (PR #963). The swarm package
// retains type aliases for backward compatibility.
package domain

import "time"

// MarketState captures market conditions at a point in time.
type MarketState struct {
	Timestamp          time.Time
	Prices             map[string]float64
	Volumes            map[string]float64
	Sentiment          float64
	Volatility         float64
	Correlation        float64
	RealizedVolatility float64
}

// FishPerformance tracks prediction accuracy for a single simulation fish.
type FishPerformance struct {
	CorrectPredictions int
	TotalPredictions   int
	Accuracy           float64
	SharpeRatio        float64
	MaxDrawdown        float64
	PnL                float64
}

// SwarmSimulationResult aggregates results from all fish in a MiroFish simulation run.
type SwarmSimulationResult struct {
	ScenarioID  string
	Timestamp   time.Time
	FishResults map[string]FishPerformance
	Consensus   map[string]ConsensusPrediction
	Anomalies   []Anomaly
	Confidence  float64
}

// ConsensusPrediction aggregates market direction predictions across fish.
type ConsensusPrediction struct {
	Symbol             string  `json:"symbol"`
	BullishCount       int     `json:"bullish_count"`
	BearishCount       int     `json:"bearish_count"`
	NeutralCount       int     `json:"neutral_count"`
	AverageConfidence  float64 `json:"average_confidence"`
	ConsensusDirection string  `json:"consensus_direction"`
}

// Anomaly flags unusual patterns detected during simulation.
type Anomaly struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Severity    float64  `json:"severity"`
	Symbols     []string `json:"symbols"`
}

// SwarmConfig configures swarm simulation behavior. Retained for backward
// compatibility even though the simulation engine has been demoted.
type SwarmConfig struct {
	FishCount            int
	SimulationHorizon    time.Duration
	TimeStep             time.Duration
	ConvergenceThreshold float64
	Parallelism          int
}

// DefaultSwarmConfig returns recommended defaults for SwarmConfig.
func DefaultSwarmConfig() SwarmConfig {
	return SwarmConfig{
		FishCount:            100,
		SimulationHorizon:    30 * 24 * time.Hour,
		TimeStep:             time.Hour,
		ConvergenceThreshold: 0.7,
		Parallelism:          10,
	}
}
