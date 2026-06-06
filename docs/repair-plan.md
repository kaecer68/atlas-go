# Atlas-Go Audit Repair Plan

> **Date**: 2026-06-01
> **Scope**: 7 root-cause findings from system audit
> **Status**: Planning -- implementation not started

---

## Audit Findings Summary

| ID | Finding | Severity | Priority |
|----|---------|----------|----------|
| R1 | All executors output BUY only; no SELL/REDUCE signal generation | Critical | P0 |
| R2 | Recommendation generation lacks portfolio context | Critical | P0 |
| R3 | No position rotation mechanism | Critical | P0 |
| R4 | Position sizing formula ignores existing holdings | High | P1 |
| R5 | PreTradeGate missing max_open_positions rule | Medium | P1 |
| R6 | baseline_policy.json and parameters.json have conflicting values | High | P1 |
| R7 | Recommendation processing order is non-deterministic | Medium | P1 |

---

## R1: All Executors Output BUY Only; No SELL/REDUCE Signal

### Current State

All 17 executors (`plugin_sector.go`, `plugin_style.go`, `plugin_control.go`) hardcode `Side` to `domain.SideBuy` in their `Recommend()` methods. The only SELL signal path is `engine.go:shouldSellPosition()` -- limited to stop-loss, take-profit, and conviction_reversal (passive exits only). Executors cannot actively recommend selling.

### Files to Change

1. `internal/domain/shared/shared.go` -- Add `SideReduce Side = "REDUCE"`
2. `internal/domain/aliases.go` -- Add `SideReduce = shared.SideReduce` alias
3. `internal/orchestrator/plugin_sector.go` -- Each executor's `Recommend()` method
4. `internal/orchestrator/plugin_style.go` -- Each executor's `Recommend()` method
5. `internal/orchestrator/plugin_control.go` -- `SuperinvestorExecutor.Recommend()` method
6. `internal/orchestrator/conviction_builder.go` -- Adjust builder for negative signal scoring (if needed)
7. `internal/sim/engine.go` -- `shouldSellPosition()`, `executeBuys()`, `executeLegacyBuys()`, `filterByPreTradeGate()`
8. `internal/orchestrator/executors_test.go` -- Add REDUCE/SELL signal test cases

### New / Modified Functions

```go
// shared.go -- new Side constant
const SideReduce Side = "REDUCE"  // partial reduction, not full sell

// Per-executor helper for reverse signal detection
func shouldSignalReduce(quote domain.Quote, prompt string, regime domain.Regime) (bool, string, int)
// Returns: (should_reduce, reason, reduction_strength 0-100)

// Each executor's Recommend() becomes two-phase:
// Phase 1: Check sell conditions (overbought, narrative reversal, regime shift, factor decay)
// Phase 2: Check buy conditions (existing logic)
// Output the direction with the higher signal strength

// engine.go -- shouldSellPosition() extended
// New: REDUCE handling -- proportional reduction instead of full exit
// New: conviction-based reduce (when executor produces REDUCE signal)
```

### Input/Output Changes

- **Input**: Executors now consider contrarian factors (RSI > 80, regime shift) and downgrade/avoid keywords in prompts
- **Output**: `Recommend()` may return `SideBuy`, `SideSell`, or `SideReduce`; `shouldSellPosition()` handles REDUCE (partial reduction)
- Collected recommendations array now contains multi-direction entries

### Risk Assessment

- **HIGH risk**: Modifying 17 executors' Recommend() logic affects all backtest and simulation results
- **Mitigation**: Start with only `ETFRotationExecutor` and `TechnicalBreakoutExecutor` (they have clear regime-contrary and breakdown-failure conditions); other executors retain BUY-only behavior
- Define explicit SELL/REDUCE trigger condition tables per executor type to avoid overtrading

---

## R2: Recommendation Generation Lacks Portfolio Context

### Current State

`executors.go:391 collectRecommendations()` does not accept existing holdings or portfolio state. The `Recommend()` interface only receives `(agent, quote, prompt, regime, FactorQuery)` -- no `[]domain.Position`. Each symbol's recommendation is generated independently. Executors have no awareness of existing positions, weights, or sector exposure.

### Files to Change

