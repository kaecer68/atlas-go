# eventdriven 事件驅動資金流預測規格

> 本文件為 `internal/eventdriven` 的技術規格補充；模組陷阱見 `internal/narrative/AGENTS.md`（Stage 5 起 detector 抽象層的關鍵設計都在 narrative 側）。Stage 5 規劃見 `../archive/2026-07-14-atlas-stage5-detector-plan.md`。

## 一、模組定位

`eventdriven` 是 thin adapter — 不持有 detector / template / model，所有 trigger 偵測邏輯位於 `internal/narrative/`。Stage 5 後模組職責僅剩：

1. **Event calendar consumption** — 從 `industry.EventCalendar` 讀取即將到來的 calendar events
2. **Type → Theme 橋接** — 透過 `EventTypeToTriggerThemes(eventType, registry)` 對應到 registry 中符合語意的 trigger themes 子集（全部 29 個；數量由 registry 決定）
3. **NarrativeModelProvider 介接** — 透過 `narrativeAdapter.ListModels()` 拿到 21 個 InvestmentModel
4. **HTTP endpoint** — `/api/events/prediction` 與 `/api/events/calendar`（cmd/atlas 端）

## 二、Confidence 計算

5 日事件驅動資金流預測的信心度範圍為 `(0.5, 1.0]`，計算方式為：

```
confidence = sigmoid(net_weight × (drivers + 1))
```

- `net_weight`：事件驅動因子的淨權重。
- `drivers`：同時作用的事件數量。

## 三、Stage 5 擴充：Trigger Themes（Stage 5 當時 24 個；現為 29 個，權威數量由 registry 決定）

模板清單由 `internal/narrative/templates.go` 的 `DefaultTemplates()` 提供，**數量是 hard gate**（detector_e2e_test.go:TestE2E_AllThemesRegistered 保證；權威數量見 internal/narrative/detector_count_gate_test.go）。

| 主題類別 | trigger_theme 範例 | Pipeline | 對應 Template ID |
|---|---|---|---|
| Fed 利率 | `US_rates_up`, `US_rates_down` | KB | 美國升息 / 美國降息 |
| 日圓套利 | `JPY_carry_unwind` | KB | 日圓套利平倉 |
| 油 / 商品 | `oil_price_shock`, `gold_rally`, `shipping_rate_spike` | KB | 油價衝擊 / 黃金避險 / 運價飆升 |
| 地緣 / 政治 | `geopolitical_risk_spike`, `taiwan_political_risk`, `china_slowdown`, `tariff_shock` | KB / snapshot | 地緣政治風險 / 台灣地緣 / 中國放緩 / **關稅衝擊** |
| 匯率 | `USD_TWD_volatility`, `dollar_surge`, `inflation_spike` | KB | USD/TWD 波動 / 美元強勢 / 通膨升溫 |
| 半導體 | `semiconductor_downturn`, `taiwan_export_boom` | KB | 半導體週期下行 / 台灣出口強勁 |
| AI / 法人 | `AI_capex_surge`, `earnings_surprise`, `retail_institutional_divergence` | KB | AI 資本支出 / 財報驚喜 / 散戶機構分歧 |
| 季節性 | `spring_festival_season`, `election_cycle`, `earnings_blackout`, `tech_peak_season`, `year_end_window_dressing`, `dividend_season` | seasonal | 春節 / 選舉 / 財報空窗 / 科技旺季 / 年底作帳 / 除權息 |

## 四、Detector 抽象層（Stage 5 新增）

```go
type Detector interface {
    Theme() string                 // 對應 trigger_theme
    Enabled() bool                 // 是否啟用
    SetEnabled(bool)               // 切換啟用狀態
    Detect(ctx, DetectorInput) (*DetectionResult, error)
}

type DetectorRegistry struct { /* sync.RWMutex + map */ }

func NewDefaultDetectorRegistry() *DetectorRegistry // 29 detector 全啟用（數量由 registry 決定，勿硬編）
func (r *DetectorRegistry) RunAll(ctx, in) ([]DetectionResult, []error) // 並發呼叫
```

每個 trigger_theme 一個獨立 detector struct（29 個；數量由 registry 決定），預設全部啟用。透過 `Registry.Enable/Disable(theme)` 動態切換。

詳細 contract 見 `internal/narrative/detector.go` 與 `detector_impls.go`。

## 五、Pipeline 架構

| Pipeline | 輸入 | 來源 | 用途 |
|---|---|---|---|
| **KB pipeline** | `MarketNarrativeData` (26 個欄位) | `narrative_detectors.go` (30+ 函式) | Authoritative — 29 個 trigger 中 21 個用此 pipeline |
| **Snapshot pipeline** | `MacroDataSnapshot` + `MacroDataPoint` | `ingestor.go` (15+ 函式) | Degraded-mode proxy — 當 full MarketNarrativeData 不可用時 fallback。`tariff_shock` 與 `conflict_deescalation` 用此 pipeline（Stage 5 當時僅 `tariff_shock`） |
| **Seasonal** | `time.Now().UTC()` 視窗判斷 | `detectSeasonalEvent()` | 6 個季節性 trigger |

