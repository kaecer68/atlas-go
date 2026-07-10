# AGENTS.md — internal/fubonproxy

Fubon-proxy 生命週期管理：non-blocking supervisor pattern，`Start()` 立即返回，背景 goroutine 處理健康檢查與崩潰重啟。

## 關鍵符號

- `ProcessManager`：Python 程序管理器。
- `Start(ctx)`：非同步啟動；失敗不阻擋 atlas 啟動。
- `Stop()`：同步停止（cancel → SIGINT + 5s graceful + 強制 kill）。
- `waitForHealthy(ctx)`：同步輪詢；deadline = `min(startupTimeout, ctx.Deadline())`。
- `supervise()`：背景 goroutine；崩潰時 backoff 重啟。
- `healthURL`：測試注入，預設 `http://127.0.0.1:18081/health`。

## fubon-proxy URL 單一來源

- `defaultProxyHost` = `"fubon-proxy"`（Docker DNS）或 `127.0.0.1`（本機）。
- `GetFubonProxyPort()` / `SetFubonProxyPort(port)`：port 讀取與注入。
- `ProxyBaseURL()`：HTTP client 的 canonical URL。
- `ProxyHostPort()`：`net.Dial` / `net.Listen` probe 的 canonical host:port。
- **禁止**其他 `.go` 檔案自行 `fmt.Sprintf("http://...:%d")` 構造；`TestFubon_URLDriftGuard` AST 禁制會擋下。

## 與 `internal/portprobe` 的界線

所有 port 探測 / 佔用者查詢 / 殭屍判定 / 強制終止邏輯皆委派給 `internal/portprobe`：

| Helper | 委派目標 |
|---|---|
| `probeProxyPort()` | `portprobe.Probe(addr)` |
| `lookupPortOccupant(port)` | `portprobe.LookupOccupant(addr)` |
| `isFubonZombie(occ)` | `portprobe.IsFubonZombie(occ)` |
| `killOccupant(pid)` | `portprobe.KillOccupant(pid)` |

改 port 探測 / 殭屍 / kill 行為時**只動 `internal/portprobe`**。

## 程序監督器不變式（F1~F9）

新增或修改 `supervise()` / `Stop()` / `Start()` 時必須遵守。詳見 `.claude/skills/atlas-fubon-supervisor-invariants/SKILL.md`。

- `Stop()` 先設 `m.stopping=true` 再檢查；`supervise()` 啟動新程序後在鎖內 re-check。
- `supervise()` 健康檢查必須同步，禁止 fire-and-forget。
- `waitForHealthy()` 尊重 `ctx.Deadline()`。
- `exec.Cmd.Start()` 在鎖外執行；supervisor 使用 lock-check-unlock-work-lock pattern。
- `Start()` 錯誤路徑清理 `m.done`，避免 `Stop()` 永久阻塞。

## Stop() 與 Backoff 契約

- `cancel()` 透過 `exec.CommandContext` 立即 SIGKILL；SIGINT + 5s 為雙重安全網。
- `restartBackoffDelay` (10s) 僅在 `cmd.Start()` 失敗時生效。
- 連環 crash：health check 持續失敗時 backoff 維持 3s，整體重啟頻率約 33s 一次。

## 修改前必讀

- 改 supervisor：重檢 F1~F9 skill。
- 改 port 探測：只動 `internal/portprobe/`。
- 改介面：更新 `doc.go`。
- 必跑：`go test -race -count=1 ./internal/fubonproxy/ ./internal/portprobe/ ./internal/startup/` + `go vet ./...` + `staticcheck`。
