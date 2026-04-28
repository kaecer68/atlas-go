# Phase 3 多策略框架設計文件

**版本**：v1.0
**建立日期**：2026-04-28
**作者**：Sisyphus AI Agent
**狀態**：已獲批准，準備實作

---

## 1. 目標

建立「事件驅動、多策略、可自我迭代」的投資研究系統的第三階段：多策略框架。

**與 Phase 1/2 的整合**：
- 策略框架作為 Orchestrator 的上層調控層
- 策略選擇結果直接影響 FactorWeightEngine 的動態因子權重
- DynamicThresholdEngine 從策略框架讀取 RiskAppetite 參數

---

## 2. 整體架構

```
Market Data
    ↓
Orchestrator (SystemCore)
    ↓
┌─────────────────────────────────────────┐
│  StrategyLayer                          │
│  ┌─────────────────────────────────┐    │
│  │ StrategyRegistry                 │    │
│  │ - 5 built-in strategies         │    │
│  │ - Custom strategy support       │    │
│  └─────────────────────────────────┘    │
│           ↓                    ↓        │
│  ┌─────────────────┐  ┌─────────────┐  │
│  │ StrategySelector │  │ Comparator  │  │
│  │ - Regime filter  │  │ - Daily perf│  │
│  │ - 20-day weight  │  │ - Sharpe    │  │
│  │ - Daily rebalance │  │ - Drawdown  │  │
│  └─────────────────┘  └─────────────┘  │
└─────────────────────────────────────────┘
    ↓ (Strategy output: enabled agents + threshold)
FactorWeightEngine ←──┐
                       │
DynamicThresholdEngine┘
    ↓
Existing Executor layers...
```

**核心原則**：
- 策略框架**不取代**現有 Executor，而是**調控**哪些 Agent/Filter 被啟用
- StrategySelector 輸出直接影響 FactorWeightEngine 的 `EventAdjustments`
- DynamicThresholdEngine 從 StrategySelector 讀取 `RiskAppetite` 參數

---

## 3. Strategy 結構與 Registry

### 3.1 核心結構

```go
// internal/strategy/types.go

type Strategy struct {
    ID            string            // 唯一識別符：all_weather, growth, value, defensive, momentum
    Name          string            // 顯示名稱
    Description   string            // 策略描述
    Enabled       bool              // 是否啟用
    Agents        []string          // 啟用的 Agent ID 列表
    Filters       []string          // 啟用的 Filter 列表
    Priority      int               // 執行優先順序（數字越小越優先）
    RiskAppetite  RiskAppetite      // 風險偏好等级
    RegimePrefs   []domain.Regime   // 偏好的市場 Regime
}

type RiskAppetite int

const (
    RiskAppetiteConservative RiskAppetite = 1  // 防御型
    RiskAppetiteBalanced     RiskAppetite = 2  // 平衡型
    RiskAppetiteAggressive   RiskAppetite = 3  // 積極型
)
```

### 3.2 內建策略配置

| ID | Name | Agents | RiskAppetite | Regime偏好 |
|----|------|--------|--------------|------------|
| `all_weather` | 全天候 | 全部 | Balanced | All |
| `growth` | 成長動能 | momentum, ai_supply_chain | Aggressive | Bull, Neutral |
| `value` | 價值投資 | value, quality | Conservative | Bull, Neutral |
| `defensive` | 防御型 | quality, low_volatility | Conservative | Bear, HighVol |
| `momentum` | 純動能 | momentum | Aggressive | Bull |

### 3.3 Registry 設計

```go
// internal/strategy/registry.go

type Registry struct {
    strategies map[string]*Strategy
    mu         sync.RWMutex
}

func NewRegistry() *Registry

func (r *Registry) Register(s *Strategy) error   // 註冊策略
func (r *Registry) Get(id string) (*Strategy, bool)  // 取得策略
func (r *Registry) List() []*Strategy               // 列出所有策略
func (r *Registry) ListByRegime(regime domain.Regime) []*Strategy  // 按Regime篩選
```

**向後相容**：
- 預設策略為 `all_weather`
- 若 Registry 為空，系統回退到現有的「全部 Agent 啟用」行為

---

## 4. Strategy Comparison 系統

