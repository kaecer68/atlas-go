---
name: reflexivity
description: "Skill for the Reflexivity area of atlas. 17 symbols across 4 files."
---

# Reflexivity

17 symbols | 4 files | Cohesion: 85%

## When to Use

- Working with code in `internal/`
- Understanding how TestReflexivityEngine, TestPriceToFundamentalsRuleReducesConvictionOnCrash, TestPnLBehaviorRuleReducesConvictionOnDrawdown work
- Modifying reflexivity-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/reflexivity/reflexivity_engine.go` | GetLoopsByTarget, PredictLoopOutcome, calculateOutcome, GetReflexivityReport, ProcessRecommendations (+3) |
| `internal/reflexivity/concrete_rules_test.go` | TestPriceToFundamentalsRuleReducesConvictionOnCrash, TestPnLBehaviorRuleReducesConvictionOnDrawdown, TestNarrativeFlowsRuleReducesCrowdedSymbols, TestMarketPolicyRuleBoostsOnBroadDecline |
| `internal/reflexivity/concrete_rules.go` | Apply, Apply, Apply, Apply |
| `internal/reflexivity/reflexivity_test.go` | TestReflexivityEngine |

## Entry Points

Start here when exploring this area:

- **`TestReflexivityEngine`** (Function) — `internal/reflexivity/reflexivity_test.go:9`
- **`TestPriceToFundamentalsRuleReducesConvictionOnCrash`** (Function) — `internal/reflexivity/concrete_rules_test.go:8`
- **`TestPnLBehaviorRuleReducesConvictionOnDrawdown`** (Function) — `internal/reflexivity/concrete_rules_test.go:27`
- **`TestNarrativeFlowsRuleReducesCrowdedSymbols`** (Function) — `internal/reflexivity/concrete_rules_test.go:42`
- **`TestMarketPolicyRuleBoostsOnBroadDecline`** (Function) — `internal/reflexivity/concrete_rules_test.go:59`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestReflexivityEngine` | Function | `internal/reflexivity/reflexivity_test.go` | 9 |
| `TestPriceToFundamentalsRuleReducesConvictionOnCrash` | Function | `internal/reflexivity/concrete_rules_test.go` | 8 |
| `TestPnLBehaviorRuleReducesConvictionOnDrawdown` | Function | `internal/reflexivity/concrete_rules_test.go` | 27 |
| `TestNarrativeFlowsRuleReducesCrowdedSymbols` | Function | `internal/reflexivity/concrete_rules_test.go` | 42 |
| `TestMarketPolicyRuleBoostsOnBroadDecline` | Function | `internal/reflexivity/concrete_rules_test.go` | 59 |
| `GetLoopsByTarget` | Method | `internal/reflexivity/reflexivity_engine.go` | 309 |
| `PredictLoopOutcome` | Method | `internal/reflexivity/reflexivity_engine.go` | 323 |
| `GetReflexivityReport` | Method | `internal/reflexivity/reflexivity_engine.go` | 368 |
| `ProcessRecommendations` | Method | `internal/reflexivity/reflexivity_engine.go` | 452 |
| `ApplyReflexivityAdjustment` | Method | `internal/reflexivity/reflexivity_engine.go` | 506 |
| `Apply` | Method | `internal/reflexivity/concrete_rules.go` | 15 |
| `Apply` | Method | `internal/reflexivity/concrete_rules.go` | 43 |
| `Apply` | Method | `internal/reflexivity/concrete_rules.go` | 63 |
| `Apply` | Method | `internal/reflexivity/concrete_rules.go` | 101 |
| `extractAgentIDs` | Function | `internal/reflexivity/reflexivity_engine.go` | 482 |
| `average` | Function | `internal/reflexivity/reflexivity_engine.go` | 494 |
| `calculateOutcome` | Method | `internal/reflexivity/reflexivity_engine.go` | 337 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Orchestrator | 5 calls |

## How to Explore

1. `gitnexus_context({name: "TestReflexivityEngine"})` — see callers and callees
2. `gitnexus_query({query: "reflexivity"})` — find related execution flows
3. Read key files listed above for implementation details
