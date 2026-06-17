---
name: risk
description: "Skill for the Risk area of atlas. 77 symbols across 11 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Risk

77 symbols | 11 files | Cohesion: 83%

## When to Use

- Working with code in `internal/`
- Understanding how TestNewCapitalPhaseController, TestGetCapitalLimit, TestGetCapitalLimitUnknownPhase work
- Modifying risk-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/risk/capital_controller_test.go` | TestNewCapitalPhaseController, TestGetCapitalLimit, TestGetCapitalLimitUnknownPhase, TestCalculateMaxPositionSize, TestCanAdvance_MinDaysNotMet (+13) |
| `internal/risk/approval_workflow_test.go` | setupWorkflow, TestNewApprovalWorkflow_CreatesDirectory, TestRequestApproval_Success, TestRequestApproval_MissingType, TestRequestApproval_MissingRequestedBy (+11) |
| `internal/risk/capital_controller.go` | NewCapitalPhaseController, GetSnapshot, UpdateMetrics, CanAdvance, AdvancePhase (+6) |
| `internal/risk/approval_workflow.go` | NewApprovalWorkflow, RequestApproval, Approve, Reject, LoadAll (+2) |
| `internal/risk/var_calculator_test.go` | TestCalculateMaxDrawdown, TestCalculateMaxDrawdownNoDecline, TestComputeRiskSnapshot, TestCalculateVaR, TestCalculateVaREmpty (+2) |
| `internal/risk/macro_aware_drawdown.go` | NewDefaultDrawdownLevels, NewMacroAwareDrawdownEngine, NewMacroAwareDrawdownEngineWithConfig, ShouldHaltTrading, GetSectorConstraints (+1) |
| `internal/risk/var_calculator.go` | CalculateMaxDrawdown, ComputeRiskSnapshot, CalculateVaR, CalculateCVaR |
| `internal/monitoring/dashboard_api.go` | handleCapitalPhase, handleRiskMetrics, loadRiskSnapshot |
| `internal/risk/macro_aware_drawdown_test.go` | TestMacroAwareDrawdownEngine_GetSectorConstraints, TestMacroAwareDrawdownEngine_ShouldHaltTrading, TestMacroAwareDrawdownEngine_CalculatePortfolioAdjustment |
| `internal/domain/types.go` | DefaultCapitalPhaseConfig |

## Entry Points

Start here when exploring this area:

- **`TestNewCapitalPhaseController`** (Function) — `internal/risk/capital_controller_test.go:9`
- **`TestGetCapitalLimit`** (Function) — `internal/risk/capital_controller_test.go:21`
- **`TestGetCapitalLimitUnknownPhase`** (Function) — `internal/risk/capital_controller_test.go:46`
- **`TestCalculateMaxPositionSize`** (Function) — `internal/risk/capital_controller_test.go:58`
- **`TestCanAdvance_MinDaysNotMet`** (Function) — `internal/risk/capital_controller_test.go:72`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestNewCapitalPhaseController` | Function | `internal/risk/capital_controller_test.go` | 9 |
| `TestGetCapitalLimit` | Function | `internal/risk/capital_controller_test.go` | 21 |
| `TestGetCapitalLimitUnknownPhase` | Function | `internal/risk/capital_controller_test.go` | 46 |
| `TestCalculateMaxPositionSize` | Function | `internal/risk/capital_controller_test.go` | 58 |
| `TestCanAdvance_MinDaysNotMet` | Function | `internal/risk/capital_controller_test.go` | 72 |
| `TestCanAdvance_DrawdownExceeded` | Function | `internal/risk/capital_controller_test.go` | 88 |
| `TestCanAdvance_SharpeNotMet` | Function | `internal/risk/capital_controller_test.go` | 105 |
| `TestCanAdvance_AllCriteriaMet` | Function | `internal/risk/capital_controller_test.go` | 119 |
| `TestAdvancePhase_Success` | Function | `internal/risk/capital_controller_test.go` | 134 |
| `TestAdvancePhase_CannotAdvance` | Function | `internal/risk/capital_controller_test.go` | 157 |
| `TestAdvancePhase_AtFinalPhase` | Function | `internal/risk/capital_controller_test.go` | 170 |
| `TestUpdateMetrics_UpdatesSnapshot` | Function | `internal/risk/capital_controller_test.go` | 184 |
| `TestCanAdvance_ConsecutiveLossesExceeded` | Function | `internal/risk/capital_controller_test.go` | 244 |
| `TestRecordLoss_IncrementsCounter` | Function | `internal/risk/capital_controller_test.go` | 261 |
| `TestRecordWin_ResetsCounter` | Function | `internal/risk/capital_controller_test.go` | 281 |
| `TestRecordLoss_BlocksAdvance` | Function | `internal/risk/capital_controller_test.go` | 298 |
| `TestRecordWin_AllowsRecovery` | Function | `internal/risk/capital_controller_test.go` | 325 |
| `TestNextPhase_Progression` | Function | `internal/risk/capital_controller_test.go` | 349 |
| `NewCapitalPhaseController` | Function | `internal/risk/capital_controller.go` | 17 |
| `DefaultCapitalPhaseConfig` | Function | `internal/domain/types.go` | 226 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleDailySummary → CalculateVaR` | cross_community | 5 |
| `LoadDailySummary → CalculateVaR` | cross_community | 5 |
| `HandleRiskExposure → CalculateVaR` | cross_community | 4 |
| `HandleDailySummary → CalculateMaxDrawdown` | cross_community | 4 |
| `LoadDailySummary → CalculateMaxDrawdown` | cross_community | 4 |
| `HandleRiskExposure → CalculateMaxDrawdown` | cross_community | 3 |
| `RunSimulation → CapitalPhaseConfig` | cross_community | 3 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Industry | 5 calls |
| Config | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestNewCapitalPhaseController"})` — see callers and callees
2. `gitnexus_query({query: "risk"})` — find related execution flows
3. Read key files listed above for implementation details
