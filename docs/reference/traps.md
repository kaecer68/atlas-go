# traps.md — 高危陷阱參考

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
| **JSONL ledger append 必須包在 Mutex 內** | ledger | `internal/ledger/event_flow_prediction_store.go` 的 `AppendPrediction` 採 read-modify-write 整段進 `sync.Mutex`（line 61-62）。**不要**為了效能改寫成 append-only mode(`os.O_APPEND`) — append-only 模式在 concurrent write 仍可能交錯，且無法實作 1000 筆 FIFO 上限。若未來引入 rotation，rotation 邏輯也必須在 lock 下跑（reader thread 不能在 rename 進行中讀到一半的檔案）。並發防護由 `TestJSONLEventFlowPredictionStore_ConcurrentAppendsNoLoss` 守住（PR #1129，5 goroutine × 50 append，驗證 250 筆唯一 `PredictedAt`）— 跑 `go test -race` 才會抓出 lock-not-held 的 race detector hit。 |
| **`CalendarEvent.EventType` 必須 set 不可省** | industry / data | `buildEventFromRule` (L971)、`buildHolidayEvent` (L1027)、`buildPositionBuildingEvent` (L1049)、`buildElectionEvent`、`buildMSCIEvent`、`buildTW50Event` 等每個 `CalendarEvent` constructor **都必須 set `EventType: rule.EventType`**。下游 `IsTaiwanTradingDay` 在 PR #1128 commit `0dcd159b` 起改用 `evt.EventType == string(EventLongHoliday)` typeFilter(取代先前 `strings.HasPrefix(..., "連假 - ")` 名稱前綴)。**任何**新增的 `build*Event` helper 若忘記設 EventType,**`IsTaiwanTradingDay` 會 silently 漏判** — 連假不再被識別為非交易日,`event-calendar-sparse` alert 會因春節/228/國慶連假誤觸發。防護:每新增一個 `build*Event`,新增一個對應的 unit test 驗證 `EventType` 欄位非空。 |

### Orchestrator / Control

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **GuardOutcomes 與 outcomes 必須對齊** | orchestrator | 控制層（CIO）輸出應**保留原始 Agent ID**，不可覆寫為自己的 ID，否則 `PassedGuards` 會全部變 `false`。 |
| **OutcomeCount 必須是單場次數量** | ledger | `RecordSessionSummary` 絕對不可用 `ledger.LoadOutcomes()`（讀取全域檔案）來填 `OutcomeCount`。 |
| **同一件事不可有三種算法** | orchestrator | 放行/過濾筆數必須由單一權威來源（如 `GuardOutcomes`）計算，前端不可各自重算。 |
| **Darwinian 權重靜默夾制** | portfolio | 權重限制在 `[0.3, 2.5]`，超界會靜默正規化，不報錯。 |
| **重複使用 mutable `[]Recommendation`** | sim | 多次 simulation run 之間不可共用同一個 slice。 |
| **Yahoo Provider range=1y 產出 YoY 而非 daily change** | marketdata | US 股票/指數 provider 若使用 `range: "1y"` + `prev := closes[0]`，會計算「年增率 (YoY)」而非「日增率 (daily change)」，導致 ChangePct 出現 +84.9% 等荒謬數值。正確模式：`range: "5d"` + `prev := closes[len(closes)-2]`，並對 `abs(changePct) > 30%` 做 bounds reject。詳見 PR #948。 |

