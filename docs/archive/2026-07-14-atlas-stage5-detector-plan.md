# Atlas Template Trigger Detector 鏈路補齊 — Stage 5 實作規劃

> **分支**：`feat/stage-5-trigger-detector`（基於 `pr-1140` @ `61248639`）
> **基礎**：Stage 4 已 ship（PR #1138 + PR #1140）— 24 templates 全面重算 hit_rate + prediction backtest engine + 4 個 SQLite history tables
> **目標**：落地 Stage 1 PR#4 規劃的 `template trigger detector`，讓 24 templates 每個都有獨立、可啟用/停用的 detector function，並建立端到端 unit test 證明 detector→template→model→prediction 鏈路完整
> **決策來源**：
>   - `~/workspace/atlas-notes/decisions/` — Stage 1-4 決策檔
>   - 本檔第三節「範圍決議」（待用戶確認）
> **語言**：繁體中文（所有 code comment + commit message + PR description）

---

## 一、現況摘要（Stage 1-4 已完成，作為 Stage 5 的基礎）

### Stage 1（4-PR 補齊，PR #1102）
- EventCalendar wire-up — `industry.NewEventCalendarWithProvider`
- CapitalFlowProvider injection — `SetCapitalFlow`
- Darwinian narrative models — `NarrativeModelProvider` + `ModelView` + `eventTypeToThemes`
- UTC + scheduling + nil guards
- **PR#4 規劃的 `template detector` 未實作**（Stage 5 主要補齊對象）

### Stage 2（data quality + EventValidator）
- EventValidator + 5 rules + QualityLog
- title sanitize + cross-source verify + template model audit

### Stage 3（scheduling + alerting + observation log）
- 5 個排程 + 3 條報警任務 wrapper
- `RecalculateTemplateHitRates` public wrapper
- `RecentEventFlowPredictions` 改為 ledger 持久化
- `IsTradingDay` 改用 `EventCalendar.IsTaiwanTradingDay`

### Stage 4（historical backfill + prediction backtest，PR #1138 + #1140）
- `cmd/atlas-stage4-backfill` — 4 個 staging JSONL 萃取器
- `cmd/atlas-stage4-loader` — 4 個 SQLite history tables
- `cmd/backtest-event-flow` — prediction backtest engine
- `feat(narrative): RecalculateAllTemplateHitRates — expand to all 24 templates`（commit 329c77a5）

### 已驗證的關鍵檔案行號（從 3 個 explore agents 真實盤查）
| 檔案 | 內容 |
|---|---|
| `internal/narrative/templates.go:1-494` | **24 個 hard-coded `CausalTemplate`**（`DefaultTemplates()`），含 `TriggerTheme string` 欄位 |
| `internal/narrative/types.go` | `CausalTemplate`、`InvestmentModel`、`NarrativeEvent` struct 定義 |
| `internal/narrative/knowledge_base.go` | **12 個 hard-coded `InvestmentModel`**（`NewNarrativeEngine()`），含 `FavoredSectors/AvoidedSectors/ActiveThemes/DarwinianWeight` 欄位 |
| `internal/narrative/narrative_detectors.go` | **30+ 個 `detectXxxKBEvent(data MarketNarrativeData) *NarrativeEvent`**（KB pipeline detector，data → NarrativeEvent） |
| `internal/narrative/ingestor.go` | **15+ 個 `detectXxxEventFromSnapshot(snap MacroDataSnapshot) *NarrativeEvent`**（ingestor pipeline detector，snapshot → NarrativeEvent） |
| `internal/eventdriven/predictor.go:14-130` | `Predictor{calendar, capitalFlow, narrativeModels}` + `Predict(now)` + `predictDay()` |
| `internal/eventdriven/predictor.go`（中段）| **`eventTypeToThemes` hard-coded map**（EventType → []TriggerTheme）— Stage 5 PR#3 主要改造對象 |
| `cmd/atlas-mcp/server/tools_events.go:17-41` | MCP tool `event_flow_prediction` 註冊 + handler |
| `internal/eventdriven/handler.go:62, 95` | HTTP endpoint `/api/events/prediction` + `HandlePrediction` |
| `cmd/atlas/main.go:90, 680` | `narrativeAdapter{eng: narrativeEngine}.ListModels()` 把 InvestmentModel 投影成 ModelView |

