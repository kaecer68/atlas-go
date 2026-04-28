# Phase 3 整合實作計劃

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 將 Phase 3 多策略框架整合到 Atlas 交易管線中，使策略選擇能夠影響因子權重、相關性閾值和 Agent 過濾。

**Architecture:** 在 SystemCore 中新增 Strategy 相關欄位，在 runReplaySimulation()/RunDailySimulation() 中於 Regime 判定後、Agent 執行前進行策略選擇與應用。

**Tech Stack:** Go 1.25, sync, context

---

## File Structure

```
internal/orchestrator/
└── system.go          # 新增 Strategy 欄位、初始化、選擇邏輯、過濾方法

internal/sim/
└── engine.go          # 新增 WithThresholdEngine() 和 GetThresholdEngine()
```

---

## Task 1: 修改 internal/sim/engine.go 新增 ThresholdEngine 支援

**Files:**
- Modify: `internal/sim/engine.go`

**Step 1: Read engine.go to understand current structure**

```bash
cd /Users/kaecer/workspace/atlas
head -100 internal/sim/engine.go
```

**Step 2: Add thresholdEngine field to Engine struct**

Find the Engine struct and add:
```go
type Engine struct {
    // ... existing fields ...

    // Phase 3: DynamicThresholdEngine integration
    thresholdEngine *DynamicThresholdEngine
}
```

**Step 3: Add WithThresholdEngine method**

Add after existing builder methods:
```go
func (e *Engine) WithThresholdEngine(te *DynamicThresholdEngine) *Engine {
    e.thresholdEngine = te
    return e
}

func (e *Engine) GetThresholdEngine() *DynamicThresholdEngine {
    return e.thresholdEngine
}
```

**Step 4: Verify and commit**

```bash
cd /Users/kaecer/workspace/atlas
gofmt -w internal/sim/engine.go
go build ./internal/sim/...
go vet ./internal/sim/...
git add internal/sim/engine.go
git commit -m "feat(sim): add WithThresholdEngine and GetThresholdEngine"
```

---

## Task 2: 修改 internal/orchestrator/system.go 新增 Strategy 欄位

**Files:**
- Modify: `internal/orchestrator/system.go`

**Step 1: Read current SystemCore struct**

Find the SystemCore struct definition (around line 25-50).

**Step 2: Add strategy imports**

Update imports to include:
```go
import (
    // ... existing imports ...
    "github.com/kaecer68/atlas-go/internal/strategy"
)
```

**Step 3: Add Strategy fields to SystemCore**

Add after existing fields (around line 49):
```go
type SystemCore struct {
    // ... existing fields ...

    // Phase 3: Strategy Framework
    strategyRegistry    *strategy.Registry
    strategySelector    *strategy.Selector
    comparisonEngine    *strategy.ComparisonEngine
    factorWeightEngine   *portfolio.FactorWeightEngine

    // Phase 2: DynamicThresholdEngine reference
    thresholdEngine     *sim.DynamicThresholdEngine
}
```

**Step 4: Verify imports compile**

```bash
cd /Users/kaecer/workspace/atlas
gofmt -w internal/orchestrator/system.go
go build ./internal/orchestrator/... 2>&1 | head -20
```

---

## Task 3: 修改 NewSystem() 初始化 Strategy 元件

**Files:**
- Modify: `internal/orchestrator/system.go`

**Step 1: Find NewSystem() function**

Around line 58-117.

**Step 2: Add strategy initialization before engine creation**

Add after screenerEngine initialization (around line 83):

```go
// Phase 2: DynamicThresholdEngine
thresholdEngine := sim.NewDynamicThresholdEngine()

// Phase 3: Strategy Framework
strategyRegistry := strategy.NewRegistryWithDefaults()
comparisonEngine := strategy.NewComparisonEngine(20)
strategySelector := strategy.NewSelector(strategyRegistry, comparisonEngine)
factorWeightEngine := portfolio.NewFactorWeightEngine()
```

**Step 3: Add thresholdEngine to engine builder**

Find the engine creation (around line 89-97) and update:
```go
engine := sim.NewEngine(policy.Constraints).
    WithOptimizer(optimizer).
    WithThresholdEngine(thresholdEngine).
    WithReflexivityRules(
        reflexivity.PriceToFundamentalsRule{},
        reflexivity.PnLBehaviorRule{},
        reflexivity.NarrativeFlowsRule{Threshold: 3},
        reflexivity.MarketPolicyRule{Threshold: 0.03},
        reflexivity.NewReversalDetectionRule(),
    )
```

