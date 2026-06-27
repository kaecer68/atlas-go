# Backtest Pipeline 規格

> **文件角色**：atlas-go 歷史回測執行與自動化排程規格（合併 `backtest` + `autobacktest` 模組）。
> **取代對象**：原 `internal/backtest/AGENTS.md` + `internal/autobacktest/AGENTS.md`（均已遷移至此規格；後者於 Wave 11 Batch 5c 合併進此文件）。

---

## internal/backtest — 歷史回測執行

**成熟度**: evolving  
**模組職責**: 歷史回測執行，在指定日期區間內逐日執行模擬並產出回測摘要。

### 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Runner` | `window.go` | 回測執行器：載入 replay 資料、逐日執行模擬、產出 BacktestWindowSummary |
| `RollingWindowSplit` | `rolling_split.go` | SK-03 滾動時間序列切割（train/valid/test），每年滾動一次 |
| `Model` | `backtest_pipeline.go` | ML 模型介面：Fit/Predict，供 BacktestPipeline 使用 |
| `BacktestPipeline` | `backtest_pipeline.go` | 回測管線：整合 RollingWindowSplit + Model，執行 train → predict → 記錄報酬 |
| `BacktestResult` | `backtest_pipeline.go` | 單一 window 回測結果（預測值、實際值、指標） |
| `WindowRange` | `rolling_split.go` | 單一 window 的日期區間（train/valid/test） |
| `BacktestWindowSummary` | `internal/domain` | 回測結果摘要（區間、場次數、outcome 數、最差 Agent） |

### 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **日期邊界為閉區間** | `Run(start, end)` 使用 `date.Before(startDate) || date.After(endDate)` 過濾，起訖日都包含在內。 |
| **Baseline 未載入時使用預設** | `baseline.Load()` 失敗時會 fallback 到 `baseline.DefaultPolicy()`，不會報錯。 |
| **需要 NextDate 才執行** | 若某日沒有次日資料（`ds.NextDate` 失敗），該日會被跳過。 |
| **Store 必須實作 BacktestStore** | `Run()` 結尾會呼叫 `bt.RecordWindowSummary()` 與 `bt.RecordMutationBrief()`，store 需實作 `ledger.BacktestStore`。 |
| **JANUS 為選配** | `WithJANUS()` 可附加 JANUS 引擎進行 A/B 驗證，未附加時不影響主流程。 |
| **Split 停止條件：valid_start.Year() > 2020** | `RollingWindowSplit.Split()` 在 valid start 年份超過 2020 時停止迭代。 |
| **ExtractFeatures/Labels 必須設定** | `BacktestPipeline.Run()` 要求 `ExtractFeatures` 和 `ExtractLabels` 均非 nil，否則回傳錯誤。 |
| **RollingWindowSplit 需 dataStart 參數** | `Split(dataStart)` 的 training window 為 expanding：從 dataStart 到 train_end，而非固定長度 sliding window。 |
| **test window 可能為空** | `BacktestPipeline.Run()` 會跳過無 test 資料的 window（data 未延伸到 test_end 時），不會報錯。 |

---

## internal/autobacktest — 自動化回測排程

**成熟度**: evolving  
**模組職責**: 自動化回測排程與執行，每日定時執行回測並記錄快照與風險訊號。

### 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Runner` | `runner.go` | 自動回測執行器：檢查重複、執行回測、產生報告、同步至 live store |
| `StartDailyLoop` | `loop.go` | 每日 13:30 台北時間觸發回測的 background loop |
| `SignalEngine` | `signals.go` | 評估風險訊號（VaR、Sharpe 退化、熔斷） |
| `Comparator` | `comparator.go` | 比較短天期（5 日）與長天期（20 日）投組績效與 Sharpe |
| `History` | `history.go` | 將 AutoSnapshot 以 JSONL 形式寫入 `autobacktest/snapshots.jsonl` |
| `AutoSnapshot` | `history.go` | 單日自動回測快照（投組價值、VaR、Sharpe、回撤、訊號） |

### 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **跳過已存在快照** | `RunAndStore()` 若當日已有快照（`LatestN(1)` 日期相同）則直接返回，不會重複執行。 |
| **週末自動跳過** | `StartDailyLoop` 與 `RunScheduledBacktest` 在週六、週日不執行回測。 |
| **時區為 Asia/Taipei** | `next13_30()` 使用台北時區，載入失敗時 fallback 到 `CST+8` 固定時區。 |
| **SignalEngine 需要 FullStore** | `NewSignalEngine()` 會將 `ledger.NewStore()` 斷言為 `ledger.FullStore`，若 store 類型不符會 panic。 |
| **syncToLiveStore 為 best-effort** | 同步至 live store 失敗時僅記錄 Warn 日誌，不會中斷主流程。 |
| **熔斷門檻為 15% 回撤** | `SignalCircuitBreaker` 在 `drawdown > 0.15` 時觸發。 |

---

## 測試

```bash
go test ./internal/backtest/...
go test ./internal/autobacktest/...
```

backtest 涵蓋單日回測執行（使用 `samples/replay/twse_stock_day_all_sample.csv`）。autobacktest 涵蓋 SignalEngine 評估、Comparator 短長期比較、History JSONL 讀寫、loop 時區計算。
