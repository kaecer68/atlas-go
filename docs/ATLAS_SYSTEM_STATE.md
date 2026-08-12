# ATLAS 系統狀態快照

> **最後更新**: 2026-07-31（公股節律修復：BTM 24h + CAPTCHA 24h cooldown）
> **本版追加**: 2026-08-07（新增附錄：55 個 7/30 → 8/7 期間合併 PR 摘要），對應 HEAD `0f8667a6`
> 維護紀律：每次 feature wave 合併後更新，維持現狀可追蹤性。

## 活躍工作區

| 工作區 | Branch | 狀態 | 說明 |
|--------|--------|------|------|

| `~/workspace/atlas` | `main` | 🟢 基準 | 主工作區 |
| `~/workspace/atlas/feat-20260730-b3-data-infra` | `feat/20260730-b3-data-infra` | 🟡 進行中 | B5-3 PR-A：公股資料基礎設施（parser/8 行庫/per-broker/sector reader），未合併 |
| `~/workspace/atlas/MoneyTrend-B5-Batch-3` | `kaecer68/MoneyTrend-B5-Batch-3` | ⚪ 待清理 | B5-3 偵察工作區 |
| `~/workspace/atlas/MoneyTrend-B5-Batch-2` | `kaecer68/MoneyTrend-B5-Batch-2` | ⚪ 待清理 | B5-2 已完成合併 |
| `~/workspace/atlas-taiex-backfill` | `fix/20260729-taiex-backfill` | ⚪ 待清理 | B5-T TAIEX 快照回補 |
| `~/workspace/atlas/MoneyTrend-TAIEX-Fix` | `fix/20260730-taiex-resilience` | 🟢 已完成 | B5-R TAIEX 抓取韌性修復已合併 |

## Feature Wave 進度

| Wave | 描述 | 狀態 | PR | 合併日期 |
|------|------|------|-----|----------|
| E4 | 方法論：七時期 UI + 因果傳導鏈頁面 | ✅ 已完成 | #1397, #1398 | 2026-07-27 |
| E5a | 策略三分類（防禦/攻擊/戰術） | ✅ 已完成 | #1404 | 2026-07-28 |
| E6a | 首頁去重與資訊架構重整 | ✅ 已完成 | #1401 | 2026-07-27 |
| E6b | 趨勢表達（七時期走勢軸 + sparkline + chip） | ✅ 已完成 | #1408 | 2026-07-28 |
| B1+B3 | 後端 API 補完（period_history + CapitalSection 數值化 + 日曆退役） | ✅ 已完成 | #1406 | 2026-07-28 |
| B4b | 成交量資料源接入（market_volume channel） | ✅ 已完成 | #1405 | 2026-07-28 |
| B4c | Margin 資料鏈修復（channel 接通 + 繼承補齊） | ✅ 已完成 | #1407 | 2026-07-28 |
| C1 | 部署完整性閘門（binary/source sync + version stamp + CI gate） | ✅ 已完成 | #1412 | 2026-07-29 |
| P0 | 請求路徑快取層（MacroSnapshot/Summary/Prediction/Bundle/Recommendations） | ✅ 已完成 | — | 2026-07-29 |
| Warmup | 開機熱機（五層 cache 暖場消滅冷路徑） | ✅ 已完成 | #1411 | 2026-07-29 |
| B2 | 時期信心度 + 觸發指標（Period Assessment 可解釋性） | ✅ 已完成 | #1413 | 2026-07-29 |
| Docs | ARCHITECTURE.md 架構活地圖（38 channel + 資料流） | ✅ 已完成 | #1414 | 2026-07-29 |
| BG | Background 首次執行跳過 startup jitter | ✅ 已完成 | #1409 | 2026-07-28 |
| B5-1 | PeriodIndicators Batch 1 — 指數/匯率/量能均線補齊 | ✅ 已完成 | #1415 | 2026-07-29 |
| B5-2 | PeriodIndicators Batch 2 — 法人/期貨/融資 chip 統計 | ✅ 已完成 | #1416 | 2026-07-29 |
| B5-3 PR-A | PeriodIndicators Batch 3 資料基礎設施（公股 parser/8 行庫/per-broker/sector reader） | ✅ 已完成 | #1421 | 2026-07-29 |
| B5-3 PR-B | PeriodIndicators Batch 3 calculator 填充（類股輪動 + 公股連買） | ✅ 已完成 | #1422 | 2026-07-31 |
| B5-T | TAIEX 7/24、7/27 快照回補（TWSE 官方源） | ✅ 已完成 | — | 2026-07-29 |
| B5-R | TAIEX 抓取韌性修復（Yahoo → TWSE fallback、失敗可見化、陳舊快取誠實化） | ✅ 已完成 | — | 2026-07-30 |
| 公股節律 | government_flow 28h→24h + weekday 15:00+ gate + CAPTCHA 24h cooldown | ✅ 已完成 | — | 2026-07-31 |
| PRISM 停用 | admin 頁面 + /api/prism/training-results + MCP tool 移除；prism_training BTM disabled（#1527） | ✅ 已完成（停用非刪核心） | #1527 | 2026-08-12 |

## E4 — 方法論頁面（已完成）

