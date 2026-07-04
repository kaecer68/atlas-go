# TRAPS.md — 高危陷阱參考

> 此文件為 `AGENTS.md` 陷阱節的詳細擴充。根 AGENTS.md 僅保留最關鍵的跨模組陷阱；模組特定陷阱請見 `internal/*/AGENTS.md`。

---

## 跨模組陷阱

### Data / Persistence

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **Replay 格式錯誤** | ledger | Replay 為 **JSONL**（每行獨立 JSON 物件），不是 JSON array。 |
| **Session 日期不可信賴 `RecordedAt`** | domain | `RecordedAt` 是計算完成時間。排序/比較請以 `SessionID` 中的交易日為準。 |
| **JSON tag 大小寫錯誤** | domain / API | API handler 讀取 JSONL 時，若 anonymous struct 的 JSON tag 用了 PascalCase 而 JSON 實際是 snake_case，unmarshal 會靜默失敗。 |
| **Scorecard OOS 欄位遺漏同步** | domain / ledger / monitoring | `domain.Scorecard` 新增 OOS 欄位（`rolling_sharpe_trend` 等）時，必須同步更新 `ledger.BuildScorecards()` 計算邏輯、`internal/monitoring/dashboard_api.go` 的 response mapping、`web-ui/js/dashboard.js` 的前端渲染，以及 `internal/web/field_types.go` 的 `go generate` 自動生成。遺漏任何一環會導致 OOS 欄位在 API 或前端消失。 |

### Orchestrator / Control

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **GuardOutcomes 與 outcomes 必須對齊** | orchestrator | 控制層（CIO）輸出應**保留原始 Agent ID**，不可覆寫為自己的 ID，否則 `PassedGuards` 會全部變 `false`。 |
| **OutcomeCount 必須是單場次數量** | ledger | `RecordSessionSummary` 絕對不可用 `ledger.LoadOutcomes()`（讀取全域檔案）來填 `OutcomeCount`。 |
| **同一件事不可有三種算法** | orchestrator | 放行/過濾筆數必須由單一權威來源（如 `GuardOutcomes`）計算，前端不可各自重算。 |
| **Darwinian 權重靜默夾制** | portfolio | 權重限制在 `[0.3, 2.5]`，超界會靜默正規化，不報錯。 |
| **重複使用 mutable `[]Recommendation`** | sim | 多次 simulation run 之間不可共用同一個 slice。 |

### 架構規範（Constitution 違反）

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **繞過共變異數優化回到線性加權** | portfolio | `optimizer.go` 已升級為 Ledoit-Wolf 共變異數矩陣 + Active-set QP。當 `o.history` 非 nil 時必須走共變異數路徑。修改 optimizer 前必須先閱讀 `docs/CONSTITUTION.md`。 |
| **繞過 BackgroundTaskManager 建立獨立排程** | apigateway | 所有定時任務**必須且只能**透過 `BackgroundTaskManager` 註冊。禁止在 goroutine 中直接啟動 `time.Ticker`。參見 `internal/apigateway/CONSTITUTION.md` 第四條。 |
| **繞過 ParametersConfig 硬編碼參數** | config | 所有可調整參數必須透過 `internal/config/parameters.go` 管理，禁止 magic number。參數必須包含 `Rationale`、`Source`、`Todo`。 |
| **建立獨立資料抓取通道** | marketdata | 所有外部資料抓取必須通過已註冊的 `marketdata.Provider`，禁止直接建立 HTTP client。參見 `internal/apigateway/CONSTITUTION.md` 第一條。 |
| **新增 internal/ 模組未標記成熟度** | 跨模組 | 每個 `internal/*/` Go package **必須**有 `doc.go` 含 `// Maturity: <tier>`。同時更新 `internal/MATURITY.md`。CI 強制。 |
| **新增/刪除/改名 FactorType** | portfolio | 因子變更必須同步更新 **7 個位置**。CI `factor-integrity` job 強制。 |
| **Save() 吃掉校準時間戳** | config | `ParametersConfig.Save()` 會覆寫整個 `parameters.json`，導致 raw JSON 欄位靜默遺失。所有校準器必須同時寫入 `last_calibrated`（Go struct）和 `calibration_timestamp`（raw JSON）。 |

