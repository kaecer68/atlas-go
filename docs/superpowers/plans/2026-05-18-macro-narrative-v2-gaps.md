# Macro Narrative v2.0 Gap Closure — Implementation Plan

**Date**: 2026-05-18  
**Branch**: feat/macro-narrative-v2-gaps  
**PR**: #49  
**Status**: Ready to Execute

---

## Problem Summary

Four confirmed gaps prevent Macro Narrative v2.0 from functioning correctly:

| # | Gap | Severity |
|---|-----|----------|
| G1 | `NewNarrativeEngine()` 未初始化 `stressCalc`，`GetCurrentStressIndex()` 永遠回傳空值 | High |
| G2 | `MarginHistoryBackfiller.Backfill()` 計算了 `date` 但未傳入 `FetchSnapshot(ctx)`，歷史資料無法正確抓取 | High |
| G3 | `margin_backfill` 已在 TaskExec 註冊但未在 BackgroundTaskManager 排程，每日自動執行不存在 | High |
| G4 | AGENTS.md / CLAUDE.md 缺乏統一架構指引，未來 AI agent 可能繞過 BackgroundTaskManager | Medium |

---

## Task Dependency Graph

```
G4 (docs)  ──────────────────────────────────────────► DONE
G1 (stressCalc init) ────────────────────────────────► G3 (BackgroundTaskManager)
G2 (Backfill date fix) ──────────────────────────────► G3 (BackgroundTaskManager)
G3 (BackgroundTaskManager register) ─────────────────► Task E (calibration, deferred)
```

**可並行執行**：G4、G1、G2（互相獨立）  
**必須順序**：G3 依賴 G1 + G2 完成後驗證

---

## Task A — 補強 AGENTS.md 統一架構指引（G4）

**可與 B、C 並行執行**

### 修改檔案
- `AGENTS.md`（根目錄）

### 修改內容

在 `## 高危陷阱` 表格後新增以下章節：

```markdown
## 統一排程架構（強制）

所有需要「定時自動執行」的後台任務，**必須且只能**透過 `BackgroundTaskManager` 管理：

- 實作位置：`internal/apigateway/background.go`
- 憲法規範：`internal/apigateway/CONSTITUTION.md`
- 註冊位置：`cmd/atlas/main.go`（`taskMgr.RegisterTask(...)` 呼叫區段）

### 禁止行為
- 禁止在 goroutine 中直接啟動 `time.Ticker` 執行定時任務
- 禁止在 `init()` 或 package-level 變數中啟動後台工作
- 禁止繞過 BackgroundTaskManager 直接呼叫業務邏輯的定時執行

### TaskExec vs BackgroundTaskManager 區別

| 機制 | 用途 | 觸發方式 |
|------|------|---------|
| `internal/taskexec` | 使用者手動提交的長時間任務（可取消、可訂閱） | HTTP API / 手動 |
| `BackgroundTaskManager` | 系統自動定時執行的維護任務 | 排程（每日/每小時等） |

兩者可共存：BackgroundTaskManager 的定時任務可直接呼叫業務邏輯，不需經過 TaskExec。
```

### 驗證方式
```bash
grep -n "BackgroundTaskManager" AGENTS.md
# 預期：找到新增的章節內容
```

---

## Task B — 修正 `NewNarrativeEngine()` 初始化 stressCalc（G1）

**可與 A、C 並行執行**

### 修改檔案
- `internal/narrative/knowledge_base.go`

### 現狀（約 line 143）
```go
func NewNarrativeEngine(provider MarketDataProvider, workDir string) *NarrativeEngine {
    return &NarrativeEngine{
        provider: provider,
        workDir:  workDir,
        // stressCalc 未初始化 → nil
    }
}
```

### 修改內容

```go
func NewNarrativeEngine(provider MarketDataProvider, workDir string) *NarrativeEngine {
    return &NarrativeEngine{
        provider:   provider,
        workDir:    workDir,
        stressCalc: NewTaiwanStressCalculator(provider, workDir),
    }
}
```

> **注意**：`NewTaiwanStressCalculator` 的 signature 為 `NewTaiwanStressCalculator(geoProvider GeoRiskProvider, workDir string)`。  
> 確認 `MarketDataProvider` 是否實作 `GeoRiskProvider` 介面，若未實作需新增 adapter 或調整 constructor 參數。

### 驗證方式
```bash
go build ./internal/narrative/...
go test ./internal/narrative/... -run TestGetCurrentStressIndex -v
# 預期：stressCalc 不為 nil，回傳有效 StressIndex 結構
```

---

## Task C — 修正 `Backfill()` 日期參數傳遞（G2）

**可與 A、B 並行執行**

### 修改檔案
- `internal/narrative/margin_history_loader.go`（約 line 152）

### 現狀
```go
for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
    snapshot, err := b.Provider.FetchSnapshot(ctx)  // date 未傳入！
    ...
}
```

### 修改內容

**選項 1（推薦）**：若 `MarginSnapshotProvider` 介面可修改，更新 signature：

