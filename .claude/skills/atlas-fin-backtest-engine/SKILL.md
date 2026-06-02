# Atlas 完整回測引擎 Skill

> ⚠️ **此技能描述的功能部分實作，但整體仍為藍圖階段**  
> **實作狀態**：⚠️ 部分實作 — 基礎 Window + BacktestPipeline + RollingSplit 已實作，但十分位、FF5、成本模型尚未建立  
> **最後審計**：2026-06-02  
> **現有基礎**：`window.go`、`backtest_pipeline.go`（含 Model 介面）、`rolling_split.go`（SK-03 expanding window）

**版本**: 1.0
**日期**: 2026-05-29
**職責**: 完整回測引擎 — 十分位投資組合、因子 Alpha、交易成本、滾動窗口

---

## Academic Foundation

基於 Fin-Skills 論文規格：

| SK 編號 | 論文主題 | Atlas 現有基礎 | 狀態 |
|---------|---------|---------------|------|
| SK-16 | Decile portfolio (十分位投資組合) | ❌ 無 | 未實作 |
| SK-17 | Equal/value weighting (等權/市值加權) | ⚠️ optimizer 有等權邏輯 | 需擴充 |
| SK-18 | FF5+MOM alpha, Newey-West t-stat | ❌ 無 | 未實作 |
| SK-19 | Taiwan tax/cost (台灣交易成本) | ✅ `internal/tax/taiwan_tax.go` | ✅ 已實作 |
| SK-29 | Rolling window backtest | ✅ `rolling_split.go` 已實作（expanding window） | ✅ SK-03 已實作 |

---

## 核心哲學

> **「Atlas 的 backtest/window.go 只能跑單一窗口的簡單模擬。Fin-Skills 規範要求嚴格的 rolling-window 設計。」**

---

## Rolling Window 設計 (SK-29/SK-03)

已實作：`internal/backtest/rolling_split.go` — expanding window split（訓練窗口逐漸擴大）。

```
Train (24 months) │ Valid (3m) │ Test (3m) → 每月滾動 1 個月
```

`BacktestPipeline`（`backtest_pipeline.go`）已整合 Model 介面，支援 ML 模型回測。

---

## 實際檔案結構

| 元件 | 實際檔案 | 狀態 |
|------|---------|------|
| Window Runner | `internal/backtest/window.go` | ✅ 已實作 |
| Backtest Pipeline | `internal/backtest/backtest_pipeline.go` | ✅ 已實作（含 Model 介面整合） |
| Rolling Split | `internal/backtest/rolling_split.go` | ✅ 已實作（SK-03 expanding window） |
| Rolling Window Engine | `internal/backtest/rolling.go` | ❌ 未實作 |
| Decile Portfolio | `internal/backtest/decile.go` | ❌ 未實作 |
| FF5+MOM Alpha | `internal/backtest/ff5.go` | ❌ 未實作 |
| Cost Model | `internal/backtest/cost_model.go` | ❌ 未實作 |
| Taiwan Tax（現有） | `internal/tax/taiwan_tax.go` | ✅ 已實作 |

## Decile Portfolio (SK-16) 與 Weighting (SK-17) — 未實作

按模型預測分數將股票分為 10 組，計算各組的等權報酬。

### 加權方案 (SK-17)

| 方案 | 公式 | 適用場景 |
|------|------|---------|
| Equal weight | wᵢ = 1/N | 基準方案 |
| Value weight | wᵢ = MVᵢ / ΣMV | 反映市場實際權重 |
| Score weight | wᵢ = softmax(scoreᵢ) | 最大化因子暴露 |

### 台灣交易成本 (SK-19) — 已實作基礎

| 成本項目 | 費率 | 收取方 | 方向 |
|---------|------|--------|------|
| 手續費 | 0.1425% × 0.6 (折扣) ≈ 0.0855% | 券商 | 買+賣 |
| 證交稅 | 0.3% | 政府 | 僅賣出 |
| 滑價 | 0.1% (預估) | 市場 | 買+賣 |
| **來回總成本** | **≈ 0.654%** | | |

`internal/tax/taiwan_tax.go` 已實作稅務計算，但獨立的 `cost_model.go`（含手續費、滑價建模）尚未建立。

---

## FF5+MOM 台灣因子資料（SK-18）— 未實作

台灣 Fama-French 五因子 + 動能因子的資料取得方式：

| 因子 | 名稱 | 台灣資料源 | 建構方式 |
|------|------|-----------|---------|
| Mkt | 市場超額報酬 | TWSE 加權指數報酬 - 台灣 10 年期公債殖利率 | TEJ / FinMind 日報酬 |
| SMB | 規模因子 | 小市值組合 - 大市值組合 | TWSE 市值分組（依市值中位數 split） |
| HML | 價值因子 | 高 B/M 組合 - 低 B/M 組合 | TEJ 財報 B/M ratio |
| MOM | 動能因子 | 過去 12 個月贏家 - 輸家 | TWSE 價格報酬 |
| RMW | 盈利因子 | 高 OP 組合 - 低 OP 組合 | TEJ 財報營業利益率 |
| CMA | 投資因子 | 低 Inv 組合 - 高 Inv 組合 | TEJ 財報總資產成長率 |

---

## 交叉參考

- **atlas-fin-ml-pipeline**: ML 模型訓練
- **atlas-fin-model-eval**: 模型評估
- **atlas-risk-management**: 風險整合
- **atlas-data-management**: 資料提供者
- **atlas-core-architecture**: 整體架構與模組邊界

---

## Go 實作骨架（FF5 — 待實作）

```go
// internal/backtest/ff5.go — 尚未實作

type FF5Factors struct {
    Mkt []float64 // 市場超額報酬
    SMB []float64 // 規模因子
    HML []float64 // 價值因子
    MOM []float64 // 動能因子
    RMW []float64 // 盈利因子
    CMA []float64 // 投資因子
}

// LoadTWFF5 loads Taiwan FF5+MOM factors from FinMind or TEJ data files.
func LoadTWFF5(startDate, endDate time.Time) (*FF5Factors, error)
```

*技能版本: 1.0*
*最後更新: 2026-06-02*
*狀態: 部分實作 — 基礎 Window/Pipeline 已完成，十分位/FF5/成本模型/rolling.go 待建立*