### Config / Agent 治理

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **Enabled agent 缺少 prompt** | spawning | `configs/agents.json` 中每個 `enabled: true` 都需對應 `prompts/agents/<name>.md`。CI `agent-prompts` job 強制。 |
| **ScreeningCriteria 靜默過濾** | screener | `configs/agents.json` 中若設定了 `screening_criteria`，標的在進入 executor **之前**就會被過濾。這是預期行為，不是 bug。 |
| **Live 交易風險** | live | `cmd/atlas` 有 `-allow-live-broker`、`-allow-real-signor` 等旗標，本地測試時切勿意外啟用。 |

### Baseline / Experiment

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **Baseline 未載入** | baseline | 實驗執行/評估前必須確認 `data/state/baseline_policy.json` 存在且有效。 |

### Deploy / Docker

| 陷阱 | 模組 | 說明 |
|------|------|------|
| **`docker-compose.yml` `command:` 跟 Dockerfile `ENTRYPOINT:` 重複** | deploy | 當 Dockerfile 用 exec form `ENTRYPOINT: ["/app/atlas-go"]` 時,docker-compose 的 `command` **只能是 args**,**不能包含 binary path**。若寫成 `command: ["/app/atlas-go", "prism", "worker"]`,Docker 會把 ENTRYPOINT 跟 CMD 串接 → 實際命令變成 `/app/atlas-go /app/atlas-go prism worker`,binary 收到 `os.Args[1] = "/app/atlas-go"`(路徑)而不是 `"prism"`,subcommand 路由永遠 false → fall through 到 `runSimulation()`(one-shot)→ 60s 重啟循環。正確寫法:`command: ["prism", "worker"]`。目前 `atlas` 服務是 `command: ["-api", "-live"]`(只有 flag 沒路徑,所以沒踩到);`prism-worker` 跟 `swarm-runner` 已修(2026-06-28)。 |
| **`environment: VAR=${VAR}` 會 shadow `env_file`** | deploy | Docker Compose 的 `env_file` 跟 `environment` 兩個 section 對同一個變數的行為是:**`environment` 段優先**(後者贏)。`environment` 段的 `${VAR}` 是 **shell 變數展開**(讀 host shell 的 `VAR`,不是讀 .env),如果 host shell 沒設就會展開成空字串,把 .env 裡的值蓋掉。具體案例:`docker-compose.yml` 的 `atlas` 服務曾有 `ATLAS_API_KEY=${ATLAS_API_KEY}`,host shell 沒設 → 容器內 `ATLAS_API_KEY=""` → `AuthMiddleware` 觸發 503(`server misconfigured: ATLAS_API_KEY required in production`)。正確做法:**不要在 `environment` 段設需要從 .env 讀的變數**,讓 `env_file` 唯一負責。CI/CD 也應該把 secrets 寫進 .env,不要靠 shell env(會跟 env_file 語意衝突)。 |
| **Dockerfile 的硬編碼 `HEALTHCHECK` 會繼承給所有用同 image 的服務** | deploy | `Dockerfile` line 92-93 寫死 `HEALTHCHECK CMD curl -f http://localhost:8080/health || exit 1`。這個 endpoint 只有 `atlas` 服務(用 `-api` 啟動)才會 listen;`prism-worker`(跑 `prism worker` daemon)跟 `cron-*` 服務都沒有 HTTP server 在 8080,繼承這個 healthcheck 會**永遠失敗**,被 Docker 視為 unhealthy。修法:在 `docker-compose.yml` 對非 API 服務用 `healthcheck: disable: true` 或自訂 healthcheck 指令。目前已對 `prism-worker` 套用 `healthcheck: disable: true`(2026-06-28);`cron-*` 仍繼承壞的 healthcheck(因 cron 是週期性 one-shot,無所謂)。 |
| **`docker compose stop <name>` 用 service name,**不是** container_name** | deploy | `docker-compose.yml` 內 `service:`(頂層 key)是 docker compose 用來管理生命週期的識別;`container_name:` 只是 Docker 的 container alias。`docker compose stop atlas-fubon-proxy` 會回 `no such service: atlas-fubon-proxy`(因為 service 是 `fubon-proxy`)。要 `docker compose stop fubon-proxy`。混用造成「看起來指令成功但實際啥都沒停」的 silent failure,下一步 ProcessManager 在 port 18081 撞到仍活著的 docker fubon-proxy 進入 supervisor loop(18081 探測委派給 `internal/portprobe.Probe`,見 `internal/fubonproxy/AGENTS.md` 啟動前 port 探測段)。**避坑**:永遠看 `docker-compose.yml` 頂層的 `services:` 區塊找 service name,不是 `container_name:`。 |
| **atlas-go 已有 ProcessManager 管 fubon-proxy,不要重造** | fubonproxy / deploy | `internal/fubonproxy/manager.go`(756 行,F1~F9 invariants)+ `cmd/atlas/bootstrap_helpers.go:131 shouldStartFubonProxy(mode, fubonAPIKey)` 已自動 spawn Python fubon-proxy subprocess,含 port 18081 pre-flight probe(委派 `internal/portprobe.Probe`,見套件 `doc.go`)、zombie auto-kill(`internal/portprobe.IsFubonZombie` + `KillOccupant`)、supervisor backoff、graceful shutdown。`shouldStartFubonProxy` 在 `mode=="live" \|\| fubonAPIKey!=""` 才 spawn。**不要**寫新的 wrapper script / Makefile target 來起 fubon-proxy,**也不要**在 dev workflow 跑 `docker compose up -d fubon-proxy`(會跟 ProcessManager 撞 port 18081)。正確 dev workflow:docker compose 只起 postgres + redis,讓 ProcessManager 自動管 fubon-proxy subprocess(`make dev` target 已是此模式)。 |

