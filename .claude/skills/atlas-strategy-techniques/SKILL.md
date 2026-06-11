---
name: atlas-strategy-techniques
description: "Use when working with the 5-layer investment techniques library (L1-L5), StrategyFrame design, or 9 production seeds. Triggers: new strategy techniques, layer classification, hit rate tracking, attribution, eventlogic migration."
---

> **實作狀態**：✅ 已實作（`internal/strategy_techniques/` 模組，成熟度 `evolving`）
> **最後審計**：2026-06-11
> **取代關係**：取代 `internal/eventlogic/`（已標 410 Gone，待 Wave 5 cleanup 刪除）
> **生產種子**：9 條（6 條遷移 + 3 條 L5 新增），載入自 `data/seeds/strategy_techniques.json`

## 描述

**投資心法庫**（Strategy Techniques Library）— 以 `StrategyFrame` 為核心的台股投資心法看板與系統決策依據。

## 任務觸發

當 AI 代理需要：
- 新增/修改/刪除投資心法（StrategyFrame）
- 為心法歸屬 5 層分類（L1~L5）
- 調整心法條件（CROSS_ABOVE/BELOW、Timeframe）
- 追蹤心法命中率（HitRate）
- 處理心法失效歸因（Attribution）
- 與 eventlogic 模組做 migration 對照

## 核心概念

### 1. 五層框架（L1~L5）

| Layer | 中文 | 對應 Atlas 模組 | 主題範例 |
|-------|------|----------------|---------|
| **L1** | 全球流動性 | narrative/templates.go | US_rates_up、JPY_carry_unwind |
| **L2** | 外資行為 | taiwan_stress_index.go、narrative | foreign_inflow_sustained、foreign_outflow_* |
| **L3** | 產業催化 | narrative | AI_capex_surge、semiconductor_downturn、NVDA_earnings |
| **L4** | 匯率籌碼 | narrative、marketdata | USD_TWD_volatility、margin_balance_* |
| **L5** | 地緣政治 | narrative、taiwan_stress_index | taiwan_political_risk、tariff_shock、china_slowdown |

### 2. 4 核心短線指標（4 Core Leading Indicators）

對短線 1~3 日台股方向判斷效果最佳的四個觀察窗口：

| 指標 | 數據源 | MacroDataSnapshot 欄位 | 提供者 |
|------|--------|----------------------|--------|
| **外資現貨買賣超** | TWSE T86 | `ForeignInvestorNet` | `twse_capital_flow` |
| **台積電 ADR** | Yahoo Finance TSM | `TSMADR` | `tsm_adr` |
| **輝達股價** | Yahoo Finance NVDA | `NVDA` | `us_nvda` |
| **美元指數** | Yahoo Finance DX-Y.NYB | `DXY` | `us_yahoo` (macro batch) |

### 3. StrategyFrame 結構

