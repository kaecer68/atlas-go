# ATLAS 系統狀態快照

> 最後更新：2026-07-30（B5-R TAIEX 抓取韌性修復）
> 維護紀律：每次 feature wave 合併後更新，維持現狀可追蹤性。

## 活躍工作區

| 工作區 | Branch | 狀態 | 說明 |
|--------|--------|------|------|
| `~/workspace/atlas` | `main` | 🟢 基準 | 主工作區，HEAD=`a335abc9` (B5-2 merge) |
| `~/workspace/atlas/feat-20260730-b3-data-infra` | `feat/20260730-b3-data-infra` | 🟡 進行中 | B5-3 PR-A：公股資料基礎設施（parser/8 行庫/per-broker/sector reader），未合併 |
| `~/workspace/atlas/MoneyTrend-B5-Batch-3` | `kaecer68/MoneyTrend-B5-Batch-3` | ⚪ 待清理 | B5-3 偵察工作區，本 worktree 將於 session close 時移除 |
| `~/workspace/atlas/MoneyTrend-B5-Batch-2` | `kaecer68/MoneyTrend-B5-Batch-2` | ⚪ 待清理 | B5-2 已完成合併，本 worktree 將於 session close 時移除 |
| `~/workspace/atlas-taiex-backfill` | `fix/20260729-taiex-backfill` | ⚪ 待清理 | B5-T TAIEX 7/24、7/27 快照回補工具與回補結果 |
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
| B5-3 PR-A | PeriodIndicators Batch 3 資料基礎設施（公股 parser/8 行庫/per-broker/sector reader） | 🟡 進行中 | — | — |
| B5-T | TAIEX 7/24、7/27 快照回補（TWSE 官方源） | ✅ 已完成 | — | 2026-07-29 |
| B5-R | TAIEX 抓取韌性修復（Yahoo → TWSE fallback、失敗可見化、陳舊快取誠實化） | ✅ 已完成 | — | 2026-07-30 |

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

## B5-3 PR-A — PeriodIndicators Batch 3 資料基礎設施（進行中）

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
