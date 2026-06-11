# Fubonproxy AGENTS.md

## 模組概述

`fubonproxy` 提供 Fubon-proxy 生命週期管理 — 自動啟動/停止/監控 Python FastAPI 微服務。
採用 **circuit breaker pattern**：`Start()` 立即返回，背景 goroutine 處理健康檢查與崩潰重啟。

// Maturity: evolving

## 關鍵符號

- `ProcessManager` — Python 程序管理器（含 supervisor goroutine）
- `Start(ctx)` — 非同步啟動；失敗不阻擋 atlas 啟動
- `Stop()` — 同步停止（SIGINT + 5s graceful + 強制 kill）
- `IsHealthy()` — 向 `/health` 端點發 GET
- `waitForHealthy(ctx)` — 同步輪詢；deadline = `min(startupTimeout, ctx.Deadline())`
- `supervise()` — 背景 goroutine；崩潰時 backoff 重啟
- `healthURL` — struct 欄位（測試注入用，預設 `http://localhost:8081/health`）

## 已知陷阱

### 啟動 / 生命週期

- **非致命失敗**：若 proxy 啟動失敗，僅記錄警告後繼續，不阻擋 atlas 啟動。
- **Python 依賴**：使用 `~/.config/atlas-go/.fubon-env/bin/python` 或系統 `python3`。
- **健康檢查**：透過 `/health` 端點偵測運行狀態。
- **自動重啟**：崩潰時自動重啟，**backoff 僅在健康通過時重置為 3s**（初始），失敗時保留 10s 重試值。
- **scriptPath 必須是絕對路徑**：`NewManager` 用 `filepath.Abs` 將 `services/fubon-proxy/main.py` 轉絕對，否則 `cmd.Dir` 與 `cmd.Start()` 解析基準不一致，導致 exit 2（PR #488）。
- **stderr 必須以 Info 級別記錄**：Python 異常是排查啟動失敗的主要線索，Debug 級別會被吞（PR #488）。
- **健康檢查位址**：使用 IPv4 `127.0.0.1:8081` 而非 `localhost:8081`（PR #495）。原因：雙棧環境下 Go `net.Dial` 預設優先走 IPv6 `[::1]`，而 fubonproxy 雖已升級為 `host="::"` 雙棧綁定，但若用戶端 host 解析優先 IPv6 仍會 connection refused。對應 `internal/marketdata/AGENTS.md` 同名段落。

### 程序監督器不變式（PR #489 — F1~F8）

新增或修改 `supervise()` / `Stop()` 必須遵守。完整發現清單見 PR #489 review。

- **F1 — Stop/restart race**：`Stop()` **必須先設 `m.stopping=true` 再做任何檢查**，並**無條件**呼叫 `cancel()`。即使 `m.running=false` 也要走完這步，否則 supervise() 會在重啟路徑中孤兒化新程序。
- **F1 — Post-start re-check**：supervise() 啟動新程序後必須在鎖內檢查 `m.stopping`，若已設則 `Kill()` 新程序並 return。
- **F2/F4 — supervise() 的 health check 必須是同步呼叫**：不要 fire-and-forget 背景 goroutine 跑 `waitForHealthy`。崩潰循環中 fire-and-forget 會堆積 goroutine（最多 30s × 連環重啟次數）並以最短 backoff 連環重啟。
- **F5 — waitForHealthy 必須尊重 ctx.Deadline()**：deadline 採 `min(startupTimeout, ctx.Deadline())`；sleep 必須用 `select { <-ctx.Done(): ... <-time.After(...): }` 包裝，讓取消能立即生效。
- **F6 — 讀 `m.ctx` 必須在鎖內**：雖單純讀取在技術上安全，但違反 codebase 慣例（後續維護者難以推理）。
- **F7 — supervisor 邏輯不用 `defer m.mu.Unlock()`**：使用 **lock-check-unlock-work-lock** pattern。`exec.Cmd.Start()` 必須在鎖外執行，否則 panic 會永久卡死 mutex。
- **F7 — Start() 錯誤路徑必須清理 m.done**：關閉 m.done + 重置 ctx/cancel/done，避免 Stop() 永久阻塞（no-supervise edge case）。
- **F8 — doc.go 必須與 code 同步**：F1 fix 後 Start() 是非同步的，doc.go 第 5~10 行的描述要與新行為對齊。

### 測試標準（F3）

- **測試必須驗證「正確性」不只「時序」**：
  - assert `m.running=true`
  - assert `m.cmd != nil`
  - assert `process.Signal(syscall.Signal(0))` 成功（process 真的活著）
  - assert `Stop()` 後 `m.cmd == nil` 且 process 退出
- **不允許只測「Start() 在 N 秒內返回」**。無 assertion 的時序測試會通過壞程式碼。
- 注入 `healthURL` 走 `httptest.Server` 才有意義；用獨立於 proxy 的 mock 沒驗證到任何東西。

### 與 `internal/live/fubon_dma.go` 的界線

兩者都用 `exec.CommandContext` 啟動 Python，但**職責不同**：

| | `fubonproxy` (本模組) | `live/fubon_dma` |
|---|---|---|
| 用途 | 啟動/監控 proxy HTTP 服務 | broker DMA wrapper |
| 監督 | supervise() + 自動重啟 | 無（一次性 connect/kill） |
| 健康檢查 | 同步 + 背景 goroutine | 無 |
| Backoff | 3s/10s | 無 |
| 共享 ProcessManager | — | **無**（重複程式碼，未來可抽 `internal/procsupervisor`） |

**不要在 fubon_dma.go 套用本模組的 audit pattern** — 它沒 supervisor 邏輯，套了會誤報。

## 相依關係

- 由 `cmd/atlas` API 模式使用（`NewManager(cfg.WorkDir)` + `defer mgr.Stop()`）
- 關閉時發送 SIGINT，等待 5 秒後強制終止
- `healthURL` 可注入為測試替身
- 在 `cmd/atlas/main.go` 是獨立於 `internal/live` 的啟動路徑，**兩者可同時使用**

## 修改前必讀

1. 必跑：`go test -race -count=1 ./internal/fubonproxy/` 確認 3 個測試全綠
2. 必跑：`go vet ./internal/fubonproxy/` + `staticcheck ./internal/fubonproxy/`
3. 若改 supervisor 邏輯：重新檢視 F1~F8 是否仍遵守，並更新本檔
4. 若改介面：更新 `doc.go`（package 層級文件）保持一致
