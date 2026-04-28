---
name: prism
description: "Skill for the Prism area of atlas. 22 symbols across 6 files."
---

# Prism

22 symbols | 6 files | Cohesion: 63%

## When to Use

- Working with code in `internal/`
- Understanding how TestTrainingQueue, NewTrainingQueue, FStr work
- Modifying prism-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/prism/prism_manager.go` | NewTrainingQueue, Enqueue, Dequeue, Len, Clear (+10) |
| `internal/logging/logger.go` | FStr, FInt |
| `internal/orchestrator/prism_executor.go` | NewPRISMTrainingExecutor, Execute |
| `internal/prism/prism_test.go` | TestTrainingQueue |
| `internal/config/config.go` | envOrInt |
| `internal/orchestrator/phase3_controller_test.go` | TestPRISMTrainingExecutorWithRealReplay |

## Entry Points

Start here when exploring this area:

- **`TestTrainingQueue`** (Function) — `internal/prism/prism_test.go:195`
- **`NewTrainingQueue`** (Function) — `internal/prism/prism_manager.go:97`
- **`FStr`** (Function) — `internal/logging/logger.go:82`
- **`FInt`** (Function) — `internal/logging/logger.go:83`
- **`NewPRISMTrainingExecutor`** (Function) — `internal/orchestrator/prism_executor.go:20`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestTrainingQueue` | Function | `internal/prism/prism_test.go` | 195 |
| `NewTrainingQueue` | Function | `internal/prism/prism_manager.go` | 97 |
| `FStr` | Function | `internal/logging/logger.go` | 82 |
| `FInt` | Function | `internal/logging/logger.go` | 83 |
| `NewPRISMTrainingExecutor` | Function | `internal/orchestrator/prism_executor.go` | 20 |
| `TestPRISMTrainingExecutorWithRealReplay` | Function | `internal/orchestrator/phase3_controller_test.go` | 18 |
| `Enqueue` | Method | `internal/prism/prism_manager.go` | 108 |
| `Dequeue` | Method | `internal/prism/prism_manager.go` | 124 |
| `Len` | Method | `internal/prism/prism_manager.go` | 192 |
| `Clear` | Method | `internal/prism/prism_manager.go` | 199 |
| `GetAllTasks` | Method | `internal/prism/prism_manager.go` | 208 |
| `String` | Method | `internal/prism/prism_manager.go` | 27 |
| `ClearQueue` | Method | `internal/prism/prism_manager.go` | 401 |
| `Rebalance` | Method | `internal/prism/prism_manager.go` | 412 |
| `UpdateTaskStatus` | Method | `internal/prism/prism_manager.go` | 171 |
| `Execute` | Method | `internal/orchestrator/prism_executor.go` | 29 |
| `abs` | Function | `internal/prism/prism_manager.go` | 663 |
| `envOrInt` | Function | `internal/config/config.go` | 95 |
| `countActiveQueues` | Method | `internal/prism/prism_manager.go` | 590 |
| `worker` | Method | `internal/prism/prism_manager.go` | 434 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → Len` | cross_community | 5 |
| `CollectMetrics → Len` | cross_community | 4 |
| `NewProductionSystem → TrainingQueue` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Live | 3 calls |
| Orchestrator | 2 calls |
| Replay | 1 calls |
| Baseline | 1 calls |
| Swarm | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestTrainingQueue"})` — see callers and callees
2. `gitnexus_query({query: "prism"})` — find related execution flows
3. Read key files listed above for implementation details