```go
// internal/narrative/margin_history_loader.go 中的 Provider 介面
type MarginSnapshotProvider interface {
    FetchSnapshot(ctx context.Context, date time.Time) (*MarginSnapshot, error)
}

// Backfill() 迴圈修正
for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
    snapshot, err := b.Provider.FetchSnapshot(ctx, date)
    if err != nil {
        log.Err("margin_backfill", err)
        continue
    }
    ...
}
```

**選項 2**：若介面不可修改（有其他實作），在 TWSE provider 內部使用 context value 傳遞日期：

```go
// 在 Backfill() 中
dateCtx := context.WithValue(ctx, marginDateKey, date)
snapshot, err := b.Provider.FetchSnapshot(dateCtx)
```

**建議選項 1**，因為 TWSE MI_MARGN API 明確需要日期參數（`?date=YYYYMMDD`）。

### 驗證方式
```bash
go build ./internal/narrative/...
go test ./internal/narrative/... -run TestBackfill -v
# 預期：Backfill() 對每個日期呼叫 FetchSnapshot 時傳入正確日期
# 可用 miniredis 或 mock provider 驗證
```

---

## Task D — 在 BackgroundTaskManager 註冊 margin daily task（G3）

**依賴 B + C 完成且通過測試**

### 修改檔案
- `cmd/atlas/main.go`

### 現狀（約 line 355）
```go
taskMgr = apigateway.NewBackgroundTaskManager(gateway)
// 無任何 margin 相關 RegisterTask 呼叫
```

### 修改內容

```go
taskMgr = apigateway.NewBackgroundTaskManager(gateway)

// Margin history daily backfill — 每日台股收盤後 18:00 執行
taskMgr.RegisterTask(apigateway.BackgroundTask{
    Name:     "margin_daily_backfill",
    Schedule: "0 18 * * 1-5", // 週一至週五 18:00
    Handler: func(ctx context.Context) error {
        backfiller := narrative.NewMarginHistoryBackfiller(
            marketDataProvider, // 已存在的 provider 實例
            cfg.WorkDir,
        )
        // 僅補填昨日資料（增量模式）
        yesterday := time.Now().AddDate(0, 0, -1)
        return backfiller.BackfillRange(ctx, yesterday, yesterday)
    },
    OnError: func(err error) {
        logging.Err("margin_daily_backfill", err)
    },
})
```

> **注意**：
> - 確認 `BackgroundTask` struct 的實際欄位名稱（參考 `internal/apigateway/background.go`）
> - 確認 `MarginHistoryBackfiller` 是否有 `BackfillRange(ctx, start, end)` 方法，若無需新增
> - `marketDataProvider` 需在 main.go 的 provider 初始化區段已存在

### 驗證方式
```bash
go build ./cmd/atlas/...
# 啟動後確認 log 中出現 margin_daily_backfill 排程註冊訊息
go test ./cmd/atlas/... -run TestBackgroundTasks -v 2>/dev/null || echo "no test yet"
```

---

## Task E — 校準 CLI 執行（延後）

**依賴**：累積 30+ 天 margin 歷史資料後執行

```bash
go run ./cmd/calibrate-seasonal --replay --update --update-threshold 30
```

此任務在 Task D 上線並穩定運行 30 天後執行。

---

## Risk Assessment

| 風險 | 可能性 | 影響 | 緩解方案 |
|------|--------|------|---------|
| `MarketDataProvider` 未實作 `GeoRiskProvider` | Medium | Task B 需額外 adapter | 先確認介面，必要時新增 wrapper struct |
| `FetchSnapshot` 介面有多個實作，改 signature 破壞其他實作 | Medium | Task C 編譯失敗 | 先 `grep -r "FetchSnapshot" .` 確認所有實作，一次性更新 |
| BackgroundTaskManager API 與預期不符 | Low | Task D 需調整 | 先讀 `internal/apigateway/background.go` 確認實際 API |
| TWSE MI_MARGN API 日期格式不符 | Low | Backfill 資料錯誤 | 參考現有 TWSE provider 的日期格式處理 |

---

## Rollback Plan

- **Task B**：`git revert` constructor 修改，`stressCalc` 回到 nil（`GetCurrentStressIndex()` 回傳空值，不影響其他功能）
- **Task C**：`git revert` Backfill 修改，手動 backfill 功能暫停
- **Task D**：移除 `RegisterTask` 呼叫，每日自動執行停止（不影響手動 TaskExec 路徑）

---

## Execution Order

```
Week 1, Day 1:
  [並行] Task A (docs) + Task B (stressCalc) + Task C (Backfill date)

Week 1, Day 2:
  [順序] Task D (BackgroundTaskManager) — 待 B + C 通過測試

Week 1, Day 3:
  CI 驗證：go build ./... && go test ./... && staticcheck ./...
  PR #49 更新並請求 review

30 天後:
  Task E (calibration CLI)
```

---

## CI Checklist（每個 Task 完成後執行）

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./...
go vet ./...
staticcheck ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
# 確認覆蓋率 >= 40%
```