### 4.1 結構

```go
// internal/strategy/comparison.go

type StrategyComparison struct {
    Date          time.Time
    StrategyID    string
    DailyReturn   float64    // 日報酬率
    SharpeRatio   float64    // 夏普比率（20日滾動）
    MaxDrawdown   float64    // 最大回撤
    WinRate       float64    // 勝率（正報酬日/總交易日）
    Outperformance float64   // 相比 benchmark
}

type ComparisonResult struct {
    Date           time.Time
    Comparisons    []*StrategyComparison
    BestByReturn   string  // 日報酬最佳策略ID
    BestBySharpe   string  // 夏普比率最佳策略ID
    BestByDrawdown string  // 回撤控制最佳策略ID
}
```

### 4.2 ComparisonEngine

```go
type ComparisonEngine struct {
    window   int           // 滾動窗口（預設20日）
    history  []*ComparisonResult  // 歷史記錄
}

func (e *ComparisonEngine) Record(trades []*Trade, benchmarkReturn float64)
func (e *ComparisonEngine) GetResult(date time.Time) (*ComparisonResult, bool)
func (e *ComparisonEngine) BestStrategy(by string) (string, error)  // by: "return", "sharpe", "drawdown"
```

### 4.3 與 Ledger 的整合

- `ComparisonEngine` 從 `Ledger` 讀取交易記錄
- 每個 `StrategyID` 有自己的子 Ledger（或用 tag 區分）
- `RecordSessionSummary` 已有的 `OutcomeCount`/`TotalReturn` 可直接使用

---

## 5. Strategy Selector

### 5.1 Selector 結構

```go
// internal/strategy/selector.go

type SelectorConfig struct {
    MinSwitchInterval time.Duration  // 最小切換間隔（預設 5 交易日）
    SwitchThreshold  float64         // 勝出差距門檻（預設 0.10）
}

type Selector struct {
    registry    *Registry
    comparison  *ComparisonEngine
    current     *Strategy
    config      SelectorConfig
    lastSwitch  time.Time
}

func (s *Selector) Select(ctx context.Context,
    vix float64,
    regime domain.Regime,
    lookbackDays int,
) (*Strategy, error)
```

### 5.2 選擇流程（4步）

```
Step 1: Regime Filter
├── 根據 current regime 篩選可用策略
└── Example: Bear/HighVol → 僅 defensive 可用

Step 2: 20日表現權重
├── 從 ComparisonEngine 取得各策略 20日表現
├── 計算 weighted score：
│   SharpeRatio × 0.4 + DailyReturn × 0.3 + WinRate × 0.3
└── 排除表現低於 threshold 的策略

Step 3: 隨機打破平手
├── 若多個策略 score 相近（差距 < 0.05）
└── 隨機選擇，避免過度擬合

Step 4: 每日重新平衡
├── 每日重新執行 Select()
└── 若策略變化，產生 RegimeChangedEvent 通知 FactorWeightEngine
```

### 5.3 與 Phase 1/2 的整合

**Strategy → FactorWeightEngine**：
```go
// FactorWeightEngine 新增方法
func (e *FactorWeightEngine) ApplyStrategy(s *Strategy)

// 根據策略的 RiskAppetite 調整因子權重：
// - Conservative: Value/Quality 權重 +20%, Momentum -20%
// - Aggressive: Momentum 權重 +20%, Value/Quality -20%
```

**Strategy → DynamicThresholdEngine**：
```go
// DynamicThresholdEngine 新增方法
func (e *DynamicThresholdEngine) SetRiskAppetite(ra RiskAppetite)

// 根據風險偏好調整 regime multiplier：
// - Conservative: multiplier +0.10（更保守的相關性閾值）
// - Aggressive: multiplier -0.05（更寬鬆的相關性閾值）
```

### 5.4 每日選擇範例