- **PR #1397**: `feat(methodology): add 七時期 UI + 因果傳導鏈頁面 (E4)`
- **PR #1398**: `fix(methodology): E4 頁面審查修復`
- **後端**：`internal/methodology/`、`internal/config/methodology_config.go`
- **前端**：`client_web/static/js/page-shells/methodology.js`

## E5a — 策略三分類（已完成）

- **PR #1404**
- **涉及檔案**：`internal/config/methodology_config.go`、`internal/methodology/advisor.go`、`cmd/atlas/main.go`

## E6a — 首頁去重（已完成）

- **PR #1401**
- **涵蓋**：首頁去重（-116 行）、側邊欄修正、route-deep-link 測試更新

## E6b — 趨勢表達（已完成）

- **PR #1408**
- **涵蓋**：前端-only 7 時期走勢軸、sparkline 視覺化、首頁時期 chip

## B1+B3 — 後端 API 補完（已完成）

- **PR #1406**（合併日期：2026-07-28）
- **涵蓋**：period_history 表、CapitalSection 雙軌數值化、日曆 308 redirect、MarginMaintenanceRatio MarshalJSON
- **已知限制**：period_history 僅自上線後寫入；事件日曆 P2 cleanup 留後續

## B4b — 成交量資料源（已完成）

- **PR #1405**
- **目標**：TWSE MI_INDEX → `MacroDataSnapshot.MarketVolume`

## B4c — Margin 資料鏈修復（已完成）

- **PR #1407**: channel 接通 + 繼承補齊 + 靜默失敗補 log

## C1 — 部署完整性閘門（已完成）

- **PR #1412**
- **涵蓋**：binary buildinfo ldflags、`make check-binaries`、`make ci-gate`/`ci-full` 標準化、VERSION stamp

## Warmup — 開機熱機（已完成）

- **PR #1411**
- **涵蓋**：goroutine-based 5 層 cache warmup、startup 後台異步執行

## B2 — 時期信心度 + 觸發指標（已完成）

- **PR #1413**:  `feat: B2 時期信心度 + 觸發指標 (Period Assessment 可解釋性)`
- **涵蓋**：
  - 每時期信心度（Formula A: hit/total）+ 觸發指標面板
  - 新增 `TriggeredIndicator`、`PeriodAssessment`、七組 `assessXxx`（鏡像 `isXxx`，邏輯零改動）
  - `DetectAssessment()`、`DetectPeriod` 向前相容委派
  - 前端 `methodology.js` 信心度 + `renderIndicatorPanel()`
  - 黃金測試：9 組零 diff；`isXxx` 一字未改（15 檔案）

## Docs — ARCHITECTURE.md（已完成）

- **PR #1414**
- **涵蓋**：38 channel 盤點、完整資料流圖、寫入路徑、前端架構、化石區

## B5-1 — PeriodIndicators Batch 1（已完成）

- **PR #1415**（合併日期：2026-07-29）
- **分支**：`fix/20260729-b5-batch1-ma-fill`
- **涵蓋**：
  - 新建 `PeriodIndicatorsCalculator`（11 計算函式，附 minDays 誠實降級）
  - 19 個單元測試、8 個降級邊界測試、黃金測試（84 snapshot 回測）
  - Dashboard API adapter 接入（`persistPeriodHistory` + `RegisterRoutes`）
  - 2026-07-28 黑天鵝事件正確改判（TAIEXMA20 由零 → 可用，consolidation → black_swan）
  - 偵測邏輯（`period_detector.go`）零改動

## B5-2 — PeriodIndicators Batch 2（已完成）

- **PR #1416**（合併日期：2026-07-29）
- **分支**：`fix/20260729-b5-batch2-chipstats-fill`
- **涵蓋**：
  - W1: `input_available` 語義升級（窗口統計改為有效非零點數守門，附誠實降級）
  - W2: 外資多日統計填充（7 欄位：Net5DayAvg, Net10DayAvg, NetPeakSell, BuyDays10, SellDays10, ConsecBuyDays, ConsecSellDays）
  - W3: 期貨統計填充（2 欄位：FuturesOIPrev, FuturesOIDelta3）
  - W4: 融資統計填充（2 欄位：MarginBalancePeak, MarginBalanceChange5D）
  - W5: 黃金測試（84 snapshot 回測，b5-batch2-backtest.txt）
  - 35 個單元測試（涵蓋 Batch 1+2 正常/降級/確定性）
  - `period_detector.go` / `period_history` / ledger 寫入路徑零改動

### 歸因（與 period_history DB 比較）
| 類別 | 數量 | 說明 |
|------|------|------|
| (a) B5-1 only | 2 | 7/28→turnaround_down, 7/29→turnaround_down |
| (b) W1 degradation | 6 | 7/24-7/29 TAIEXMA20 非零→零（誠實降級） |
| (c) B5-2 only | 0 | 無新增改判 |
| (d) W1+B5-2 combined | 2 | 7/24 與 7/27 W1 input 降級 + B5-2 chip 統計添加後，2 筆額外改判（7/24 與 7/27 由 B5-1 W1 缺失 → B5-2 補 chip 後觸發額外差異） |

## B5-T — TAIEX 7/24、7/27 快照回補（已完成）

