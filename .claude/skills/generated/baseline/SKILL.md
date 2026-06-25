---
name: baseline
description: "Skill for the Baseline area of atlas. 38 symbols across 8 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Baseline

38 symbols | 8 files | Cohesion: 70%

## When to Use

- Working with code in `internal/baseline/`
- Understanding how `Trigger`, `NewTrigger`, `Violation` and constraint evaluation work
- Modifying baseline-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/baseline/policy.go` | Save, ApplyConstraintCandidate, parseIntValue, parseInt64Value, parseFloatValue (+6) |
| `internal/baseline/rollback_test.go` | TestRevert_LastPromotion, TestRevert_ToVersion, TestRevert_ToExperiment, TestRevert_ValidationErrors, TestGetPromotionHistory (+3) |
| `internal/baseline/trigger.go` | Trigger, NewTrigger, Start, Stop, evaluate, Violation |
| `internal/baseline/trigger_test.go` | TestTrigger_StartStopLifecycle, TestTrigger_Evaluate_StopLoss, TestTrigger_Evaluate_TakeProfit, TestTrigger_Evaluate_MaxHoldingDays, TestTrigger_OnPositionUpdate_Integration (+5) |
| `cmd/revert-baseline/main.go` | main, showPromotionHistory, truncate |
| `internal/baseline/policy_test.go` | TestPromoteAcceptedPromptExperiment, TestPromoteAcceptedConstraintExperiment |
| `internal/baseline/manager.go` | NewManager |
| `internal/experiment/replay_compare_test.go` | TestApplyConstraintCandidateParsesRiskAndPortfolioFields |

## Entry Points

Start here when exploring this area:

- **`TestRevert_LastPromotion`** (Function) — `internal/baseline/rollback_test.go:8`
- **`TestRevert_ToVersion`** (Function) — `internal/baseline/rollback_test.go:53`
- **`TestRevert_ToExperiment`** (Function) — `internal/baseline/rollback_test.go:95`
- **`TestTrigger_StartStopLifecycle`** (Function) — `internal/baseline/trigger_test.go:43`
- **`NewTrigger`** (Function) — `internal/baseline/trigger.go:34`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestRevert_LastPromotion` | Function | `internal/baseline/rollback_test.go` | 8 |
| `TestRevert_ToVersion` | Function | `internal/baseline/rollback_test.go` | 53 |
| `TestRevert_ToExperiment` | Function | `internal/baseline/rollback_test.go` | 95 |
| `TestRevert_ValidationErrors` | Function | `internal/baseline/rollback_test.go` | 126 |
| `TestGetPromotionHistory` | Function | `internal/baseline/rollback_test.go` | 162 |
| `TestGetRevertHistory` | Function | `internal/baseline/rollback_test.go` | 201 |
| `TestTrigger_StartStopLifecycle` | Function | `internal/baseline/trigger_test.go` | 43 |
| `TestTrigger_StartRequiresManager` | Function | `internal/baseline/trigger_test.go` | 74 |
| `TestTrigger_StartRequiresBus` | Function | `internal/baseline/trigger_test.go` | 82 |
| `TestTrigger_Evaluate_StopLoss` | Function | `internal/baseline/trigger_test.go` | 90 |
| `TestTrigger_Evaluate_TakeProfit` | Function | `internal/baseline/trigger_test.go` | 115 |
| `TestTrigger_Evaluate_MaxHoldingDays` | Function | `internal/baseline/trigger_test.go` | 140 |
| `TestTrigger_Evaluate_NoViolation` | Function | `internal/baseline/trigger_test.go` | 162 |
| `TestTrigger_Evaluate_AllViolations` | Function | `internal/baseline/trigger_test.go` | 183 |
| `NewTrigger` | Function | `internal/baseline/trigger.go` | 34 |
| `Trigger` | Struct | `internal/baseline/trigger.go` | 24 |
| `Violation` | Struct | `internal/baseline/trigger.go` | 14 |
| `Save` | Function | `internal/baseline/policy.go` | 91 |
| `NewManager` | Function | `internal/baseline/manager.go` | 13 |
| `ApplyConstraintCandidate` | Function | `internal/baseline/policy.go` | 127 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `RunLiveTrading → Policy` | cross_community | 5 |
| `RunLiveTrading → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `RunSimulation → Policy` | cross_community | 5 |
| `RunSimulation → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `NewProductionSystem → Policy` | cross_community | 5 |
| `Main → Policy` | cross_community | 4 |
| `Main → ExecutionPolicyFromConstraints` | cross_community | 4 |
| `HandleInbox → Policy` | cross_community | 4 |
| `HandleInbox → ExecutionPolicyFromConstraints` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Industry | 2 calls |

## How to Explore

1. `gitnexus_context({name: "NewTrigger"})` — see callers and callees
2. `gitnexus_query({query: "baseline"})` — find related execution flows
3. Read key files listed above for implementation details