### 24 trigger themes 完整清單（從 templates.go 真實實讀）

| # | Trigger Theme | Template ID | 現有 Detector |
|---|---------------|-------------|---------------|
| 1 | `US_rates_up` | 美國升息 / 鷹派聯準會 | ✅ narrative_detectors |
| 2 | `JPY_carry_unwind` | 日圓套利平倉 | ✅ narrative_detectors |
| 3 | `AI_capex_surge` | AI 資本支出激增 | ✅ narrative_detectors |
| 4 | `geopolitical_risk_spike` | 地緣政治風險飆升 | ✅ narrative_detectors |
| 5 | `oil_price_shock` | 油價衝擊 | ✅ narrative_detectors |
| 6 | `spring_festival_season` | 春節行情 | ✅ seasonal |
| 7 | `election_cycle` | 選舉週期 | ✅ seasonal |
| 8 | `taiwan_political_risk` | 台灣地緣政治風險 | ✅ narrative_detectors |
| 9 | `USD_TWD_volatility` | USD/TWD 劇烈波動 | ✅ narrative_detectors |
| 10 | `semiconductor_downturn` | 半導體週期下行 | ✅ narrative_detectors |
| 11 | `retail_institutional_divergence` | 散戶機構分歧 | ✅ narrative_detectors |
| 12 | `gold_rally` | 黃金避險行情 | ✅ narrative_detectors |
| 13 | `dollar_surge` | 美元強勢 | ✅ narrative_detectors |
| 14 | `inflation_spike` | 通膨升溫 | ✅ narrative_detectors |
| 15 | `earnings_surprise` | 財報驚喜 | ✅ narrative_detectors |
| 16 | `earnings_blackout` | 財報空窗期 | ✅ seasonal |
| 17 | `tech_peak_season` | 科技旺季效應 | ✅ seasonal |
| 18 | `year_end_window_dressing` | 年底作帳行情 | ✅ seasonal |
| 19 | `US_rates_down` | 美國降息 / 鴿派聯準會 | ✅ narrative_detectors |
| 20 | `dividend_season` | 除權息旺季 | ✅ seasonal |
| 21 | `shipping_rate_spike` | 運價飆升 | ✅ narrative_detectors |
| 22 | `china_slowdown` | 中國經濟放緩 | ✅ narrative_detectors |
| 23 | `taiwan_export_boom` | 台灣出口強勁 | ✅ narrative_detectors |
| 24 | `tariff_shock` | 關稅衝擊 | ⚠️ **只有 ingestor-level，缺 KB pipeline detector**（鏈路不對稱） |

### 真實架構現實（與 Stage 1 規劃假設的落差）

1. **24 templates 與 12 InvestmentModel 是兩個獨立物件**：
   - `CausalTemplate` 沒有 `FavoredSectors/AvoidedSectors` 欄位
   - `KnowledgeBase.MatchChains(event)` 從 template 算出 `CausalChain`（內含 sector 分類）
   - 但 `ListModels()` 回的是 hard-code 的 12 個 InvestmentModel，不是從 template 即時匹配
2. **Detect 函式散落兩個檔案**：
   - `narrative_detectors.go` 處理 KB pipeline（30+ 個）
   - `ingestor.go` 處理 snapshot pipeline（15+ 個）
   - 沒有共同介面、無法統一啟用/停用
3. **EventType → TriggerTheme 對應寫死**：
   - `internal/eventdriven/predictor.go` 的 `eventTypeToThemes` map
   - 5 個 EventType（etf_rebalance / msci_rebalance / earnings_release / month_end / holiday）× 多個 TriggerTheme 的硬編碼
4. **Stage 1 規劃的 `internal/narrative/detector.go` 不存在**（commit grep 確認）

### 真正的缺口（Stage 5 必須補齊）