### Dev Workflow / 造輪子陷阱

| 陷阱 | 模組 | 說明 |
|------|------|------|
| **造輪子前先搜尋既有 infrastructure** | 跨模組 | 在新增任何「自動起 fubon-proxy」「自動連 postgres」「port 衝突處理」之前,**務必**先 `grep -rnE` 整個 codebase 是否已有 ProcessManager / Bootstrap / BackgroundTaskManager 對應的 supervisor。每個 lifecycle pattern 都有對應 module:`internal/fubonproxy`(fubon-proxy)、`internal/apigateway/background.go`(background tasks)、`internal/bootstrap/`(bootstrap orchestration)。直接造新的會:(a) 跟既有 module 重複職責;(b) 破壞 F1~F9 invariants;(c) 為「找到正確 module + 重新閱讀」額外浪費時間。**Worked example**:2026-06-28 用戶要求「`go run ./cmd/atlas -api` 一鍵啟動」,初版考慮寫新的 wrapper script → 查 `shouldStartFubonProxy` 才發現 ProcessManager 已自動處理 fubon-proxy spawn,只需用 `make dev` 串接 docker deps + 讓 port 8080 → ProcessManager 自動接手 — **沒有寫任何新 Go code**。 |
| **`atlas-go` 啟動 postgres race 一定要用 `depends_on: condition: service_healthy`** | apigateway / deploy | `docker-compose.yml` 的 `atlas` service 若不依賴 postgres/redis healthcheck,docker compose 預設**平行**起所有服務 → atlas 啟動時 ping postgres 還沒 ready → `SQLSTATE 57P03 (database system is starting up)` → bootstrap 走 warning 路徑但首次連線失敗。修法:`atlas:` 加 `depends_on: postgres: condition: service_healthy`(及 redis),前提是 postgres/redis **必須有 healthcheck**(postgres 用 `pg_isready`,redis 用 `redis-cli ping`)。沒 healthcheck 就無法等 service_healthy,docker compose 會直接報「no healthcheck configured」失敗。 |
| **本機 dev 不要 `docker compose up -d fubon-proxy`** | fubonproxy | 若 docker compose 起了 fubon-proxy(在 18081),又 `go run` atlas-go,ProcessManager 透過 `internal/portprobe.Probe` 看到 port 18081 已有 healthy fubon-proxy → 跳過 spawn(這步是對的);**但**如果 dev 時 docker fubon-proxy 還沒 healthy(例如剛 restart 中),`Probe` 進入 foreign 分支 → ProcessManager 拒絕 spawn 並回 actionable error(不啟動自己的副本,避免 EADDRINUSE)→ supervisor loop。**最簡單**:dev 時 `docker compose up -d postgres redis`(省略 fubon-proxy),完全讓 ProcessManager 管。若 production 部署已驗證 docker fubon-proxy + `host.docker.internal:18081` 通了,維持 docker compose 統一管理也行,但 dev 不要混用。 |

