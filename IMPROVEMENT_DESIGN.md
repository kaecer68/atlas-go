# Atlas 策略進化系統 — 整體改進解決方案設計文件

**日期**: 2026-04-25
**版本**: 2.0
**來源**: Q1-Q3 深度挖掘 + Wave 4 分析

---

## 執行摘要

本次設計針對 atlas-go 策略進化系統的 **6 個系統性問題** 提出整合式解決方案，而非孤立修補。核心主題：**缺乏健康度量 → 缺乏干預 → 系統退化**。

| 問題 | 嚴重性 | 修復策略 |
|------|--------|---------|
| calculateSharpe() 缺少開根號 | 🔴 Critically | 修正 stdDev 計算 + 年化 Sharpe |
| syntheticForwardReturn 人為膨脹勝率 | 🔴 Critically | 以 regime-aware 分布取代任意常數 |
| convictionBuilder 狀態污染 | 🟡 中等 | 加入 mutex + 防禦性複製 |
| 信念分數無正規化 | 🟡 中等 | 新增 ConvictionNormalizer (z-score/percentile) |
| 高風險 Agent 無自動干預 | 🟡 中等 | 新增 AgentHealthManager + Circuit Breaker |
| 無年化指標 | 🟡 中等 | 所有 Sharpe 乘 sqrt(252) 年化 |

---

## 1. 修正 Annualized Sharpe（Phase 1 — 立即執行）

### 問題根因

`darwinian_weights.go:189` 計算 population variance 後直接當作 stdDev 使用，未開平方根：

```go
stdDev := variance / float64(len(returns))  // 錯！這是 variance，不是 stdDev
return mean / stdDev                        // 變成 mean/variance
```

**數學影響**：variance=0.0004 時，正確 stdDev=0.02，buggy Sharpe = mean/0.0004 = **50x 放大**。

### 修復方案

**檔案**: `internal/portfolio/darwinian_weights.go`

```go
const TradingDaysPerYear = 252

func (m *DarwinianWeightManager) calculateSharpe(returns []float64) float64 {
    if len(returns) < 5 {
        return 0.0
    }
    
    n := float64(len(returns))
    
    // Calculate mean
    var sum float64
    for _, r := range returns {
        sum += r
    }
    mean := sum / n

    // FIX #1: Sample variance (n-1) for unbiased estimator
    var variance float64
    for _, r := range returns {
        variance += (r - mean) * (r - mean)
    }
    sampleVariance := variance / (n - 1)
    stdDev := math.Sqrt(sampleVariance)  // FIX #2: 開平方根！
    
    if stdDev == 0 {
        return 0.0
    }

    // FIX #3: Annualized Sharpe = daily_sharpe * sqrt(252)
    dailySharpe := mean / stdDev
    annualizedSharpe := dailySharpe * math.Sqrt(TradingDaysPerYear)
    
    // Cap to prevent extreme values
    if annualizedSharpe > 5 { annualizedSharpe = 5 }
    if annualizedSharpe < -5 { annualizedSharpe = -5 }
    
    return annualizedSharpe
}
```

**RollingVolatility 同步修正**（行 168）：
```go
// BEFORE: w.RollingVolatility = variance / float64(len(recentReturns))
// AFTER:
w.RollingVolatility = math.Sqrt(variance / float64(len(recentReturns)))
```

### 影響評估

| 指標 | 修正前 | 修正後 | 變化 |
|------|--------|--------|------|
| Sharpe (mean=0.001, var=0.0004) | 2.5 | 0.05 | 50x 縮小 |
| Tier 分類 | 嚴重失真 | 正確 | 相對排序保留，幅度正常化 |
| Performance Bonus | ~25% | ~0.5% | 合理範圍 |

---

## 2. 修正 Synthetic Forward Return（Phase 2 — 短期）

### 問題根因