| # | 缺口 | 嚴重性 | 影響 |
|---|---|---|---|
| 1 | ❌ 缺獨立 `Detector` interface（Stage 1 PR#4 規劃） | 🔴 | 24 個 detect 函式各自獨立，無法統一管理 |
| 2 | ❌ 缺 `DetectorRegistry`（統一註冊/啟用/停用） | 🔴 | 無法實作「可單獨啟用/停用」需求 |
| 3 | ❌ `internal/narrative/detector.go` 不存在 | 🔴 | Stage 1 PR#4 規劃落空 |
| 4 | ⚠️ `tariff_shock` 缺 KB pipeline detector | 🟠 | 24 個 template 中只有 23 個有對稱的 detector |
| 5 | ❌ 缺 `template_detector_scan` 排程 | 🟠 | Stage 1 PR#4 規劃落空；trigger 偵測需手動呼叫 |
| 6 | ❌ EventType → TriggerTheme 寫死 | 🟠 | 新增 EventType 需動 eventdriven 程式碼 |
| 7 | ❌ 缺端到端 unit test | 🔴 | 沒有測試證明 detector→template→model→prediction 鏈路 |
| 8 | ❌ `docs/specs/eventdriven.md` 只有 30 行 | 🟡 | spec 與實作落差大 |

---

## 二、5-PR 拆分與依賴關係

```
PR#1 (P0)  Detector interface + Registry 抽象層
   │  internal/narrative/detector.go (新建)
   │  定義 Detector interface + DetectorRegistry + DetectorInput 統一型別
   │  → 不實作任何 detector impl（純抽象層）
   │  → 向後相容（既有 detect 函式繼續 work）
   ▼
PR#2 (P1)  24 個 Detector impl + tariff_shock 補齊
   │  internal/narrative/detector_impls.go (新建)
   │  把 narrative_detectors.go + ingestor.go 的 detect 函式包成 24 個 Detector struct
   │  → 補齊 tariff_shock KB pipeline detector
   │  → 透過 DetectorRegistry 註冊
   │  → 既有 detect 函式標 deprecated（但保留 backward-compat wrapper）
   ▼
PR#3 (P2)  EventType → TriggerTheme 動態對應
   │  internal/eventdriven/type_theme_mapping.go (新建)
   │  取代 hard-coded eventTypeToThemes
   │  → 透過 DetectorRegistry.ThemeOfTrigger() 查詢
   │  → 把 eventTypeToThemes 改為 lookup function（向後相容）
   ▼
PR#4 (P3)  template_detector_scan 排程 + MCP tool
   │  internal/scheduler/template_detector_scan.go (新建)
   │  透過 BackgroundTaskManager (遵守 apigateway/CONSTITUTION.md Art.4)
   │  → 每 1h 掃描所有啟用的 detector 並記錄 last_triggered_at
   │  → 新增 MCP tool: template_detector_status / detector_registry_list
   │  → 不發 bus event（避免污染；改為記錄到 SQLite 供查詢）
   ▼
PR#5 (P4)  端到端 unit test + spec 文件更新
   │  internal/narrative/detector_e2e_test.go (新建)
   │  → 每個 detector 獨立 test（24 個）
   │  → detector→template 鏈路 test
   │  → template→model→prediction 鏈路 test
   │  → e2e: synthetic MacroDataSnapshot → 24 detector 並發 → 觸發 template → 計算 prediction
   │  → docs/specs/eventdriven.md (30 行 → 完整 spec)
   │  → `2026-07-14-atlas-stage5-detector-plan.md`（本檔）
```

**依賴嚴格** — PR#2 需要 PR#1 的 Detector interface；PR#3 需要 PR#2 的 DetectorRegistry 已註冊 24 個 detector；PR#4 需要 PR#3 的 lookup function；PR#5 需要所有前 4 個 PR 完成才能寫 e2e test。

**PR#1 與 PR#2 必須向後相容** — 既有 `narrative_detectors.go` 與 `ingestor.go` 內的 detect 函式已被多處呼叫，重構為 Detector 介面不能 break 既有行為。

---

## 三、範圍決議（待用戶於 2026-07-14 確認）

### 決議 A：Detector 抽象邊界
- **選擇 A1**：Detector 只負責「從 data 偵測 trigger_theme」，不負責發 bus event、不負責寫 history
- **選擇 A2**：Detector 同時負責偵測 + 發 bus event + 寫 history
- **建議**：**選 A1** — 單一職責原則；發 event/寫 history 由 scheduler 統一處理

### 決議 B：EventType 與 TriggerTheme 的關係
- **選擇 B1**：保留兩個獨立概念；EventType 由 industry 模組管、TriggerTheme 由 narrative 模組管；中間用 lookup table 橋接
- **選擇 B2**：合併為單一概念；刪除 EventType，全部用 TriggerTheme
- **建議**：**選 B1** — Stage 4 已 ship 的 `prediction_backtest` 表結構依賴 EventType，合併會破壞向後相容

