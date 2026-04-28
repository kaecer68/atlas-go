# Skill: atlas-multi-strategy

## 描述

**多策略框架** - 支援策略選擇、切換與比較，取代現有的「所有 Agent 全量執行」模式。

## 任務觸發

當 AI 代理需要：
- 實作策略選擇邏輯
- 新增/修改交易策略
- 實作策略比較系統
- 實作策略Registry

## 核心概念

### 1. Strategy 結構

```go
type Strategy struct {
    ID          string
    Name        string
    Description string
    Enabled     bool
    Agents      []string  // 使用的 Agent ID 列表
    Filters     []string  // 使用的 Filter 列表
    Priority    int       // 執行優先順序
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

### 3. StrategyRegistry

```go
type StrategyRegistry struct {
    strategies map[string]*Strategy
    activeID   string
}

func (r *StrategyRegistry) Get(id string) (*Strategy, bool)
func (r *StrategyRegistry) Register(s *Strategy)
func (r *StrategyRegistry) SetActive(id string) error
func (r *StrategyRegistry) GetActive() (*Strategy, bool)
```

### 4. 策略比較系統

**比較維度**：
- 日報酬率
- 夏普比率（20日滾動）
- 最大回撤
- 勝率

**輸出結構**：

```go
type StrategyComparison struct {
    Date           time.Time
    StrategyID     string
    DailyReturn    float64
    SharpeRatio    float64
    MaxDrawdown    float64
    WinRate        float64
    Outperformance float64  // vs benchmark
}
```

### 5. 策略選擇引擎

**邏輯流程**：
1. 根據 Regime 初步篩選可用策略
2. 根據近 20 日表現給予權重
3. 選擇表現最佳的策略組合
4. 每日重新平衡

## 實作位置

| 元件 | 檔案 | 狀態 |
|------|------|------|
| Strategy | `internal/strategy/types.go` | 待建立 |
| StrategyRegistry | `internal/strategy/registry.go` | 待建立 |
| StrategySelector | `internal/strategy/selector.go` | 待建立 |
| StrategyComparator | `internal/strategy/comparator.go` | 待建立 |

## 與現有系統整合

### 修改 internal/orchestrator/plugin_registry.go

新增 `GetStrategyAgents()` 方法，根據當前策略返回對應的 Agent 列表。

### 修改 internal/orchestrator/system.go

在 `CollectRecommendations()` 前呼叫 `StrategySelector.Select()` 確定當前策略。

## 策略切換觸發條件

| 條件 | 觸發動作 |
|------|----------|
| RegimeChange | 重新評估策略適用性 |
| 策略勝率連續 5 日低於基准 | 考慮切換 |
| 策略 Sharpe 比率跌破 0.5 | 考慮切換 |
| 手動干預 | 立即切換 |

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
