# Skill: atlas-dynamic-correlation

## 描述

**動態相關性閾值系統** - 將靜態相關性閾值 0.7 改為 VIX × Regime 混合計算的動態閾值。

## 任務觸發

當 AI 代理需要：
- 實作或修改相關性閾值邏輯
- 調整因子選擇的相關性門檻
- 實作 VIX 感知機制
- 修改市場狀態評估邏輯

## 核心概念

### 1. 靜態閾值的問題

現有系統使用固定閾值 0.7，存在以下問題：
- 高波動期：因子相關性天然升高，0.7 太高會過度篩選
- 低波動期：因子之間相關性天然降低，0.7 可能太低

### 2. 動態閾值公式

```
BaseThreshold = 0.70
VIXAdjustment = (VIX - 20) / 100
RegimeAdjustment = RegimeMultiplier[Regime]
DynamicThreshold = BaseThreshold + VIXAdjustment + RegimeAdjustment
DynamicThreshold = clamp(DynamicThreshold, 0.40, 0.85)
```

### 3. Regime 倍數表

| Regime | 倍數 |
|--------|------|
| Bull | -0.05 |
| Bear | +0.10 |
| Neutral | 0.00 |
| HighVol | +0.15 |

### 4. 閾值範圍

- **最小閾值**：0.40（高相關性市場，如 2020 COVID 崩盤）
- **最大閾值**：0.85（低相關性市場，如 2017 牛 市）

### 5. 閾值計算範例

| VIX | Regime | 計算 | 結果 |
|-----|--------|------|------|
| 15 | Bull | 0.70 - 0.05 + (15-20)/100 | 0.60 |
| 35 | HighVol | 0.70 + 0.15 + (35-20)/100 | 0.90 → clamp to 0.85 |
| 20 | Neutral | 0.70 + 0.00 + (20-20)/100 | 0.70 |
| 25 | Bear | 0.70 + 0.10 + (25-20)/100 | 0.85 |

## 實作位置

| 元件 | 檔案 | 狀態 |
|------|------|------|
| DynamicThresholdEngine | `internal/portfolio/dynamic_threshold.go` | 待建立 |
| VIXProvider | `internal/marketdata/vix_provider.go` | 待建立 |

## 驗證頻率

- **每日快速更新**：根據收盤 VIX 重新計算
- **每 5 交易日完整驗證**：回測驗證閾值有效性

## 與其他技能整合

- `atlas-event-driven-weights`：RegimeChange 觸發時重新計算閾值
- `atlas-core-architecture`：影響 Optimizer 的因子選擇邏輯

## 數據來源

- VIX 指數：從市場數據提供者取得
- Regime 狀態：來自 `internal/portfolio/regime.go`

## 設計原則

1. **閾值是變量不是常量**：VIX × Regime 混合計算
2. **範圍限制**：永遠夾制在 [0.40, 0.85] 範圍內
3. **平滑過渡**：Regime 變化時採用移動平均，避免頻繁切換

## 驗證要求

```bash
go test ./internal/portfolio/...      # 閾值計算測試
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total  # ≥ 40%
```

## 回測驗證框架

```go
type ThresholdValidation struct {
    Date           time.Time
    VIX            float64
    Regime         string
    DynamicThreshold float64
    ActualCorrelation float64  // 實際觀察到的因子相關性
    WasEffective   bool       // 閾值是否有效篩選
}
```

驗證成功標準：
- DynamicThreshold 版本的報酬率勝過固定閾值版本
- 或 DynamicThreshold 版本的最大回撤較小