### Build Pipeline / 程式碼生成

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **手動編輯 `field_types.ts` 或 `valid_fields.json`**（存在於 3 個 active web 目錄） | domain / web / 全前端 | 這兩個檔案由 `cmd/gentags` 從 `internal/*/*.go` 的 struct JSON tag 自動產出。`go generate .` 會同時輸出到 **3 個 active web 目錄** (`admin_web/`、`client_web/`、`shared_web/`)。`web/` 已 deprecated，不再由 gentags 更新。<br><br>**禁止手動編輯任何一份** — 任何變更會在下次 `go generate` 被覆寫。<br><br>若需新增/修改/刪除前端可見的欄位或介面:<br>1. 修改對應 Go struct 的 `json:\"...\"` tag(在 `internal/<pkg>/`)<br>2. 跑 `go generate .` 重新產出全部 3 份<br>3. **不要**直接編輯這兩個檔<br><br>違反的後果:`go generate .` 會覆寫你的手動編輯,並且會在 quality.yml 的 `generate` job 報 "uncommitted changes" → frontend PR 全 CI fail。<br><br>防護:`.githooks/pre-commit` Phase 5 自動跑 `go generate .`,若**任一 copy** 有 drift 會**阻擋 commit**。修正方式見 `shared_web/AGENTS.md`「Generated Files」章節。 |

---

## 啟動異常關閉連環事件(2026-06-28)

5 個服務(prism-worker、grafana、alertmanager、otel-collector、atlas-go 的 `/health`)同時 crash loop 的完整根因分析見:

**[`docs/investigations/2026-06-28-boot-loop-multi-service.md`](investigations/2026-06-28-boot-loop-multi-service.md)**

涵蓋:docker-compose `command` 跟 ENTRYPOINT 衝突、`env_file` vs `environment` precedence、Dockerfile 硬編碼 healthcheck 繼承、PRISM system 實作但不完整、fubon-neo PyPI 404、兩個 .env 模板 stale orphan。

---

## Health Endpoint 401 防回歸(2026-07-03,PR #931 修復)

**症狀**:`curl http://localhost:8080/api/llm/health` 回 `{"error":"unauthorized"}`(HTTP 401)。

**根因**:`/api/llm/health` 是觀測型端點,需在兩處**同步**列入 auth-free path,只改一處仍 401:

1. `internal/monitoring/api/shared/handler.go` 的 `authFreeExactPaths` map(直接被 `AuthMiddleware` 檢查)
2. `cmd/atlas/main.go` 的 `isPublicPath()` switch case(在 top-level mux final 繞過 AuthMiddleware)

**修法**:兩處都加,順序:

```go
// 檔案 1: internal/monitoring/api/shared/handler.go
var authFreeExactPaths = map[string]bool{
    "/health":          true,
    "/metrics":         true,
    "/admin":           true,
    "/client":          true,
    "/api/llm/health":  true,  // ← 必加
}

// 檔案 2: cmd/atlas/main.go
func isPublicPath(p string) bool {
    switch {
    case p == "/" || p == "/health" || p == "/ready" || p == "/metrics":
        return true
    case p == "/api/llm/health":  // ← 必加
        return true
    case p == "/admin" || ...
    }
}
```