### 決議 C：tariff_shock 補齊方式
- **選擇 C1**：在 `narrative_detectors.go` 新增 `detectTariffShockKBEvent`，從 MarketNarrativeData.TradeNewsProvider 抓取
- **選擇 C2**：把 tariff_shock 從 templates.go 移除（不再視為 macro narrative 主題），改為 EventType（calendar event）
- **建議**：**選 C1** — tariff_shock 確實是 macro 主題（關稅政策影響外資流向），不應降級為 calendar event

---

## 四、各 PR 的根因解方（非補丁式）

### PR#1 (P0) — Detector interface + Registry 抽象層
**根因**：現有 50+ 個 detect 函式散落在 `narrative_detectors.go`（KB pipeline）與 `ingestor.go`（snapshot pipeline），沒有共同介面、無法統一管理。

**解方**：
1. **建立 `internal/narrative/detector.go`**（新建，Stage 1 PR#4 規劃的檔案）
2. **定義核心型別**：
   ```go
   // DetectorInput 統一輸入型別
   type DetectorInput struct {
       MacroSnapshot MacroDataSnapshot      // 來自 ingestor pipeline
       MarketData    MarketNarrativeData    // 來自 KB pipeline
       Calendar      []CalendarEvent        // 來自 eventdriven
   }

   // DetectionResult 統一輸出型別
   type DetectionResult struct {
       Theme       TriggerTheme  // 對應到 template 的 trigger_theme
       Severity    Severity      // critical / high / medium / low
       Confidence  float64       // 0-1
       DetectedAt  time.Time
       Source      string        // "snapshot" | "kb_pipeline"
       Metadata    map[string]any
   }

   // Detector interface
   type Detector interface {
       Theme() TriggerTheme
       Enabled() bool
       SetEnabled(bool)
       Detect(ctx context.Context, in DetectorInput) (*DetectionResult, error)
   }

   // DetectorRegistry 註冊表
   type DetectorRegistry struct {
       mu       sync.RWMutex
       detectors map[TriggerTheme]Detector
   }

   func (r *DetectorRegistry) Register(d Detector) error
   func (r *DetectorRegistry) Get(theme TriggerTheme) (Detector, bool)
   func (r *DetectorRegistry) List() []Detector
   func (r *DetectorRegistry) ListEnabled() []Detector
   func (r *DetectorRegistry) Enable(theme TriggerTheme) error
   func (r *DetectorRegistry) Disable(theme TriggerTheme) error
   func (r *DetectorRegistry) RunAll(ctx context.Context, in DetectorInput) ([]DetectionResult, error)
   ```
3. **預設行為**：`DetectorRegistry` 創建時不自動註冊任何 detector（純骨架）
4. **測試**：寫 `detector_test.go` 驗證 interface 契約（mock detector、nil handling、enable/disable）

**避免的補丁式**：不把 Detector interface 與現有 `KnowledgeBase.MatchChains()` 混在一起（前者負責 trigger_theme 偵測，後者負責 template→chain 匹配）。

### PR#2 (P1) — 24 個 Detector impl + tariff_shock 補齊
**根因**：Stage 1 PR#4 規劃了 detector，但當時只規劃了 1 個總體「template detector」；實際上 24 個 template 需要 24 個獨立 detector 才能達到「可單獨啟用/停用」需求。

**解方**：
1. **建立 `internal/narrative/detector_impls.go`**（新建，~700 LOC）
2. **24 個 Detector struct**（每個實作 Detector interface）：
   ```go
   type USRatesUpDetector struct { enabled bool }
   func (d *USRatesUpDetector) Theme() TriggerTheme { return "US_rates_up" }
   func (d *USRatesUpDetector) Enabled() bool { return d.enabled }
   func (d *USRatesUpDetector) SetEnabled(b bool) { d.enabled = b }
   func (d *USRatesUpDetector) Detect(ctx context.Context, in DetectorInput) (*DetectionResult, error) {
       // 從 in.MacroSnapshot.US10Y 判斷閾值（從 ParametersConfig 讀取）
       // 參考 narrative_detectors.go 的 detectUSRatesEvent 既有邏輯
   }
   // ... 23 個其他 detector
   ```
