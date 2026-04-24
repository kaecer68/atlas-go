---
name: experiment
description: "Skill for the Experiment area of atlas. 75 symbols across 16 files."
---

# Experiment

75 symbols | 16 files | Cohesion: 66%

## When to Use

- Working with code in `internal/`
- Understanding how ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins, TestScreenedRecommendationsFlowThroughExperimentAndJudge, TestEvaluateUpdatesStatus work
- Modifying experiment-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/experiment/executor.go` | mutateCandidate, mutatePromptCandidate, mutatePromptCandidateV3, stripMutationSections, v2ControlBlockAndBullets (+17) |
| `internal/experiment/judge.go` | NewJudge, Evaluate, loadExperimentResult, loadWindowSummary, windowSummaryPath (+8) |
| `internal/experiment/judge_test.go` | TestEvaluateUpdatesStatus, TestEvaluateRejectsMalformedResultContract, TestEvaluateRejectsInvalidStatusTransition, TestPassesAcceptanceUsesMaturityThresholds, TestPassesAcceptanceUsesMutationTypeProfiles (+4) |
| `internal/experiment/replay_compare.go` | comparePromptPerformance, comparePromptPerformanceDetailed, fallbackWindow, scoreConstraintWindowWithObservations, scoreSimulationResult (+1) |
| `internal/experiment/executor_test.go` | createTestReplayCSV, TestExecuteCreatesCandidatePrompt, TestExecuteUsesRiskRuleTemplate, TestExecuteUsesPortfolioConstraintTemplate, TestExecuteRejectsInvalidBriefContract |
| `cmd/judge-experiment/main.go` | main, findLatestExperiment, extractTimestamp, run |
| `internal/experiment/replay_compare_test.go` | TestComparePromptPerformance, TestComparePromptPerformanceSupportsConstraintMutations, TestComparePromptPerformanceSupportsGovernanceRouting, TestFallbackWindowExpandsUntilMinDatesMet |
| `internal/experiment/orchestrator_integration_test.go` | TestScreenedRecommendationsFlowThroughExperimentAndJudge, int64Ptr |
| `internal/ledger/ledger.go` | RecordPromptExperimentResult, UpdatePromptExperimentResult |
| `internal/experiment/validation.go` | ValidateReplayData, parseWindowDates |

## Entry Points

Start here when exploring this area:

- **`ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins`** (Function) — `internal/orchestrator/executors.go:33`
- **`TestScreenedRecommendationsFlowThroughExperimentAndJudge`** (Function) — `internal/experiment/orchestrator_integration_test.go:20`
- **`TestEvaluateUpdatesStatus`** (Function) — `internal/experiment/judge_test.go:13`
- **`TestEvaluateRejectsMalformedResultContract`** (Function) — `internal/experiment/judge_test.go:255`
- **`TestEvaluateRejectsInvalidStatusTransition`** (Function) — `internal/experiment/judge_test.go:292`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins` | Function | `internal/orchestrator/executors.go` | 33 |
| `TestScreenedRecommendationsFlowThroughExperimentAndJudge` | Function | `internal/experiment/orchestrator_integration_test.go` | 20 |
| `TestEvaluateUpdatesStatus` | Function | `internal/experiment/judge_test.go` | 13 |
| `TestEvaluateRejectsMalformedResultContract` | Function | `internal/experiment/judge_test.go` | 255 |
| `TestEvaluateRejectsInvalidStatusTransition` | Function | `internal/experiment/judge_test.go` | 292 |
| `NewJudge` | Function | `internal/experiment/judge.go` | 21 |
| `TestPassesAcceptanceUsesMaturityThresholds` | Function | `internal/experiment/judge_test.go` | 99 |
| `TestPassesAcceptanceUsesMutationTypeProfiles` | Function | `internal/experiment/judge_test.go` | 139 |
| `TestPassesAcceptanceRejectsWhenObservationsInsufficient` | Function | `internal/experiment/judge_test.go` | 179 |
| `TestPassesAcceptanceReportsNoConstraintDeltaWhenEqual` | Function | `internal/experiment/judge_test.go` | 374 |
| `TestRenderPromptControl` | Function | `internal/domain/prompt_control_test.go` | 40 |
| `RenderPromptControl` | Function | `internal/domain/prompt_control.go` | 39 |
| `TestRecordAndUpdatePromptExperimentResult` | Function | `internal/ledger/ledger_persistence_test.go` | 68 |
| `ValidateReplayData` | Function | `internal/experiment/validation.go` | 14 |
| `TestComparePromptPerformance` | Function | `internal/experiment/replay_compare_test.go` | 13 |
| `TestComparePromptPerformanceSupportsConstraintMutations` | Function | `internal/experiment/replay_compare_test.go` | 95 |
| `TestComparePromptPerformanceSupportsGovernanceRouting` | Function | `internal/experiment/replay_compare_test.go` | 138 |
| `TestFallbackWindowExpandsUntilMinDatesMet` | Function | `internal/experiment/replay_compare_test.go` | 181 |
| `TestExecuteCreatesCandidatePrompt` | Function | `internal/experiment/executor_test.go` | 27 |
| `TestExecuteUsesRiskRuleTemplate` | Function | `internal/experiment/executor_test.go` | 86 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → Dataset` | cross_community | 6 |
| `Main → Value` | cross_community | 6 |
| `HandleJudgeExperiment → Dataset` | cross_community | 5 |
| `HandleJudgeExperiment → Value` | cross_community | 5 |
| `HandleJudgeExperiment → ParseFloat` | cross_community | 5 |
| `HandleJudgeExperiment → ParseInt` | cross_community | 5 |
| `Main → Close` | cross_community | 5 |
| `HandleJudgeExperiment → ReplayScoreSummary` | cross_community | 4 |
| `HandleJudgeExperiment → ResolvePromptOverride` | cross_community | 4 |
| `RunDailySimulation → QuotesToNarrativeData` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Monitoring | 15 calls |
| Orchestrator | 12 calls |
| Domain | 4 calls |
| Baseline | 3 calls |
| Portfolio | 3 calls |
| Replay | 2 calls |
| Sim | 2 calls |
| Narrative | 2 calls |

## How to Explore

1. `gitnexus_context({name: "ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins"})` — see callers and callees
2. `gitnexus_query({query: "experiment"})` — find related execution flows
3. Read key files listed above for implementation details