**為何需要兩處**:handler.go 的 `authFreeExactPaths` 給 `Adapt()` 與其他直接 wrap `AuthMiddleware` 的呼叫端用;main.go 的 `isPublicPath` 是 top-level mux 繞過,語意不同但需保持 sync(handler.go 註解已明示)。

**回歸偵測**:本地或 CI 加一個 grep guard:

```bash
test -n "$(grep -c '"/api/llm/health"' internal/monitoring/api/shared/handler.go)" || \
  (echo "ERROR: /api/llm/health missing from authFreeExactPaths" && exit 1)
test -n "$(grep -c '/api/llm/health' cmd/atlas/main.go)" || \
  (echo "ERROR: /api/llm/health missing from isPublicPath" && exit 1)
```

**T11 E2E 驗證**:PR #931 merge 後,rebuild `atlas` image,`curl /api/llm/health` 應回 200 與 providers 狀態(200 + `{"providers":{...}}`)。若回 401 即表示有步驟遺漏。

**參考**:
- PR #931:commit `82e26982`
- `docs/specs/wave9-observability.md` §7
- `docs/operations/wave9-runbook.md` §3.4

---

## Prometheus Metric 命名空間(2026-07-03,PR #926 + Issue #927)

**症狀**:`/metrics` 端點回 200 但 body 為空,**或** alert rule 永遠不觸發(dead code)。

**根因**:atlas-go 的 Prometheus 框架就位但業務邏輯端從未 increment counter,或使用無 `atlas_` 前綴的 metric 名稱(與 Prometheus default metric 衝突)。

**修法**:新增 metric 必須遵循 `atlas_<feature>_<measurement>_total` 格式:

| 情境 | 範例 |
|------|------|
| ✅ 正確 | `atlas_db_init_failures_total`, `atlas_channel_health_errors_total` |
| ❌ 錯誤 | `db_init_failures_total`(無前綴,可能與 Prom default 衝突), `channel_errors_total`(Issue #927 經典案例,dead code) |

**規則**:
1. `_total` 後綴標示 counter(Prometheus 慣例)
2. `atlas_` 前綴避免衝突
3. label 名小寫 snake_case;值域受限避免 cardinality 爆炸
4. helper function 必須 nil collector 安全(bootstrap 早期 collector 可能尚未建立,參考 `startup_metrics.go` 的 `RecordDBInitFailure`)

**回歸偵測**:CI `generate` job 已檢查 Go struct JSON tag drift;建議加 `monitoring/rules/*` 對 metric 名稱的 grep guard(Issue #927 那種 dead reference 自動偵測)。

**參考**:
- PR #926:commit `9d9a1502`
- Issue #927:`channel_errors_total` dead code 案例
- `docs/specs/wave9-observability.md` §5
- `internal/monitoring/startup_metrics.go`

---

## 模組特定陷阱

以下陷阱屬於特定模組範圍，詳見各模組的 `AGENTS.md`：

- **Portfolio**: 權重、FactorEngine、FactorType 變更流程 → `internal/portfolio/AGENTS.md`
- **Orchestrator**: 三層 executor 路由、GuardOutcomes 對齊 → `internal/orchestrator/AGENTS.md`
- **Live**: 交易安全旗標 → `internal/live/AGENTS.md`
- **MarketData**: Provider 註冊規則 → `internal/marketdata/AGENTS.md`
- **Experiment**: Mutation → execute → judge → promote 生命週期 → `internal/experiment/AGENTS.md`
- **Baseline**: 升降級與版本控制 → `internal/baseline/AGENTS.md`
- **Monitoring**: Dashboard API、人工干預 → `internal/monitoring/AGENTS.md`
- **Narrative**: 宏觀敘事、因果鏈 → `internal/narrative/AGENTS.md`

---

## 文件歸屬規則

本文檔會隨項目演進持續更新。新增陷阱時，請判斷：

1. **跨模組**（影響 2+ 模組、無歸屬單一模組）→ 加入本文件
2. **單一模組** → 加入該模組的 `internal/<mod>/AGENTS.md`
3. **CI/流程相關** → 可能歸屬 `.github/instructions/` 下的領域守則