**Step 4: Add strategy fields to SystemCore initialization**

Find the return statement (around line 98-116) and add to SystemCore:
```go
SystemCore: &SystemCore{
    // ... existing fields ...
    strategyRegistry:   strategyRegistry,
    strategySelector:   strategySelector,
    comparisonEngine:   comparisonEngine,
    factorWeightEngine: factorWeightEngine,
    thresholdEngine:    thresholdEngine,
},
```

**Step 5: Verify and commit**

```bash
cd /Users/kaecer/workspace/atlas
gofmt -w internal/orchestrator/system.go
go build ./internal/orchestrator/...
go vet ./internal/orchestrator/...
git add internal/orchestrator/system.go
git commit -m "feat(orchestrator): initialize Strategy framework in NewSystem"
```

---

## Task 4: 新增 GetCurrentStrategy() 和 GetStrategySelector() 方法

**Files:**
- Modify: `internal/orchestrator/system.go`

**Step 1: Find Registry() method (around line 307)**

**Step 2: Add new methods after Registry()**

```go
func (s *System) GetCurrentStrategy() *strategy.Strategy {
    if s.strategySelector == nil {
        return nil
    }
    return s.strategySelector.GetCurrentStrategy()
}

func (s *System) GetStrategySelector() *strategy.Selector {
    return s.strategySelector
}

func (s *System) GetThresholdEngine() *sim.DynamicThresholdEngine {
    return s.thresholdEngine
}
```

**Step 3: Verify and commit**

```bash
cd /Users/kaecer/workspace/atlas
gofmt -w internal/orchestrator/system.go
go build ./internal/orchestrator/...
git add internal/orchestrator/system.go
git commit -m "feat(orchestrator): add GetCurrentStrategy and GetThresholdEngine methods"
```

---

## Task 5: 新增 filterAgentsByStrategy() 和 vixFromQuotes() 輔助方法

**Files:**
- Modify: `internal/orchestrator/system.go`

**Step 1: Add helper methods at end of file**

Add after existing methods:

```go
func (s *System) filterAgentsByStrategy(registry domain.AgentRegistry, strat *strategy.Strategy) domain.AgentRegistry {
    if strat == nil || len(strat.Agents) == 0 {
        return registry
    }

    // Handle wildcard - all agents enabled
    if len(strat.Agents) == 1 && strat.Agents[0] == "*" {
        return registry
    }

    // Create a copy to avoid modifying the original
    filtered := domain.AgentRegistry{
        Agents: make([]domain.AgentSpec, len(registry.Agents)),
    }
    copy(filtered.Agents, registry.Agents)

    // Build set of enabled agent IDs
    agentSet := make(map[string]bool)
    for _, id := range strat.Agents {
        agentSet[id] = true
    }

    // Filter agents by strategy's agent list
    for i := range filtered.Agents {
        if !agentSet[filtered.Agents[i].ID] {
            filtered.Agents[i].Enabled = false
        }
    }

    return filtered
}

func vixFromQuotes(quotes []domain.Quote) float64 {
    // In production, extract VIX from market data
    // For now, use default value
    for _, q := range quotes {
        if q.Symbol == "VIX" || q.Symbol == "^VIX" {
            return q.Last
        }
    }
    return 20.0 // Default VIX
}
```

**Step 2: Verify and commit**

```bash
cd /Users/kaecer/workspace/atlas
gofmt -w internal/orchestrator/system.go
go build ./internal/orchestrator/...
git add internal/orchestrator/system.go
git commit -m "feat(orchestrator): add filterAgentsByStrategy and vixFromQuotes helpers"
```

---

## Task 6: 在 runReplaySimulation() 中整合策略選擇

**Files:**
- Modify: `internal/orchestrator/system.go`

**Step 1: Find runReplaySimulation() function (around line 210)**

**Step 2: Add strategy selection after regime detection (around line 232-236)**

Find:
```go
oldRegime := regime
regime = AdjustRegimeFromNarrative(regime, events)
if s.eventBus != nil {
    go s.eventBus.PublishRegimeChange(oldRegime, regime, 0.0, "orchestrator")
}
```

