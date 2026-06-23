// Package orchestrator is the brain of atlas-go: it coordinates domain
// experts, control-layer filtering, and extensible plugins to drive
// daily simulation runs.
//
// The executors subsystem (originally a single 1284-line executors.go) is
// now split across multiple files by concern. See executor_*.go for the
// individual implementation files:
//
//   - executor_types.go         — public types: LayerRouter, ExecutionContext,
//     ResearchResult, FilterAgentsByLayer
//   - executor_strategies.go    — 6 strategy interfaces + 6 default impls
//   - executor_pipeline.go      — ExecuteWithContext + 5 ExecuteRegistry*
//     wrappers
//   - executor_darwinian.go     — ExecuteRegistryResearchWithDarwinianWeights
//   - executor_policy.go        — DefaultExecutionPolicy
//   - executor_muted_filter.go  — filterMutedAgents, loadRecOverrides
//   - executor_regime.go        — inferRegime
//   - executor_collection.go    — collectRecommendations, avgConvictionScore
//   - executor_momentum_crash.go — applyMomentumCrashProtection
//   - executor_control.go       — applyControlLayerWithOutcomes +
//     applyCrowdingPenalty +
//     applyAntiCorrelationLayer +
//     severityForControlAgent + passRatio
//   - executor_symbols.go       — DefaultSymbols, loadSymbolsFromCSV,
//     ExpandUniverse, RegistrySymbols,
//     SymbolsForSkill, symbolIterator
package orchestrator
