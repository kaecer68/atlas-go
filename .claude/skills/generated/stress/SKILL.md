---
name: stress
description: "Skill for the Stress area of atlas. 20 symbols across 6 files."
---

# Stress

20 symbols | 6 files | Cohesion: 89%

## When to Use

- Working with code in `internal/`
- Understanding how TestRunnerRunScenario, TestRunnerRunScenarioMomentumDisabled, TestRunnerRunAll work
- Modifying stress-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/stress/runner_test.go` | TestRunnerRunScenario, TestRunnerRunScenarioMomentumDisabled, TestRunnerRunAll, TestAllScenariosReturnsFive, TestGetScenarioByID (+5) |
| `internal/stress/runner.go` | NewRunner, RunScenario, RunAll, FormatReport |
| `internal/stress/scenario.go` | AllScenarios, GetScenarioByID, MergeQuotes |
| `internal/orchestrator/plugin_control_test.go` | TestCIOPortfolioExecutorDeterministicTieBreak |
| `internal/orchestrator/plugin_control.go` | Apply |
| `internal/orchestrator/executors.go` | DefaultExecutionPolicy |

## Entry Points

Start here when exploring this area:

- **`TestRunnerRunScenario`** (Function) — `internal/stress/runner_test.go:61`
- **`TestRunnerRunScenarioMomentumDisabled`** (Function) — `internal/stress/runner_test.go:87`
- **`TestRunnerRunAll`** (Function) — `internal/stress/runner_test.go:111`
- **`NewRunner`** (Function) — `internal/stress/runner.go:41`
- **`TestCIOPortfolioExecutorDeterministicTieBreak`** (Function) — `internal/orchestrator/plugin_control_test.go:100`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestRunnerRunScenario` | Function | `internal/stress/runner_test.go` | 61 |
| `TestRunnerRunScenarioMomentumDisabled` | Function | `internal/stress/runner_test.go` | 87 |
| `TestRunnerRunAll` | Function | `internal/stress/runner_test.go` | 111 |
| `NewRunner` | Function | `internal/stress/runner.go` | 41 |
| `TestCIOPortfolioExecutorDeterministicTieBreak` | Function | `internal/orchestrator/plugin_control_test.go` | 100 |
| `DefaultExecutionPolicy` | Function | `internal/orchestrator/executors.go` | 469 |
| `AllScenarios` | Function | `internal/stress/scenario.go` | 93 |
| `GetScenarioByID` | Function | `internal/stress/scenario.go` | 104 |
| `TestAllScenariosReturnsFive` | Function | `internal/stress/runner_test.go` | 9 |
| `TestGetScenarioByID` | Function | `internal/stress/runner_test.go` | 16 |
| `TestGetScenarioByIDNotFound` | Function | `internal/stress/runner_test.go` | 26 |
| `TestFormatReport` | Function | `internal/stress/runner_test.go` | 137 |
| `FormatReport` | Function | `internal/stress/runner.go` | 156 |
| `TestScenarioMergeQuotes` | Function | `internal/stress/runner_test.go` | 33 |
| `RunScenario` | Method | `internal/stress/runner.go` | 56 |
| `RunAll` | Method | `internal/stress/runner.go` | 116 |
| `Apply` | Method | `internal/orchestrator/plugin_control.go` | 89 |
| `MergeQuotes` | Method | `internal/stress/scenario.go` | 114 |
| `contains` | Function | `internal/stress/runner_test.go` | 167 |
| `containsHelper` | Function | `internal/stress/runner_test.go` | 171 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Orchestrator | 1 calls |
| Config | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestRunnerRunScenario"})` — see callers and callees
2. `gitnexus_query({query: "stress"})` — find related execution flows
3. Read key files listed above for implementation details
