# AGENTS.md — internal/ledger

**成熟度**: stable
**模組職責**: JSONL/SQLite 雙後端 append-only 持久化，負責交易結果、實驗記錄、人為干預與分數卡的生命週期管理。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Store` | `store.go` | JSONL 預設後端，append-only 寫入 |
| `OutcomeStore` | `store.go` | 推薦結果、實驗、人為干預的統一介面 |
| `FullStore` | `store.go` | 組合所有 ledger 介面的工廠入口 |
| `SQLiteStore` | `sqlite_store.go` | SQLite 後端（WAL 模式、外鍵約束） |
| `SessionWriter` | `session_writer.go` | 原子寫入（temp-dir → rename） |
| `Archiver` | `archiver.go` | 自動 gzip 歸檔與過期清理 |
| `SpawnRecord` | `spawn_records.go` | Agent 生成稽核軌跡 |

## 資料流

```
Orchestrator/Simulator
  → SessionWriter.WriteSession()  [原子寫入]
  → sessions/<sessionID>/
      ├─ recommendation_outcomes.jsonl
      ├─ screened_symbols.jsonl
      ├─ trades.jsonl
      ├─ summary.json
      └─ experiments.jsonl
  → 全域檔案（baseDir 層級）
      ├─ recommendation_outcomes.jsonl
      ├─ experiments.jsonl
      └─ human_interventions.jsonl
```

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **JSONL 不是 JSON array** | 每行是獨立 JSON 物件，不可包成 `[]` 或整體 unmarshal |
| **append-only 語義** | 永遠不修改既有記錄；更新即寫新行，由讀取端去重 |
| **RecordedAt ≠ 交易日** | `RecordedAt` 是計算完成時間；交易日請從 `SessionID` 解析（如 `session-20260413-daily`） |
| **LoadOutcomes() 讀取全域稀疏檔** | 絕對不可用於計算單場 `OutcomeCount`，否則會得到跨 session 累計值 |
| **OutcomeCount 必須是單場次數量** | `RecordSessionSummary` 的 `OutcomeCount` 只能來自當場 `GuardOutcomes` 長度 |
| **RecordSessionTrades 靜默跳過空 slice** | `len(trades) == 0` 時直接 `return nil`，不寫入任何檔案 |
| **SQLiteSessionStore 未實作 LoadAllSessionScorecards** | 呼叫會回傳 `nil, nil, nil`，勿用於生產查詢 |

## 測試

- `go test ./internal/ledger/...`
- 整合測試：`//go:build integration`（`store_factory_integration_test.go`）
- SQLite 記憶體模式：`outcome_store_sqlite_test.go` 等無需外部服務
