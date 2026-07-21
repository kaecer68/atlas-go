# Repository Dual-Write Persistence 規格

> **文件角色**：atlas-go PostgreSQL + JSONL 雙寫持久化規格。
> **取代對象**：原 internal/repository/AGENTS.md（已遷移至此）。

**成熟度**: stable  
**模組職責**: PostgreSQL + JSONL 雙寫持久化，PG 為首選讀取路徑，JSONL 為 source of truth。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `DualWriteRepository` | `dual_write_repository.go` | 協調 PG + JSONL 雙寫，PG 失敗不阻斷 |
| `PostgresRepository` | `postgres_repository.go` | PostgreSQL 實作，支援 pgx.Batch 批量寫入 |
| `JSONLRepository` | `jsonl_repository.go` | JSONL 後端實作，source of truth |
| `TaskExecutionStore` | `postgres_task_execution.go` | 任務執行全生命週期稽核 |
| `MetricsSnapshot` | `types.go` | 指標快照 DTO |

## 資料流

```
寫入：
  Caller → DualWriteRepository.Record()
    → JSONL（必寫，成功才回傳）
    → PostgreSQL（best effort，失敗僅記錄不返回 error）

讀取：
  Caller → DualWriteRepository.QueryLatest()
    → PostgreSQL（首選）
    → JSONL fallback（PG 失敗或無資料）
```

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **Dual-write 協調** | PG 寫入失敗不會回傳 error，呼叫方可能誤以為「已持久化至 PG」 |
| **PG 無 QueryAllOutcomes** | `DualWriteRepository` 的 `QueryAllOutcomes` 直接 fallback 到 JSONL |
| **Window 欄位即 session_id** | `RecordOutcomes` 將 `o.Window` 寫入 PG 的 `session_id` 欄位，語義需對齊 |
| **scanRecommendationOutcomes 靜默跳過錯誤** | 單列掃描失敗 `continue` 不中斷，可能遺失資料 |
| **ROC 年分轉換** | `SaveExportStats` 將民國年 (+1911) 轉西元年，跨世紀邊界需留意 |
| **TimescaleDB 依賴** | `LoadToday` 使用 `time_bucket('1 minute', time)`，非 TimescaleDB 會報錯 |
| **動態查詢組裝** | `QueryLatest` 用 `fmt.Fprintf` 組 SQL，雖輸入受信任但仍屬動態組字串 |

## 測試

- `go test ./internal/repository/...`
- 整合測試需 `-tags=integration` + 運行中的 PostgreSQL + Redis
- `dual_write_*_test.go` 驗證雙寫協調邏輯
- `postgres_task_execution_test.go` 驗證任務生命週期
