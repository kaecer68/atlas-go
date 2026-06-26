# Phase 3 整合設計文件

**版本**：v1.0
**建立日期**：2026-04-28
**作者**：Sisyphus AI Agent
**狀態**：已獲批准，準備實作

---

## 1. 目標

將 Phase 3 多策略框架（Strategy/Registry/ComparisonEngine/Selector）整合到 Atlas 交易管線中，使策略選擇能夠：
1. 影響 FactorWeightEngine 的動態因子權重
2. 影響 DynamicThresholdEngine 的相關性閾值
3. 根據策略過濾要執行的 Agents

---

## 2. 整合架構

```
Market Data → SystemCore.RunDailySimulation()
                      ↓
              Regime 判定 (Risk On/Off/Neutral)
                      ↓
    ┌───────────────────────────────────────┐
    │  PHASE 3: Strategy Selection           │
    │                                       │
    │  strategySelector.Select(ctx, VIX,   │
    │                              regime)  │
    │                      ↓                │
    │  factorWeightEngine.ApplyStrategy()  │
    │  thresholdEngine.SetRiskAppetite()    │
    │  filterAgentsByStrategy(registry)      │
    └───────────────────────────────────────┘
                      ↓
              ExecuteWithContext(Registry)
                      ↓
              ...existing pipeline...
```

---

## 3. SystemCore 擴展

### 3.1 新增欄位

```go
type SystemCore struct {
    // ... existing fields ...

    // Phase 3: Strategy Framework
    strategyRegistry   *strategy.Registry
    strategySelector    *strategy.Selector
    comparisonEngine    *strategy.ComparisonEngine
    factorWeightEngine  *portfolio.FactorWeightEngine

    // Phase 2: DynamicThresholdEngine reference
    thresholdEngine     *sim.DynamicThresholdEngine
}
```

### 3.2 NewSystem() 初始化

```go
func NewSystem(cfg config.Config) *System {
    // ... existing initialization ...

    // Phase 2: DynamicThresholdEngine
    thresholdEngine := sim.NewDynamicThresholdEngine()

    // Phase 3: Strategy Framework
    strategyRegistry := strategy.NewRegistryWithDefaults()
    comparisonEngine := strategy.NewComparisonEngine(20)
    strategySelector := strategy.NewSelector(strategyRegistry, comparisonEngine)
    factorWeightEngine := portfolio.NewFactorWeightEngine()

    engine := sim.NewEngine(policy.Constraints).
        WithOptimizer(optimizer).
        WithThresholdEngine(thresholdEngine).
        WithReflexivityRules(...)

    return &System{
        SystemCore: &SystemCore{
            // ... existing fields ...
            strategyRegistry:   strategyRegistry,
            strategySelector:   strategySelector,
            comparisonEngine:   comparisonEngine,
            factorWeightEngine: factorWeightEngine,
            thresholdEngine:    thresholdEngine,
        },
    }
}
```

---

## 4. 策略選擇時機與應用

### 4.1 在 runReplaySimulation() 中整合

在 Regime 判定後、Agent 執行前選擇並應用策略：

```go
func (s *System) runReplaySimulation(sessionDate time.Time) (domain.SimulationResult, error) {
    // ... existing code up to regime detection ...

    regime := researchResult.Regime
    regime = AdjustRegimeFromNarrative(regime, events)

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

            // Filter agents based on strategy
            registry = s.filterAgentsByStrategy(registry, selectedStrategy)
        }
    }
    // ===== END PHASE 3 =====

    // Continue with ExecuteWithContext using (possibly filtered) registry
    researchResult := ExecuteWithContext(ExecutionContext{
        Registry:      registry,  // Use filtered registry
        // ... other fields ...
    })
    // ... rest of existing code ...
}
```

### 4.2 Agent 過濾方法

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

    // Filter agents by strategy's agent list
    agentSet := make(map[string]bool)
    for _, id := range strat.Agents {
        agentSet[id] = true
    }

    for i := range filtered.Agents {
        if !agentSet[filtered.Agents[i].ID] {
            // Mark as filtered by setting a special field or handling in executor
            filtered.Agents[i].Enabled = false
        }
    }

    return filtered
}
```

### 4.3 VIX 輔助函數

```go
func vixFromQuotes(quotes []domain.Quote) float64 {
    // In production, extract VIX from market data
    // For now, use default value or derive from quote data
    for _, q := range quotes {
        if q.Symbol == "VIX" || q.Symbol == "^VIX" {
            return q.Last
        }
    }
    return 20.0  // Default VIX
}
```

---

## 5. GetCurrentStrategy() 方法

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
```

---

## 6. Simulator 整合

### 6.1 sim.Engine 新增 WithThresholdEngine()

```go
func (e *Engine) WithThresholdEngine(te *DynamicThresholdEngine) *Engine {
    e.thresholdEngine = te
    return e
}

func (e *Engine) GetThresholdEngine() *DynamicThresholdEngine {
    return e.thresholdEngine
}
```

### 6.2 Engine 結構更新

```go
type Engine struct {
    // ... existing fields ...
    thresholdEngine *DynamicThresholdEngine
}
```

---

## 7. 實作檔案清單

| 檔案 | 變更類型 | 說明 |
|------|----------|------|
| `internal/orchestrator/system.go` | 修改 | 新增 Strategy 欄位、初始化、選擇邏輯、過濾方法 |
| `internal/sim/engine.go` | 修改 | 新增 `WithThresholdEngine()` 和 `GetThresholdEngine()` |

---

## 8. CI 要求

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./...
go vet ./...
staticcheck ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total  # ≥ 40%
```

---

## 9. 測試策略

1. **單元測試**：驗證 StrategySelector.Select() 正確根據 Regime 過濾
2. **整合測試**：驗證 SystemCore 正確初始化所有 Strategy 元件
3. **端到端測試**：驗證策略選擇影响因子權重和閾值

---

## 10. 風險與緩解

| 風險 | 影響 | 緩解措施 |
|------|------|----------|
| 策略過濾導致無 Agent 可執行 | 高 | 保留 "all_weather" 作為 fallback |
| VIX 取得失敗 | 中 | 使用預設值 20.0 |
| 迴圈相依 | 低 | Phase 3 是裝飾性增強，不影響核心邏輯 |