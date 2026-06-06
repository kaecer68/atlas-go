# AGENTS.md — internal/autobacktest

**成熟度**: evolving
**模組職責**: 自動化回測排程與執行，每日定時執行回測並記錄快照與風險訊號。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Runner` | `runner.go` | 自動回測執行器：檢查重複、執行回測、產生報告、同步至 live store |
| `StartDailyLoop` | `loop.go` | 每日 13:30 台北時間觸發回測的 background loop |
| `SignalEngine` | `signals.go` | 評估風險訊號（VaR、Sharpe 退化、熔斷） |
| `Comparator` | `comparator.go` | 比較短天期（5 日）與長天期（20 日）投組績效與 Sharpe |
| `History` | `history.go` | 將 AutoSnapshot 以 JSONL 形式寫入 `autobacktest/snapshots.jsonl` |
| `AutoSnapshot` | `history.go` | 單日自動回測快照（投組價值、VaR、Sharpe、回撤、訊號） |

---

## 本模組特有陷阱

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

- `go test ./internal/autobacktest/...`
- 涵蓋 SignalEngine 評估、Comparator 短長期比較、History JSONL 讀寫、loop 時區計算

(End of file - total 35 lines)
