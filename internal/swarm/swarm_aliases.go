// Package swarm — backward-compatible type aliases for types migrated to domain.
//
// The simulation engine was demoted in PR #963. These aliases ensure
// existing consumers continue to compile without changes while allowing
// new code to import directly from internal/domain.
package swarm

import "github.com/kaecer68/atlas-go/internal/domain"

// MarketState is an alias for domain.MarketState.
type MarketState = domain.MarketState

// FishPerformance is an alias for domain.FishPerformance.
type FishPerformance = domain.FishPerformance

// ConsensusPrediction is an alias for domain.ConsensusPrediction.
type ConsensusPrediction = domain.ConsensusPrediction

// Anomaly is an alias for domain.Anomaly.
type Anomaly = domain.Anomaly

// SwarmConfig is an alias for domain.SwarmConfig.
type SwarmConfig = domain.SwarmConfig

// DefaultSwarmConfig delegates to domain.DefaultSwarmConfig.
var DefaultSwarmConfig = domain.DefaultSwarmConfig

// SimulationResult is an alias for domain.SwarmSimulationResult.
type SimulationResult = domain.SwarmSimulationResult

// NewMiroFishSwarm delegates to NewSwarmState for backward compatibility.
var NewMiroFishSwarm = NewSwarmState
