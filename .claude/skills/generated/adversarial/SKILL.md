---
name: adversarial
description: "Skill for the Adversarial area of atlas. 27 symbols across 5 files."
---

# Adversarial

27 symbols | 5 files | Cohesion: 68%

## When to Use

- Working with code in `internal/`
- Understanding how DefaultAdversarialConfig, NewAdversarialTrainer, TestAdversarialTrainer work
- Modifying adversarial-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/adversarial/adversarial_trainer.go` | executeBattle, selectStrategy, calculateEffectiveness, DefaultAdversarialConfig, NewAdversarialTrainer (+18) |
| `internal/swarm/mirofish_swarm.go` | calculateSentiment |
| `internal/metalearning/metalearner.go` | Float64 |
| `internal/marketdata/retail_sentiment_provider.go` | FetchSnapshot |
| `internal/adversarial/adversarial_test.go` | TestAdversarialTrainer |

## Entry Points

Start here when exploring this area:

- **`DefaultAdversarialConfig`** (Function) — `internal/adversarial/adversarial_trainer.go:125`
- **`NewAdversarialTrainer`** (Function) — `internal/adversarial/adversarial_trainer.go:150`
- **`TestAdversarialTrainer`** (Function) — `internal/adversarial/adversarial_test.go:9`
- **`Float64`** (Method) — `internal/metalearning/metalearner.go:781`
- **`FetchSnapshot`** (Method) — `internal/marketdata/retail_sentiment_provider.go:28`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `DefaultAdversarialConfig` | Function | `internal/adversarial/adversarial_trainer.go` | 125 |
| `NewAdversarialTrainer` | Function | `internal/adversarial/adversarial_trainer.go` | 150 |
| `TestAdversarialTrainer` | Function | `internal/adversarial/adversarial_test.go` | 9 |
| `Float64` | Method | `internal/metalearning/metalearner.go` | 781 |
| `FetchSnapshot` | Method | `internal/marketdata/retail_sentiment_provider.go` | 28 |
| `StressTestAgent` | Method | `internal/adversarial/adversarial_trainer.go` | 707 |
| `GetVulnerabilities` | Method | `internal/adversarial/adversarial_trainer.go` | 638 |
| `GenerateReport` | Method | `internal/adversarial/adversarial_trainer.go` | 782 |
| `RunTraining` | Method | `internal/adversarial/adversarial_trainer.go` | 320 |
| `calculateSentiment` | Function | `internal/swarm/mirofish_swarm.go` | 660 |
| `sortAgents` | Function | `internal/adversarial/adversarial_trainer.go` | 586 |
| `executeBattle` | Method | `internal/adversarial/adversarial_trainer.go` | 358 |
| `selectStrategy` | Method | `internal/adversarial/adversarial_trainer.go` | 439 |
| `calculateEffectiveness` | Method | `internal/adversarial/adversarial_trainer.go` | 476 |
| `initializeScenarios` | Method | `internal/adversarial/adversarial_trainer.go` | 227 |
| `simulateScenario` | Method | `internal/adversarial/adversarial_trainer.go` | 754 |
| `countTeamWins` | Method | `internal/adversarial/adversarial_trainer.go` | 601 |
| `calculateSeverity` | Method | `internal/adversarial/adversarial_trainer.go` | 678 |
| `generateRecommendation` | Method | `internal/adversarial/adversarial_trainer.go` | 691 |
| `calculateWinRate` | Method | `internal/adversarial/adversarial_trainer.go` | 842 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `RunTraining → Float64` | cross_community | 4 |
| `Main → Float64` | cross_community | 4 |
| `HandleRetailSentiment → Float64` | cross_community | 3 |

## How to Explore

1. `gitnexus_context({name: "DefaultAdversarialConfig"})` — see callers and callees
2. `gitnexus_query({query: "adversarial"})` — find related execution flows
3. Read key files listed above for implementation details
