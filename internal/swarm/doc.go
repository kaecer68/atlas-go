// Package swarm implements MiroFish Swarm - parallel simulated futures training.
//
// The swarm simulates multiple possible market futures to train agents on diverse
// scenarios. It runs 100 MiroFish agents across 5 market scenarios (bull, bear,
// high-volatility, low-volatility, regime transition). Each fish uses a GARCH
// process for volatility evolution, correlated shocks, and jump-diffusion to
// generate price paths. Fish predictions are evaluated against actual simulated
// price movements (not random).
//
// Key components:
//   - MiroFishSwarm: orchestrates parallel batch simulation
//   - MiroFish: individual simulation unit with PredictionRule and GARCH state
//   - Calibration framework (calibration.go): compares simulated statistics
//     against target market data and suggests parameter adjustments
//   - TrainingScenario export: feeds simulation results into the MetaLearner
//     via metalearning.SubmitTrainingScenarios for strategy evolution
//
// Maturity: experimental
package swarm