1. `internal/orchestrator/plugin.go` -- Add optional `RecommendWithContext()` method to `AgentExecutor` interface, or extend `Recommend()` signature
2. `internal/orchestrator/executors.go` -- `collectRecommendations()` accepts and passes `PortfolioSnapshot`
3. `internal/orchestrator/executors.go:140` -- Call site passes current portfolio state
4. `internal/orchestrator/conviction_builder.go` -- Add `addPortfolioOverlapPenalty()` method

### New Types and Functions

```go
// Light-weight portfolio snapshot
type PortfolioSnapshot struct {
    Positions       []domain.Position
    TotalValue      float64
    SectorWeights   map[string]float64
    CashRatio       float64
    ActivePositions int
    AvailableSlots  int  // max_open - current
}

// Optional interface (backward-compatible)
type ContextAwareExecutor interface {
    RecommendWithContext(
        agent domain.AgentSpec,
        quote domain.Quote,
        prompt string,
        regime domain.Regime,
        fq FactorQuery,
        pf PortfolioSnapshot,
    ) (domain.Recommendation, bool)
}
```

### Input/Output Changes

- **Input**: `collectRecommendations()` gains a `PortfolioSnapshot` parameter
- **Output**: When a symbol is already held near its weight cap, recommendation conviction is penalized; when sector exposure is at limit, same-sector recommendations are downgraded
- Executors implementing `ContextAwareExecutor` can perceive portfolio state; non-implementing executors retain existing behavior (graceful degradation)

### Risk Assessment

- **LOW-MEDIUM risk**: Optional interface method ensures backward compatibility
- If `PortfolioSnapshot` construction is expensive, compute once per trading day and pass to all executors
- May change backtest results; conviction thresholds may need recalibration

---

## R3: No Position Rotation Mechanism

### Current State

Only passive sell logic exists (stop-loss / take-profit). When executor A generates 60 conviction for held symbol X and 80 conviction for candidate Y, if portfolio is full (MaxOpenPositions=5), Y's buy is skipped and X is not sold -- even though Y is clearly superior.

`engine.go:593-596 executeLegacyBuys()`:
```go
if len(positions) >= e.constraints.MaxOpenPositions {
    break  // abandons without considering rotation
}
```

### Files to Change

1. `internal/sim/engine.go` -- `executeLegacyBuys()` and `executeOptimizerBuys()` add rotation logic
2. `internal/portfolio/rotation.go` -- **NEW FILE**: rotation decision logic
3. `configs/parameters.json` -- Add `rotation` section

### New Types and Functions

```go
// internal/portfolio/rotation.go

type RotationConfig struct {
    MinConvictionSpread int     // minimum conviction gap to trigger rotation (default: 20)
    HoldingPenaltyDays  int     // new positions protected from rotation for N days (default: 3)
    MaxTurnoverDaily    float64 // max daily turnover ratio (default: 0.20)
}

type RotationSignal struct {
    SellSymbol string
    BuySymbol  string
    SellReason string
    BuyReason  string
    Spread     int  // buy_conviction - sell_conviction
}

func EvaluateRotation(
    existingPositions []domain.Position,
    buyCandidates []domain.Recommendation,
    config RotationConfig,
    quotes map[string]domain.Quote,
) []RotationSignal
```

### Input/Output Changes

- **Input**: Existing positions + buy candidate recommendations
- **Output**: `RotationSignal` list specifying which symbols to sell and buy
- `executeLegacyBuys()` in `sortedRecs` iteration: when portfolio is full, call `EvaluateRotation()` to check if any candidate significantly outperforms an existing holding (gap > MinConvictionSpread); if so, sell the weak holding and buy the strong candidate

### Risk Assessment

- **HIGH risk**: Rotation logic directly affects trading frequency and holding periods; may cause overtrading and increased transaction costs
- Must respect `max_turnover_daily` parameter (already in parameters.json: optimizer.max_turnover_daily = 0.2)
- Start with conservative parameters (MinConvictionSpread=25, HoldingPenaltyDays=5) and validate in backtests
- Ensure rotation decisions are deterministic (see R7 fix)

---

## R4: Position Sizing Formula Ignores Existing Holdings

### Current State