3. **tariff_shock 補齊**：
   - 新增 `detectTariffShockKBEvent(data MarketNarrativeData) *NarrativeEvent` 在 `narrative_detectors.go`
   - 從 `data.TradeNews` 或 `data.NewsProvider` 偵測關稅相關新聞關鍵字
   - 包裝成 `TariffShockDetector` struct 實作 Detector interface
4. **Backward-compat wrapper**：
   - `narrative_detectors.go` 與 `ingestor.go` 內的 50+ 個 detect 函式**保留**
   - 新增 `KBDetectorAdapter` 與 `IngestorDetectorAdapter` 兩個 wrapper struct
   - 讓既有呼叫點（`KnowledgeBase.MatchChains` 等）繼續 work
   - 但在 wrapper 內部統一呼叫 `DetectorRegistry.RunAll()`，避免雙路徑執行
5. **註冊到 Registry**：
   - `NewNarrativeEngine()` 內新增一行 `registry.RegisterAll(detectorImpls)`（預設 24 個全開）
6. **測試**：寫 `detector_impls_test.go`（24 個 test case）+ `tariff_shock_test.go`

**避免的補丁式**：不改 `narrative_detectors.go` 與 `ingestor.go` 內既有 detect 函式的 signature（保持向後相容）；只在 detector_impls.go 內新增 wrapper。

### PR#3 (P2) — EventType → TriggerTheme 動態對應
**根因**：`internal/eventdriven/predictor.go` 的 `eventTypeToThemes` 是 hard-coded map，新增 EventType 需動 eventdriven 程式碼；trigger_theme 新增也需同步兩處。

**解方**：
1. **建立 `internal/eventdriven/type_theme_mapping.go`**（新建）
2. **改寫 `eventTypeToThemes`**：
   ```go
   // 既有（hard-coded）：
   var eventTypeToThemes = map[string][]string{
       "etf_rebalance":     {"dividend_season", "tech_peak_season", ...},
       "msci_rebalance":    {"taiwan_export_boom", "USD_TWD_volatility", ...},
       ...
   }

   // 新版（透過 DetectorRegistry）：
   func ThemesForEventType(reg *narrative.DetectorRegistry, eventType string) []TriggerTheme {
       // 從 reg.List() 查詢每個 detector 的 Theme()，與 EventType 對應表比對
       // EventType → Theme 的對應表改為 lookup function 而非 hard-coded map
   }
   ```
3. **分離對應表**：把 `eventTypeToThemes` map 抽出到 `internal/eventdriven/type_theme_mapping_table.go`，加註解說明對應依據
4. **既有行為保留**：5 個 EventType 的對應結果與 Stage 4 完全一致（用 stage4_test fixture 驗證）
5. **測試**：寫 `type_theme_mapping_test.go` 驗證 5 個 EventType × 24 個 Theme 的對應正確

**避免的補丁式**：不把 EventType 直接合併到 TriggerTheme（決議 B 已說明）；保留兩個獨立概念，用 lookup function 橋接。

### PR#4 (P3) — template_detector_scan 排程 + MCP tool
**根因**：Stage 1 PR#4 規劃了 `template_detector_scan` 排程（每 1h 掃描 trigger 條件並發 bus event）但未實作；現在需要落地，且改為「記錄到 SQLite」而非「發 bus event」（避免污染）。

**解方**：
1. **建立 `internal/scheduler/template_detector_scan.go`**（新建）
2. **排程註冊**：
   ```go
   func RegisterTemplateDetectorScanTasks(btm *BackgroundTaskManager, registry *narrative.DetectorRegistry, store *ledger.DetectorScanStore) {
       btm.Register("template_detector_scan", "1h", func(ctx context.Context) error {
           in := narrative.DetectorInput{MacroSnapshot: latestSnapshot()}
           results, err := registry.RunAll(ctx, in)
           if err != nil { return err }
           return store.AppendScan(results)
       })
   }
   ```
3. **遵守 apigateway/CONSTITUTION.md Art.4**：所有排程必須透過 `BackgroundTaskManager`，不直接啟 goroutine
4. **SQLite store**：
   - 新增 `internal/ledger/detector_scan_store.go`
   - 1 個新 table `detector_scan_log`：`(scan_id, theme, severity, confidence, detected_at, source)`
   - 用既有 `data/state/atlas.db`（不開新檔案）
