# Screening Layer 操作手冊

## 概述

Screening Layer 在 `sector/style` executor 生成推薦**之前**執行宣告式篩選，確保只有符合條件的標的進入後續分析。

## 篩選條件

### 基本面篩選

| 條件 | 類型 | 說明 | 範例 |
|------|------|------|------|
| P/E | RangeFilter | 本益比區間 | `{"min": 5, "max": 18}` |
| P/B | RangeFilter | 股價淨值比區間 | `{"max": 2}` |
| DividendYield | RangeFilter | 股息率區間 | `{"min": 2.0}` |

### 技術面篩選

| 條件 | 類型 | 說明 | 範例 |
|------|------|------|------|
| Momentum20Day | RangeFilter | 20 日動能區間 | `{"min": 0}` |
| Volatility20Day | RangeFilter | 20 日波動率區間 | `{"max": 5}` |
| VolumeIntraday | MinFilter | 日內成交量下限 | `{"min": 1000000}` |

### 綜合篩選

| 條件 | 類型 | 說明 | 範例 |
|------|------|------|------|
| MinTotalFactorScore | float64 | 最小總因子分數 | `0.6` |
| RequiredFactors | []string | 必填因子清單 | `["momentum", "value"]` |

## 配置方式

在 `configs/agents.json` 中為每個 agent 設定 `screening_criteria`：

```json
{
  "id": "value-yield-01",
  "screening_criteria": {
    "pe": {"max": 18},
    "pb": {"max": 2},
    "dividend_yield": {"min": 2.0}
  }
}
```

## 驗證篩選邏輯

```bash
# 執行 screener 測試
go test ./internal/screener/... -v

# 查看篩選結果範例
go test ./internal/screener/... -run TestScreenFiltersByPE -v
```

## 注意事項

1. **篩選條件過嚴**可能導致某檔標的「完全沒有推薦」，這是預期行為
2. **nil 或 absent** 的欄位表示不篩選（pass-through）

## 缺資料拒絕（fail-closed）

2026-06-05 起，三大基本面欄位都會在資料缺失時主動 fail（不再靜默放行）：

| 欄位 | 缺資料失敗代碼 | 說明 |
|------|---------------|------|
| P/E | `pe_missing` | `data.PriceToEarnings <= 0` 時 |
| P/B | `pb_missing` | `data.PriceToBook <= 0` 時 |
| DividendYield | `dividend_yield_missing` | `data.DividendYield <= 0` 時 |

行為：當 screener 收到 `RangeFilter` 或 `MinFilter` 類型的條件但底層資料缺失，會以 `criterion_label`、閾值、實際值（`0`）記錄到 `ScreeningFailure`，供 dashboard `pipeline` 頁面的「被篩除」區塊展示。
3. 調整門檻前請先用 `go test ./internal/screener/...` 驗證
4. 篩選結果會記錄 `ScreeningReject` 供稽核

## 監控指標

- **篩選率** = 通過篩選標的數 / 總標的數
- **拒絕原因分佈** = 各篩選條件的拒絕次數