- **PR #1417**（合併日期：2026-07-29）
- **分支**：`fix/20260729-taiex-backfill`（worktree：`~/workspace/atlas-taiex-backfill`）
- **根因**：Yahoo Finance `^TWII` 在 7/24 與 7/27 兩天 fetch 間歇性失敗，造成 `MacroDataSnapshot` 中 `taiex` 鍵整欄缺失（其他欄位如 `foreign_investor_net`、`retail_margin_balance` 仍正常寫入）。同期 `historical_volatility` 雖有值但為共用 `^TWII` 快取，已在另一個問題上跟進。
- **回補來源**：TWSE OpenAPI `https://www.twse.com.tw/exchangeReport/MI_INDEX?response=json&date=YYYYMMDD&type=IND`（即時拉取，無硬編碼）
- **回補工具**：`cmd/macrobackfill/`（獨立 commit，11 個單元測試全綠）
  - 拒絕覆寫既有 `taiex` 鍵
  - 拒絕週末日期
  - 從最接近的前一交易日計算 `change_pct` 基準
  - 每筆回補以 JSONL 寫入 `<dir>/backfill_log.jsonl`（欄位：`date` / `field` / `value` / `change_pct` / `source_url` / `source_fetched_at` / `backfilled_at` / `baseline_date` / `baseline_value`）
- **回補結果**（gitignored 資料檔，已在 main repo 落盤並由工具驗證）：
  - `2026-07-24` TAIEX = 43654.84，change_pct = -2.6122%（基準 2026-07-23=44825.78）
  - `2026-07-27` TAIEX = 43634.19，change_pct = -0.0473%（基準 2026-07-26=43654.84）
- **驗證**：
  - 黃金測試重跑（`go test -tags=golden -run TestGolden_BacktestAllDates`）→ 84 snapshot，1 筆改判
  - **2026-07-28：consolidation → black_swan**（TAIEX 偏離 MA20 -5.93% < -5%）
  - 5 個 black_swan 條件逐條比對，僅「大盤偏離月線跌幅」一條命中
- **回補日誌位置**：`data/state/macro/backfill_log.jsonl`（gitignored，依部署環境落盤）
- **範圍外（未動）**：
  - `historical_volatility` 7/27 過時值 43654.84（屬另一個快取陳舊問題，進 backlog）
  - `latest.json` / `previous.json`
  - `market_volume` 歷史（屬 Batch 後續決定）
  - `MacroDataPoint` schema
  - `period_history` 寫入與 `period_detector.go` 零改動

## B5-R — TAIEX 抓取韌性修復（已完成）

- **分支**：`fix/20260730-taiex-resilience`（worktree：`~/workspace/atlas/MoneyTrend-TAIEX-Fix`）
- **根因**：Yahoo Finance `^TWII` 間歇性失敗時，`TAIEXIndexProvider` 無替代來源，導致 `taiex` 鍵整欄缺失；`TaiwanVolatilityProvider` 與 `TAIEXIndexProvider` 共用 `twiiCache`，Yahoo 失敗後 cache 中的陳舊值被用來計算 `historical_volatility`，產生「有值但已過時」的靜默污染。
- **修復內容**：
  - **W1（TAIEX fallback）**：`TAIEXIndexProvider.FetchSnapshot` 在 Yahoo 失敗（網路、非 2xx、空 chart、無有效 TAIEX 列）時，自動 fallback 至 TWSE OpenAPI `MI_INDEX?type=IND`；回應日期與請求日期不符（含週末/休市）時拒絕寫入。
  - **W2（失敗可見化）**：`macroDataGatewayAdapter.fetchFresh` 在合併 snapshot 時，將 hard-failed channels 寫入 `FailedChannels`、stale-only channels 寫入 `StaleChannels`，並更新頂層 `DataStatus`（`ok`/`degraded`/`stale`）。
  - **W3（陳舊快取誠實化）**：`TaiwanVolatilityProvider` 在 cache hit 時檢查 `^TWII` 資料時間戳是否為當前交易日；若為陳舊快取則回傳 error，拒絕以舊值計算 20 日年化波動率。
  - **W4（測試）**：新增 7 個單元測試，涵蓋 TWSE fallback 成功、日期不符拒絕、雙路皆敗、陳舊快取拒絕、新鮮快取接受、snapshot `DataStatus=degraded/stale`。
- **涉及檔案**：
  - `internal/marketdata/taiex_twse_fallback.go`（新增）
  - `internal/marketdata/taiex_index_provider.go`
  - `internal/marketdata/taiwan_index_cache.go`
  - `internal/marketdata/taiwan_volatility_provider.go`
  - `internal/marketdata/taiex_index_provider_test.go`
  - `internal/marketdata/taiwan_volatility_provider_test.go`（新增）
  - `internal/monitoring/gateway_adapter.go`
  - `internal/monitoring/gateway_adapter_test.go`
  - `docs/data-architecture.md`、`docs/architecture.md`（本文件）
- **不動範圍**：`period_detector.go`、所有 detector / calculator / period_history 寫入路徑、`MacroDataPoint` schema、既有 snapshot 歷史檔。
- **預期影響**：日後 Yahoo 間歇性失敗時，`taiex` 鍵由官方 TWSE 回填；若兩路皆敗，`DataStatus` 與 `FailedChannels` 會如實反映，不會再出現「有值但非當日」的靜默錯誤。

---

