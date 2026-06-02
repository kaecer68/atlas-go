# Skill: atlas-multi-strategy

> **實作狀態**：✅ 已實作（`internal/strategy/` 模組，成熟度 `evolving`）  
> **最後審計**：2026-06-02  
> **實際檔案結構**：`types.go`、`registry.go`、`selector.go`、`comparison.go`、`allocator.go`、`doc.go`

## 描述

**多策略框架** — 支援策略選擇、切換與比較，取代「所有 Agent 全量執行」模式。

## 任務觸發

當 AI 代理需要：
- 實作策略選擇邏輯
- 新增/修改交易策略
- 實作策略比較系統
- 實作策略 Registry

## 核心概念

### 1. Strategy 結構

```go
type Strategy struct {
    ID           string
    Name         string
    Description  string
    Enabled      bool
    Agents       []string  // 使用的 Agent ID 列表
    Filters      []string  // 使用的 Filter 列表
    Priority     int       // 執行優先順序
    RiskAppetite RiskAppetite
    RegimePrefs  []domain.Regime
}
```

### 2. 內建策略

| 策略 ID | 名稱 | 說明 | Agent 子集 |
|---------|------|------|------------|
| `all_weather` | 全天候 | 所有 Agent，保守閾值 | 全部 |
| `growth` | 成長動能 | 動能 + AI supply chain | momentum, ai_supply_chain |
| `value` | 價值投資 | Value + Quality | value, quality |
| `defensive` | 防御型 | 高品質 + 低波動 | quality, low_volatility |
| `momentum` | 純動能 | 僅動能因子 | momentum |

### 3. StrategyRegistry（Registry）

```go
type Registry struct {
    mu         sync.RWMutex
    strategies map[string]*Strategy
}

func (r *Registry) Get(id string) (*Strategy, bool)
func (r *Registry) Register(s *Strategy)
func (r *Registry) ListByRegime(regime domain.Regime) []*Strategy
```

### 4. 策略比較系統（ComparisonEngine）

**比較維度**：
- 日報酬率
- 夏普比率（20日滾動）
- 最大回撤
- 勝率

**輸出結構**：

```go
type StrategyComparison struct {
    Date           string
    StrategyID     string
    DailyReturn    float64
    SharpeRatio    float64
    MaxDrawdown    float64
    WinRate        float64
    Outperformance float64  // vs benchmark
}

type ComparisonResult struct {
    Date           string
    Comparisons    []*StrategyComparison
    BestByReturn   string
    BestBySharpe   string
    BestByDrawdown string
}
```

### 5. 策略選擇引擎（Selector）

**邏輯流程**：
1. 根據 Regime 初步篩選可用策略（`Registry.ListByRegime()`）
2. 根據近 20 日表現給予權重（`ComparisonEngine.GetScore()`）
3. 選擇表現最佳的策略組合
4. 設有冷卻期（`MinSwitchInterval`），防止反覆切換

### 6. 資金配置器（StrategyAllocator）

風險平價資金配置：依波動率倒數分配策略權重。  

| 約束 | 預設值 |
|------|--------|
| 最大權重 | 0.50 |
| 最小權重 | 0.05 |
| 波動率預設 | 0.20（年化，資料不足時 fallback） |

## 實作位置

| 元件 | 檔案 | 狀態 |
|------|------|------|
| Strategy | `internal/strategy/types.go` | ✅ 已實作 |
| Registry | `internal/strategy/registry.go` | ✅ 已實作（含 5 種預設策略） |
| Selector | `internal/strategy/selector.go` | ✅ 已實作（regime-based + 冷卻期） |
| ComparisonEngine | `internal/strategy/comparison.go` | ✅ 已實作（Sharpe/Drawdown/WinRate） |
| StrategyAllocator | `internal/strategy/allocator.go` | ✅ 已實作（風險平價） |
| 模組文件 | `internal/strategy/doc.go` | ✅ 成熟度: evolving |

## 與現有系統整合

### 已整合：internal/orchestrator/

`Selector.Select()` 在 orchestrator 流程中根據當前盤勢與績效選擇最適策略。

## 策略切換觸發條件

| 條件 | 觸發動作 |
|------|----------|
| RegimeChange | 重新評估策略適用性 |
| 策略勝率連續 5 日低於基准 | 考慮切換 |
| 策略 Sharpe 比率跌破 0.5 | 考慮切換 |
| 手動干預 | 立即切換 |

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **策略切換有冷卻期** | `Selector.shouldSwitch()` 檢查 `MinSwitchInterval`（來自參數配置），短時間內不會反覆切換 |
| **無候選時 fallback** | `Selector.Select()` 在無 regime 匹配策略時回傳 `all_weather` |
| **Allocator 權重上限** | `StrategyAllocator` 單策略上限 50%、下限 5%，以迭代方式重新正規化 |

## 與其他技能整合

- `atlas-event-driven-weights`：動態因子權重影響策略表現
- `atlas-dynamic-correlation`：動態閾值影響策略篩選效果

## 設計原則

1. **策略隔離**：每個策略維護獨立的 Agent 組合和 Filter 組合
2. **漸進式切換**：不一次性全部切換，而是逐步調整權重
3. **可回測**：所有策略變更都必須支援回測驗證

## 驗證要求

```bash
go test ./internal/strategy/...      # 策略邏輯測試
go test ./internal/orchestrator/...  # 整合測試
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total  # ≥ 40%
```

## 數據持久化

策略狀態儲存至 `data/state/strategies/`：
```
strategies/
├── registry.json      # 策略註冊表
├── active.json        # 當前活躍策略
├── comparisons/       # 每日比較結果
│   └── 2026-04-28.json
└── history/           # 策略切換歷史
    └── 切換記錄.jsonl
```
