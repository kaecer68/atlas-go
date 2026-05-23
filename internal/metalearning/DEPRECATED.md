# DEPRECATED — internal/metalearning

**Status**: Deprecated (Phase 1, 2026-05-23)  
**Re-enablement target**: Phase 5 of the System Health Remediation Plan

## What this package does

This package implements an ML-based meta-learning engine designed to optimize agent strategies using a genetic algorithm approach. It includes:

- `MetaLearner` — genetic algorithm strategy optimizer with population management, elite selection, crossover, and mutation
- `LearningStrategy` / `StrategyType` — typed strategy definitions (momentum, adaptive, curriculum, ensemble, evolutionary)
- `SwarmLearningData` — data ingestion from MiroFish swarm simulations
- `TrainingResult` — individual agent training outcomes
- Persistence layer (`Save`/`Load`) for strategy state

## Why it was deprecated

This package was **built and tested but never integrated** into the production pipeline for two reasons:

1. **Missing producer infrastructure**: The `SubmitSwarmData()` and `SubmitTrainingResult()` APIs require a data producer (swarm simulation → training pipeline) that was never built. The `internal/evolution/` package supersedes this with a simpler prompt-mutation approach that operates directly on agent configuration.

2. **No integration surface**: The `MetaLearner` has zero callers anywhere in the production codebase (confirmed via static analysis — no imports, no references). The genetic algorithm and strategy optimization features would require:
   - A scheduler to periodically trigger training cycles
   - A training data source (currently provided by `internal/ledger/` via `LoadAllSessionScorecards()`)
   - A dashboard visualization layer for human oversight
   - A promotion mechanism to feed optimized strategies back into the evolution pipeline

## Re-enablement conditions (Phase 5)

To restore this package to active use:

1. **Restore from deprecation**: Remove `DEPRECATED.md`, remove `// Deprecated` comment from `metalearner.go`, revert build constraint on `metalearning_test.go`
2. **Build training CLI**: `cmd/train-metalearner` that reads ledger scorecards, runs the genetic optimizer, and outputs `data/metalearning/strategies.json`
3. **Dashboard integration**: `/api/metalearning/strategies` endpoint + visualization
4. **Promotion CLI**: `cmd/promote-strategy` to feed optimized strategies into the evolution pipeline
5. **Scheduling** (Phase 5b): BackgroundTaskManager registration for automated training runs

## Original design intent

The package was envisioned as an autonomous learning layer that would:

- Collect training data from swarm simulations and agent performance
- Use genetic algorithms to evolve optimal strategy configurations
- Replace or augment the manual agent prompt tuning process
- Enable continuous self-improvement of the agent ecosystem

This vision is preserved for Phase 5 re-enablement. See `docs/iteration_playbook.md` and `docs/evolution_loop.md` for the current evolution system design.