In `engine.go:467-558 executeOptimizerBuys()` and `engine.go:560-640 executeLegacyBuys()`:
```go
maxPerPosition := maxDeployableCash * maxPositionWeight
quantity := int(math.Floor(maxPerPosition/price/100.0) * 100)
```
For already-held symbols this **stacks** positions (merged via `appendOrUpdatePosition()`), ignoring current weight. If symbol X is already 15% of portfolio, a new buy adds another 18%, totaling 33% -- significantly exceeding the `max_position_weight` cap. PreTradeGate's `max_position_pct` rule (in `filterByPreTradeGate`) blocks the oversized order, but this happens after sizing: oversized order computed -> blocked by gate -> entire recommendation discarded, rather than sizing being adjusted.

### Files to Change

1. `internal/sim/engine.go` -- `executeLegacyBuys()` and `executeOptimizerBuys()` add existing-holding deduction
2. `internal/sim/engine.go` -- `buildOrderIntent()` considers existing holdings when computing notional

### New Functions

```go
// engine.go -- in executeLegacyBuys()
func calculateAdjustedSize(
    symbol string,
    baseSize float64,       // maxDeployableCash * maxPositionWeight
    existingPositions []domain.Position,
    totalValue float64,
    maxPositionPct float64,
) float64 {
    currentValue := 0.0
    for _, p := range existingPositions {
        if p.Symbol == symbol {
            currentValue = p.MarketValue
            break
        }
    }
    currentPct := currentValue / totalValue
    remainingRoom := (maxPositionPct - currentPct) * totalValue
    if remainingRoom <= 0 {
        return 0  // already at cap, no further increase
    }
    return math.Min(baseSize, remainingRoom)
}
```

### Input/Output Changes

- **Input**: Existing `[]domain.Position` (already available)
- **Output**: Adjusted position size not exceeding `maxPositionPct` limit
- For already-held symbols: no further increase (or only top-up to cap); for new symbols: original logic preserved

### Risk Assessment

- **LOW risk**: Pure correction, no strategy logic change, only formula correction
- May reduce overall deployment (cannot add to already-held positions); monitor deploy ratio changes
- After R1 (SELL signals) is implemented, re-buy after reduction will correctly calculate available space

---

## R5: PreTradeGate Missing max_open_positions Rule

### Current State

`risk/pre_trade.go` has 5 rules: `max_position_pct`, `sector_exposure`, `var_limit`, `cash_buffer`, `retail_sentiment`. Missing `max_open_positions` rule. The `MaxOpenPositions` limit is only enforced in `engine.go`'s `SimulationConstraints` (`len(positions) >= e.constraints.MaxOpenPositions`), not in PreTradeGate -- meaning the optimizer path (`executeOptimizerBuys`) has its own separate limiting logic, and the two are inconsistent.

### Files to Change

1. `internal/risk/pre_trade.go` -- Add `ruleMaxOpenPositions()` method
2. `internal/risk/pre_trade.go` -- `Check()` method adds invocation
3. `internal/risk/types.go` -- Extend `PortfolioState` with `OpenPositions int`
4. `configs/parameters.json` -- Add `max_open_positions` under `risk_gate.pre_trade`

### New Fields and Functions

```go
// PreTradeGate new field
type PreTradeGate struct {
    // ... existing fields ...
    maxOpenPositions int  // NEW
}

// NewPreTradeGate reads new parameter
func NewPreTradeGate() *PreTradeGate {
    cfg := config.GetParametersConfig()
    return &PreTradeGate{
        // ... existing ...
        maxOpenPositions: cfg.RiskGate.PreTrade.MaxOpenPositions.Value,
    }
}

// New rule
func (g *PreTradeGate) ruleMaxOpenPositions(order OrderIntent, pf PortfolioState) RuleResult {
    current := pf.OpenPositions
    isNew := pf.Positions[order.Symbol] == 0
    newCount := current
    if isNew {
        newCount++
    }
    return RuleResult{
        RuleName:     "max_open_positions",
        Passed:       newCount <= g.maxOpenPositions,
        CurrentValue: float64(newCount),
        Threshold:    float64(g.maxOpenPositions),
        Severity:     verdictSeverity(float64(newCount), float64(g.maxOpenPositions)),
        Message:      fmt.Sprintf("would open position %d/%d", newCount, g.maxOpenPositions),
    }
}
```

