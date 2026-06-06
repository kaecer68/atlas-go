# AGENTS.md — internal/retail

`internal/retail` 提供 RSI-tw（Retail Sentiment Index — Taiwan），一個台灣股市散戶情緒綜合指標計算引擎。

---

## 架構概覽

RSI-tw 由三個加權組件構成：

| 組件 | 權重 | 子指標 | 資料來源 |
|------|------|--------|----------|
| **Part A** | 40% | A1 融資餘額 Z-score、A2 當沖比率、A3 維持率代理、A4 VIX 映射、A5 週選擇權 PCR、A6 零股交易失衡 | MacroDataSnapshot + 滾動歷史 |
| **Part C** | 25% | C1 散戶期貨 OI、C2 機構法人流向、C3 ETF 申購贖回 | Gateway channels（taifex_daily, twse_oddlot, twse_etf） |
| **Part D** | 乘數 | D1 地緣政治風險、D2 VIX 飆升、D3 信貸緊縮、D4 閃崩 | MacroDataSnapshot + Narrative events |

**核心型別**：
- `RSITwInput`：計算輸入（所有資料由 handler 從 snapshot + fetcher 組裝）
- `RSITwSnapshot`：計算輸出（Score, PartAScore, PartCScore, AdjustmentFactor, SubIndicators）
- `RSISubIndicator`：單一子指標明細（Value, Weight, ZScore, IsFallback）
- `Calculator`：Singleton 計算器（含 90 筆滾動歷史用於 Z-score）

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **靜默 fallback 到 0.5** | 當 Gateway channel 回傳錯誤或 fetcher 為 nil 時，C1/C2/C3 子指標靜默 fallback 到 0.5 或 0。前端無從得知是否為真實數據。 |
| **GeopoliticalRisk 硬編碼為 0** | `handlers.go:204` 直接寫 `GeopoliticalRisk: 0`，導致 Part D 地緣政治風險調整永遠不觸發。需從 Narrative provider 讀取真實值。 |
| **Part A 權重寫死在程式碼** | `computePartA()` 及 subA1-A6 的權重（0.25, 0.20, 0.20, 0.15, 0.10, 0.10）為 literal constant，無法透過 `parameters.json` 調整。 |
| **Part C 子權重亦寫死** | subC1 0.40、subC2 0.35、subC3 0.25 亦為 literal constant。 |
| **vixMap / PCR / odd-lot 閾值不可配置** | VIX 閾值 `[15,20,25,30,35]`、PCR 閾值 `[1.5,1.0,0.8]`、odd-lot 閾值 `[0.2,0.1,-0.1,-0.2]` 全部寫死在對應 sub 函數中。 |
| **無校準機制** | 所有 `RSITwParameters` 均為 `SourceHeuristic`，無 backtest pipeline、無 Bayesian optimization、無 autonomous recalibration。 |
| **無下游消費** | RSI-tw 值僅在前端展示，未被 orchestrator、risk manager、factor engine 或任何 executor 使用。 |
| **滾動歷史上限固定** | `UpdateHistory()` 最多保留 90 筆，此值寫死在程式碼中。 |
| **A3 Z-score formula 硬編碼** | `(percentile - 0.5) * 2` 的 midpoint 和 scaling factor 不可調整。 |
| **缺乏測試** | 模組無任何測試檔案。 |

---

## 資料來源對照表

| RSITwInput 欄位 | Handler 來源 (handlers.go) | 真實性 |
|-----------------|---------------------------|--------|
| `MarginBalance` | `snap.RetailMarginBalance.Value` | ✅ 來自 macro snapshot |
| `MarginPercentile` | `calculateMarginPercentile()` | ✅ 百分位計算 |
| `DayTrading` | `DayTradingFetcher` → Gateway channel | ✅ 已接線 |
| `VIXLevel` | `snap.VIX.Value` | ✅ 來自 macro snapshot |
| `ForeignInvestorNet` | `snap.ForeignInvestorNet.Value` | ✅ 來自 macro snapshot |
| `DomesticFundNet` | `snap.DomesticFundNet.Value` | ✅ 來自 macro snapshot |
| `GeopoliticalRisk` | `0` (硬編碼) | ❌ 未接線 |
| `CreditTightening` | — (未填充) | ❌ 未接線 |
| `PutCallRatio` | `TaifexFetcher` → Gateway channel | ⚠️ 已接線，但若失敗 fallback |
| `OddLotImbalance` | `OddLotFetcher` → Gateway channel | ⚠️ 已接線，但若失敗 fallback |
| `RetailFuturesPct` | `TaifexFetcher` → Gateway channel | ⚠️ 已接線，但若失敗 fallback |
| `ETFNetSubscription` | `ETFFetcher` → Gateway channel | ⚠️ 已接線，但若失敗 fallback |

---

## Sub-Indicator Key Convention

計算過程中儲存在 `RSITwSnapshot.SubIndicators` map 中的 key：

| Key | 對應子指標 | 前端顯示名稱 |
|-----|-----------|-------------|
| `a1_margin_z` | A1 | 融資餘額 Z-score |
| `a2_day_trading` | A2 | 當沖比率 |
| `a3_margin_maint` | A3 | 維持率 Z-score |
| `a4_vix_map` | A4 | VIX 風險分數 |
| `a5_pcr_proxy` | A5 | 週選擇權 PCR |
| `a6_odd_lot` | A6 | 零股交易失衡 |
| `c1_futures_oi` | C1 | 散戶期貨 OI |
| `c2_inst_flow` | C2 | 券商分點流向 |
| `c3_etf_sub` | C3 | ETF 申購分數 |
| `d1_geopolitical` | D1 | 地緣政治風險 |
| `d2_vix_spike` | D2 | VIX 飆升 |
| `d3_credit_control` | D3 | 信貸緊縮 |
| `d4_flash_crash` | D4 | 閃崩 |

---

## 相關文件

| 文件 | 內容 |
|------|------|
| `doc.go` | Package 概述、Maturity 標記 |
| `rsi_tw_calculator.go` | 計算器主邏輯（504 行） |
| `internal/config/parameters.go:971` | `RSITwParameters` 定義 |
| `internal/config/parameters_defaults.go:3020` | 預設參數值 |
| `internal/monitoring/api/system/handlers.go:148` | API handler 與資料組裝 |
| `internal/monitoring/dashboard_api.go:544` | Fetcher 接線 |
| `internal/monitoring/gateway_adapter.go:319` | Gateway adapter 實作 |
| `docs/superpowers/plans/2026-05-31-rsi-tw-calibration-autonomy.md` | 校準與自主進化計劃 |