5. **MCP tools**（新增 2 個 read-only）：
   - `template_detector_status`：回傳最後一次 scan 的所有 theme 狀態
   - `detector_registry_list`：回傳當前 24 個 detector 的 enabled/disabled 狀態
6. **測試**：寫 `template_detector_scan_test.go` 驗證排程註冊 + scan 邏輯

**避免的補丁式**：不發 bus event（Stage 1 規劃的方案會污染 eventdriven/predictor.go 既有路徑）；改為 SQLite 查詢，遵循 Stage 4 的「離線分析用 SQLite」原則。

### PR#5 (P4) — 端到端 unit test + spec 文件更新
**根因**：Stage 1-4 累積的 detector→template→model→prediction 鏈路沒有 unit test 證明；spec 文件（`eventdriven.md`）只有 30 行，無法反映 24 templates + 12 models + DetectorRegistry 的複雜度。

**解方**：
1. **建立 `internal/narrative/detector_e2e_test.go`**（新建）
2. **24 個獨立 detector test**：
   - 每個 detector 一個 test case
   - 給定 synthetic MacroDataSnapshot + MarketNarrativeData + Calendar
   - 驗證 Detect() 回傳正確的 Theme + Severity + Confidence
3. **detector→template 鏈路 test**：
   - 給定 `DetectionResult{Theme: "US_rates_up"}`
   - 呼叫 `KnowledgeBase.MatchChains(event)`
   - 驗證回傳的 chain 包含「美國升息 / 鷹派聯準會」template
4. **template→model→prediction 鏈路 test**：
   - 給定 chain + InvestmentModel.ActiveThemes
   - 計算 weighted score
   - 驗證 `Predictor.predictDay()` 的 output 與 chain 一致
5. **e2e test**：
   - 給定 synthetic MacroDataSnapshot（含 US10Y 急升信號）
   - 並發執行 24 個 detector
   - 觸發 ≥ 1 個 template（US_rates_up）
   - 計算 prediction
   - 驗證 direction != neutral 且 confidence > 0.5
6. **Spec 文件更新**：
   - `docs/specs/eventdriven.md`：30 行 → 完整 spec（定義 24 templates + 12 models + DetectorRegistry + EventType 對應）
   - 引用本檔 `2026-07-14-atlas-stage5-detector-plan.md`

**避免的補丁式**：不只寫 1-2 個 happy-path test；必須 cover 24 detector × 多種 severity × enable/disable 切換。

---

## 五、每 PR 的執行流程（嚴格依照）

1. **跑 atlas-pre-change-protocol 8 步**（Step 0 重疊檢查 → Step 7 設計意圖）
2. **寫 code + 對應測試**（同 package `*_test.go`，coverage ≥ 60%）
3. **跑驗證清單**：
   ```bash
   test -z "$(gofmt -l .)"
   go vet ./...
   staticcheck ./...
   golangci-lint run --timeout=5m
   go test ./internal/narrative/... ./internal/eventdriven/... ./internal/scheduler/... ./internal/ledger/...
   go test -coverprofile=coverage.out ./internal/narrative/... ./internal/eventdriven/... && go tool cover -func=coverage.out | grep total  # ≥ 60%
   ```
4. **integration test via MCP**：重啟 server，呼叫新增的 MCP tools 驗證回傳值
5. **commit + push**（每 PR 一個 commit，commit message 寫根因）
6. **下一個 PR**

---

## 六、最終驗證清單（5 PR 全完成後跑）

### Detector 抽象層
- [ ] `internal/narrative/detector.go` 已建立
- [ ] `Detector` interface 定義完整（Theme/Enabled/SetEnabled/Detect）
- [ ] `DetectorRegistry` 支援 Register/Get/List/Enable/Disable/RunAll
- [ ] 24 個 detector struct 全部實作完成
- [ ] `tariff_shock` KB pipeline detector 已補齊
- [ ] 既有 `narrative_detectors.go` 與 `ingestor.go` 的 detect 函式**仍然可呼叫**（向後相容）

### 動態對應
- [ ] `eventTypeToThemes` 改為透過 `DetectorRegistry` 查詢
- [ ] 5 個 EventType × 24 個 Theme 對應表正確
- [ ] 既有 `Predictor.Predict()` 行為**完全不變**（用 stage4_test fixture 比對）

