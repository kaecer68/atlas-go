# ATLAS 系統狀態快照

> 最後更新：2026-07-29（B5-T TAIEX 快照回補）
> 維護紀律：每次 feature wave 合併後更新，維持現狀可追蹤性。

## 活躍工作區

| 工作區 | Branch | 狀態 | 說明 |
|--------|--------|------|------|
| `~/workspace/atlas` | `main` | 🟢 基準 | 主工作區，HEAD=`a335abc9` (B5-2 merge) |
| `~/workspace/atlas/MoneyTrend-B5-Batch-2` | `kaecer68/MoneyTrend-B5-Batch-2` | ⚪ 待清理 | B5-2 已完成合併，本 worktree 將於 session close 時移除 |
| `~/workspace/atlas-taiex-backfill` | `fix/20260729-taiex-backfill` | 🟡 待 review | B5-T TAIEX 7/24、7/27 快照回補工具與回補結果 |

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
| B5-T | TAIEX 7/24、7/27 快照回補（TWSE 官方源） | ✅ 已完成 | — | 2026-07-29 |

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
| (d) W1+B5-2 combined | 0 | — |

## B5-T — TAIEX 7/24、7/27 快照回補（已完成）

- **分支**：`fix/20260729-taiex-backfill`（worktree：`~/workspace/atlas-taiex-backfill`，待 review）
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
