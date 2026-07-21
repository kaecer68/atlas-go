# strategy_techniques 心法庫規格

> 本文件為 `internal/strategy_techniques` 的技術規格補充；模組陷阱見 `internal/strategy_techniques/AGENTS.md`。

## 五層框架（L1～L5）

`StrategyFrame` 以五層主題分類台股投資心法，每層對應不同 Atlas 模組與 narrative theme。

| Layer | 中文 | 對應 Atlas 模組 | Theme 範例 |
|-------|------|----------------|-----------|
| L1 | 全球流動性 | narrative/templates.go | US_rates_up, JPY_carry_unwind |
| L2 | 外資行為 | taiwan_stress_index.go, narrative | foreign_inflow_*, foreign_outflow_* |
| L3 | 產業催化 | narrative | AI_capex_surge, semiconductor_downturn, NVDA_earnings |
| L4 | 匯率籌碼 | narrative, marketdata | USD_TWD_volatility, margin_balance_* |
| L5 | 地緣政治 | narrative, taiwan_stress_index | geopolitical_risk_spike, tariff_shock, taiwan_political_risk |

## 4 核心短線指標

下列欄位由 `MacroDataSnapshot` 承載，對應不同 data provider 與 channel。

| 指標 | MacroDataSnapshot 欄位 | Provider | Channel |
|------|----------------------|----------|---------|
| 外資現貨買賣超 | ForeignInvestorNet | TWSE T86 | twse_capital_flow |
| 台積電 ADR | TSMADR | Yahoo Finance TSM | tsm_adr |
| 輝達股價 | NVDA | Yahoo Finance NVDA | us_nvda |
| 美元指數 | DXY | Yahoo Finance DX-Y.NYB | us_yahoo (macro batch) |
