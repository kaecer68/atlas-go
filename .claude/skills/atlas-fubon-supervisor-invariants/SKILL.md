---
name: atlas-fubon-supervisor-invariants
description: "Mandatory invariants (F1-F9) for modifying the fubon-proxy ProcessManager supervisor, Start pre-flight, Stop cancellation, backoff, and zombie auto-kill logic in atlas-go. Prevents orphan processes, goroutine leaks, Stop deadlocks, and EADDRINUSE backoff loops."
version: "1.0"
category: debug
auto_load: false
load_policy: manual_only
created: "2026-06-26"
updated: "2026-06-26"
target_audience: developer
---

# Atlas Fubon Supervisor 不變式（F1~F9）

本技能定義修改 `internal/fubonproxy` 的 `ProcessManager` 時必須遵守的不變式，涵蓋 `supervise()`、`Stop()`、`Start()` pre-flight、backoff 與 zombie auto-kill 等核心邏輯。

## 何時觸發

- 修改 `internal/fubonproxy/manager.go` 或相關 supervisor 邏輯
- 調整 `Start()`、`Stop()`、`supervise()`、`waitForHealthy()`、port probe、zombie kill 流程
- 新增或變更 fubon-proxy 測試（尤其是 race / 生命週期測試）
- 發現 fubon-proxy 有 orphan process、goroutine 堆積、Stop 阻塞、EADDRINUSE backoff loop 等現象

## 核心概念

### ProcessManager

- 定義：`internal/fubonproxy` 中管理 Python FastAPI 微服務生命週期的結構。
- 實作位置：`internal/fubonproxy/manager.go`
- 關鍵：採 non-blocking supervisor pattern，`Start()` 立即返回，背景 goroutine 處理健康檢查與崩潰重啟。

### supervise()

- 定義：背景 goroutine，負責啟動 Python 程序、執行同步健康檢查、崩潰時 backoff 重啟。
- 關鍵：健康檢查必須是同步呼叫，禁止 fire-and-forget。

### Start() pre-flight probe

- 定義：`Start()` 在 spawn 前以 `net.Listen("tcp", "127.0.0.1:8081")` 探測 port 狀態。
- 關鍵：區分 `portStateFree` / `Healthy` / `Foreign`，Foreign 占用必須回傳 actionable error。

## 實作位置

| 概念 | 檔案路徑 | 關鍵函數 / 結構 |
|------|---------|----------------|
| ProcessManager | `internal/fubonproxy/manager.go` | `ProcessManager`、`Start()`、`Stop()`、`supervise()`、`waitForHealthy()` |
| Fubon proxy | `services/fubon-proxy/main.py` | FastAPI 啟動與 `host` 綁定 |
| 測試 | `internal/fubonproxy/manager_test.go` | `TestProcessManager_*` |

## F1 — Stop/restart race

- `Stop()` **必須先設 `m.stopping=true` 再做任何檢查**，並呼叫 `cancel()`（nil-check cancel 函式）。即使 `m.running=false` 也要走完這步，否則 `supervise()` 會在重啟路徑中孤兒化新程序。
- `supervise()` 啟動新程序後必須在鎖內檢查 `m.stopping`；若已設則 `Kill()` 新程序並 return。

## F2/F4 — supervise() 健康檢查必須同步

- 不要 fire-and-forget 背景 goroutine 跑 `waitForHealthy`。
- 崩潰循環中 fire-and-forget 會堆積 goroutine（最多 30s × 連環重啟次數）並以最短 backoff 連環重啟。

## F3 — 測試必須驗證正確性，不僅時序

- assert `m.running=true`
- assert `m.cmd != nil`
- assert `process.Signal(syscall.Signal(0))` 成功（process 真的活著）
- assert `Stop()` 後 `m.cmd == nil` 且 process 退出
- 不允許只測「Start() 在 N 秒內返回」。

### F9 port 探測測試標準

- 必須用真實 `net.Listen` 佔位（透過 package-level `proxyListenPort` var;測試用 `withFreeEphemeralPort(t)` / `bindEphemeralPort(t, handler)` 取得 OS-assigned ephemeral port 並覆寫 `proxyListenPort`）,不可用 mock。
- Production `proxyListenPort` 預設值仍為 8081（`manager.go:74`）,不變。
- ephemeral port 永遠可用,無需 `t.Skip`;若 ephemeral bind 失敗（極罕見,僅在 fd 耗盡時）視為環境異常 `t.Fatal`。
- 涵蓋：Free → spawn、Healthy（bind + /health=200）→ 跳過 spawn 且 `m.running=false`、Foreign（bind + /health=404）→ error 含 `"port %d"`（用 `proxyListenPort`）/ `"foreign"` / `"kill"`。
- `lookupPortOccupant` 單元測試可在 lsof 不可用時 skip。

