# Capital Management 操作手冊

## 概述

Capital Management 提供階段式資金管理，從 simulation 逐步晉升至 full trading。

## 資金階段

預設比例來自 [`domain.DefaultCapitalPhaseConfig()`](../../internal/domain/types.go)。實際部署資本由 `TotalCapital × CapitalLimits[phase]` 決定。

| 階段 | 說明 | 資金上限比例 | 晉升條件 |
|------|------|--------------|----------|
| Simulation | 純模擬 | 100% | 累積 30 天數據 |
| Paper | 模擬交易 | 10% | 最大回撤 < 10% |
| Live | 小額實盤 | 30% | Sharpe > 1.0 |
| Full | 全額交易 | 100% | 人工審批通過 |

## 使用方法

### 初始化控制器

```go
import "github.com/kaecer68/atlas-go/internal/domain"

cfg := domain.CapitalPhaseConfig{
    CurrentPhase:   domain.PhaseSimulation,
    PhaseStartDate: time.Now().Format("2006-01-02"),
}

controller := risk.NewCapitalPhaseController(cfg)
```

### 檢查晉升條件

```go
canAdvance, reason := controller.CanAdvance()
if canAdvance {
    err := controller.AdvancePhase()
}
```

### 資金分配

```go
allocator := portfolio.NewCapitalAllocator()
allocation := allocator.Allocate(
    phaseConfig,
    recommendations,
    totalCapital,
    reserveFraction,  // 例如 0.1 表示保留 10%
)
```

## Approval Workflow

Live → Full 晉升需要人工審批：

```go
workflow, _ := risk.NewApprovalWorkflow("data/state/approvals/")
workflow.Submit(domain.ApprovalRequest{
    Phase:      domain.PhaseFull,
    RequestedAt: time.Now(),
})
```

## 監控指標

- **當前階段**: controller.GetSnapshot().Phase
- **階段天數**: 從 PhaseStartDate 計算
- **Sharpe Ratio**: 滾動計算
- **最大回撤**: 持續追蹤

## 注意事項

1. **晉升是自動的**（除了 Live → Full）
2. **條件不滿足時**會返回原因說明
3. 已到最終階段時 **AdvancePhase()** 會返回錯誤

## 測試

```bash
go test ./internal/risk/... -v
go test ./internal/portfolio/... -run TestCapital -v
```
