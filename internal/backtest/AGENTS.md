# AGENTS.md — internal/backtest

**成熟度**: evolving
**模組職責**: 歷史回測執行，在指定日期區間內逐日執行模擬並產出回測摘要。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Runner` | `window.go` | 回測執行器：載入 replay 資料、逐日執行模擬、產出 BacktestWindowSummary |
| `RollingWindowSplit` | `rolling_split.go` | SK-03 滾動時間序列切割（train/valid/test），每年滾動一次 |
| `Model` | `backtest_pipeline.go` | ML 模型介面：Fit/Predict，供 BacktestPipeline 使用 |
| `BacktestPipeline` | `backtest_pipeline.go` | 回測管線：整合 RollingWindowSplit + Model，執行 train → predict → 記錄報酬 |
| `BacktestResult` | `backtest_pipeline.go` | 單一 window 回測結果（預測值、實際值、指標） |
| `WindowRange` | `rolling_split.go` | 單一 window 的日期區間（train/valid/test） |
| `BacktestWindowSummary` | `internal/domain` | 回測結果摘要（區間、場次數、outcome 數、最差 Agent） |

---

## 本模組特有陷阱

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

## 測試

- `go test ./internal/backtest/...`
- 涵蓋單日回測執行（使用 samples/replay/twse_stock_day_all_sample.csv）

(End of file - total 30 lines)