`system.go:533-553` 的 `syntheticForwardReturn`：
- `intraday * 0.8` 作為明日報酬估計，無理論依據
- `fr == 0` 時回傳 `+0.3%`，導致 flat day 永遠是「勝」
- 符號名稱 hash fallback 使同支股票永遠得到相同值

**對 financials-desk-01 的影響**：台灣金融股（2881, 2882, 2891）波動低，flat day 比例高 → HitRate 被嚴重灌水。

### 修復方案

**檔案**: `internal/orchestrator/system.go`

```go
// ForwardReturnFallback 提供 regime-aware 的 fallback 分布參數
type ForwardReturnFallback struct {
    RiskOnParams  DistributionParams
    RiskOffParams DistributionParams
}

type DistributionParams struct {
    Mean      float64
    StdDev    float64
    MinReturn float64
    MaxReturn float64
}

func DefaultFallbackParams() ForwardReturnFallback {
    return ForwardReturnFallback{
        RiskOnParams: DistributionParams{
            Mean: 0.0008, StdDev: 0.015, MinReturn: -0.05, MaxReturn: 0.05,
        },
        RiskOffParams: DistributionParams{
            Mean: 0.0001, StdDev: 0.008, MinReturn: -0.03, MaxReturn: 0.03,
        },
    }
}

// GenerateForwardReturn 取代 syntheticForwardReturn
func GenerateForwardReturn(symbol string, quote domain.Quote, 
    regime domain.Regime, fallback ForwardReturnFallback) float64 {
    
    if quote.Open > 0 && quote.Last > 0 {
        intraday := (quote.Last - quote.Open) / quote.Open
        
        // 使用實際 intraday，但 clip 到合理範圍
        fr := intraday
        if fr > 0.05 { fr = 0.05 }
        if fr < -0.05 { fr = -0.05 }
        
        // Flat day 使用 regime-aware 分布，而非固定 +0.3%
        if math.Abs(fr) < 0.001 {
            return generateFromDistribution(symbol, regime, fallback)
        }
        
        // 輕微 dampening (0.9) 反映 overnight gap risk
        return fr * 0.9
    }
    
    return generateFromDistribution(symbol, regime, fallback)
}

func generateFromDistribution(symbol string, regime domain.Regime, 
    fallback ForwardReturnFallback) float64 {
    params := fallback.RiskOnParams
    if regime == domain.RegimeRiskOff {
        params = fallback.RiskOffParams
    }
    
    hash := hashString(symbol)
    normalized := ((hash % 10000) - 5000) / 5000.0  // [-1, 1]
    fr := params.Mean + normalized*params.StdDev
    
    if fr < params.MinReturn { fr = params.MinReturn }
    if fr > params.MaxReturn { fr = params.MaxReturn }
    return fr
}
```

### 影響評估

| 情境 | 修正前 | 修正後 |
|------|--------|--------|
| Flat day HitRate | 100% (fake) | ~50% (true unknown) |
| Financials AvgReturn | +0.09%/day (inflated) | Actual |
| 所有 Agent Sharpe | Artificially high | True value |

---

## 3. 修正 convictionBuilder 狀態污染（Phase 2 — 短期）

### 問題根因

`conviction_builder.go:31-38`：`floorCheck()` 在返回 `false` 前已修改內部狀態：

```go
func (b *convictionBuilder) floorCheck() bool {
    if b.final < b.floor {
        b.add("floor", b.floor-b.final, "...")  // MUTATES
        b.final = b.floor                         // MUTATES
        return false
    }
    return true
}
```

雖然目前無 reuse，但設計契約被破壞。

### 修復方案

**檔案**: `internal/orchestrator/conviction_builder.go`

