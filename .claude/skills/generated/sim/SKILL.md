---
name: sim
description: "Skill for the Sim area of atlas. 54 symbols across 6 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Sim

54 symbols | 6 files | Cohesion: 70%

## When to Use

- Working with code in `internal/`
- Understanding how TestEngineUsesDynamicSlippage, TestEngineFallbackToFixedSlippage, TestRunBuildsPositions work
- Modifying sim-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/sim/engine.go` | NewEngine, WithOptimizer, WithSlippageModel, WithTaxCalculator, WithDividends (+14) |
| `internal/sim/slippage_model_test.go` | TestEngineUsesDynamicSlippage, TestEngineFallbackToFixedSlippage, TestDefaultSlippageModelTiers, TestSlippageModelZeroVolume, TestSlippageModelNil (+10) |
| `internal/sim/engine_test.go` | TestRunBuildsPositions, TestRunDeterministicTieBreakForEqualConviction, TestRunWithOptimizerProducesOrders, TestRunWithOptimizerRespectsMaxOpenPositions, TestRunWithTaxCalculatorPopulatesTaxFields (+6) |
| `internal/sim/slippage_model.go` | CalculateSlippageBPS, Precompute, DefaultSlippageModel, AdjustPriceForSlippage, EstimateMarketImpact |
| `internal/domain/sim.go` | NewSimulationState, PortfolioValue, SellLogicEnabled |
| `internal/portfolio/optimizer.go` | OptimizeToOrders |

## Entry Points

Start here when exploring this area:

- **`TestEngineUsesDynamicSlippage`** (Function) — `internal/sim/slippage_model_test.go:79`
- **`TestEngineFallbackToFixedSlippage`** (Function) — `internal/sim/slippage_model_test.go:129`
- **`TestRunBuildsPositions`** (Function) — `internal/sim/engine_test.go:12`
- **`TestRunDeterministicTieBreakForEqualConviction`** (Function) — `internal/sim/engine_test.go:43`
- **`TestRunWithOptimizerProducesOrders`** (Function) — `internal/sim/engine_test.go:74`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestEngineUsesDynamicSlippage` | Function | `internal/sim/slippage_model_test.go` | 79 |
| `TestEngineFallbackToFixedSlippage` | Function | `internal/sim/slippage_model_test.go` | 129 |
| `TestRunBuildsPositions` | Function | `internal/sim/engine_test.go` | 12 |
| `TestRunDeterministicTieBreakForEqualConviction` | Function | `internal/sim/engine_test.go` | 43 |
| `TestRunWithOptimizerProducesOrders` | Function | `internal/sim/engine_test.go` | 74 |
| `TestRunWithOptimizerRespectsMaxOpenPositions` | Function | `internal/sim/engine_test.go` | 105 |
| `TestRunWithTaxCalculatorPopulatesTaxFields` | Function | `internal/sim/engine_test.go` | 264 |
| `TestTaxAdjustedPnLEqualsGrossMinusTax` | Function | `internal/sim/engine_test.go` | 297 |
| `TestRunWithoutTaxCalculatorLeavesTaxFieldsZero` | Function | `internal/sim/engine_test.go` | 325 |
| `NewEngine` | Function | `internal/sim/engine.go` | 26 |
| `TestRunDayStopLoss` | Function | `internal/sim/engine_test.go` | 135 |
| `TestRunDayTakeProfit` | Function | `internal/sim/engine_test.go` | 169 |
| `TestRunDayConvictionReversal` | Function | `internal/sim/engine_test.go` | 197 |
| `TestRunMultiDayTwentyDays` | Function | `internal/sim/engine_test.go` | 226 |
| `NewSimulationState` | Function | `internal/domain/sim.go` | 17 |
| `TestDefaultSlippageModelTiers` | Function | `internal/sim/slippage_model_test.go` | 9 |
| `TestSlippageModelZeroVolume` | Function | `internal/sim/slippage_model_test.go` | 55 |
| `TestSlippageModelNil` | Function | `internal/sim/slippage_model_test.go` | 67 |
| `TestSlippageModelConsistencyWithAndWithoutPrecompute` | Function | `internal/sim/slippage_model_test.go` | 301 |
| `TestSlippageModelPrecompute` | Function | `internal/sim/slippage_model_test.go` | 197 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Tax | 4 calls |
| Portfolio | 3 calls |
| Config | 2 calls |

## How to Explore

1. `gitnexus_context({name: "TestEngineUsesDynamicSlippage"})` — see callers and callees
2. `gitnexus_query({query: "sim"})` — find related execution flows
3. Read key files listed above for implementation details