### 排程 + MCP
- [ ] `template_detector_scan` 排程已註冊到 `BackgroundTaskManager`
- [ ] `detector_scan_log` SQLite table 已建立
- [ ] MCP tool `template_detector_status` 可查詢
- [ ] MCP tool `detector_registry_list` 可查詢 24 個 detector 狀態

### 端到端測試
- [ ] 24 個 detector 獨立 unit test 全綠
- [ ] detector→template 鏈路 unit test 全綠
- [ ] template→model→prediction 鏈路 unit test 全綠
- [ ] e2e test（synthetic MacroDataSnapshot → prediction）全綠
- [ ] coverage ≥ 60%（narrative + eventdriven + scheduler + ledger）

### 工程品質
- [ ] `golangci-lint run --timeout=5m` 0 issues
- [ ] `staticcheck ./...` 0 issues
- [ ] `go vet ./...` 0 issues
- [ ] 既有測試全綠（不引入 regression）

### 文檔
- [ ] `docs/specs/eventdriven.md` 已擴展（30 行 → 完整 spec）
- [ ] `2026-07-14-atlas-stage5-detector-plan.md`（本檔）已建立
- [ ] `internal/narrative/AGENTS.md` 新增 Detector 抽象層 pitfalls
- [ ] `CHANGELOG.md` 新增 Stage 5 entries
- [ ] `TRAPS.md` 新增 Stage 5 任何 trap

---

## 七、嚴格遵守的紅線（不可違反）

1. ✅ 不准改既有的 24 個 template ID 字串（templates.go 的 TriggerTheme 值）
2. ✅ 不准改既有的 12 個 InvestmentModel 結構（knowledge_base.go）
3. ✅ 不准改既有 `narrative_detectors.go` 與 `ingestor.go` 的 detect 函式 signature（向後相容）
4. ✅ 不准在 production 直接動資料庫 — 所有 SQLite 寫入遵循 Stage 4 規範
5. ✅ 不准繞過品質檢查 — 既有 EventValidator 仍生效
6. ✅ 不准直接啟 goroutine — 所有排程走 BackgroundTaskManager（apigateway/CONSTITUTION.md Art.4）
7. ✅ 不准發 bus event 污染 eventdriven 既有路徑 — 改為 SQLite 查詢

**bonus 規範**：
- 不動 Atlas Postgres（用 SQLite 只新增 1 個 table）
- 不動既有 MCP tool 介面（除新增的 2 個 read tools）
- 不動既有 recommendation_outcomes.jsonl schema
- 不動既有 audit log schema

---

## 八、不做的事（Out of scope）

明確不做，避免 scope creep：
- ❌ 重建 24 templates（Stage 4 已 ship；本檔不重做）
- ❌ 重建 12 InvestmentModel（已 ship）
- ❌ 改既有 `Predictor.Predict()` forward-looking 行為
- ❌ 改既有 `eventTypeToThemes` 對應結果（只改實作方式，不改對應表本身）
- ❌ 把 EventType 合併到 TriggerTheme（決議 B 已說明）
- ❌ 自動 debias darwinian weight（Stage 4 已決定不處理）
- ❌ 從真實 90 天資料跑 backfill（Stage 4 已 ship）
- ❌ 即時 webhook / 即時 trigger（Stage 5 只做 polling 排程）

---

## 九、rollback 策略

每個 PR 都可以 `git revert <sha>` 撤銷：
- **PR#1**：刪除 `internal/narrative/detector.go`（既有程式不受影響，因為不刪任何既有檔案）
- **PR#2**：刪除 `internal/narrative/detector_impls.go` + 從 `NewNarrativeEngine()` 移除 `registry.RegisterAll`
- **PR#3**：把 `ThemesForEventType` function 換回 `eventTypeToThemes` map
- **PR#4**：刪除 `internal/scheduler/template_detector_scan.go` + DROP TABLE `detector_scan_log`
- **PR#5**：刪除 e2e test file + spec 文件還原

**關鍵**：PR#2 一定要把 `RegisterAll` 做成**可選呼叫**（透過參數或環境變數），預設開啟（向後相容），這樣 rollback 時只需從 main.go 移除呼叫即可。

---

## 十、決策檔輸出