## B5-3 PR-A — PeriodIndicators Batch 3 資料基礎設施（已完成）

- **分支**：`feat/20260730-b3-data-infra`（worktree：`~/workspace/atlas/feat-20260730-b3-data-infra`）
- **目標**：修復公股資料取得、擴充行庫名單至 8 家、新增 per-broker 明細輸出、提供 sector_index 公共讀取器。本 PR-A **不改變任何時期判定**（detector/calculator/struct 零改動）。
- **W1（公股 parser 修復）**：`GovernmentBrokerAggregator` 改走 TWSE `bsMenu.aspx` ASP.NET 表單流程（GET 取 token → POST 查詢），並偵測 CAPTCHA 頁面，遇 captcha 時回傳錯誤而非靜默寫入 `total_net=0`。
- **W2（行庫名單對齊）**：`coreBankBranches` 由 5 家擴充為 8 家：合庫(8060)、土銀(8030)、臺灣銀(8040)、台企銀(8010)、彰化(8064)、兆豐(8061)、第一金(8011)、華南永昌(8080)。
- **W3（per-broker 明細）**：`AggregateDate` 在寫入 `YYYYMMDD.json` 與 `YYYYMMDD_insurance.json` 之外，新增 `YYYYMMDD_brokers.json`，含各券商當日買/賣/淨買。
- **W4（sector_index 公共讀取器）**：新增 `SectorIndexReader`（`internal/marketdata/sector_index_reader.go`），統一讀取 `data/state/sector_index/` 下的單日/批次檔，將 8 產業與 18 產業 schema 映射為 canonical 18 產業 return map，缺檔日期（如 2026-06-24）留空、不零填充。
- **涉及檔案**：
  - `internal/marketdata/government_broker_aggregator.go`（重寫 parser/8 行庫/per-broker）
  - `internal/marketdata/government_broker_aggregator_test.go`（新增）
  - `internal/apigateway/adapter_government_broker_test.go`（stub 改為 POST 流程）
  - `internal/marketdata/sector_index_reader.go`（新增）
  - `internal/marketdata/sector_index_reader_test.go`（新增）
  - `docs/data-sources.md`、`docs/data-architecture.md`（本文件）
- **不動範圍**：`period_detector.go`、`period_calculator.go`、`PeriodIndicators` struct、`period_history` 寫入路徑、既有 6 個全零 `government_flow` 歷史檔。
- **已知限制**：TWSE `bsr.twse.com.tw` 目前已啟用 CAPTCHA，自動抓取將回傳錯誤；需等待官方移除 CAPTCHA、導入官方 API，或經人工/授權管道匯入資料後，parser 才能產出非零資料。

---

## 公股節律修復（已完成）

- **分支**：`fix/20260731-govflow-cadence`（worktree：`~/workspace/atlas-govflow-cadence`）
- **根因**：PR #1421 已讓公股通道誠實報錯（靜默寫零 → 明確 CAPTCHA 錯誤），但 BTM `government_flow_aggregate` 仍以 28h 週期抓取 TWSE bsr.twse.com.tw CAPTCHA 頁面；高頻輪詢等於邀請封鎖，阻斷公股欄位復活路徑。
- **修復內容**：
  - **W1（抓取節律下調）**：
    - BTM `government_flow_aggregate`（`cmd/atlas/operations_tasks.go`）：28h → **24h**，加上 weekday 15:00+ Taipei gate（沿用 `auto_taifex_institutional` 模式，TWSE 收盤 13:30 + 2h 結算）。每交易日收盤後最多 1 次。
    - Gateway `auto_government_flow`（`cmd/atlas/capital_tasks.go`）：1h → **24h**，加上相同 weekday 15:00+ gate。1h tick 過去純屬浪費（底層 state 檔每天只變一次）；現在跟 BTM 路徑同節律。
    - **兩條路徑維持分工**，不整併：BTM 是 upstream fetcher（會打 TWSE），gateway 是 local state 重新讀取（無 upstream）。節律對齊 = 兩者同日同一窗口運作。
  - **W2（CAPTCHA 退避）**：
    - 新增 `internal/marketdata/captcha_cooldown.go`：`CaptchaCooldown` 結構，per-channel 進程內記憶體、預設 24h cooldown。process restart = reset（刻意：保護對象是上游的 bot 偵測視角，不是本機 retry 歷史）。
    - 新增 `marketdata.ErrCaptchaRequired` typed sentinel，aggregator 內 2 個 `captcha required` 錯誤路徑 wrap 此 sentinel（`fmt.Errorf("%w ...", ErrCaptchaRequired, ...)`），`errors.Is` 為唯一偵測介面。
    - BTM task body 接入：fetch 前 `cd.ShouldSkip` 查 cooldown、fetch 失敗時 `IsCaptchaErr` 觸發 `RecordCaptcha`、成功時 `RecordSuccess` 立即清 cooldown。Gateway 路徑不接 cooldown（無 upstream 觸點）。
  - **W3（測試 + 文件）**：
    - 7 個 `CaptchaCooldown` 單元測試（含**連續 CAPTCHA → 跳過後續嘗試**案例、RecordSuccess 立即清除、ChannelIsolation、NilSafe、IsCaptchaErr 對 wrapped sentinel 匹配、-race 併發安全）。
    - 4 個 `registerOperationsTasks` 整合測試（cadence 24h、CAPTCHA cooldown 跳過 fetch、連續 CAPTCHA、weekday gate）。
    - `docs/data-architecture.md` 同步更新。