### 架構規範（Constitution 違反）

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **繞過共變異數優化回到線性加權** | portfolio | `optimizer.go` 已升級為 Ledoit-Wolf 共變異數矩陣 + Active-set QP。當 `o.history` 非 nil 時必須走共變異數路徑。修改 optimizer 前必須先閱讀 `../constitution.md`。 |
| **繞過 BackgroundTaskManager 建立獨立排程** | apigateway | 所有定時任務**必須且只能**透過 `BackgroundTaskManager` 註冊。禁止在 goroutine 中直接啟動 `time.Ticker`。參見 `internal/apigateway/CONSTITUTION.md` 第四條。 |
| **Once-guard 是 process-local 狀態,重啟後會 double-fire** | scheduler / apigateway | Stage 3 的 5 個排程任務（`sync-events-daily` / `sync-macro-daily` 等）內部 `dailyOnceGuard` / `weeklyOnceGuard` / `monthlyOnceGuard` 採 closure-local `var lastRun time.Time`（`internal/scheduler/stage3_tasks.go:199`, `:217`, `:237`)。daemon 在排程時間點（例:06:00 整）被 OOM kill 或 `docker compose restart` 重啟,**新的 process 該變數從零開始**,第一個 tick 又會 fire → 同一個交易日 **double-run**。**修法**:Stage 3.1+ 改用 `OncestampStore` 持久化記錄到 `data/ledger/stage3_oncestamps.json`（`internal/scheduler/stage3_oncestamps.go`,atomic .tmp+rename）。若 daemon fallback `OncestampStore == nil`（檔案無法創建）**不會**保留 lastRun,只退化成 in-memory 行為 — 此時仍會 double-run。監控:`atlas_stage3_task_runs_total{task=X}` 若同一日連續看到 2 次 success → oncestamp store 壞了,查 `[Stage3] oncestamp store unavailable` log。 |
| **繞過 ParametersConfig 硬編碼參數** | config | 所有可調整參數必須透過 `internal/config/parameters.go` 管理，禁止 magic number。參數必須包含 `Rationale`、`Source`、`Todo`。 |
| **建立獨立資料抓取通道** | marketdata | 所有外部資料抓取必須通過已註冊的 `marketdata.Provider`，禁止直接建立 HTTP client。參見 `internal/apigateway/CONSTITUTION.md` 第一條。 |
| **新增 internal/ 模組未標記成熟度** | 跨模組 | 每個 `internal/*/` Go package **必須**有 `doc.go` 含 `// Maturity: <tier>`。同時更新 `internal/MATURITY.md`。CI 強制。 |
| **新增/刪除/改名 FactorType** | portfolio | 因子變更必須同步更新 **8 個位置**。CI `factor-integrity` job 強制。 |
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
| **`docker-compose.yml` `command:` 跟 Dockerfile `ENTRYPOINT:` 重複** | deploy | 當 Dockerfile 用 exec form `ENTRYPOINT: ["/app/atlas-go"]` 時,docker-compose 的 `command` **只能是 args**,**不能包含 binary path**。若寫成 `command: ["/app/atlas-go", "prism", "worker"]`,Docker 會把 ENTRYPOINT 跟 CMD 串接 → 實際命令變成 `/app/atlas-go /app/atlas-go prism worker`,binary 收到 `os.Args[1] = "/app/atlas-go"`(路徑)而不是 `"prism"`,subcommand 路由永遠 false → fall through 到 `runSimulation()`(one-shot)→ 60s 重啟循環。正確寫法:`command: ["prism", "worker"]`。目前 `atlas` 服務是 `command: ["-api", "-live"]`(只有 flag 沒路徑,所以沒踩到);`prism-worker` 已修(2026-06-28);`swarm-runner` 已於 2026-07 隨 MiroFish Swarm 模擬引擎降級而移除(PR #963)。 |
| **`environment: VAR=${VAR}` 會 shadow `env_file`** | deploy | Docker Compose 的 `env_file` 跟 `environment` 兩個 section 對同一個變數的行為是:**`environment` 段優先**(後者贏)。`environment` 段的 `${VAR}` 是 **shell 變數展開**(讀 host shell 的 `VAR`,不是讀 .env),如果 host shell 沒設就會展開成空字串,把 .env 裡的值蓋掉。具體案例:`docker-compose.yml` 的 `atlas` 服務曾有 `ATLAS_API_KEY=${ATLAS_API_KEY}`,host shell 沒設 → 容器內 `ATLAS_API_KEY=""` → `AuthMiddleware` 觸發 503(`server misconfigured: ATLAS_API_KEY required in production`)。正確做法:**不要在 `environment` 段設需要從 .env 讀的變數**,讓 `env_file` 唯一負責。CI/CD 也應該把 secrets 寫進 .env,不要靠 shell env(會跟 env_file 語意衝突)。 |
| **Dockerfile 的硬編碼 `HEALTHCHECK` 會繼承給所有用同 image 的服務** | deploy | `Dockerfile` line 92-93 寫死 `HEALTHCHECK CMD curl -f http://localhost:18080/health || exit 1`。這個 endpoint 只有 `atlas` 服務(用 `-api` 啟動)才會 listen;`prism-worker`(跑 `prism worker` daemon)跟 `cron-*` 服務都沒有 HTTP server 在 18080,繼承這個 healthcheck 會**永遠失敗**,被 Docker 視為 unhealthy。修法:在 `docker-compose.yml` 對非 API 服務用 `healthcheck: disable: true` 或自訂 healthcheck 指令。目前已對 `prism-worker` 套用 `healthcheck: disable: true`(2026-06-28);`cron-*` 仍繼承壞的 healthcheck(因 cron 是週期性 one-shot,無所謂)。 |
| **`docker compose stop <name>` 用 service name,**不是** container_name** | deploy | `docker-compose.yml` 內 `service:`(頂層 key)是 docker compose 用來管理生命週期的識別;`container_name:` 只是 Docker 的 container alias。`docker compose stop atlas-fubon-proxy` 會回 `no such service: atlas-fubon-proxy`(因為 service 是 `fubon-proxy`)。要 `docker compose stop fubon-proxy`。混用造成「看起來指令成功但實際啥都沒停」的 silent failure,下一步 ProcessManager 在 port 18081 撞到仍活著的 docker fubon-proxy 進入 supervisor loop(18081 探測委派給 `internal/portprobe.Probe`,見 `internal/fubonproxy/AGENTS.md` 啟動前 port 探測段)。**避坑**:永遠看 `docker-compose.yml` 頂層的 `services:` 區塊找 service name,不是 `container_name:`。 |
| **atlas-go 已有 ProcessManager 管 fubon-proxy,不要重造** | fubonproxy / deploy | `internal/fubonproxy/manager.go`(756 行,F1~F9 invariants)+ `cmd/atlas/bootstrap_helpers.go:131 shouldStartFubonProxy(mode, fubonAPIKey)` 已自動 spawn Python fubon-proxy subprocess,含 port 18081 pre-flight probe(委派 `internal/portprobe.Probe`,見套件 `doc.go`)、zombie auto-kill(`internal/portprobe.IsFubonZombie` + `KillOccupant`)、supervisor backoff、graceful shutdown。`shouldStartFubonProxy` 在 `mode=="live" \|\| fubonAPIKey!=""` 才 spawn。**不要**寫新的 wrapper script / Makefile target 來起 fubon-proxy,**也不要**在 dev workflow 跑 `docker compose up -d fubon-proxy`(會跟 ProcessManager 撞 port 18081)。正確 dev workflow:docker compose 只起 postgres + redis,讓 ProcessManager 自動管 fubon-proxy subprocess(`make dev` target 已是此模式)。 |

### Dev Workflow / 造輪子陷阱

| 陷阱 | 模組 | 說明 |
|------|------|------|
| **無 manifest 的除錯 / 修復** | 跨模組 | 任何 bug hunt 或審計若未先寫成 manifest，容易遺漏問題、過度修復、或留下未驗證的 code。強制載入 `atlas-audit-manifest-protocol`，為每個問題建立 ID、根因假設、檔案、驗收方式，再動手改 code。 |
| **造輪子前先搜尋既有 infrastructure** | 跨模組 | 在新增任何「自動起 fubon-proxy」「自動連 postgres」「port 衝突處理」之前,**務必**先 `grep -rnE` 整個 codebase 是否已有 ProcessManager / Bootstrap / BackgroundTaskManager 對應的 supervisor。每個 lifecycle pattern 都有對應 module:`internal/fubonproxy`(fubon-proxy)、`internal/apigateway/background.go`(background tasks)、`internal/bootstrap/`(bootstrap orchestration)。直接造新的會:(a) 跟既有 module 重複職責;(b) 破壞 F1~F9 invariants;(c) 為「找到正確 module + 重新閱讀」額外浪費時間。**Worked example**:2026-06-28 用戶要求「`go run ./cmd/atlas -api` 一鍵啟動」,初版考慮寫新的 wrapper script → 查 `shouldStartFubonProxy` 才發現 ProcessManager 已自動處理 fubon-proxy spawn,只需用 `make dev` 串接 docker deps + 讓 port 18080 → ProcessManager 自動接手 — **沒有寫任何新 Go code**。 |
| **新增 user-auth / login wall / tier gate 前必讀 [`docs/specs/guest-mode-spec.md`](../specs/guest-mode.md)** | subscription / web | atlas-go 已有 canonical user-auth bypass pattern(PR #1084)— backend `ATLAS_REQUIRE_USER_AUTH` env var + frontend `GUEST_MODE` const 雙旗標。任何「加 SSO / OAuth / 登入 / 登入驗證 / 認證 / auth / user / tier / member / 帳號 / free tier 升級 / 商業化 / 付費牆」任務**必須沿用**這個 pattern,只翻兩個旗標,不需新 middleware。常見錯誤:(a) 從零實作新 JWT middleware,(b) 寫 dev-mode token / magic header bypass 平行路徑,(c) hardcode token 跳過 auth,(d) 刪除 `/api/auth/*` 或 `/api/user/*` 端點(已驗證保留,翻旗標即可恢復)。完整 SOP、4 步翻轉驗證計畫、禁止事項見 spec 文件;AI agent 自動觸發指引見 [`.claude/skills/atlas-guest-mode/SKILL.md`](../../.claude/skills/atlas-guest-mode/SKILL.md)。 |
| **`atlas-go` 啟動 postgres race 一定要用 `depends_on: condition: service_healthy`** | apigateway / deploy | `docker-compose.yml` 的 `atlas` service 若不依賴 postgres/redis healthcheck,docker compose 預設**平行**起所有服務 → atlas 啟動時 ping postgres 還沒 ready → `SQLSTATE 57P03 (database system is starting up)` → bootstrap 走 warning 路徑但首次連線失敗。修法:`atlas:` 加 `depends_on: postgres: condition: service_healthy`(及 redis),前提是 postgres/redis **必須有 healthcheck**(postgres 用 `pg_isready`,redis 用 `redis-cli ping`)。沒 healthcheck 就無法等 service_healthy,docker compose 會直接報「no healthcheck configured」失敗。 |
| **本機 dev 不要 `docker compose up -d fubon-proxy`** | fubonproxy | 若 docker compose 起了 fubon-proxy(在 18081),又 `go run` atlas-go,ProcessManager 透過 `internal/portprobe.Probe` 看到 port 18081 已有 healthy fubon-proxy → 跳過 spawn(這步是對的);**但**如果 dev 時 docker fubon-proxy 還沒 healthy(例如剛 restart 中),`Probe` 進入 foreign 分支 → ProcessManager 拒絕 spawn 並回 actionable error(不啟動自己的副本,避免 EADDRINUSE)→ supervisor loop。**最簡單**:dev 時 `docker compose up -d postgres redis`(省略 fubon-proxy),完全讓 ProcessManager 管。若 production 部署已驗證 docker fubon-proxy + `host.docker.internal:18081` 通了,維持 docker compose 統一管理也行,但 dev 不要混用。<br><br>**PR #943 變更**：fubon-proxy 的 `/health` 端點改為快速 process-only check（不呼叫上游 Fubon API，永遠回 200 只要 FastAPI 活著）。若需驗證上游連線，改用 `/health/deep`。Dockerfile HEALTHCHECK 也改用 `/health`，container 不再因上游短暫斷線顯示 unhealthy。 |
| **fubon-proxy `/health` 不再驗證上游連線** | fubonproxy | PR #943 後 `/health` 改為 process-only check（快速回 200），不再呼叫上游 Fubon API。若程式碼（如 ProcessManager `IsHealthy()`）依賴 `/health` 回 503 來判斷上游異常，需改用 `/health/deep` 或 fubon-proxy log 來偵測。**但** ProcessManager 的既有邏輯（/health=200 = proxy 活著、port 被正常佔用 → 跳過 spawn）反而因此**正確性提升** — 不再因上游故障將活的 proxy 誤判為 foreign port。 |

### Build Pipeline / 程式碼生成

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **手動編輯 `field_types.ts` 或 `valid_fields.json`**（存在於 3 個 active web 目錄） | domain / web / 全前端 | 這兩個檔案由 `cmd/gentags` 從 `internal/*/*.go` 的 struct JSON tag 自動產出。`go generate .` 會同時輸出到 **3 個 active web 目錄** (`admin_web/`、`client_web/`、`shared_web/`)。`web/` 已 deprecated，不再由 gentags 更新。<br><br>**禁止手動編輯任何一份** — 任何變更會在下次 `go generate` 被覆寫。<br><br>若需新增/修改/刪除前端可見的欄位或介面:<br>1. 修改對應 Go struct 的 `json:\"...\"` tag(在 `internal/<pkg>/`)<br>2. 跑 `go generate .` 重新產出全部 3 份<br>3. **不要**直接編輯這兩個檔<br><br>違反的後果:`go generate .` 會覆寫你的手動編輯,並且會在 quality.yml 的 `generate` job 報 "uncommitted changes" → frontend PR 全 CI fail。<br><br>防護:`.githooks/pre-commit` Phase 5 自動跑 `go generate .`,若**任一 copy** 有 drift 會**阻擋 commit**。修正方式見 `shared_web/AGENTS.md`「Generated Files」章節。 |

---

## 啟動異常關閉連環事件(2026-06-28)

5 個服務(prism-worker、grafana、alertmanager、otel-collector、atlas-go 的 `/health`)同時 crash loop 的完整根因分析見:

**[`docs/../investigations/2026-06-28-boot-loop-multi-service.md`](../investigations/2026-06-28-boot-loop-multi-service.md)**

涵蓋:docker-compose `command` 跟 ENTRYPOINT 衝突、`env_file` vs `environment` precedence、Dockerfile 硬編碼 healthcheck 繼承、PRISM system 實作但不完整、fubon-neo PyPI 404、兩個 .env 模板 stale orphan。

---

## Health Endpoint 401 防回歸(2026-07-03,PR #931 修復)

**症狀**:`curl http://localhost:18080/api/llm/health` 回 `{"error":"unauthorized"}`(HTTP 401)。

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
- `docs/specs/wave9-observability-spec.md` §7
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
- `docs/specs/wave9-observability-spec.md` §5
- `internal/monitoring/startup_metrics.go`

---

## v0.0.0.31 Auth/Tier/Cookie 陷阱（Wave 11 PR #972 + #974）

Phase B/C 引入 `internal/subscription`（3-tier JWT 認證）+ `internal/recommender`（tier-gated API）+ `client_web` Phase A0/A（401 interceptor + tier-gated home dashboard）。**任何繞過 tier middleware 或錯誤處理 Cookie Secure 旗標都會導致越權存取**，以下列出必須遵守的契約：

### 1. tier middleware bypass（絕對禁止）

`internal/subscription/handler.go` 提供 `ValidateTier(minTier)` middleware，保護 `/api/recommendations` 等敏感端點。**繞過情境與後果**：

- **錯誤示範**：`mux.HandleFunc("GET /api/recommendations", h.HandleRecommendations)` 直接註冊，沒有套 `ValidateTier("public")` wrapper
- **後果**：未登入使用者可直接呼叫，繞過 tier 限制，premium 級內容（進出場點位、深度回測）外洩
- **正確做法**：所有 sensitive endpoint 必須透過 `ValidateTier(minTier)` middleware，且 `minTier` 必須是該 endpoint 設計的最低 tier（不能是 `premium`，否則 registered user 會被擋下）

### 2. Cookie Secure 旗標（local dev vs production）

`internal/subscription/handler.go::Login` 設定 cookie 時使用 `Secure: false`，這是**本機開發**（無 HTTPS）的取捨。**production 部署必須改為 `Secure: true`**：

- 開發環境：`Secure: false` + `//nolint:gosec` 標註（CI 不會 fail）
- Production：`Secure: true` + `SameSite=Strict` + 加 `__Host-` prefix
- **檢查方式**：搜尋 `setSecure.*false|Secure:\s*false` 應只出現在 dev 配置文件

### 3. JWT 過期與 refresh（v0.0.0.31 已知 P1 殘留）

`internal/subscription/jwt.go::Generate` 發出 HS256 token，TTL 24h。**目前沒有 refresh endpoint**，過期後使用者必須重新登入：

- **症狀**：登入 24 小時後呼叫 `/api/recommendations` 突然回 401，前端 tier badge 從 `premium` 變回 `free`
- **影響**：client_web 的 `invalidateAuth()` 會被呼叫，UI 退回 login shell
- **Workaround（v0.0.0.32 計畫）**：加 `/api/auth/refresh` + sliding window JWT

### 4. 401 interceptor 與 `silentGetJSON` 衝突

`client_web/static/js/services/auth.js::invalidateAuth()` 會在收到 401 時清掉 cookie + 重導。但 `shared/app-utils.js::silentGetJSON()` 設計為**不 throw**，會安靜返回 `null`：

- **正確組合**：`getJSON(url)` 用於需要 401 處理的 endpoint（如 `/api/recommendations`），`silentGetJSON(url)` 用於可降級的 endpoint（如 `/api/capital-flow/summary` 失敗仍可顯示 partial dashboard）
- **錯誤組合**：對 `silentGetJSON('/api/recommendations')` 結果呼叫 `if (recs.tier === 'premium')` 永遠 false，因為 401 時返回 null
- **規則**：tier-gated API 一律用 `getJSON`，不要用 `silentGetJSON`

### 5. CLI `atlas register` 未實作（v0.0.0.31 已知）

`POST /api/auth/register` 僅透過 HTTP API 暴露。**CLI 無對應指令**。若需要 headless 註冊：

- **臨時**：直接打 `curl -X POST /api/auth/register -d '{"email":"...","password":"..."}'`
- **永久**：v0.0.0.32 計畫加 `cmd/atlas-user`

### 6. defaultProvider fallback 不寫檔（dailyreport）

`internal/dailyreport/report.go::defaultProvider` 在缺少真實 DataProvider 時返回 `NoDataAvailable: true` 但**不寫入磁碟**：

- **症狀**：`/api/reports/latest` 在剛啟動且 scheduler 還沒跑完時回 503
- **正確處理**：handler 應回 HTTP 503 + `Retry-After: 60`，而非 500
- **檢查**：呼叫端用 `if (res.status === 503) showFallback()` 而非 `if (res.status === 200)`

---

## AI-Generated Doc 處理原則 (2026-07-08, followup.md §1 反思)

**陷阱**：`followup.md` / `docs/specs/*.md` / `docs/operations/*.md` 等**都是 AI coding agent 寫的**，可能是過時或決策本身可挑戰的（**不是 human owner 的 hard rule**）。AI agent（包括未來的我）容易把它們當成不可挑戰的權威來引用。

**失敗案例**（2026-07-08 L2.4 prep session）：
- `docs/operations/l2-4-followup.md §1` 寫「Auto-cron 是否現在可以開始實作？否」
- 我把這個「否」當成 hard rule 來擋 T13 main 的實作，連續 7+ 次拒絕
- 實際上這個決策本身**就是 AI 寫的**，應該被視為 proposal 而非 gospel
- User 提醒後我才修正：**followup.md 都是 AI coding 的時候，agent 寫的，所以正確與否若有問題，你可以及時討論**

**協議**（任何 AI-generated doc 與 code 衝突時）：

1. **讀 doc 看它說什麼**（可能過時）
2. **讀 code 看實際是什麼**（ground truth）
3. **衝突時**：
   - **標記 doc 過時**：在 doc 旁加 `> ⚠️ 已被 <commit/PR> 取代` 註記
   - **修 code 或 doc**：不擋自己的 work 等 doc 同步
   - **如果決策本身可疑**（如「否,不要現在做」）：直接問 user 是否要 override，不要假裝它是 hard rule

**判斷哪些 doc 是 AI 寫的**：
- `docs/operations/l2-4-*.md` — L2.4 prep session 期間由 AI agent 撰寫
- `docs/specs/*.md` — 設計文件,可能混合 human owner + AI 補充
- `docs/observations/*.md` — 觀察日誌範本,可能是 AI 模板
- `docs/archive/*.md` — 已被歸檔的 AI-generated docs(明確標示「已完成」)
- 對照 `[docs/../documentation-map.md](../documentation-map.md)` 確認文件歸屬與維護者

**不要擋 work 的情境**：
- Doc 說「不能做 X」但你 grep code 沒看到實際阻擋邏輯
- Doc 引用不存在的 PR# 或文件路徑
- Doc 的「前提」清單跟現況對不上（prereqs 未被驗證,但 doc 把「未做」當作「禁止做」）

**這個協議也適用於**：
- Session 內的同類型決策（user 早期同意的 scope 可能已變,不必死守）
- 跨 session 的 lesson（user 在某 session 同意的做法,可能在新 session 已經過時）

---

## 模組特定陷阱

以下陷阱屬於特定模組範圍，詳見集群 AGENTS.md 或 root `AGENTS.md` 陷阱速查表：

- **Portfolio**: 權重、FactorEngine、FactorType 變更流程 → root `AGENTS.md` 陷阱表
- **Orchestrator**: 三層 executor 路由、GuardOutcomes 對齊 → `internal/orchestrator/AGENTS.md`
- **Live**: 交易安全旗標 → `internal/live/AGENTS.md`
- **MarketData**: Provider 註冊規則 → `internal/marketdata/AGENTS.md`
- **Experiment**: Mutation → judge → promote 生命週期 → root `AGENTS.md` 陷阱表
- **Baseline**: 升降級與版本控制 → root `AGENTS.md` 陷阱表
- **Monitoring**: Dashboard API、人工干預 → `internal/monitoring/AGENTS.md`
- **Narrative**: 宏觀敘事、因果鏈 → root `AGENTS.md` 陷阱表

---

## 文件歸屬規則

本文檔會隨項目演進持續更新。新增陷阱時，請判斷：

1. **跨模組**（影響 2+ 模組、無歸屬單一模組）→ 加入本文件
2. **單一模組** → 加入該模組的 `internal/<mod>/AGENTS.md`
3. **CI/流程相關** → 可能歸屬 `.github/instructions/` 下的領域守則