```go
type StrategyFrame struct {
    ID          string         // 唯一識別
    Name        string         // 投資心法名稱
    Layer       Layer          // L1~L5
    Summary     string         // 一句話投資直覺
    Rationale   string         // 為何這個邏輯成立
    Conditions  []Condition    // 觸發條件（可跨指標 AND/OR）
    Themes      []string       // 適用 NarrativeEvent Themes
    Regimes     []string       // 適用 JANUS Regime
    Sectors     []string       // 受影響板塊
    Direction   Direction      // up/down/volatile
    Risk        Risk           // low/medium/high
    Source      Source         // backtest/manual/auto_discovered
    Status      Status         // active/degraded/expired
    AttributionMode AttributionMode  // rule_based/llm_annotated
    Attribution []string       // 失效歸因歷史（自我修正線索）
    HitRate     float64        // 命中率 (0~1)
    TotalTests  int
    TotalHits   int
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 4. Condition 觸發條件

```go
type Condition struct {
    Field       string   // 例如 "ForeignInvestorNet.Value"、"NVDA.ChangePct"
    Operator    string   // eq/neq/gt/lt/gte/lte/cross_above/cross_below
    Value       float64
    StringValue string
    Timeframe   string   // 1D/3D/5D/20D/60D
    Source      string   // 數據來源（用於追溯）
}
```

**8 種運算子**：`eq`、`neq`、`gt`、`lt`、`gte`、`lte`、`cross_above`、`cross_below`

**Field 命名規範**（沿用 evaluator NumericFields dot notation）：
- `ForeignInvestorNet.Value` / `ForeignInvestorNet.ChangePct`
- `TSMADR.Value` / `TSMADR.ChangePct`
- `NVDA.Value` / `NVDA.ChangePct`
- `DXY.Value` / `DXY.ChangePct`
- `USD_TWD.Value` / `USD_TWD.ChangePct`
- `US10Y.Value` / `US10Y.ChangePct`
- `VIX.Value` / `VIX.ChangePct`
- `SPXIndex.ChangePct`、`NDXIndex.ChangePct`、`DJIIndex.ChangePct`
- `SOXIndex.ChangePct`、`Bdi.ChangePct`
- `Gold.ChangePct`、`Oil.ChangePct`、`JPY.ChangePct`

### 5. 9 條 Production Seeds

| ID | Layer | 條件 | 用途 |
|----|-------|------|------|
| `dxy-weak-us10y-down` | L1 | DXY<105 (3D) AND US10Y<4.5 (5D) | 美元轉弱 → 資金回流亞洲 |
| `foreign-3day-inflow` | L2 | ForeignInvestorNet>0 (3D) | 外資連 3 日買超 |
| `nvidia-tsmadr-confirm` | L3 | NVDA>0.5% AND TSMADR>0.3% (1D) | 4 指標整合心法 |
| `us-market-overnight-confirm` | L3 | SPX>0 AND NDX>0 AND SOX>0 AND DJI>0 (1D) | 美股夜盤全漲確認 |
| `usd-twd-32-managed-float` | L4 | USD_TWD>32 (3D) | 央行 32 防線 |
| `sox-foreignflow-semiconductor` | L2 | SOX>0.5% AND ForeignInvestorNet>0 (1D) | 半導體 + 外資共振 |
| `taiwan-strait-tension` | L5 | DXY>106 AND VIX>25 (1D) | 台海緊張（LLM 加註）|
| `china-slowdown-export-pressure` | L5 | SPX<-1 (5D) AND DXY>104 (5D) | 中國經濟放緩傳導 |
| `us-tariff-shock-tech` | L5 | SPX<-2 AND VIX>30 (1D) | 美國關稅衝擊科技股 |

### 6. 自我修正機制（混合歸因）

- **規則化打底**：8 種歸因類型 — regime shift / 政策衝擊 / 結構斷裂 / 數據異常 / 季節性 / 流動性 / 板塊輪動 / 未知
- **LLM 加註**：Wave 4 接入，自然語言解釋失效原因
- **Regime 標籤**：依 `JANUS` RegimeClassification（NOVEL/HISTORICAL/MIXED）做命中率分桶
- **多時間尺度驗證**：5D/20D/60D rolling HitRate

## 擴展性設計

- **熱加載**：JSON 外部化（`data/seeds/strategy_techniques.json`），不需重編譯即可新增心法
- **向後相容**：新代碼優先用 strategy_techniques，舊 eventlogic 路由（已 410 Gone）保留 migration window
- **新 Layer**：Layer 為 string enum，未來可加 L6（市場結構）/ L7（散戶情緒）
- **新指標**：在 `marketdata/macro_provider.go` 加欄位 → Condition 自然支援

## 與 eventlogic 取代關係

| 事件邏輯（舊） | 投資心法（新） |
|--------------|--------------|
| `EventRule{ID, Pattern, Conditions, ...}` | `StrategyFrame{ID, Name, Layer, Rationale, ...}` |
| 6 條 seed | 9 條 seed（6 遷移 + 3 L5 新增）|
| 單一 `eventlogic` enum | 6 個 enum（Layer/Status/Direction/Risk/Source/AttributionMode）|
| `corector.Evaluate` | 混合歸因（rule_based + llm_annotated）|
| 無 4 指標整合 | 4 核心指標整合心法（nvidia-tsmadr-confirm）|
| `/api/eventlogic/rules` | `/api/strategies`（已 410 Gone 舊路徑）|

## 實作位置

| 元件 | 檔案 | 狀態 |
|------|------|------|
| 套件核心 | `internal/strategy_techniques/doc.go`、`enums.go`、`frame.go`、`condition.go` | ✅ 已實作 |
| 註冊表 | `internal/strategy_techniques/registry.go` | ✅ 已實作 |
| 測試 | `internal/strategy_techniques/*_test.go` | ✅ 29 tests PASS |
| 種子資料 | `data/seeds/strategy_techniques.json` | ✅ 9 frames |
| API 端點 | `internal/monitoring/api/strategies/handlers.go` | ✅ 7 routes |
| 前端 | `web/static/js/pages/strategies.js` | ✅ 5 層 tabs + 4 指標 strip |
| 整合 | `cmd/atlas/main.go` | ✅ WithStrategyTechniques registered |
| 插件 | `internal/orchestrator/strategy_techniques_plugin.go` | ✅ 已實作 |

## 與其他技能整合

- `atlas-macro-narrative`：5 層框架的 theme 詞彙 80% 來自 `narrative/templates.go`
- `atlas-taiwan-leading-indicators`：4 指標的數據源 + 計算口徑
- `atlas-event-driven-weights`：心法命中 → 因子權重調整（見該 skill 第 7 節）
- `atlas-multi-strategy`：心法庫為策略選擇的上游輸入

## 驗證要求

```bash
go test ./internal/strategy_techniques/...          # 29 tests
go test ./internal/monitoring/api/strategies/...    # 22 tests
go build ./...                                       # 0 errors
gofmt -l .                                           # 0 dirty
staticcheck ./...                                    # 0 issues
esbuild build                                        # 0 errors
```

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **L4 種子為零** | 9 條 seeds 中 L4 僅 1 條（usd-twd-32），L4 心法庫稀疏，待擴充 |
| **ConsecutiveDays 缺失** | `ForeignInvestorNet.ConsecutiveDays` 欄位不存在於 MacroDataSnapshot，須從歷史或 eventbus 累積 |
| **eventlogic 並未刪除** | Wave 5 cleanup 才正式刪除 `internal/eventlogic/`，目前雙軌制（eventlogic 路由 410 Gone，但 package 仍 import）|
| **StrategyTechniquesPlugin 簡化版** | `PostSimulation` 為 no-op 簡化版，完整 detector + corrector 待 Wave 4 補 |

## 設計原則

1. **投資心法優先**：UI 層級為「投資心法庫」而非「規則管理」
2. **5 層為骨架**：所有心法歸屬 L1~L5 之一，可疊加跨層
3. **自我修正強化**：歸因欄位（失效原因）+ Regime 標籤 + 多時間尺度 rolling
4. **熱擴充**：規則 JSON 化，存於 `data/seeds/strategy_techniques.json`
5. **向後相容**：保留 `/api/eventlogic/*` 410 Gone，新路徑 `/api/strategies/*` 並行

---

*技能版本: 1.0*
*最後更新: 2026-06-11*
*適用對象: Atlas-Go AI Agent*
