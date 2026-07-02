# AGENTS.md — internal/narrative

本目錄負責**巨集觀敘事（Macro Narrative）**事件偵測與因果鏈推導。

---

## 概覽

**核心資料流**：`MacroIngestor (MarketData) → NarrativeEvent → KnowledgeBase (Match Template) → CausalChain`

**Bundle API**：前端【宏觀敘事】頁面透過 `/api/narrative/bundle` 取得事件、因果鏈、模型與季節性分析。

**News Sentiment 限制**：Finnhub News Sentiment API **僅支援美股**，台股無法直接使用。策略：(1) 美股大盤作為代理（NASDAQ、S&P 500）；(2) 外資流向推斷：美股下跌 + VIX 上升 → 撤離台股；(3) TWSE 開放資料作為本地情緒代理。

---

## NarrativeEvent 核心欄位

- `Theme`（如 `US_rates_up`, `AI_capex_surge`）、`Confidence` [0,1] + `ConfidenceSource`、`HitRate`（**必須**從 `hitRateForTheme()` 取得）、`SourceData`（不可遺漏）、`Duration` / `ExpiresAt`、`Severity`（low/medium/high/critical → 因子權重調整 ±5/10/20/30%）、`Status`（active/confirmed/faded/expired，由 `EventLifecycleManager` 管理）

完整主題列表見 `internal/narrative/templates.go`。

---

## Event 狀態機

`active → confirmed → faded → expired`（可直接跳轉）。`active`：信心度 > 閾值；`confirmed`：2+ 獨立數據源；`faded`：経過時間 > Duration × 0.8；`expired`：経過時間 > ExpiresAt。

**重複偵測**：相同 Theme 在 active 狀態時更新現有事件 Confidence，不建立新事件。

---

## RegimeChange 觸發條件

- VIX 突破 30（HighVol）
- VIX 突破 25 且趨勢向下（Bear）
- VIX 跌破 15 且趨勢向上（Bull）
- `critical` 等級事件觸發
- StressIndex 突破 80

---

## 敘事感知矩陣調變

`SeasonalBridge` 實作 `NarrativeLinkageProvider` 介面，內建 5 個主題乘數（`oil_price_shock`、`AI_capex_surge`、`US_rates_up`、`JPY_carry_unwind`、`geopolitical_risk_spike`）。無配置時自動降級為空列表。

---

## ANTI-PATTERNS

- **手動計算 HitRate**：必須透過 `hitRateForTheme()` 從 `DefaultTemplates` 取得
- **遺漏 SourceData**：每個事件必含觸發時原始數值
- **直接修改模型權重**：由 `UpdateModelWeights` 統一處理
- **手動設定 Status**：detector 僅初始化為 `active`
- **遺漏 causal chain 方向性**：`CausalChain.FavoredSectors` / `AvoidedSectors` 為前端必用欄位

---

## KEY TYPES

`NarrativeEvent`（`types.go`）、`KnowledgeBase`（`knowledge_base.go`）、`CausalChain`（`types.go`）、`EventLifecycleManager`（`lifecycle.go`）、`TaiwanStressIndex`（`taiwan_stress_index.go`）、`SeasonalBridge`（`seasonal_bridge.go`）、`ValidateTaxonomy`（`taxonomy.go`，5 L1 × 5 L2 分類，Phase 3.5 M2 引入）。

---

## 滾動校準框架（簡述）

台灣壓力指數自動校準五層架構（Baseline / Scale / Regime / Validation / Scheduler）詳見 **`docs/MACRO_CALIBRATION.md`**。

**核心規則**：`Calculate` 用 `max(|level_z|, |change_pct|)`（Hybrid Signal）；`FactorBaseline` 只存 `Mean`+`Count`（z-score 即時算）；`ValidateCalibration` 失敗不寫新 config（當日用舊）；校準參數一律取自 `ParametersConfig` 禁 hardcode；`calibration_enabled` 預設 false，啟用前需 staging 驗證 ≥ 30 日。

**Maturity-gated 行為**：`BURN_IN` → log `burn_in_skip` 跳過；`CALIBRATING` / `FULL_AUTO` → 執行校準 + validation gate。

---

## 測試

```bash
go test -v ./internal/narrative/calibration_baseline_test.go
```