- **涉及檔案**：
  - `cmd/atlas/operations_tasks.go`（BTM 24h + 5 種 gate/captcha 邏輯；operationsDeps 新增 `captchaCooldown` 欄位）
  - `cmd/atlas/capital_tasks.go`（gateway 1h → 24h + weekday gate；file header 任務數 7→8）
  - `cmd/atlas/main.go`（注入 `marketdata.NewCaptchaCooldown()` 單例）
  - `cmd/atlas/operations_tasks_test.go`（新增 helper + 4 個 W1/W2 測試）
  - `internal/marketdata/government_broker_aggregator.go`（`ErrCaptchaRequired` sentinel + 2 處錯誤 wrap）
  - `internal/marketdata/captcha_cooldown.go`（新檔，CaptchaCooldown 實作）
  - `internal/marketdata/captcha_cooldown_test.go`（新檔，7 個單元測試）
  - `docs/data-architecture.md`（表格更新）
- **不動範圍**：`period_detector.go`、`period_calculator.go`、`PeriodIndicators` struct、`period_history` 寫入路徑、所有 `YYYYMMDD.json` / `_brokers.json` / `_insurance.json` 資料檔格式、`MacroDataPoint` schema。
- **預期影響**：
  - 短期（CAPTCHA 仍啟用）：BTM 每日 1 次 + 24h cooldown → 上游 24h 內最多收到 1 個請求，給 CAPTCHA 機制「冷卻」機會。
  - 長期（CAPTCHA 解封）：cooldown 從未被觸發，行為等同每日 1 次穩定抓取；公股欄位 `PublicBankConsecBuyDays` 開始有真實資料。
  - 與 PR #1422 黃金測試零差異：BTM 排程改變不影響 `period_calculator` / `period_detector` 邏輯；calculator 端拿到的 `PublicBankConsecBuyDays` 仍依既有 0/84 顯示（CAPTCHA 解封前本就無法產出有效資料日）。

---



- **PR #1422**（合併日期：2026-07-31）
- **前導 PR-A（#1421）**：SectorIndexReader、GovernmentBrokerAggregator（per-broker 輸出）
- **本支範圍**：把 sector_index（去程）與 government_flow（回程）兩路資料接入 calculator，產生 SectorRotationFlag 與 PublicBankConsecBuyDays 兩欄位。
- **W1（來源優先序）**：`SectorIndexReader.ReadRange()` 改寫為兩段掃描—載入時依 18 產業 batch 檔的原生 key 數≥18 分為 native/legacy；Phase 1 填 native 純覆蓋，Phase 2 僅補 native 未覆蓋的 (date,industry) 組合。解決 PR-A 遺留的 cross-schema averaging 問題（7/1 electronics 從跨檔平均 3.55 還原為 18 產業原生 3.15）。
- **W2（EnrichSectorRotation）**：用 SectorIndexReader 取近 5 日 vs 前 5 日 industry 累積 return 的 top 3，比較兩 window 集合是否相同（同→false；不同→true，表示類股輪動）。MinDays=10。
- **W3（EnrichGovernmentBroker）**：P0-3 規則—`YYYYMMDD.json` 必須有同名 `_brokers.json` 才算有效資料日，否則排除。legacy 零檔全部被排除，預期 0 個有效日直到 CAPTCHA 解決。MinDays=5。
- **W4（dashboard API 掛載）**：`dashboard_api.go` 兩處（`persistPeriodHistory` 中呼叫 `EnrichBatch3`；`PeriodProvider` 路由 handler 中掛載）。`executor_pipeline.go` 不接（無 ExecutionContext sector/gov 注入機制，進 backlog）。
- **P0-1 可用性銜接（honest degradation）**：`SectorRotationFlag` 僅在≥10 個 sector index 交易日才計算，`PublicBankConsecBuyDays` 僅在≥5 個政府資金資料日才計算。不足時欄位=0（=unavailable per detector contract）。
- **P0-2 schema 策略（18 產業優先）**：18 產業 batch 檔（key≥18）為 native source，8 產業單日檔為 legacy fallback。8→18 映射規則：ai_supply_chain→electronics, robotics→machinery, 其餘 6 個 1:1。reader 保留原有平均邏輯（僅用於同一檔案內多筆 entry）。
- **P0-3 資料日有效性**：`_brokers.json` 存在才認證。6 個 legacy 零政府資金檔（無 _brokers.json）被排除。
- **黃金測試結果**：84 dates / 1 changed / 82 unknown / 1 unchanged。唯一改判 7/28 consolidation→black_swan（TAIEX=41603.36 vs MA20=44167.17，偏離 -5.93% < -5% threshold），與 B5-T 同筆。SectorRotationFlag 對 7/28 改判無貢獻（black_swan 只由 twii_crash 條件 1-5 決定，SRot 僅用於其它 5 時期判定）。
- **新測試 15 個**：
  - sector rotation: 正常 6 個（不含輪動、含輪動、資料缺口、空目錄、空字串、確定性測試）
  - gov broker: 5 個（legacy 零檔、連買、中斷、不足、空目錄）
  - availability semantic: 2 個（sector rotation 不足、gov broker 不足→ zero 代表 unavailable）
  - W1: 2 個 (source priority 18 wins / no average)
