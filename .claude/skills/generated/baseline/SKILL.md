---
name: baseline
description: "Skill for the Baseline area of atlas. 25 symbols across 6 files."
---

# Baseline

25 symbols | 6 files | Cohesion: 76%

## When to Use

- Working with code in `internal/`
- Understanding how TestRevert_LastPromotion, TestRevert_ToVersion, TestRevert_ToExperiment work
- Modifying baseline-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/baseline/policy.go` | Save, ApplyConstraintCandidate, parseIntValue, parseInt64Value, parseFloatValue (+5) |
| `internal/baseline/rollback_test.go` | TestRevert_LastPromotion, TestRevert_ToVersion, TestRevert_ToExperiment, TestRevert_ValidationErrors, TestGetPromotionHistory (+3) |
| `cmd/revert-baseline/main.go` | main, showPromotionHistory, truncate |
| `internal/baseline/policy_test.go` | TestPromoteAcceptedPromptExperiment, TestPromoteAcceptedConstraintExperiment |
| `internal/baseline/manager.go` | NewManager |
| `internal/experiment/replay_compare_test.go` | TestApplyConstraintCandidateParsesRiskAndPortfolioFields |

## Entry Points

Start here when exploring this area:

- **`TestRevert_LastPromotion`** (Function) — `internal/baseline/rollback_test.go:8`
- **`TestRevert_ToVersion`** (Function) — `internal/baseline/rollback_test.go:53`
- **`TestRevert_ToExperiment`** (Function) — `internal/baseline/rollback_test.go:95`
- **`TestRevert_ValidationErrors`** (Function) — `internal/baseline/rollback_test.go:126`
- **`TestGetPromotionHistory`** (Function) — `internal/baseline/rollback_test.go:162`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestRevert_LastPromotion` | Function | `internal/baseline/rollback_test.go` | 8 |
| `TestRevert_ToVersion` | Function | `internal/baseline/rollback_test.go` | 53 |
| `TestRevert_ToExperiment` | Function | `internal/baseline/rollback_test.go` | 95 |
| `TestRevert_ValidationErrors` | Function | `internal/baseline/rollback_test.go` | 126 |
| `TestGetPromotionHistory` | Function | `internal/baseline/rollback_test.go` | 162 |
| `TestGetRevertHistory` | Function | `internal/baseline/rollback_test.go` | 201 |
| `Save` | Function | `internal/baseline/policy.go` | 91 |
| `NewManager` | Function | `internal/baseline/manager.go` | 13 |
| `TestApplyConstraintCandidateParsesRiskAndPortfolioFields` | Function | `internal/experiment/replay_compare_test.go` | 48 |
| `ApplyConstraintCandidate` | Function | `internal/baseline/policy.go` | 127 |
| `TestPromoteAcceptedPromptExperiment` | Function | `internal/baseline/policy_test.go` | 24 |
| `TestPromoteAcceptedConstraintExperiment` | Function | `internal/baseline/policy_test.go` | 49 |
| `DefaultPolicy` | Function | `internal/baseline/policy.go` | 43 |
| `ExecutionPolicyFromConstraints` | Function | `internal/baseline/policy.go` | 106 |
| `Promote` | Function | `internal/baseline/policy.go` | 177 |
| `contains` | Function | `internal/baseline/rollback_test.go` | 188 |
| `containsHelper` | Function | `internal/baseline/rollback_test.go` | 192 |
| `main` | Function | `cmd/revert-baseline/main.go` | 12 |
| `showPromotionHistory` | Function | `cmd/revert-baseline/main.go` | 88 |
| `truncate` | Function | `cmd/revert-baseline/main.go` | 120 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `RunSimulation → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `NewProductionSystem → Policy` | cross_community | 5 |
| `Main → Policy` | cross_community | 5 |
| `Main → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `Main → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `Main → Policy` | cross_community | 4 |
| `Main → ExecutionPolicyFromConstraints` | cross_community | 4 |
| `HandleSystemHealth → Policy` | cross_community | 4 |
| `HandleSystemHealth → ExecutionPolicyFromConstraints` | cross_community | 4 |
| `HandleExperimentInbox → Policy` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Live | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestRevert_LastPromotion"})` — see callers and callees
2. `gitnexus_query({query: "baseline"})` — find related execution flows
3. Read key files listed above for implementation details