**兩 pipeline 不可合併**：narrative_detectors.go:108-113 明確標示 — KB 讀 DXY 綜合指標為 authoritative，snapshot 用 ChangePct 為代理。新增 trigger detector 時須先判斷歸屬。

## 六、EventType → TriggerTheme 對應

`internal/eventdriven/type_theme_mapping.go` 提供：

```go
func EventTypeToTriggerThemes(eventType string, registry *narrative.DetectorRegistry) []string
```

14 個 TaiwanEventType 中 7 個對應到 29 templates：

| EventType | Trigger Theme |
|---|---|
| `EventSpringFestival` | `spring_festival_season` |
| `EventExDividend`, `EventDividendPayout` | `dividend_season` |
| `EventWindowDressing` | `year_end_window_dressing` |
| `EventElection` | `election_cycle` |
| `EventMonthlyRevenue`, `EventFinancialReport` | `earnings_surprise` |

其餘 7 個 EventType（`EventMSCIRebalance` / `EventTaiwan50Rebalance` / `EventFuturesSettlement` / `EventShareholderMeeting` / `EventInvestorConf` / `EventLongHoliday` / `EventPositionBuilding`）**無 trigger theme 對應**，calendar 顯示為 informational，不觸發 narrative chain。

## 七、Store + Scheduler（Stage 5 PR#4）

| 元件 | 檔案 | 職責 |
|---|---|---|
| `DetectorScanStore` interface | `internal/ledger/detector_scan_store.go` | `AppendScan(results []DetectionResult) (batchID, error)` + `LoadRecentScans(limit)` |
| `SQLiteDetectorScanStore` impl | 同上 | SQLite-only；寫到 `data/state/atlas.db` 的 `detector_scan_log` table |
| `RegisterTemplateDetectorScanTasks` | `internal/scheduler/template_detector_scan.go` | 透過 BackgroundTaskManager 註冊每 1h 排程（遵守 apigateway/CONSTITUTION.md Art.4） |

`detector_scan_log` table schema：

```sql
CREATE TABLE detector_scan_log (
    scan_id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_batch_id TEXT NOT NULL,        -- UUID per RunAll call
    theme TEXT NOT NULL,
    severity TEXT NOT NULL,
    confidence REAL NOT NULL,
    detected_at TEXT NOT NULL,
    source TEXT NOT NULL,
    metadata_json TEXT
);
```

## 八、MCP Tool 層（Deferred to follow-up PR）

原 Stage 5 PR#4 規劃的 2 個 MCP tools 因 scope 過大延後至 follow-up PR：

- `template_detector_status` — 查詢 detector scan 結果（call `/api/detector/scan/status`）
- `detector_registry_list` — 列出 29 個 detector 與 enable/disable 狀態（call `/api/detector/registry/list`）

需要新增：cmd/atlas 2 個 HTTP endpoint + cmd/atlas-mcp tools_template_detector.go + tool count hard gate 106-108 → 108-110 + `docs/reference/tool-catalog.md` 更新 + `go generate ./cmd/atlas-mcp`。

## 九、事件類型（既有 — Stage 5 前）

目前涵蓋的事件類型包括：
- ETF 換股
- MSCI 調整
- 月營收公告
- 季底作帳
- 國定假日

## 十、資料注意事項

- 假日效應需要 historical window ≥ 3 年才穩定。
- MSCI pre-positioning 通常在公告前一週開始反映。
- 電子 / 傳產 / 金融的營收截止日不同，需用 calendar 區分產業別。
- **Stage 5 新增**：detector_scan_log 的 SQLite 路徑由 `config.SQLitePath` 決定，預設 `data/state/atlas.db`（沿用既有 ledger DB）。
- **Stage 5 新增**：排程 Jitter 由 BackgroundTaskManager 自動設為 6min（10% of 1h interval）。

## 十一、構造性棄權與建議下架（2026-10-06，Phase 0）

> **狀態**：`/api/events/prediction` **保留**（研究／回放消費者仍可讀取），但**不得作為建議依據**；
> 零售端首頁的「未來 5 日錢潮預測」卡已**移除**。凍結清單見
> [`docs/operations/EDGE-PROGRAM-FREEZE.md`](../operations/EDGE-PROGRAM-FREEZE.md)（N1–N7）；
> **命中率語彙的定義以該檔為唯一 SSOT**，本規格只引用、不複寫。

### 11.1 為什麼下架：兩條構造性棄權（機制，已由程式碼與測試驗證）