- **不動範圍**：`period_detector.go`、`PeriodIndicators` struct、`period_history` 寫入路徑、legacy 零政府資金檔。


---

## Hermes MCP — 對外 period 欄位接源修復（已完成）

- **分支**：`fix/20260730-mcp-period-source-of-truth`
- **根因**：`buildRegimeHistoryData()`（`internal/monitoring/service/pipeline.go`）在計算對外 `period` 欄位時，並未讀取 `period_history`（PeriodDetector 真值），而是以三態 `regime` 為輸入呼叫 `methodology.RegimeToPeriod()` 反向推導：`RISK_ON → bull`、`RISK_OFF → downturn`、`NEUTRAL → consolidation`。但同一週期內七時期分類可為 `bull / turnaround_up / plateau / black_swan / consolidation` 等多種真值，反向推導必然失真。
  - 典型受污染日：**2026-07-29** regime_history=`RISK_ON`（推導 `bull`） vs period_history=`consolidation`（真值）。外部 hermes agent 依「period=bull」做決策即誤判。
- **修復內容**：
  - **W1（接源修復）**：`buildRegimeHistoryData()` 的 `period` / `period_name_zh` / `current_period` 改讀 `period_history`（與 `market_period` 同源），不再用 `RegimeToPeriod()` 推導。
  - **W2（缺值誠實）**：period_history 缺該日資料時，`period` 留空（`omitempty`），**不** fallback 回 `RegimeToPeriod` 推導（那就是假資料）。`regime` 欄位語意不變（三態向下相容層，下游在消費）。
  - **W3（source 語意拆開）**：原本單一 `Source` 欄位永遠是 `macro_ingest`（regime_history 寫入端），語意只涵蓋 regime。改源後拆成 `regime_source`（regime_history row source）+ `period_source`（`period_history` 或空），外部 agent 不再誤判。
  - **W4（向下相容）**：`market_period` 保留為 `period` 的 deprecated alias，兩值同源。新對外回應同時帶三個欄位：`period`（新真值）+ `market_period`（deprecated alias）+ `period_source`（來源標記）。
- **涉及檔案**：
  - `internal/monitoring/service/pipeline.go`（`RegimeSessionEntry` struct 拆欄位、`buildRegimeHistoryData` 接源、`loadRegimeHistoryFromSessions` 走 legacy 時 period 留空）
  - `internal/monitoring/service/pipeline_test.go`（4 個新測試：period 接源正確、缺值誠實空字串、period_history store 錯誤仍誠實、legacy 路徑 period 留空）
  - `client_web` / `admin_web` / `shared_web`：`static/js/shared/field_types.ts` 的 `RegimeSessionEntry` TS interface 同步更新欄位
- **新測試 4 個**：
  - `TestBuildRegimeHistoryData_PeriodFromHistory`：seed 7/29 period=consolidation + regime=RISK_ON，斷言 `Period == "consolidation"`（接源正確的直接證明）
  - `TestBuildRegimeHistoryData_PeriodMissingIsEmpty`：period_history 缺該日 → `Period == ""`
  - `TestBuildRegimeHistoryData_PeriodStoreErrorIsEmpty`：period_history load 失敗 → `Period == ""`
  - `TestLoadRegimeHistoryFromSessions_PeriodEmpty`：legacy 路徑無 HistoricalStore → `Period == ""`
- **不動範圍**：`period_detector.go`、PeriodIndicators struct、`period_history` 寫入端、RegimeToPeriod 函式（仍用於 `internal/recommender/handler.go` 的策略白名單查詢，與對外 MCP 欄位無關）、legacy 零政府資金檔、黃金測試 fixtures。
- **預期影響（API 行為變更，外部 agent 須知）**：
  - **對外 `period` 欄位**：以前是 `RegimeToPeriod(regime)` 推導的近似值，現在是 `period_history` 真值。當兩者不一致時，外部 agent 會看到新值（這是目的）。
  - **對外 `market_period` 欄位**：保留以維持向後相容；值等同 `period` 同源。
  - **對外 `source` 欄位**：已移除。新讀者請改用 `regime_source` / `period_source`。


## 附錄：2026-07-30 → 2026-08-07 期間合併 PR 摘要（55 個 feature wave + docs）

> **本版更新**: 2026-08-07，對應 HEAD `0f8667a6`
>
> 本附錄列出 2026-07-30 → 8/7 期間合併進 main 的 PR；對應憲章治理更新見 `docs/ATLAS_CONSTITUTION_AUDIT.md` 附錄 D / E / F。

### Feature wave 表（最新在前）