> 由於 `proxyListenPort` 是 package-level var,這些 helper **不可** 用於 `t.Parallel()` 測試（race）。

## F5 — waitForHealthy 必須尊重 ctx.Deadline()

- deadline 採 `min(startupTimeout, ctx.Deadline())`。
- sleep 必須用 `select { <-ctx.Done(): ... <-time.After(...): }` 包裝，讓取消能立即生效。

## F6 — 重啟路徑必須在鎖內 snapshot m.ctx

- 重啟路徑讀取共享狀態需在鎖內以避免與 `Stop()` 競爭。
- `m.ctx.Done()` 在 `select` 中讀取為例外（stdlib 慣例，安全）。

## F7 — 不用 defer 解鎖 + Start 錯誤路徑清理

- supervisor 邏輯使用 **lock-check-unlock-work-lock** pattern。
- `exec.Cmd.Start()` 必須在鎖外執行，否則 panic 會永久卡死 mutex。
- `Start()` 錯誤路徑必須關閉 `m.done` 並重置 ctx/cancel/done，避免 `Stop()` 永久阻塞（no-supervise edge case）。

## F8 — doc.go 必須與 code 同步

- supervisor decouple fix 後 `Start()` 是非同步的；package doc 必須反映此行為。

## F9 — Start() 啟動前 port 探測

- 在 `scriptPath` 與 `IsHealthy` 檢查之間，必須以 3-state switch 取代單一 `IsHealthy()` 早退：
  - `portStateFree` → fall through to spawn
  - `portStateHealthy` → 視為外部已管理，跳過 spawn
  - `portStateForeign` → 回傳 actionable error（含 PID 與 `kill` 指令）
- Probe 用 `net.Listen` 而非 `net.Dial` 避免 side effect。
- probe 僅在 `Start()` 進行一次，不在 `supervise()` 重啟路徑中重複。

### F9 zombie auto-kill

- `isFubonZombie` 比對 lsof command name 是否含 `"python"` 或 `"uvicorn"`。
- 非 Python 程序（java, nginx, node, go, sh 等）不會被 auto-kill，直接回傳 actionable error。
- `killOccupant` 二段式：SIGTERM → 等待 1 秒 → `signal(0)` 檢查 → 若仍在則 SIGKILL。
- `killOccupant` 不回傳 process 資源給呼叫端，由 caller 自行 `cmd.Wait()` 收屍。
- macOS zombie 陷阱：被 SIGKILL 的行程可能變成 zombie，`syscall.Kill(pid, 0)` 對 zombie 仍回 `nil`，必須用 `cmd.Wait()` 真正回收。
- auto-kill 後立即 re-probe：Free → spawn；Healthy → 跳過；Foreign → 回傳 "still held after auto-kill"。
- re-probe 失敗：log warn + fall through to spawn。

## Stop() 取消路徑（#500）

- `cancel()` 會透過 `exec.CommandContext` 立即 SIGKILL cmd。
- 後續的 SIGINT 與 5s `gracefulShutdownTimeout` 為雙重安全網，正常情況下不會觸發。
- 若需要 SIGINT-based 優雅關閉，需改用 `exec.Command` 並把 cancel 路徑改為 SIGINT → wait → SIGKILL 三段式。
- `TestProcessManager_Stop_SIGINTGracefulThenSIGKILL` 驗證實際 cancel-based kill 路徑。

## Backoff 觸發條件（#500）

- `restartBackoffDelay` (10s) 僅在 `cmd.Start()` 失敗時生效（`supervise()` 重啟路徑中 `newCmd.Start()` 回傳 error）。
- 連環 crash 場景：程式崩潰後若 health check 持續失敗，backoff 維持 3s（`restartInitialDelay`），並被 30s `waitForHealthy` timeout 阻塞主導。整體重啟頻率約 33s 一次。
- `TestProcessManager_BackoffStateMachine_3sThen10s` 簡化為只測 1st→2nd 的 3s gap，並在 docstring 記錄 2nd→3rd 為何不可觀察。

## 驗證規則

- [ ] 修改 supervisor 邏輯後重新檢視 F1~F8
- [ ] 修改 `Start()` pre-flight 後重新檢視 F9
- [ ] 跑 `go test -race -count=1 ./internal/fubonproxy/`
- [ ] 跑 `go vet ./internal/fubonproxy/` + `staticcheck ./internal/fubonproxy/`
- [ ] 若改介面：更新 `internal/fubonproxy/doc.go`
- [ ] 更新 `internal/fubonproxy/AGENTS.md` 與本 skill 保持同步

## 相關技能

| 技能 | 關聯 |
|------|------|
| `atlas-pre-change-protocol` | 修改 `internal/fubonproxy/` 前必須執行 7 步檢查 |

## 版本歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | 2026-06-26 | 從 `internal/fubonproxy/AGENTS.md` 抽出 F1~F9 不變式為獨立 skill |