| # | 機制 | 程式碼位置（撰寫時） | 效果 |
|---|---|---|---|
| 1 | `mixed` 方向事件同時加到多空兩側、權重相同（`mixedEventCancellationFactor = 0.3`） | `internal/eventdriven/predictor.go:363-364`、常數 `:515` | 這些事件對淨權重的貢獻**恰為 0**，但仍被列進 `drivers`，使輸出看起來「有依據」 |
| 2 | 資金流 baseline 在非 eligible 狀態下被折扣 | `predictor.go:500`（`calibratingBaselineDiscount = 0.5`）× `:496`（`baselineWeightNearDay = 0.7`）× baseline 上限 0.8（`scaleQualityScoreToBaseline`） | day-1 baseline 上限 = **0.8 × 0.7 × 0.5 = 0.28 < 0.3**（`neutralBand`，`:510`；方向判定 `:397`）⇒ 校準中時 baseline **不可能單獨決定方向** |

**生產觀測（root／PLAN-v2 §3.3 的實測記錄；本 lane 未連生產複核）**：`/api/events/prediction` 的
day1–day5 呈現**同方向、同信心、同驅動**（恆中性），沒有鑑別度；業主本人曾因此被誤導，這才是下架的理由
（不是「訊號不夠」）。

### 11.2 API 契約（加法；端點不刪、欄位不減）

`PredictionReport` 新增 `advisory_status`（`internal/eventdriven/types.go:231`（型別）、`:275`（欄位）），
由 `Predictor.Predict` 每次請求重建（`predictor.go:225`，掛載於 `:210`）：

| 欄位 | 值／語意 |
|---|---|
| `advisory_usable` | 本階段**恆為 `false`**（治理決定，非推導：該訊號家族未通過 G2/G3/G4'） |
| `status` | `withdrawn_constructive_abstention` |
| `abstention_reasons` | 排序後的機制清單（可為空）：`no_edge_evidence`（全窗 5 天皆落中性帶）／`mixed_event_cancellation`（窗內有 mixed 事件且判中性）／`calibration_discount_below_threshold`（非 eligible 且 `\|baseline\| × day-1 權重 < 0.3`） |
| `message` | 人可讀警告（`AdvisoryMessage`）；消費端必須照實呈現，不得只取 `predictions` |
| `evidence_ref` | 指向本節 |

- `predictions`、`active_events`、`sector_predictions`、`historical_hit_rate` 等既有欄位**不變**，且
  `predictions` **不為空**（端點未被下架、未被掏空；`advisory_status` 是純加法）。
- 消費端（網頁、MCP client、agent）**不得**把 `predictions` 當成預測或建議；`advisory_usable = false`
  就是機器可讀的禁止訊號。`cmd/atlas-mcp` 的 `event_flow_prediction` 原樣回傳本回應，因此自動帶上警告。
- 測試：`internal/eventdriven/advisory_status_test.go` — 含①「有方向的日子仍不得作為建議」對照組、
  ②三種 reason 的推導、③`0.28 < 0.3` 的不等式、④JSON wire 契約（端點仍回 200、`predictions` 仍 5 天）。

### 11.3 前端

- `shared_web/static/js/pages/home.js`：預測卡（`#home-predictions`）與其**資料抓取一併移除**
  （首頁不再呼叫 `/api/events/prediction`）；`shared_web/static/css/pages/home.css` 的 `.pred-*`
  樣式同步移除。
- 回歸防護：`shared_web/static/js/__tests__/home.test.mjs`（不得再渲染該卡）、
  `client_web/tests/client-web-trust.spec.ts`（`#home-predictions` 必須 `count = 0`）。
- 重新上架條件 = freeze 文件 §② 的解除條件（G2/G3/G4' 全通過的 gate report ＋ 業主授權，缺一不可）。

### 11.4 未證實／未做（誠實清單）

- `abstention_reasons` 的**機制**已由 `go test` 覆蓋；但「生產 5 天同值」是 **root／PLAN-v2 的實測記錄**，
  本 lane 未連生產複核。
- 本次**只**做加法標示與前端下架：未移除任何欄位、未改 `computeHistoricalHitRate` 的語意、
  未改校準門檻（`MinHitSamples`）。
- `cmd/atlas-mcp` 的 tool **描述文字未改**（回應本體已帶警告）；tool catalog 文字變更需要 `go generate`
  與 `docs/reference/tool-catalog.md` 同步，屬另案。

## 十二、相關文件

- [`2026-07-14-atlas-stage5-detector-plan.md`（內部 plan，已併入架構） — Stage 5 完整規劃
- [`internal/narrative/AGENTS.md`](../../internal/narrative/AGENTS.md) — narrative 模組陷阱 + Detector 抽象層設計
- [`internal/ledger/AGENTS.md`](../../internal/ledger/AGENTS.md) — （Stage 5 預計新增，目前以 narrative 模組 AGENTS.md 為主入口）
- [`internal/scheduler/AGENTS.md`](../../internal/scheduler/AGENTS.md) — （Stage 5 預計新增，目前以 narrative 模組 AGENTS.md 為主入口）
- [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) — Art.4 BackgroundTaskManager 強制
- [`docs/operations/EDGE-PROGRAM-FREEZE.md`](../operations/EDGE-PROGRAM-FREEZE.md) — edge 驗證計畫 Phase 0 凍結清單與命中率語彙 SSOT（十一節的上位依據）