### Input/Output Changes

- **Input**: `PortfolioState` gains `OpenPositions int` field
- **Output**: New risk rule check; orders may be BLOCKED due to open positions exceeding limit
- Unifies the max_open_positions constraint source between engine layer and risk layer

### Risk Assessment

- **LOW risk**: New rule does not affect existing behavior (default value consistent with existing constraints)
- Ensure `parameters.json` `risk_gate.pre_trade.max_open_positions` matches `baseline.max_open_positions`
- `PortfolioState` must be extended to carry open positions count -- computed when building in `filterByPreTradeGate`

---

## R6: baseline_policy.json and parameters.json Parameter Conflicts

### Current State

Direct comparison of the two configuration files:

| Parameter | `baseline_policy.json` | `parameters.json` (baseline) |
|-----------|----------------------|------------------------------|
| `max_position_weight` | **0.25** | **0.18** |
| `min_recommendation_conviction` | **35** | **60** |
| `transaction_cost_bps` | **1.425** | **14.25** |
| `min_tradable_volume` | **2,000,000** | **1,000,000** |
| `conviction_floor` (execution_policy) | **35** | **50** (orchestrator) |

The `transaction_cost_bps` discrepancy is the most severe: `baseline_policy.json` uses 1.425 bps (1/10th of the actual TWSE rate of 0.1425% = 14.25 bps), causing transaction costs to be underestimated by 10x in backtests.

Loading logic: `baseline_policy.json`'s `constraints` block builds `SimulationConstraints`; `parameters.json`'s `baseline` section appears to be fallback/default values. It is currently unclear which value is actually used -- potential race condition or load-order issue.

### Files to Change

1. `data/state/baseline_policy.json` -- Fix conflicting values to match `parameters.json`
2. `configs/parameters.json` -- Ensure baseline section values are correct
3. `internal/config/config.go` -- Add parameter source consistency validation
4. `cmd/promote-baseline/main.go` -- Auto-sync corresponding values in parameters.json on promote

### New Functions

```go
// config.go -- startup consistency check
func ValidateParameterConsistency(baselinePolicy domain.BaselinePolicy, params ParametersConfig) []string {
    var warnings []string
    if math.Abs(baselinePolicy.Constraints.MaxPositionWeight - params.Baseline.MaxPositionWeight.Value) > 0.001 {
        warnings = append(warnings, fmt.Sprintf(
            "max_position_weight conflict: baseline_policy.json=%.4f, parameters.json=%.4f",
            baselinePolicy.Constraints.MaxPositionWeight,
            params.Baseline.MaxPositionWeight.Value,
        ))
    }
    // ... other parameters ...
    return warnings
}
```

### Input/Output Changes

- WARN-level log output on startup when conflicting parameters are detected
- Corrected `baseline_policy.json` values consistent with `parameters.json`
- `transaction_cost_bps` fixed to 14.25 (accurately reflecting TWSE 0.1425% rate)

### Risk Assessment

- **HIGH risk**: Changing `transaction_cost_bps` significantly alters backtest results (10x cost increase); all historical experiment baselines need recalculation
- **Mitigation**: First fix only `baseline_policy.json` values to match `parameters.json`, then add startup validation
- After correction: re-promote baseline and re-judge all pending experiments

---

## R7: Recommendation Processing Order Is Non-Deterministic

### Current State

Three non-deterministic sources:

1. **Agent iteration in `collectRecommendations()`** (`executors.go:402`):
   ```go
   for _, agent := range registry.Agents { ... }
   ```
   `registry.Agents` is loaded from `configs/agents.json` array. Currently stable but depends on JSON array order with no explicit sort -- fragile and implicit.