每 PR 結束時在 `~/workspace/atlas-notes/decisions/` 開對應檔案：
- `2026-07-14-stage-5.1-detector-interface.md`
- `2026-07-14-stage-5.2-24-detector-impls.md`
- `2026-07-14-stage-5.3-type-theme-mapping.md`
- `2026-07-14-stage-5.4-detector-scan-scheduler.md`
- `2026-07-14-stage-5.5-e2e-test-spec.md`

每檔結構：
1. Stage / PR 編號
2. 改動清單（新增 / 修改 / 刪除 檔案）
3. 測試結果（覆蓋率、golangci-lint、go vet）
4. 已知待辦 / follow-up
5. 驗證指令（MCP 查詢 + SQLite 查詢 sample）

---

*建立時間：2026-07-14 (Asia/Taipei)*
*分支：`feat/stage-5-trigger-detector`（基於 `pr-1140` @ `61248639`）*
*負責人：kaecer + opencode Sisyphus*
*基礎 commit：61248639（fix(backtest): resolve 2 golangci-lint issues on PR #1140）*
*真實盤查來源：3 個 explore agents（eventdriven / narrative templates / MCP tool chain）*

---

## 十一、執行狀態（2026-07-14 更新）

### 已 ship commits

| Commit | PR# | 內容 |
|---|---|---|
| `9ec34b10` | PR#1 | feat(narrative): template trigger Detector interface + Registry — `detector.go` + `detector_test.go` + field_types.ts regenerated |
| `41461db6` | PR#2 | feat(narrative): 24 trigger theme Detector impls + `NewDefaultDetectorRegistry()` — `detector_impls.go` + 23 tests |
| `f7d9552f` | PR#3 | feat(eventdriven): `EventTypeToTriggerThemes` 動態對應 — 8 tests，無 regression |
| `17f9b6d0` | PR#4A | feat(stage5): detector_scan_log store + template_detector_scan scheduler — 9 store + 6 scheduler tests |
| （待 commit） | PR#5 | detector_e2e_test.go（4 e2e tests）+ internal/narrative/AGENTS.md + docs/specs/eventdriven.md 擴充 |

### 已驗證的 e2e 鏈路

```
DetectorRegistry.RunAll(ctx, DetectorInput{MarketNarrativeData})
  → 17 KB-pipeline detectors fire (US_rates_up / JPY_carry_unwind / oil_price_shock / etc.)
DetectorRegistry.RunAll(ctx, DetectorInput{MacroDataSnapshot})
  → 1 snapshot-pipeline detector fires (tariff_shock)
DetectorRegistry.RunAll
  → []DetectionResult → ledger.SQLiteDetectorScanStore.AppendScan → SQLite detector_scan_log
→ LoadRecentScans → JSON via MCP tool (deferred)
```

### Deferred 到 follow-up PR（原 Stage 5 PR#4 Stage B）

| 項目 | 範圍 | 阻擋原因 |
|---|---|---|
| `cmd/atlas/...` 2 個 HTTP endpoint | `/api/detector/scan/status` + `/api/detector/registry/list` | 需要擴展 `narrativeAdapter`、wire registry/scanStore 到 main.go、wiring 侵入性大 |
| `cmd/atlas-mcp/server/tools_template_detector.go` | 2 個 MCP tool + handler | 需先有 HTTP endpoint |
| `server.go:174` tool count hard gate | 106-108 → 108-110 | 同上 |
| `tools_transport_sse_test.go` tool count | 106-108 → 108-110 | 同上 |
| `docs/reference/tool-catalog.md` | 新增 2 個 tool 條目 | 同上 |
| `go generate ./cmd/atlas-mcp` | 重產 auto-desc.gen.json | 同上 |
| `cmd/atlas/main.go` 註冊 `RegisterTemplateDetectorScanTasks` | wiring | 同上 |

預期 follow-up PR scope：6 檔案 + 1 command，30 分鐘可完成。

### 驗證指令

```bash
# PR#1+PR#2 既有測試
go test -count=1 ./internal/narrative/

# PR#3 既有測試
go test -count=1 ./internal/eventdriven/

# PR#4 Stage A 既有測試
go test -count=1 ./internal/ledger/ ./internal/scheduler/

# PR#5 e2e chain test
go test -count=1 -v -run TestE2E ./internal/narrative/

# 全套（無 MCP tool、無 cmd/atlas wiring）
go test -count=1 ./internal/narrative/... ./internal/eventdriven/... ./internal/ledger/... ./internal/scheduler/...
```
