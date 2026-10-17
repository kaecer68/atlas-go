# `store_backend_empty_default_jsonl` WARN 的定位（2026-10-17）

> **一句話**：這個 WARN 來自 `session_summary_reconcile` 背景任務——
> 它以 `ledger.NewSessionStore(config.Config{LedgerDir: …})` **省略**
> `StoreBackend` 來取得「flat JSONL 那一側」，於是 `ResolveStoreBackend("")`
> 走預設並報警。**功能上是預期的**（那個 job 本來就要 JSONL 側），
> 但意圖是用「不寫」表達的 ⇒ 每 12h 一則雜訊，且行為依賴空字串的預設值。
> **本票只提案，未改程式**（依「只提案」護欄）。

## 1. 現象（生產實測，唯讀）

```
time=2026-10-17T06:27:20.317Z level=INFO msg=task_started name=session_summary_reconcile component=background_task
time=2026-10-17T06:27:20.326Z level=WARN msg=store_backend_empty_default_jsonl resolved_backend=jsonl component=ledger_store_factory
time=2026-10-17T06:27:20.378Z level=INFO msg=reconcile_done pg=230 jsonl=229 backfilled_to_jsonl=1 backfilled_to_pg=0 conflicts=39 errors=0 component=session_summary_reconcile
```

* 頻率：24 小時內 **1 筆**（任務 `Interval: 12 * time.Hour`，`cmd/atlas/reconcile_task.go:35`）。
* 緊接在 `task_started name=session_summary_reconcile` 之後 ⇒ 呼叫端就是該任務。
* 它**不是**「後端壞掉」：同一輪 `reconcile_done` 正常完成（pg=230 / jsonl=229）。

## 2. 是哪個 store

| 項目 | 值 |
|---|---|
| 呼叫端 | `cmd/atlas/reconcile_task.go:57` — `ledger.NewSessionStore(config.Config{LedgerDir: d.Cfg.LedgerDir})` |
| 解析點 | `internal/ledger/store_factory.go:79` — `ResolveStoreBackend("")` ⇒ WARN + `"jsonl"` |
| store | `ledger.SessionStore`（JSONL 實作 `*ledger.Store`） |
| 路徑 | 生產 `<LedgerDir> = /app/data/state` ⇒ 每場次 `<…>/sessions/<session-id>/summary.json`；實測容器內 `sessions/` 有 **231** 個 session 目錄（最新 `session-20261017-daily`） |
| 為何必為 JSONL | 這個 job 的職責就是**把 PG SSoT 與 flat ledger 對帳**（`internal/reconcile`）。PG 那一側由 `repository.NewPostgresRepository(pool)` **直接**建立，JSONL 這一側才走 store factory ⇒ 這裡的 `jsonl` 是**語意正確**的選擇 |

## 3. 為什麼是「空」

呼叫端寫的是 `config.Config{LedgerDir: …}` —— 一個**只填 LedgerDir** 的字面值，
`StoreBackend` 因此是 `""`。`ResolveStoreBackend` 對 `""` 的政策是
「保留 legacy 預設（JSONL）並記一則 WARN」（`store_factory.go:73-82`，註解自述）。

⇒ 「空但可用」= **空字串走 legacy 預設**，不是「DB 空」。

## 4. 是否需修（**只提案**）

**需要（低風險、純清理）**，理由兩點：

1. **雜訊**：每 12h 一則 WARN 指向一個「本來就該是 jsonl」的呼叫端；WARN 的價值
   （「有人忘了設後端」）被自己稀釋。
2. **意圖沒有寫出來**：同 repo 已有一個明確表達同一意圖的先例——
   `internal/calibration/calibrate.go:203-206` 用
   `config.Config{StoreBackend: "jsonl", LedgerDir: …}` 並附註解說明「這裡刻意用
   JSONL，不是部署後端」；`NewReportOutcomeStore` 甚至把已正規化的值寫回去以避免
   重複 WARN（`store_factory.go` 內註解「avoids a duplicate empty-backend warn」）。
   本處是唯一還靠「空字串預設」表達的地方。

**提案（未實作）**：

```diff
--- a/cmd/atlas/reconcile_task.go
+++ b/cmd/atlas/reconcile_task.go
@@ func (d calibrationDeps) registerSessionSummaryReconcile() {
-			jsonlStore, err := ledger.NewSessionStore(config.Config{LedgerDir: d.Cfg.LedgerDir})
+			// 這裡刻意是 flat JSONL 側（PG 側由 repository.NewPostgresRepository 直接建立）。
+			// 明示 StoreBackend 而不是靠空字串的 legacy 預設:後者每 12h 記一則
+			// store_backend_empty_default_jsonl WARN,而且把「意圖」寄託在預設值上。
+			jsonlStore, err := ledger.NewSessionStore(config.Config{
+				StoreBackend: "jsonl",
+				LedgerDir:    d.Cfg.LedgerDir,
+			})
```

**行為差異：無**（`ResolveStoreBackend("jsonl") == ResolveStoreBackend("") == "jsonl"`）。
**必要測試（同 commit）**：以 log capture 斷言該任務路徑**不再**出現
`store_backend_empty_default_jsonl`，且仍然寫入同一條 `<LedgerDir>/sessions/…`。

**不建議**的另一種修法：讓 `ResolveStoreBackend("")` 靜默（不記 WARN）。
那會把「有人真的忘了設 `ATLAS_STORE_BACKEND`」這個訊號一起消掉 —— 本 repo 的
`ResolveStoreBackend` 設計意圖正是「未知值要吵、空值要留痕」（`store_factory.go:73-82`）。

## 5. 已知限制

* 本票**未**改任何程式（依「只提案」護欄）；提案的 3 行改動未經 CI。
* 生產容器內 `ATLAS_STORE_BACKEND=postgres`（已設定）⇒ 這則 WARN **不可能**來自
  「部署忘了設後端」，只可能來自以字面值建構 config 的呼叫端。
  逐一檢查（`grep -rn "ledger.New\(Session\|Outcome\|Full\|Quote\|ReportOutcome\)Store(\|
  ledger.ResolveStoreBackend(" --include=*.go cmd/ internal/ | grep -v _test.go`，
  共 57 處）後，**只有 2 處**省略 `StoreBackend`：
  本節的 `cmd/atlas/reconcile_task.go:54`，以及同族的 CLI
  `cmd/reconcile-sessions/main.go:81`（同一個對帳任務的手動版，執行時也會記同一則 WARN）。
  其餘呼叫端全部明示 `"jsonl"`（例：`internal/experiment/auto.go:182,427`、
  `internal/monitoring/api/experiment/handlers.go:71`、
  `internal/calibration/calibrate.go:203`）或傳入 `cfg.StoreBackend`。
