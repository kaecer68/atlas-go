# Learnings

## [2026-05-20] Session Init
- Active branch: feat/hardcoded-params-config-migration (from feat/pipeline-narrative-industry-alignment)
- All new JSON fields must use omitempty
- All modulators must be nil-safe
- Three parallel weight systems intentionally preserved:
  OptimizerParameters.FactorWeights (6), OrchestratorParameters.FactorWeight* (4), FactorWeightEngine.baseWeights (8)
- TDD strategy: characterization test first → extract to config → verify zero behavior regression
- New structs are additive — do NOT modify existing FactorParameters or NarrativeParameters
- sector_rotator.go is partially config-driven; only applyMacroAdjustments + applyFlowAdjustments method bodies remain hardcoded