```
Date: 2026-04-28
VIX: 18.5, Regime: Bull

Step 1: 可用策略 = [all_weather, growth, value, momentum]（defensive 排除）
Step 2: 20日表現
  - all_weather: Sharpe=1.2, Return=0.8%, WinRate=55% → Score=0.95
  - growth:      Sharpe=1.5, Return=1.2%, WinRate=52% → Score=1.13
  - value:       Sharpe=0.9, Return=0.5%, WinRate=60% → Score=0.78
  - momentum:    Sharpe=1.8, Return=1.5%, WinRate=48% → Score=1.29
Step 3: 最高分 = momentum
Result: 選擇 momentum 策略
Action: FactorWeightEngine.ApplyStrategy(momentum) → Momentum權重提高
```

---

## 6. 錯誤處理與邊界情況

### 6.1 策略選擇失敗的 Fallback

```go
func (s *Selector) Select(ctx context.Context, ...) (*Strategy, error) {
    // 嘗試正常選擇
    strategy, err := s.selectImpl(ctx, vix, regime, lookbackDays)
    if err != nil {
        // Fallback 1: 嘗試 all_weather
        if s.registry.Get("all_weather") != nil {
            return s.registry.Get("all_weather")
        }
        // Fallback 2: 回退到靜態行為（全部 Agent 啟用）
        return &Strategy{
            ID:       "fallback",
            Name:     "Fallback",
            Agents:   []string{"*"},  // wildcard 表示全部
        }, nil
    }
    return strategy, nil
}
```

### 6.2 策略切換的平滑過渡

```go
func (s *Selector) shouldSwitch(from, to *Strategy, scoreDelta float64) bool {
    if from.ID == to.ID {
        return false
    }
    if s.config.MinSwitchInterval > 0 && time.Since(s.lastSwitch) < s.config.MinSwitchInterval {
        return false
    }
    if scoreDelta < s.config.SwitchThreshold {
        return false
    }
    return true
}
```

### 6.3 歷史數據不足的處理

```go
func (e *ComparisonEngine) GetScore(strategyID string, days int) (float64, error) {
    if len(e.history) < days {
        // 數據不足，回退到中性分數
        return 0.5, nil  // 中性分數，不偏向任何策略
    }
    // 正常計算...
}
```

### 6.4 Regime 與策略組合矩陣

| Regime | all_weather | growth | value | defensive | momentum |
|--------|-------------|--------|-------|-----------|----------|
| Bull | ✅ | ✅ | ✅ | ❌ | ✅ |
| Bear | ✅ | ❌ | ❌ | ✅ | ❌ |
| Neutral | ✅ | ✅ | ✅ | ❌ | ✅ |
| HighVol | ✅ | ❌ | ❌ | ✅ | ❌ |

---

## 7. 實作檔案清單

| 檔案 | 職責 | 狀態 |
|------|------|------|
| `internal/strategy/types.go` | Strategy、RiskAppetite、ComparisonResult 結構 | 新建 |
| `internal/strategy/registry.go` | Registry：註冊/查詢/列表 | 新建 |
| `internal/strategy/comparison.go` | ComparisonEngine：計算策略表現 | 新建 |
| `internal/strategy/selector.go` | Selector：選擇最佳策略 | 新建 |
| `internal/portfolio/factor_weight_engine.go` | 新增 `ApplyStrategy()` | 擴展 |
| `internal/sim/dynamic_threshold.go` | 新增 `SetRiskAppetite()` | 擴展 |

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

## 9. 風險與緩解

| 風險 | 影響 | 緩解措施 |
|------|------|----------|
| 策略過度擬合 | 高 | Stickiness 機制 + 5日最小切換間隔 |
| 歷史數據不足 | 中 | 中性分數 fallback |
| Regime 判斷不穩定 | 中 | 使用 5 日移動平均平滑 Regime 變化 |
| 策略切換頻繁 | 中 | SwitchThreshold 門檻控制 |

---

## 10. 依賴關係

```
Phase 1 (已完成)
├── NarrativeEvent 擴展
├── FactorWeightEngine
├── FactorBridge
├── InstitutionalSentiment 因子
├── Liquidity 因子
└── RegimeChange 機制

Phase 2 (已完成)
└── DynamicThresholdEngine

Phase 3 (本設計)
├── StrategyRegistry
├── ComparisonEngine
└── Selector

依賴：
Phase 3 → Phase 1 FactorWeightEngine.ApplyStrategy()
Phase 3 → Phase 2 DynamicThresholdEngine.SetRiskAppetite()
```
