# Atlas 完整回測引擎 Skill

**版本**: 1.0
**日期**: 2026-05-29
**職責**: 完整回測引擎 — 十分位投資組合、因子 Alpha、交易成本、滾動窗口

---

## Academic Foundation

基於 Fin-Skills 論文規格：

| SK 編號 | 論文主題 | Atlas 現有基礎 | 需擴充 |
|---------|---------|---------------|--------|
| SK-16 | Decile portfolio (十分位投資組合) | ❌ 無 | 新建 |
| SK-17 | Equal/value weighting (等權/市值加權) | ⚠️ optimizer 有等權邏輯 | 擴充 |
| SK-18 | FF5+MOM alpha, Newey-West t-stat | ❌ 無 | 新建 |
| SK-19 | Taiwan tax/cost (手續費 0.00654+0.003) | ✅ taiwan_tax.go | 擴充 |
| SK-29 | Rolling window backtest | ⚠️ window.go 僅單窗口 | 重構 |

---

## 核心哲學

> **「Atlas 的 backtest/window.go 只能跑單一窗口的簡單模擬。Fin-Skills 規範要求嚴格的 rolling-window 設計。」**

---

## Rolling Window 設計 (SK-29)

```
Train (24 months) │ Valid (3m) │ Test (3m) → 每月滾動 1 個月
```

---

## Decile Portfolio (SK-16) 與 Weighting (SK-17)

按模型預測分數將股票分為 10 組，計算各組的等權報酬。

### 加權方案 (SK-17)

| 方案 | 公式 | 適用場景 |
|------|------|---------|
| Equal weight | wᵢ = 1/N | 基準方案 |
| Value weight | wᵢ = MVᵢ / ΣMV | 反映市場實際權重 |
| Score weight | wᵢ = softmax(scoreᵢ) | 最大化因子暴露 |

### 台灣交易成本 (SK-19)

| 成本項目 | 費率 | 收取方 | 方向 |
|---------|------|--------|------|
| 手續費 | 0.1425% × 0.6 (折扣) ≈ 0.0855% | 券商 | 買+賣 |
| 證交稅 | 0.3% | 政府 | 僅賣出 |
| 滑價 | 0.1% (預估) | 市場 | 買+賣 |
| **來回總成本** | **≈ 0.654%** | | |

---

## 新建模組結構

```
internal/backtest/
├── window.go          # 現有：單窗口 runner
├── rolling.go         # 新建：SK-29 rolling window 引擎
├── decile.go          # 新建：SK-16 decile portfolio 分析
├── ff5.go             # 新建：SK-18 FF5+MOM alpha 回歸
└── cost_model.go      # 新建：SK-19 台灣交易成本模型
```

---

## 交叉參考

- **atlas-fin-ml-pipeline**: ML 模型訓練
- **atlas-fin-model-eval**: 模型評估
- **atlas-risk-management**: 風險整合
- **atlas-data-management**: 資料提供者
- **atlas-core-architecture**: 整體架構與模組邊界

---

## Go 實作骨架

### FF5+MOM 台灣因子資料源說明

台灣 Fama-French 五因子 + 動能因子的資料取得方式：

| 因子 | 名稱 | 台灣資料源 | 建構方式 |
|------|------|-----------|---------|
| Mkt | 市場超額報酬 | TWSE 加權指數報酬 - 台灣 10 年期公債殖利率 | TEJ / FinMind 日報酬 |
| SMB | 規模因子 | 小市值組合 - 大市值組合 | TWSE 市值分組（依市值中位數 split） |
| HML | 價值因子 | 高 B/M 組合 - 低 B/M 組合 | TEJ 財報 B/M ratio |
| MOM | 動能因子 | 過去 12 個月贏家 - 輸家 | TWSE 價格報酬 |
| RMW | 盈利因子 | 高 OP 組合 - 低 OP 組合 | TEJ 財報營業利益率 |
| CMA | 投資因子 | 低 Inv 組合 - 高 Inv 組合 | TEJ 財報總資產成長率 |

```go
// internal/factor_model/ff5.go

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
*最後更新: 2026-05-29*
