# Strategy Techniques AGENTS.md

## 模組概述

`strategy_techniques` 提供投資心法庫（Strategy Techniques Library）——
以 `StrategyFrame` 為核心的規則引擎，作為台股投資心法看板與系統決策依據。

// Maturity: evolving (Wave 2 接入 main.go 後升 stable)

## 五層框架（L1～L5）

| Layer | 中文 | 對應 Atlas 模組 | Theme 範例 |
|-------|------|----------------|-----------|
| L1 | 全球流動性 | narrative/templates.go | US_rates_up, JPY_carry_unwind |
| L2 | 外資行為 | taiwan_stress_index.go, narrative | foreign_inflow_*, foreign_outflow_* |
| L3 | 產業催化 | narrative | AI_capex_surge, semiconductor_downturn, NVDA_earnings |
| L4 | 匯率籌碼 | narrative, marketdata | USD_TWD_volatility, margin_balance_* |
| L5 | 地緣政治 | narrative, taiwan_stress_index | geopolitical_risk_spike, tariff_shock, taiwan_political_risk |

## 4 核心短線指標

| 指標 | MacroDataSnapshot 欄位 | Provider | Channel |
|------|----------------------|----------|---------|
| 外資現貨買賣超 | ForeignInvestorNet | TWSE T86 | twse_capital_flow |
| 台積電 ADR | TSMADR | Yahoo Finance TSM | tsm_adr |
| 輝達股價 | NVDA | Yahoo Finance NVDA | us_nvda |
| 美元指數 | DXY | Yahoo Finance DX-Y.NYB | us_yahoo (macro batch) |

## 自我修正機制

- **混合歸因**（AttributionMode.RuleBased + AttributionMode.LLMAnnotated）：
  - 規則分類器：regime shift / 政策衝擊 / 結構斷裂 / 數據異常 / 季節性 / 流動性 / 板塊輪動 / 未知
  - LLM 加註：natural language 歸因（待 Wave 2 接入）
- **Regime 標籤**：Janus RegimeClassification（NOVEL/HISTORICAL/MIXED）作分桶
- **多時間尺度驗證**：5D/20D/60D rolling HitRate

## 關鍵符號

- `Layer` / `Status` / `AttributionMode` — 三個核心 enum
- `StrategyFrame` — 心法主結構（取代舊 `EventRule`）
- `Condition` — 觸發條件（擴充 Timeframe/Source）
- `Registry` — 心法儲存（JSON 外部化至 `data/seeds/strategy_techniques.json`）

## 已知陷阱

- **演進中**：Wave 1 期間 API 可能調整，Wave 2 接入主程式後升 stable。
- **S-tier 替代品**：完成 Wave 5 清理（刪除 `internal/eventlogic/`）後，本模組需升 S-tier。

## 相依關係

- 將被 `cmd/atlas/main.go` 匯入（Wave 2）
- 取代 `internal/eventlogic/`（Wave 5 清理後）
- 與 `internal/narrative/`、`internal/portfolio/`、`internal/monitoring/` 互動
