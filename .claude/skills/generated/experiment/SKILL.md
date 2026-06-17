---
name: experiment
description: "Skill for the Experiment area of atlas. 102 symbols across 18 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Experiment

102 symbols | 18 files | Cohesion: 71%

## When to Use

- Working with code in `internal/`
- Understanding how TestEvaluateUpdatesStatus, TestEvaluateRejectsMalformedResultContract, TestEvaluateRejectsInvalidStatusTransition work
- Modifying experiment-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/experiment/executor.go` | mutateCandidate, mutatePromptCandidate, mutatePromptCandidateV3, stripMutationSections, v2ControlBlockAndBullets (+17) |
| `internal/experiment/judge.go` | NewJudge, Evaluate, loadExperimentResult, loadWindowSummary, windowSummaryPath (+10) |
| `internal/experiment/judge_test.go` | TestEvaluateUpdatesStatus, TestEvaluateRejectsMalformedResultContract, TestEvaluateRejectsInvalidStatusTransition, TestPassesAcceptanceUsesMaturityThresholds, TestPassesAcceptanceUsesMutationTypeProfiles (+7) |
| `internal/monitoring/api/experiment/handlers.go` | writeJSON, writeJSONError, promotionHistoryToAPI, HandlePromote, HandleRevert (+4) |
| `internal/experiment/oos_validator_test.go` | TestOOSValidator_ValidateWithConstraints, TestOOSAcceptanceThreshold, TestOOSMinimumObservations, TestOOSValidator_NewOOSValidator, TestOOSValidator_Validate_OOSWindow (+2) |
| `internal/experiment/oos_validator.go` | oosAcceptanceThreshold, oosMinimumObservations, oosWindow, ValidateWithBrief, ValidateWithConstraints (+2) |
| `internal/experiment/executor_test.go` | createTestReplayCSV, TestExecuteCreatesCandidatePrompt, TestExecuteUsesRiskRuleTemplate, TestExecuteUsesPortfolioConstraintTemplate, TestExecuteRejectsInvalidBriefContract |
| `cmd/judge-experiment/main.go` | main, findLatestExperiment, extractTimestamp, run |
| `internal/experiment/replay_compare_test.go` | TestFallbackWindowExpandsUntilMinDatesMet, TestComparePromptPerformance, TestComparePromptPerformanceSupportsConstraintMutations, TestComparePromptPerformanceSupportsGovernanceRouting |
| `internal/experiment/sharpe_stability_test.go` | TestSharpeStabilityCheck_InsufficientData, TestSharpeStabilityCheck_StableSeries, TestSharpeStabilityCheck_UnstableSeries, TestSharpeStabilityCheck_BoundaryThreshold |

## Entry Points

Start here when exploring this area:

- **`TestEvaluateUpdatesStatus`** (Function) — `internal/experiment/judge_test.go:14`
- **`TestEvaluateRejectsMalformedResultContract`** (Function) — `internal/experiment/judge_test.go:256`
- **`TestEvaluateRejectsInvalidStatusTransition`** (Function) — `internal/experiment/judge_test.go:293`
- **`NewJudge`** (Function) — `internal/experiment/judge.go:22`
- **`TestFallbackWindowExpandsUntilMinDatesMet`** (Function) — `internal/experiment/replay_compare_test.go:181`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestEvaluateUpdatesStatus` | Function | `internal/experiment/judge_test.go` | 14 |
| `TestEvaluateRejectsMalformedResultContract` | Function | `internal/experiment/judge_test.go` | 256 |
| `TestEvaluateRejectsInvalidStatusTransition` | Function | `internal/experiment/judge_test.go` | 293 |
| `NewJudge` | Function | `internal/experiment/judge.go` | 22 |
| `TestFallbackWindowExpandsUntilMinDatesMet` | Function | `internal/experiment/replay_compare_test.go` | 181 |
| `TestOOSValidator_ValidateWithConstraints` | Function | `internal/experiment/oos_validator_test.go` | 81 |
| `TestOOSAcceptanceThreshold` | Function | `internal/experiment/oos_validator_test.go` | 104 |
| `TestOOSMinimumObservations` | Function | `internal/experiment/oos_validator_test.go` | 111 |
| `TestPassesAcceptanceUsesMaturityThresholds` | Function | `internal/experiment/judge_test.go` | 100 |
| `TestPassesAcceptanceUsesMutationTypeProfiles` | Function | `internal/experiment/judge_test.go` | 140 |
| `TestPassesAcceptanceRejectsWhenObservationsInsufficient` | Function | `internal/experiment/judge_test.go` | 180 |
| `TestPassesAcceptanceReportsNoConstraintDeltaWhenEqual` | Function | `internal/experiment/judge_test.go` | 375 |
| `TestRenderPromptControl` | Function | `internal/domain/prompt_control_test.go` | 40 |
| `RenderPromptControl` | Function | `internal/domain/prompt_control.go` | 39 |
| `TestRecordAndUpdatePromptExperimentResult` | Function | `internal/ledger/ledger_persistence_test.go` | 68 |
| `ValidateReplayData` | Function | `internal/experiment/validation.go` | 12 |
| `TestExecuteCreatesCandidatePrompt` | Function | `internal/experiment/executor_test.go` | 27 |
| `TestExecuteUsesRiskRuleTemplate` | Function | `internal/experiment/executor_test.go` | 86 |
| `TestExecuteUsesPortfolioConstraintTemplate` | Function | `internal/experiment/executor_test.go` | 138 |
| `TestExecuteRejectsInvalidBriefContract` | Function | `internal/experiment/executor_test.go` | 190 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleJudge → Dataset` | cross_community | 5 |
| `HandleJudge → Value` | cross_community | 5 |
| `HandleJudge → ParseFloat` | cross_community | 5 |
| `Run → Dataset` | cross_community | 5 |
| `Run → Value` | cross_community | 5 |
| `HandleInbox → Policy` | cross_community | 4 |
| `HandleInbox → ExecutionPolicyFromConstraints` | cross_community | 4 |
| `HandleInbox → CanTransitionExperimentStatus` | cross_community | 4 |
| `HandleJudge → OOSValidator` | cross_community | 4 |
| `HandleJudge → ReplayScoreSummary` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Baseline | 13 calls |
| Ledger | 12 calls |
| Domain | 7 calls |
| Replay | 6 calls |
| Narrative | 3 calls |
| Industry | 1 calls |
| Config | 1 calls |
| Promote-baseline | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestEvaluateUpdatesStatus"})` — see callers and callees
2. `gitnexus_query({query: "experiment"})` — find related execution flows
3. Read key files listed above for implementation details
