---
name: domain
description: "Skill for the Domain area of atlas. 17 symbols across 10 files."
---

# Domain

17 symbols | 10 files | Cohesion: 80%

## When to Use

- Working with code in `internal/`
- Understanding how TestExpireOldExperiments, ExpireOldExperiments, TestTransitionExperimentStatus_ValidTransitions work
- Modifying domain-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/domain/experiment_state_machine_test.go` | TestTransitionExperimentStatus_ValidTransitions, TestTransitionExperimentStatus_InvalidTransition, TestCanTransitionExperimentStatus_InitialState, TestTransitionExperimentStatus_ExpiredTransitions |
| `internal/experiment/ttl_test.go` | TestExpireOldExperiments, writeExp |
| `internal/domain/experiment_state_machine.go` | CanTransitionExperimentStatus, TransitionExperimentStatus |
| `internal/domain/prompt_control_test.go` | TestExtractPromptControl, TestExtractPromptControlMissing |
| `internal/domain/domain_test.go` | TestFlexTime_MarshalJSON_NonZero, TestFlexTime_MarshalJSON_Zero |
| `internal/experiment/ttl.go` | ExpireOldExperiments |
| `internal/monitoring/dashboard_api.go` | buildMutationSummary |
| `internal/domain/prompt_control.go` | ExtractPromptControl |
| `internal/baseline/policy.go` | ResolvePromptOverride |
| `internal/domain/time.go` | MarshalJSON |

## Entry Points

Start here when exploring this area:

- **`TestExpireOldExperiments`** (Function) — `internal/experiment/ttl_test.go:12`
- **`ExpireOldExperiments`** (Function) — `internal/experiment/ttl.go:17`
- **`TestTransitionExperimentStatus_ValidTransitions`** (Function) — `internal/domain/experiment_state_machine_test.go:7`
- **`TestTransitionExperimentStatus_InvalidTransition`** (Function) — `internal/domain/experiment_state_machine_test.go:18`
- **`TestCanTransitionExperimentStatus_InitialState`** (Function) — `internal/domain/experiment_state_machine_test.go:30`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestExpireOldExperiments` | Function | `internal/experiment/ttl_test.go` | 12 |
| `ExpireOldExperiments` | Function | `internal/experiment/ttl.go` | 17 |
| `TestTransitionExperimentStatus_ValidTransitions` | Function | `internal/domain/experiment_state_machine_test.go` | 7 |
| `TestTransitionExperimentStatus_InvalidTransition` | Function | `internal/domain/experiment_state_machine_test.go` | 18 |
| `TestCanTransitionExperimentStatus_InitialState` | Function | `internal/domain/experiment_state_machine_test.go` | 30 |
| `TestTransitionExperimentStatus_ExpiredTransitions` | Function | `internal/domain/experiment_state_machine_test.go` | 42 |
| `CanTransitionExperimentStatus` | Function | `internal/domain/experiment_state_machine.go` | 4 |
| `TransitionExperimentStatus` | Function | `internal/domain/experiment_state_machine.go` | 27 |
| `TestExtractPromptControl` | Function | `internal/domain/prompt_control_test.go` | 7 |
| `TestExtractPromptControlMissing` | Function | `internal/domain/prompt_control_test.go` | 33 |
| `ExtractPromptControl` | Function | `internal/domain/prompt_control.go` | 26 |
| `ResolvePromptOverride` | Function | `internal/baseline/policy.go` | 117 |
| `TestFlexTime_MarshalJSON_NonZero` | Function | `internal/domain/domain_test.go` | 90 |
| `TestFlexTime_MarshalJSON_Zero` | Function | `internal/domain/domain_test.go` | 111 |
| `MarshalJSON` | Method | `internal/domain/time.go` | 41 |
| `writeExp` | Function | `internal/experiment/ttl_test.go` | 77 |
| `buildMutationSummary` | Function | `internal/monitoring/dashboard_api.go` | 610 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleExperimentInbox → CanTransitionExperimentStatus` | cross_community | 4 |
| `HandleJudgeExperiment → ResolvePromptOverride` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Orchestrator | 1 calls |
| Live | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestExpireOldExperiments"})` — see callers and callees
2. `gitnexus_query({query: "domain"})` — find related execution flows
3. Read key files listed above for implementation details