2. **Recommendation sort in `executeLegacyBuys()`** (`engine.go:573-584`):
   ```go
   sort.Slice(sortedRecs, func(i, j int) bool {
       if sortedRecs[i].Conviction != sortedRecs[j].Conviction {
           return sortedRecs[i].Conviction > sortedRecs[j].Conviction
       }
       if sortedRecs[i].Symbol != sortedRecs[j].Symbol {
           return sortedRecs[i].Symbol < sortedRecs[j].Symbol
       }
       // ...
   })
   ```
   Sorts by conviction -> symbol -> agent -> reason. When two recommendations have equal conviction, the alphabetically-first symbol is processed first, potentially consuming capital and position slots before the later one. Although `sort.Slice` itself is stable, if `registry.Agents` iteration order changes, the input order of collected recommendations affects tie-break results.

3. **Iteration in `filterByPreTradeGate()`** (`engine.go:136`):
   ```go
   for _, rec := range recs { ... }
   ```
   Depends on input order, which comes from sorted recs in `executeBuys()`.

### Files to Change

1. `internal/orchestrator/executors.go` -- Sort recs at end of `collectRecommendations()`
2. `internal/sim/engine.go` -- Improve tie-breaking logic in `executeLegacyBuys()`
3. `internal/orchestrator/plugin_control.go` -- `sort.Strings(symbols)` already deterministic; verify

### Changes

```go
// executors.go -- at end of collectRecommendations()
// Ensure consistent output regardless of agent iteration order
sort.SliceStable(recs, func(i, j int) bool {
    if recs[i].Conviction != recs[j].Conviction {
        return recs[i].Conviction > recs[j].Conviction
    }
    if recs[i].Symbol != recs[j].Symbol {
        return recs[i].Symbol < recs[j].Symbol
    }
    return recs[i].Agent < recs[j].Agent
})

// engine.go -- executeLegacyBuys() tie-breaking adds:
// When conviction is equal, prioritize symbols already held (reduce unnecessary churn)
```

### Input/Output Changes

- Given identical inputs, `collectRecommendations()` always produces identically-ordered recommendations
- `executeLegacyBuys()` provides more meaningful tie-breaking on conviction ties
- Backtest results become reproducible

### Risk Assessment

- **LOW risk**: No strategy logic change, only ensuring determinism and reproducibility
- `sort.SliceStable` preferred over `sort.Slice` (preserves original order as final tie-breaker)
- Existing tests may need updates for new output ordering

---

## Task Tracking Table

| ID | Task | Priority | Est. Effort | Risk | Status |
|----|------|----------|-------------|------|--------|
| R1 | Executor SELL/REDUCE signal generation | P0 | 3-5 days | HIGH | [ ] Not Started |
| R2 | Portfolio context in recommendations | P0 | 2-3 days | MEDIUM | [ ] Not Started |
| R3 | Position rotation mechanism | P0 | 3-4 days | HIGH | [ ] Not Started |
| R4 | Fix position sizing formula | P1 | 1-2 days | LOW | [ ] Not Started |
| R5 | PreTradeGate max_open_positions rule | P1 | 1 day | LOW | [ ] Not Started |
| R6 | Fix baseline/parameters conflicts | P1 | 1-2 days | HIGH (tx cost: P0) | [ ] Not Started |
| R7 | Deterministic recommendation ordering | P1 | 0.5-1 day | LOW | [ ] Not Started |

---

## Implementation Order Recommendation

```
Week 1:
  Day 1-2:  R7 (deterministic ordering) -- foundation for reproducible testing
  Day 2-3:  R4 (fix position sizing) -- pure bugfix, low risk
  Day 3-5:  R5 (PreTradeGate rule) -- isolated addition

Week 2:
  Day 1-3:  R6 (parameter conflicts) -- start with tx_cost_bps fix (P0 urgency)
  Day 3-5:  R1 Phase 1 (ETFRotationExecutor + TechnicalBreakoutExecutor SELL only)

Week 3:
  Day 1-3:  R2 (portfolio context) -- prerequisite for R3
  Day 3-5:  R3 Phase 1 (rotation with conservative params, backtest validation)

Week 4+:
  R1 Phase 2 (remaining executors SELL logic, guided by backtest results)
  R3 Phase 2 (tune rotation params based on backtest performance)
  Full regression backtest on all historical experiments
```

### Rationale