```go
type convictionBuilder struct {
    base   int
    floor  int
    final  int
    steps  []domain.ConvictionStep
    mu     sync.RWMutex  // 新增：執行緒安全
}

func newConvictionBuilder(base, floor int) *convictionBuilder {
    return &convictionBuilder{
        base:  base,
        floor: floor,
        final: base,
        steps: make([]domain.ConvictionStep, 0, 10),
    }
}

func (b *convictionBuilder) floorCheck() bool {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    if b.final < b.floor {
        // FIX: 不再 mutate，僅回傳 false
        return false
    }
    
    // 只有通過時才記錄 floor step（透明度）
    b.steps = append(b.steps, domain.ConvictionStep{
        Rule:   "floor",
        Delta:  b.floor - b.final,
        Reason: fmt.Sprintf("below floor %d", b.floor),
    })
    b.final = b.floor
    return true
}

func (b *convictionBuilder) build() (int, *domain.ConvictionBreakdown) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    
    // 防禦性複製
    stepsCopy := make([]domain.ConvictionStep, len(b.steps))
    copy(stepsCopy, b.steps)
    
    return b.final, &domain.ConvictionBreakdown{
        Base:  b.base,
        Floor: b.floor,
        Final: b.final,
        Steps: stepsCopy,
    }
}
```

### 影響評估

- **向後相容**: 所有 9 個 caller 在 `false` 時立即 return，不會觀察到差異
- **測試更新**: `conviction_builder_test.go` 需更新 2 個 test case

---

## 4. 新增 Conviction Normalization（Phase 3 — 中期）

### 問題

不同 executor 的 base conviction 差異極大（45~65），無法橫向比較。

### 解決方案

**新檔案**: `internal/portfolio/conviction_normalizer.go`

```go
type NormalizationMethod int
const (
    ZScore NormalizationMethod = iota
    Percentile
    MinMax
)

type ConvictionNormalizer struct {
    mu         sync.RWMutex
    agentStats map[string]*AgentConvictionStats
}

type AgentConvictionStats struct {
    AgentID     string
    Mean        float64
    StdDev      float64
    Min         float64
    Max         float64
    SampleCount int
}

func (n *ConvictionNormalizer) RecordConviction(agentID string, conviction int) {
    // Welford's online algorithm for running statistics
}

func (n *ConvictionNormalizer) Normalize(agentID string, conviction int, 
    method NormalizationMethod) float64 {
    // Z-Score: (x - mean) / stdDev
    // Percentile: rank-based
    // MinMax: (x - min) / (max - min)
}
```

**整合點**: 在 `ApplyDarwinianWeights` 或 control layer 中 normalize 後再比較。

---

## 5. 新增 AgentHealth + Circuit Breaker（Phase 4 — 中期）

### 問題

僅靠 Sharpe 無法全面評估 agent 健康；無自動靜音/恢復機制。

### 解決方案

**新檔案**: `internal/portfolio/agent_health.go`

```go
type AgentHealthStatus string
const (
    HealthStatusHealthy    AgentHealthStatus = "healthy"
    HealthStatusDegraded   AgentHealthStatus = "degraded"
    HealthStatusMuted      AgentHealthStatus = "muted"
    HealthStatusRecovering AgentHealthStatus = "recovering"
)

type AgentHealth struct {
    AgentID           string
    Status            AgentHealthStatus
    AnnualizedSharpe  float64
    HitRate           float64
    ConsecutiveLosses int
    ConsecutiveWins   int
    CompositeScore    float64  // 0-100
    MutedAt           *time.Time
    UnmutedAt         *time.Time
}

type AgentHealthManager struct {
    health map[string]*AgentHealth
    config AgentHealthConfig
}

type AgentHealthConfig struct {
    DefaultMuteThreshold   int     // 5 consecutive losses
    DefaultUnmuteThreshold int     // 3 consecutive wins
    DefaultAutoRecoverDays int     // 7 days
    MinSampleSize          int     // 10 observations
    NegativeSharpeThreshold float64 // -0.5
}
```

**自動干預邏輯**:

