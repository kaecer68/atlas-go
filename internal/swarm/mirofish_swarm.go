// Package swarm implements MiroFish Swarm — demoted in PR #963.
//
// The simulation engine (GARCH, copula, jump-diffusion, calibration) has been
// removed. The MiroFishSwarm and MiroFish struct types are retained for backward
// compatibility with test code and the snapshot persistence format.
//
// Use swarm.SwarmState for new code.
package swarm

import (
	"time"
)

// MiroFishSwarm manages parallel market simulations — deprecated.
// Use SwarmState instead.
type MiroFishSwarm struct {
	config      SwarmConfig
	fish        []*MiroFish
	scenarios   []MarketScenario
	results     []SimulationResult
	generations int
}

// MiroFish represents a single simulation unit — deprecated.
type MiroFish struct {
	ID           string
	Scenario     MarketScenario
	CurrentState MarketState
	History      []MarketState
	Predictions  []Prediction
	Performance  FishPerformance
	isAlive      bool
	spawnedAt    time.Time
}

// MarketScenario defines a possible market future.
type MarketScenario struct {
	ID         string
	Name       string
	Regime     string
	Volatility float64
	Trend      float64
	Duration   time.Duration
	Events     []MarketEvent
}

// MarketEvent represents a discrete market-moving event.
type MarketEvent struct {
	Time        time.Time
	Type        string
	Magnitude   float64
	Description string
}

// Prediction represents a fish's prediction at a moment — deprecated.
type Prediction struct {
	Timestamp  time.Time
	Symbol     string
	Direction  string // "up", "down", "neutral"
	Confidence float64
	Conviction int
	Rationale  string
}