1. **R7 first** -- Deterministic ordering is the foundation. Without it, no backtest result can be trusted as reproducible. Also the lowest effort.
2. **R4 + R5** -- Pure bugfixes with isolated scope. Build confidence in the modified codebase without touching strategy logic.
3. **R6** -- The `transaction_cost_bps` fix (10x cost change) is urgent (effectively P0) because all backtest results with the wrong rate are invalid. This must be fixed before any strategy changes (R1, R2, R3) can be meaningfully validated.
4. **R1 (Phase 1)** -- Start with only 2 executors that have the clearest SELL conditions. This limits blast radius and allows controlled validation.
5. **R2 before R3** -- Rotation (R3) fundamentally needs portfolio context (R2) to make intelligent swap decisions. R2 provides the data foundation.
6. **R1 + R3 (Phase 2)** -- Expand based on validated Phase 1 results and backtest evidence.

---

## Acceptance Criteria

### R1: SELL/REDUCE Signals
- [ ] `SideReduce` constant defined in shared.go and aliased in aliases.go
- [ ] At least 2 executors (ETFRotation, TechnicalBreakout) produce SELL/REDUCE signals under defined conditions
- [ ] `shouldSellPosition()` handles REDUCE with proportional reduction (not full exit)
- [ ] Unit tests cover: SELL signal generation, REDUCE partial exit, conviction-based reduce
- [ ] Backtest on 30-day window shows diversified signal directions without overtrading
- [ ] `collectedRecs` contains multi-direction entries

### R2: Portfolio Context
- [ ] `PortfolioSnapshot` type defined and populated once per trading day
- [ ] `ContextAwareExecutor` interface defined (optional, backward-compatible)
- [ ] `collectRecommendations()` accepts and passes PortfolioSnapshot
- [ ] At least 1 executor demonstrates conviction adjustment based on existing position overlap
- [ ] Non-ContextAware executors continue to work without modification

### R3: Position Rotation
- [ ] `internal/portfolio/rotation.go` created with `EvaluateRotation()` function
- [ ] `RotationConfig` parameters in parameters.json with conservative defaults
- [ ] Rotation triggers only when conviction spread > MinConvictionSpread
- [ ] New positions protected from rotation for HoldingPenaltyDays
- [ ] Daily turnover does not exceed MaxTurnoverDaily
- [ ] Rotation decisions are deterministic (compatible with R7)
- [ ] Backtest shows rotation improves portfolio conviction-weighted score without excessive turnover

### R4: Position Sizing Fix
- [ ] `calculateAdjustedSize()` implemented and called in both executeLegacyBuys() and executeOptimizerBuys()
- [ ] Already-held symbols do not receive additional buys beyond max_position_pct
- [ ] New symbols sizing unchanged
- [ ] Unit test: symbol at 12% with max 18% correctly calculates remaining 6% room
- [ ] Unit test: symbol at 18% with max 18% returns 0 (no further increase)

### R5: PreTradeGate Rule
- [ ] `ruleMaxOpenPositions()` implemented in PreTradeGate
- [ ] `PortfolioState.OpenPositions` field added and populated
- [ ] `parameters.json` `risk_gate.pre_trade.max_open_positions` configured
- [ ] Rule correctly distinguishes new vs. existing positions (existing doesn't increase count)
- [ ] BLOCK verdict correctly returned when count exceeds limit

### R6: Parameter Consistency
- [ ] `baseline_policy.json` `transaction_cost_bps` corrected to 14.25
- [ ] All conflicting parameters resolved (max_position_weight, min_recommendation_conviction, min_tradable_volume, conviction_floor)
- [ ] `ValidateParameterConsistency()` runs at startup and logs WARN on conflicts
- [ ] Promote-baseline command syncs parameters.json baseline section
- [ ] All pending experiments re-judged after parameter correction

### R7: Deterministic Ordering
- [ ] `collectRecommendations()` sorts output with `sort.SliceStable` (conviction -> symbol -> agent)
- [ ] `executeLegacyBuys()` tie-breaking prioritizes already-held symbols on equal conviction
- [ ] Same input data produces identical backtest results across 3 consecutive runs
- [ ] Existing tests pass or are updated for new ordering

### Global
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `staticcheck ./...` passes
- [ ] CI coverage >= 40%
- [ ] Full regression backtest on 2026-Q1 window produces reasonable results