| Wave | 範圍 | PR | 合併日期 | 摘要 |
|------|------|----|----------|------|
| Stock coverage notice | stocktools | #1477 | 2026-08-06 | out-of-scope TWSE symbols coverage notice |
| TPEX scope §8 | investigation | #1478 | 2026-08-06 | industry §8 external-verified affirmation |
| Traps index M1 | docs (traps) | #1474 | 2026-08-06 | traps.md frontmatter + FinMind/Quota trap 群組（行數 300→330） |
| FinMind HF-1c | industry | #1473 | 2026-08-06 | fetch ctx 5s→10s 對齊 rate limiter 6s token |
| FinMind HF-1a+b | industry | #1472 | 2026-08-06 | 透傳 rate-limit/402 error + classify 402→quota |
| PRISM #1447 closure | prism | #1471 | 2026-08-06 | autoBalancer 從 rogue ticker 遷移至 BTM |
| BTM ticker closure | docs (manifest) | #1475 | 2026-08-06 | 17 ticker 重新分類：15 例外合法 / 1 已遷 / 1 dead code |
| L2.4 cleanup | chore | #1467 | 2026-08-06 | close #825 #826 — dead code cleanup + docs alignment |
| L2.4 cleanup checklist | docs (manifest) | #1468, #1469 | 2026-08-06 | §3.0/§4.0/§5.0 ACI 盤查 + §7.2 AC-1..AC-8 驗收清單 |
| ACI hook | feat | #1464 | 2026-08-06 | PreToolUse soft reminder for hot-path Go access |
| Industry symbol coverage | industry (test) | #1463 | 2026-08-06 | live FinMind symbol coverage 驗證（build tag `livefinmind`） |
| Industry FinMind classifier | industry | #1462 | 2026-08-05 | `classifyFinMindError` 識別「no valid data for industry X」彙總錯誤 |
| Industry observability | industry | #1461 | 2026-08-05 | auto_cycle_update 失敗 metric + symbol coverage validator |
| Operations PR lifecycle | docs | #1460 | 2026-08-05 | consolidate PR lifecycle spec + AGENTS.md reference |
| Crossmarket recovery | monitoring | #1459 | 2026-08-05 | 擴充 crossmarket recovery 路徑含 stale status |
| taifex-daily badge | monitoring | #1458 | 2026-08-05 | 為 dead `taifex-daily` alias 註冊 known-issue badge |
| Crossmarket stale clear | monitoring | #1457 | 2026-08-05 | 復原時清空 stale crossmarket degraded records |
| FinMind endDate | marketdata | #1456 | 2026-08-05 | 改由實際 last day of month 推算 |
| Channel alias | monitoring | #1455 | 2026-08-05 | 註冊 dash-separated runtime aliases |
| Long-stale badge | monitoring | #1454 | 2026-08-05 | 為 long-stale channels 浮現 known-issue badge |
| tw_vol auto-refetch | marketdata | #1453 | 2026-08-05 | tw_vol stale cache 在 trading day rollover 自動 refetch |
| FinMind error body | marketdata | #1452 | 2026-08-05 | channel health 內 capture FinMind API error body |
| Unified quota | apigateway+marketdata+monitoring | #1451 | 2026-08-05 | 三層 unified quota management |
| TEJ inactive health | apigateway | #1450 | 2026-08-04 | `TEJ_API_KEY` 未設置時寫 inactive health record |
| Fugle v1.0 migration | marketdata | #1448 | 2026-08-04 | migrate Fugle quote/meta from retired v0.3 → v1.0 API |
| Fugle docs | docs (marketdata) | #1449 | 2026-08-04 | 校正 Fugle key misconceptions in comments |
| Fugle rate limiter | marketdata | #1446 | 2026-08-03 | unify Fugle clients onto shared rate limiter + constitution cleanup |
| TWSE quote fallback | stocktools | #1445 | 2026-08-03 | TWSE quote fallback 給予獨立 timeout budget |
| Sessions pagination | pipeline | #1444 | 2026-08-02 | paginate sessions endpoint + zero-outcome data-loss monitor |
| experiment_diff metrics | experiment | #1443 | 2026-08-02 | expose judge-collected metrics |
| atlas-mcp experiment_id | atlas-mcp | #1441 | 2026-08-02 | experiment_diff 補上 experiment_id |
| stress_test_daily | orchestrator | #1440 | 2026-08-02 | RunDailyStressTests 跑完後呼叫 drawdownReporter 更新 dashboard |
| TEJ disable | chore | #1439 | 2026-08-01 | disable TEJ channel + scheduler, fix T3-A47 enable inconsistency |
| Govflow daily-once guard | govflow | #1437 | 2026-08-01 | BTM 1h + daily-once guard，修復 24h 排程餓死 |
| Eventdriven baseline | test | #1436 | 2026-08-01 | time-anchored calendar + saturated bullish baseline |
| MCP migration roadmap | docs (operations) | #1435 | 2026-08-01 | MCP 2026-07-28 migration roadmap |
| Sector gitignore | chore | #1434 | 2026-07-30 | ignore `data/sector/` (simulation closure output) |
| Sector allocation reader | sectorallocation | #1433 | 2026-07-30 | 對齊 API reader path 與 writer |
| Sectorallocation SA08 | sectorallocation | #1432 | 2026-07-30 | close SA08 closure writer loop |
| Reporting corrupted | reporting | #1431 | 2026-07-30 | drop corrupted session summaries from performance report |
| Industry fallback_reason | industry | #1430 | 2026-07-30 | industry-map empty state 浮現 fallback_reason |
| Docker 禁令落地 | cleanup | #1429 | 2026-07-30 | Makefile 防呆 + 文件改寫，禁止 AI 重建容器 |
| CI test isolation | test | #1428 | 2026-07-30 | ResetYahooTestLimiters + injectable rate limiters |
| Main 修復列車 | ci | #1427 | 2026-07-30 | sector 映射平均 + ineffassign + eventdriven 校準斷言 |
| Hermes MCP period | mcp | #1426 | 2026-07-30 | period 欄位接 period_history 而非 RegimeToPeriod 推導 |
| Constitution check speedup | ci | #1423 | 2026-07-30 | 憲章檢查腳本提速 25×（BSD sed 修正） |
| Govflow cadence | govflow | #1424 | 2026-07-30 | BTM 28h→24h + weekday 15:00+ + CAPTCHA 24h cooldown |
| skills todo schema | docs (skills) | #1442 | 2026-08-02 | todo schema discipline 加 Session-End Checklist |
| gitnexus SKILL align | docs (skills) | #1438 | 2026-08-01 | 對齊 gitnexus SKILL.md with project-local runner |