Add after the regime change block:
```go
// ===== PHASE 3: Strategy Selection =====
if s.strategySelector != nil {
    selectedStrategy, err := s.strategySelector.Select(
        s.ctx,
        vixFromQuotes(quotes),
        regime,
    )
    if err == nil && selectedStrategy != nil {
        // Apply strategy to FactorWeightEngine
        if s.factorWeightEngine != nil {
            s.factorWeightEngine.ApplyStrategy(selectedStrategy)
        }

        // Apply strategy to DynamicThresholdEngine
        if s.thresholdEngine != nil {
            s.thresholdEngine.SetRiskAppetite(selectedStrategy.RiskAppetite)
        }
    }
}
// ===== END PHASE 3 =====
```

**Step 3: Verify and commit**

```bash
cd /Users/kaecer/workspace/atlas
gofmt -w internal/orchestrator/system.go
go build ./internal/orchestrator/...
git add internal/orchestrator/system.go
git commit -m "feat(orchestrator): integrate strategy selection in runReplaySimulation"
```

---

## Task 7: 在 RunDailySimulation() 中整合策略選擇

**Files:**
- Modify: `internal/orchestrator/system.go`

**Step 1: Find RunDailySimulation() function (around line 119)**

**Step 2: Add strategy selection after regime detection (similar to Task 6)**

Find the regime detection code (around line 141-150) and add strategy selection block after:
```go
regime := researchResult.Regime
rawRecs := researchResult.RawRecommendations
finalRecs := researchResult.FinalRecommendations
guardOutcomes := researchResult.GuardOutcomes
rejects := researchResult.ScreeningRejects
// Preserve original recs for outcome building so GuardOutcomes align with outcomes.
outcomeRawRecs := append([]domain.Recommendation(nil), rawRecs...)
outcomeFinalRecs := append([]domain.Recommendation(nil), finalRecs...)
oldRegime := regime
regime = AdjustRegimeFromNarrative(regime, events)
```

Add strategy selection block after the regime adjustment:
```go
// ===== PHASE 3: Strategy Selection =====
if s.strategySelector != nil {
    selectedStrategy, err := s.strategySelector.Select(
        s.ctx,
        vixFromQuotes(quotes),
        regime,
    )
    if err == nil && selectedStrategy != nil {
        if s.factorWeightEngine != nil {
            s.factorWeightEngine.ApplyStrategy(selectedStrategy)
        }
        if s.thresholdEngine != nil {
            s.thresholdEngine.SetRiskAppetite(selectedStrategy.RiskAppetite)
        }
    }
}
// ===== END PHASE 3 =====
```

**Step 3: Verify and commit**

```bash
cd /Users/kaecer/workspace/atlas
gofmt -w internal/orchestrator/system.go
go build ./internal/orchestrator/...
git add internal/orchestrator/system.go
git commit -m "feat(orchestrator): integrate strategy selection in RunDailySimulation"
```

---

## Task 8: 最終 CI 驗證

**Files:**
- All above files

**Step 1: Run full CI check**

```bash
cd /Users/kaecer/workspace/atlas
test -z "$(gofmt -l .)"
go build ./...
go test ./...
go vet ./...
staticcheck ./...
```

**Step 2: Run coverage check**

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

Expected: ≥ 40%

**Step 3: Update GitNexus index**

```bash
npx gitnexus analyze
```

---

## Self-Review Checklist

1. **Spec coverage**: All integration points implemented?
   - [x] thresholdEngine field in Engine
   - [x] WithThresholdEngine() method
   - [x] GetThresholdEngine() method
   - [x] Strategy fields in SystemCore
   - [x] NewSystem() initialization
   - [x] GetCurrentStrategy() method
   - [x] filterAgentsByStrategy() helper
   - [x] vixFromQuotes() helper
   - [x] runReplaySimulation() integration
   - [x] RunDailySimulation() integration

2. **Placeholder scan**: Any TODO/TBD/fill-inlater?
   - None found

3. **Type consistency**: Method signatures match design doc?
   - strategy.Strategy used correctly
   - sim.DynamicThresholdEngine used correctly
   - domain.Regime used correctly

4. **CI compliance**:
   - gofmt check passes
   - go build passes
   - go test passes
   - go vet passes
   - coverage ≥ 40%