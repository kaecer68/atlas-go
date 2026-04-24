---
name: risk
description: "Skill for the Risk area of atlas. 64 symbols across 11 files."
---

# Risk

64 symbols | 11 files | Cohesion: 75%

## When to Use

- Working with code in `internal/`
- Understanding how TestNewApprovalWorkflow_CreatesDirectory, TestRequestApproval_Success, TestRequestApproval_MissingType work
- Modifying risk-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/risk/approval_workflow_test.go` | setupWorkflow, TestNewApprovalWorkflow_CreatesDirectory, TestRequestApproval_Success, TestRequestApproval_MissingType, TestRequestApproval_MissingRequestedBy (+11) |
| `internal/risk/macro_aware_drawdown.go` | NewDefaultDrawdownLevels, NewMacroAwareDrawdownEngine, NewMacroAwareDrawdownEngineWithConfig, ShouldHaltTrading, GetSectorConstraints (+4) |
| `internal/risk/capital_controller_test.go` | TestAdvancePhase_Success, TestAdvancePhase_CannotAdvance, TestAdvancePhase_AtFinalPhase, TestNextPhase_Progression, TestCanAdvance_MinDaysNotMet (+3) |
| `internal/risk/approval_workflow.go` | NewApprovalWorkflow, RequestApproval, Approve, Reject, LoadAll (+2) |
| `internal/risk/var_calculator_test.go` | TestCalculateMaxDrawdown, TestCalculateMaxDrawdownNoDecline, TestComputeRiskSnapshot, TestCalculateVaR, TestCalculateVaREmpty (+2) |
| `internal/risk/capital_controller.go` | UpdateMetrics, AdvancePhase, evaluateAdvanceCriteria, nextPhase, CanAdvance |
| `internal/risk/macro_aware_drawdown_test.go` | TestMacroAwareDrawdownEngine_GetSectorConstraints, TestMacroAwareDrawdownEngine_ShouldHaltTrading, TestMacroAwareDrawdownEngine_CalculatePortfolioAdjustment, TestMacroAwareDrawdownEngine_Evaluate |
| `internal/risk/var_calculator.go` | CalculateMaxDrawdown, ComputeRiskSnapshot, CalculateVaR, CalculateCVaR |
| `internal/narrative/macro_assessment.go` | String, buildRationale |
| `internal/monitoring/dashboard_api.go` | loadRiskSnapshot |

## Entry Points

Start here when exploring this area:

- **`TestNewApprovalWorkflow_CreatesDirectory`** (Function) — `internal/risk/approval_workflow_test.go:17`
- **`TestRequestApproval_Success`** (Function) — `internal/risk/approval_workflow_test.go:29`
- **`TestRequestApproval_MissingType`** (Function) — `internal/risk/approval_workflow_test.go:51`
- **`TestRequestApproval_MissingRequestedBy`** (Function) — `internal/risk/approval_workflow_test.go:60`
- **`TestApprove_Success`** (Function) — `internal/risk/approval_workflow_test.go:69`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestNewApprovalWorkflow_CreatesDirectory` | Function | `internal/risk/approval_workflow_test.go` | 17 |
| `TestRequestApproval_Success` | Function | `internal/risk/approval_workflow_test.go` | 29 |
| `TestRequestApproval_MissingType` | Function | `internal/risk/approval_workflow_test.go` | 51 |
| `TestRequestApproval_MissingRequestedBy` | Function | `internal/risk/approval_workflow_test.go` | 60 |
| `TestApprove_Success` | Function | `internal/risk/approval_workflow_test.go` | 69 |
| `TestApprove_NotFound` | Function | `internal/risk/approval_workflow_test.go` | 94 |
| `TestApprove_AlreadyApproved` | Function | `internal/risk/approval_workflow_test.go` | 103 |
| `TestReject_Success` | Function | `internal/risk/approval_workflow_test.go` | 115 |
| `TestReject_AlreadyRejected` | Function | `internal/risk/approval_workflow_test.go` | 134 |
| `TestLoadAll_Empty` | Function | `internal/risk/approval_workflow_test.go` | 146 |
| `TestLoadAll_Persistence` | Function | `internal/risk/approval_workflow_test.go` | 158 |
| `TestGetRequest_NotFound` | Function | `internal/risk/approval_workflow_test.go` | 179 |
| `TestPendingRequests` | Function | `internal/risk/approval_workflow_test.go` | 188 |
| `TestApprovalWorkflow_StatusTracking` | Function | `internal/risk/approval_workflow_test.go` | 207 |
| `TestApprovalWorkflow_FilePersistsAcrossInstances` | Function | `internal/risk/approval_workflow_test.go` | 226 |
| `NewApprovalWorkflow` | Function | `internal/risk/approval_workflow.go` | 36 |
| `TestMacroAwareDrawdownEngine_GetSectorConstraints` | Function | `internal/risk/macro_aware_drawdown_test.go` | 128 |
| `TestMacroAwareDrawdownEngine_ShouldHaltTrading` | Function | `internal/risk/macro_aware_drawdown_test.go` | 189 |
| `TestMacroAwareDrawdownEngine_CalculatePortfolioAdjustment` | Function | `internal/risk/macro_aware_drawdown_test.go` | 215 |
| `NewDefaultDrawdownLevels` | Function | `internal/risk/macro_aware_drawdown.go` | 55 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleDailySummary → CalculateVaR` | cross_community | 5 |
| `HandleDailySummary → CalculateMaxDrawdown` | cross_community | 4 |
| `HandleRiskMetrics → CalculateVaR` | cross_community | 4 |
| `HandleDailySummary → Max` | cross_community | 3 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Orchestrator | 11 calls |
| Portfolio | 8 calls |
| Swarm | 4 calls |
| Config | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestNewApprovalWorkflow_CreatesDirectory"})` — see callers and callees
2. `gitnexus_query({query: "risk"})` — find related execution flows
3. Read key files listed above for implementation details
