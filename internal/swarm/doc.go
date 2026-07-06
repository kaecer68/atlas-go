// Package swarm provides backward-compatible type aliases and a lightweight
// state container for MiroFish Swarm consumers.
//
// The MiroFish simulation engine (GARCH, copula, jump-diffusion, calibration)
// was removed in PR #963.  This package now only contains:
//   - SwarmState: a pass-through state container (no-op Start/Stop methods)
//   - Type aliases (swarm_aliases.go): MarketState, SimulationResult,
//     ConsensusPrediction, Anomaly, FishPerformance, SwarmConfig
//   - Snapshot read helpers (snapshot.go): for reading legacy snapshot JSON files
//
// All simulation logic has been retired.  SwarmPlugin.ProcessRecommendations
// is now pass-through (no conviction adjustment).  The five swarm_get_* MCP
// tools remain registered for API backward compatibility and return empty results.
//
// Maturity: archived
package swarm