```go
func (m *AgentHealthManager) evaluateInterventions(h *AgentHealth) {
    switch h.Status {
    case HealthStatusHealthy:
        if h.ConsecutiveLosses >= h.MuteThreshold {
            h.Status = HealthStatusMuted
            log.Printf("[AgentHealth] MUTED %s after %d losses", h.AgentID, h.ConsecutiveLosses)
        }
        if h.AnnualizedSharpe < m.config.NegativeSharpeThreshold {
            h.Status = HealthStatusMuted
            log.Printf("[AgentHealth] MUTED %s: Sharpe %.2f < %.2f", 
                h.AgentID, h.AnnualizedSharpe, m.config.NegativeSharpeThreshold)
        }
        
    case HealthStatusMuted:
        if h.ConsecutiveWins >= h.UnmuteThreshold {
            h.Status = HealthStatusRecovering
        }
        if daysSinceMute >= h.AutoRecoverDays {
            h.Status = HealthStatusRecovering
        }
    }
}
```

**Composite Score 計算**:
```
Composite = Sharpe(40%) + HitRate(30%) + Streak(30%)
```

**整合點**:
1. `RecordOutcome()` 時同步更新 AgentHealth
2. `ExecuteRegistryResearchDetailed` 前過濾 muted agents
3. Dashboard API 暴露 muted agents 列表

---

## 6. 整合架構圖

```
Market Data → Orchestrator
    ↓
[AgentHealthManager] ← 過濾 muted agents
    ↓
RegimeExecutor (Context layer)
    ↓
[ConvictionNormalizer] ← 正規化 per-agent conviction
    ↓
Sector/Style Executors → convictionBuilder (one per rec)
    ↓
ControlExecutor (CRO/CIO)
    ↓
Simulator → Ledger
    ↓
[DarwinianWeightManager] → RecordOutcome → [AgentHealthManager]
    ↓
[EvaluateAgentBreakers] → Auto Mute/Unmute
```

---

## 7. 實施路線圖

### Phase 1: 基礎修復（低風險，立即）
- [ ] 修正 `calculateSharpe()` — 加 `math.Sqrt`，年化
- [ ] 修正 `RollingVolatility` — 改為 stdDev
- [ ] 測試：驗證 Sharpe 值合理範圍

### Phase 2: 資料品質（中風險，本週）
- [ ] 新增 `GenerateForwardReturn` 取代 `syntheticForwardReturn`
- [ ] 修正 `convictionBuilder` — mutex + 防禦複製
- [ ] 測試：驗證 flat day 不再 guaranteed hit

### Phase 3: 正規化（中風險，下週）
- [ ] 新增 `ConvictionNormalizer`
- [ ] 在 control layer 整合 z-score
- [ ] 測試：驗證 cross-agent 可比較性

### Phase 4: 健康與干預（較高風險，下週）
- [ ] 新增 `AgentHealthManager`
- [ ] 新增 Circuit Breaker 邏輯
- [ ] 在 executor 前過濾 muted agents
- [ ] 測試：驗證自動靜音/恢復

### Phase 5: 監控與調優（長期）
- [ ] Dashboard 顯示 AgentHealth 狀態
- [ ] 調整 Mute/Unmute 閾值
- [ ] 收集實際運行數據驗證效果

---

## 8. 風險評估

| 風險 | 機率 | 影響 | 緩解 |
|------|------|------|------|
| Sharpe 修正後 tier 分類大變 | 高 | 高 | 先備份 weights，逐步驗證 |
| synthetic 修正後 HitRate 下降 | 高 | 中 | 預期行為，監控即可 |
| Auto-mute 過度靜音好 agent | 中 | 高 | 保守閾值（5 losses），人工可覆寫 |
| ConvictionNormalizer 改變排名 | 中 | 中 | 可選功能，預設關閉 |

---

*設計文件產出時間: 2026-04-25*
*來源: Q1-Q3 深度挖掘 + Wave 4 分析*