### 活躍工作區（2026-08-07 現況）

| 工作區 | Branch | 狀態 | 說明 |
|--------|--------|------|------|
| `~/workspace/atlas` | `main` | 🟢 基準 | 主工作區，HEAD `0f8667a6` (2026-08-06) |
| `~/workspace/atlas-forecast-ledger-wireup` | `feat/20260806-forecast-ledger-wireup` | 🟡 進行中 | forecast ledger wireup：3 issue + 4 candidate gap 盤查（b55aa5a4），尚未合併 |
| `~/workspace/atlas/monthly-revenue-endpoint` | `kaecer68/monthly-revenue-endpoint` | 🟡 進行中 | monthly revenue endpoint：新增 `internal/marketdata/revenue_provider.go`（untracked），尚未合併 |

### 重大事件摘要

1. **Hermes MCP period 欄位接源（#1426）**：原對外 `period` 用 `RegimeToPeriod(regime)` 反推會失真（典型：2026-07-29 regime=`RISK_ON` 推導 `bull` vs period_history 真值 `consolidation`）。修後對外回應拆 `regime_source` / `period_source`，`market_period` 保留為 deprecated alias。詳見上文 §Hermes MCP 段。
2. **FinMind 600/hr server-side quota 撞牆（#1446→#1474 共 19 個 PR）**：從 rate limiter unification → unified quota → error body capture → long-stale badge → endDate 修復 → HF-1 hotfix → traps.md 索引。完整 14+ 循環沉澱為 traps 知識庫。
3. **PRISM autoBalancer §4.5.2 違規 closure（#1447）**：`prism_manager.go:564` 從 rogue ticker 改成 BTM 任務排程（#1471）；剩 17 ticker 評估：15 例外合法 / 1 已遷 / 1 dead code（#1475）。
4. **Govflow 排程餓死修復（#1424 + #1437）**：BTM 24h + CAPTCHA cooldown 仍會在冷重啟卡 24h；#1437 加 1h tick + daily-once guard，不違反 #1424 節律。
5. **stress_test_daily dashboard latestDrawdown 修復（#1440）**：`RunDailyStressTests` 跑完後呼叫 `drawdownReporter`，dashboard 不再永遠 nil。
6. **Stock coverage notice（#1477 + #1478）**：`stock_get_*` MCP 工具以 TWSE 上市普通股為主（≈1070 names），新增 coverage notice + TPEX scope external verification 防誤用。
7. **L2.4 cleanup & ACI hook（#1464, #1467-#1470）**：把憲章追蹤表從「無驗收」改為「驗收清單導向」（§3.0/§4.0/§5.0/§7.2 補 ACI 盤查 + AC 驗收清單）；PreToolUse soft reminder 補強 hot-path Go access 治理。
8. **PRISM 未啟用決策（#1527，2026-08-12）**：盤查結論 — 現有 Darwinian 權重（20 agent 動態 0.3~2.5）+ scorecards `regime_breakdown`（per-regime win_rate/session_count）已覆蓋「動態權重 + 歷史稽核」目標，PRISM/JANUS 無消費端且與 Darwinian 構成第三套平行權重。處置：刪 admin prism 頁面、刪 `/api/prism/training-results`、停用 `prism_training` BTM（`Enabled: false`）、移除 MCP tool `prism_get_training_results`。**以下皆為預期狀態，非 bug**：`/api/prism/training-results` 回 404；`scheduler_get_status` / `/api/scheduler/status` 顯示 `prism_training` disabled；MCP tool 清單無 prism 工具。`internal/prism/`、`internal/janus/`、`prismPlugin`（factory WithPRISM）、`ApplyPRISMWeights`（無資料時 no-op）保留 dormant，未來啟用可從 git history 恢復 API 層並將 BTM 設回 `Enabled: true`。

### 不變的歷史段落提醒

§E4 / §E5a / §E6a / §E6b / §B1+B3 / §B4b / §B4c / §C1 / §Warmup / §B2 / §Docs / §B5-1 / §B5-2 / §B5-T / §B5-R / §B5-3 PR-A / §公股節律 / §B5-3 PR-B / §Hermes MCP 等**已合併**段落保留（7/31 之前）。本附錄僅追加 7/30 → 8/7 之間的新事件；對應憲章治理表更新見 `docs/ATLAS_CONSTITUTION_AUDIT.md` 附錄 D / E / F。