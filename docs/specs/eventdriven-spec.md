# eventdriven 事件驅動資金流預測規格

> 本文件為 `internal/eventdriven` 的技術規格補充；模組陷阱見 `internal/narrative/AGENTS.md`（Stage 5 起 detector 抽象層的關鍵設計都在 narrative 側）。Stage 5 規劃見 `../archive/2026-07-14-atlas-stage5-detector-plan.md`。

## 一、模組定位

`eventdriven` 是 thin adapter — 不持有 detector / template / model，所有 trigger 偵測邏輯位於 `internal/narrative/`。Stage 5 後模組職責僅剩：

1. **Event calendar consumption** — 從 `industry.EventCalendar` 讀取即將到來的 calendar events
2. **Type → Theme 橋接** — 透過 `EventTypeToTriggerThemes(eventType, registry)` 對應到 24 個 trigger themes
3. **NarrativeModelProvider 介接** — 透過 `narrativeAdapter.ListModels()` 拿到 21 個 InvestmentModel
4. **HTTP endpoint** — `/api/events/prediction` 與 `/api/events/calendar`（cmd/atlas 端）

## 二、Confidence 計算

5 日事件驅動資金流預測的信心度範圍為 `(0.5, 1.0]`，計算方式為：

```
confidence = sigmoid(net_weight × (drivers + 1))
```

- `net_weight`：事件驅動因子的淨權重。
- `drivers`：同時作用的事件數量。

## 三、Stage 5 擴充：24 個 Trigger Themes

模板清單由 `internal/narrative/templates.go` 的 `DefaultTemplates()` 提供，**數量是 hard gate**（detector_e2e_test.go:TestE2E_All24ThemesRegistered 保證）。

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

func NewDefaultDetectorRegistry() *DetectorRegistry // 24 detector 全啟用
func (r *DetectorRegistry) RunAll(ctx, in) ([]DetectionResult, []error) // 並發呼叫
```

每個 trigger_theme 一個獨立 detector struct（24 個），預設全部啟用。透過 `Registry.Enable/Disable(theme)` 動態切換。

詳細 contract 見 `internal/narrative/detector.go` 與 `detector_impls.go`。

## 五、Pipeline 架構

| Pipeline | 輸入 | 來源 | 用途 |
|---|---|---|---|
| **KB pipeline** | `MarketNarrativeData` (26 個欄位) | `narrative_detectors.go` (30+ 函式) | Authoritative — 24 個 trigger 中 17 個用此 pipeline |
| **Snapshot pipeline** | `MacroDataSnapshot` + `MacroDataPoint` | `ingestor.go` (15+ 函式) | Degraded-mode proxy — 當 full MarketNarrativeData 不可用時 fallback。`tariff_shock` 是 Stage 5 唯一仍用此 pipeline 的 detector |
| **Seasonal** | `time.Now().UTC()` 視窗判斷 | `detectSeasonalEvent()` | 6 個季節性 trigger |

**兩 pipeline 不可合併**：narrative_detectors.go:108-113 明確標示 — KB 讀 DXY 綜合指標為 authoritative，snapshot 用 ChangePct 為代理。新增 trigger detector 時須先判斷歸屬。

## 六、EventType → TriggerTheme 對應

`internal/eventdriven/type_theme_mapping.go` 提供：

```go
func EventTypeToTriggerThemes(eventType string, registry *narrative.DetectorRegistry) []string
```

14 個 TaiwanEventType 中 7 個對應到 24 templates：

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
- `detector_registry_list` — 列出 24 個 detector 與 enable/disable 狀態（call `/api/detector/registry/list`）

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

## 十一、相關文件

- [`2026-07-14-atlas-stage5-detector-plan.md`（內部 plan，已併入架構） — Stage 5 完整規劃
- [`internal/narrative/AGENTS.md`](../../internal/narrative/AGENTS.md) — narrative 模組陷阱 + Detector 抽象層設計
- [`internal/ledger/AGENTS.md`](../../internal/ledger/AGENTS.md) — （Stage 5 預計新增，目前以 narrative 模組 AGENTS.md 為主入口）
- [`internal/scheduler/AGENTS.md`](../../internal/scheduler/AGENTS.md) — （Stage 5 預計新增，目前以 narrative 模組 AGENTS.md 為主入口）
- [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) — Art.4 BackgroundTaskManager 強制
