# AGENTS.md — internal/fubonproxy

## 模組概述

`fubonproxy` 提供 Fubon-proxy 生命週期管理 — 自動啟動/停止/監控 Python FastAPI 微服務。
採用 **non-blocking supervisor pattern**：`Start()` 立即返回，背景 goroutine 處理健康檢查與崩潰重啟。

**Maturity**: evolving

## 關鍵符號

- `ProcessManager` — Python 程序管理器（含 supervisor goroutine）
- `Start(ctx)` — 非同步啟動；失敗不阻擋 atlas 啟動
- `Stop()` — 同步停止（cancel → SIGINT + 5s graceful + 強制 kill）
- `IsHealthy()` — 向 `/health` 端點發 GET
- `waitForHealthy(ctx)` — 同步輪詢；deadline = `min(startupTimeout, ctx.Deadline())`
- `supervise()` — 背景 goroutine；崩潰時 backoff 重啟
- `healthURL` — struct 欄位（測試注入用，預設 `http://127.0.0.1:8081/health`，見 `manager.go:40`）

## 已知陷阱

### 啟動 / 生命週期

- **非致命失敗**：若 proxy 啟動失敗，僅記錄警告後繼續，不阻擋 atlas 啟動。
- **Python 依賴**：使用 `~/.config/atlas-go/.fubon-env/bin/python` 或系統 `python3`。
- **健康檢查位址**：使用 IPv4 `127.0.0.1:8081` 而非 `localhost:8081`；Python proxy 綁定 `host="0.0.0.0"`。
- **啟動前 port 探測**：委派給 `internal/portprobe.Probe`。`Probe` 用 wildcards `net.Listen("tcp", "0.0.0.0:<port>")` + `[::]:<port>`（避免 IPv4-only bind 漏掉 IPv6 listener 的偽陰性），**無 side effect**（用 `net.Listen` 而非 `net.Dial`，命中後立刻關閉 listener）。`probePort8081()` 傳入的 addr 來自 package-level var `proxyListenPort`（production 預設 `8081`）。Free → fall through；healthy → 視為外部已管理、跳過 spawn；Foreign → 回傳 actionable error（含 PID 與 kill 指令），**不**進入 supervise 3s backoff-loop。**測試**用 `withFreeEphemeralPort(t)` / `bindEphemeralPort(t, handler)` 覆寫 `proxyListenPort` 到 OS-assigned ephemeral port,避免 Docker Desktop / 系統服務佔 `:8081` 導致測試在開發機上 skip 或 fail;production 預設值不變。
- **殭屍自動清除**：Foreign 占用若判定為 fubon-proxy 殭屍（command name 含 `python`/`uvicorn`），會先 SIGTERM 再 SIGKILL 清除，然後 re-probe。非 Python 程序不會被 auto-kill。
- **macOS zombie 陷阱**：行程被 SIGKILL 後可能變成 zombie，`syscall.Kill(pid, 0)` 對 zombie 仍回 `nil`，必須用 `cmd.Wait()` 真正回收。

### 與 `internal/portprobe` 的界線

所有 port 探測 / 佔用者查詢 / 殭屍判定 / 強制終止邏輯皆委派給 `internal/portprobe`（stateless、獨立測試、可被 `internal/startup.Preflight` 複用）：

| Helper | 委派目標 | 契約 |
|---|---|---|
| `probePort8081()` | `portprobe.Probe(addr)` | 回 `State{Free/Healthy/Foreign}` + `Occupant` |
| `lookupPortOccupant(port)` | `portprobe.LookupOccupant(addr)` | `lsof -FpcL` 解析 PID + command |
| `isFubonZombie(occ)` | `portprobe.IsFubonZombie(occ)` | `python` 或 `uvicorn` lowercase contains |
| `killOccupant(pid)` | `portprobe.KillOccupant(pid)` | SIGTERM → 1s wait → `signal(0)` → SIGKILL + 500ms |

fubonproxy 內只保留兩個型別別名（`portState = portprobe.State`、`portOccupant = portprobe.Occupant`）。改 port 探測 / 殭屍 / kill 行為時**只動 `internal/portprobe`**，勿在本模組重複實作；既有 `manager_test.go` F9 守則測試監測對應的行為契約。

### 程序監督器不變式（F1~F9）

新增或修改 `supervise()` / `Stop()` / `Start()` pre-flight / backoff / 測試時，必須遵守 F1~F9 不變式。完整清單與驗證規則見 **`.claude/skills/atlas-fubon-supervisor-invariants/SKILL.md`**。

簡要原則：

- `Stop()` 必須先設 `m.stopping=true` 再檢查，`supervise()` 啟動新程序後必須在鎖內 re-check。
- `supervise()` 健康檢查必須同步，禁止 fire-and-forget。
- `waitForHealthy()` 必須尊重 `ctx.Deadline()`。
- `exec.Cmd.Start()` 必須在鎖外執行；supervisor 使用 lock-check-unlock-work-lock pattern。
- `Start()` 錯誤路徑必須清理 `m.done`，避免 `Stop()` 永久阻塞。
- `doc.go` 必須與 code 同步（`Start()` 是非同步的）。
- 測試必須驗證正確性（`m.running`、`m.cmd`、process 存活、Stop 後退出），不只時序。

### Stop() 取消路徑

- `cancel()` 會透過 `exec.CommandContext` 立即 SIGKILL cmd。
- 後續的 SIGINT 與 5s `gracefulShutdownTimeout` 為雙重安全網，正常情況下不會觸發。
- 若需要 SIGINT-based 優雅關閉，需改用 `exec.Command` 並把 cancel 路徑改為 SIGINT → wait → SIGKILL 三段式。

### Backoff 觸發條件

- `restartBackoffDelay` (10s) 僅在 `cmd.Start()` 失敗時生效。
- 連環 crash 場景：health check 持續失敗時 backoff 維持 3s，並被 30s `waitForHealthy` timeout 阻塞，整體重啟頻率約 33s 一次。

### 與 `internal/live/fubon_dma.go` 的界線

兩者都用 `exec.CommandContext` 啟動 Python，但**職責不同**。

| | `fubonproxy` (本模組) | `live/fubon_dma` |
|---|---|---|
| 用途 | 啟動/監控 proxy HTTP 服務 | broker DMA wrapper |
| 監督 | supervise() + 自動重啟 | 無（一次性 connect/kill） |
| 健康檢查 | 同步 + 背景 goroutine | 無 |
| Backoff | 3s/10s | 無 |
| 共享 ProcessManager | — | 無 |

**不要在 fubon_dma.go 套用本模組的 audit pattern** — 它沒 supervisor 邏輯，套了會誤報。

## 相依關係

由 `cmd/atlas` API 模式使用（`NewManager(cfg.WorkDir)` + `defer mgr.Stop()`）。`healthURL` 可注入為測試替身。在 `cmd/atlas/main.go` 是獨立於 `internal/live` 的啟動路徑，**兩者可同時使用**。

## 修改前必讀

1. 必跑：`go test -race -count=1 ./internal/fubonproxy/ ./internal/portprobe/ ./internal/startup/` + `go vet ./...` + `staticcheck`
2. 改 supervisor 邏輯或測試：重檢 **`.claude/skills/atlas-fubon-supervisor-invariants/SKILL.md`** 的 F1~F9
3. 改 port 探測 / 殭屍判定 / 強制終止邏輯：只動 `internal/portprobe/`,勿在 fubonproxy 重複實作
4. 改介面：更新 `doc.go` 保持一致
