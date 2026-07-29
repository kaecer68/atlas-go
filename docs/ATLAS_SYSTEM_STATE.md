# ATLAS 系統狀態快照

> 最後更新：2026-07-29（B2 時期信心度 + 觸發指標合併）
> 維護紀律：每次 feature wave 合併後更新，維持現狀可追蹤性。

## 活躍工作區

| 工作區 | Branch | 狀態 | 說明 |
|--------|--------|------|------|
| `~/workspace/atlas` | `main` | 🟢 基準 | 主工作區，HEAD=`2a4ae0ba` (B2 merge) |
| `~/workspace/atlas/MoneyTrend-E6b` | `feat/20260729-b2-period-confidence` | ⚪ 待清理 | B2 已完成合併，本 worktree 將於 session close 時移除 |

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
| B5-1 | PeriodIndicators Batch 1 — 指數/匯率/量能均線補齊 | 🟡 進行中 | TBD | — |

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

## B5-1 — PeriodIndicators Batch 1（進行中）

- **分支**：`fix/20260729-b5-batch1-ma-fill`
- **涵蓋**：
  - 新建 `PeriodIndicatorsCalculator`（11 計算函式，附 minDays 誠實降級）
  - 19 個單元測試（含邊界天數、不足降級、確定性）
  - Dashboard API adapter 接入（`persistPeriodHistory` + `RegisterRoutes`）
  - executor pipeline 後續再接（ExecutionContext 尚未有 SnapshotDir）
  - 待完成：黃金測試工具、已知事件驗證、不變性測試
  - 偵測邏輯（`period_detector.go`）零改動
