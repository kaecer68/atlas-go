# Atlas-Go Install / Deploy Guide

> **Audience**: developers onboarding to atlas-go + operators deploying it.
> **Related**: [`../../AGENTS.md`](../../AGENTS.md), [`../../CLAUDE.md`](../../CLAUDE.md), [`../environment.md`](../environment.md), [`../reference/traps.md`](../reference/traps.md)

This guide covers:
1. **Prerequisites** — 3rd party dependencies you must install
2. **Local Development Setup** — clone → run → debug cycle
3. **Environment Variables** — all keys + their sources
4. **Deployment** — local Docker, production via ghcr.io
5. **Pre-Deployment Checklist** — the 4 things you MUST do before going live

If any step fails, check the [Troubleshooting](#troubleshooting) section at the bottom.

---

## 1. Prerequisites

Atlas-go runs in Docker. To install and start the server, you only need:

| Dependency | Version | Why | Install |
|---|---|---|---|
| **Docker** + compose | 24+ | The entire stack (postgres, redis, atlas-go, etc.) runs in containers | [docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop) |
| **git** | 2.30+ | Clone the repo + pre-commit hooks | macOS 預設 |

Everything else (Go 1.26+, Node.js 20+, PostgreSQL 15, Redis 8) is bundled inside the Docker image — you do **not** need to install them on your host.

**Verify your setup**:

```bash
docker --version # 24.x or higher
git --version    # 2.30 or higher
```

If either is missing, install before continuing.

---

## 2. Local Development Setup

### 2.1 Clone the repo

```bash
git clone https://github.com/kaecer68/atlas-go.git
cd atlas-go
```

### 2.2 Install git hooks (CRITICAL)

```bash
bash scripts/install-hooks.sh
```

This installs `.githooks/pre-commit` (5 phases: binary detection, PID files, coverage output, frontend imports, **go generate drift check**) + `.githooks/pre-push` (blocks redundant pushes when HEAD == origin/main or zero diff vs main) into `.git/hooks/`. Without this, the pre-commit Phase 5 won't block manual edits to `admin_web/static/js/shared/field_types.ts` + `valid_fields.json` (and the identical copies in `client_web/` and `shared_web/`), and the pre-push won't catch the "empty branch → closed-as-redundant PR" failure mode (PR #799 lesson).

> **如果略過這步,未來 AI Coding agent 可能手動改這兩個 generated 檔案,被 CI `generate` job 擋下,造成 PR 整批 fail。** 詳見 `shared_web/AGENTS.md` "Generated Files" 章節。

### 2.2a Optional: AI Coding safety net (opencode plugin)

由於 `.opencode/` 是 gitignored（本機設定),每台機器若希望本機 AI agent 享有「修改 `internal/` 或 `cmd/` 前提示呼叫 `atlas-pre-change-protocol` skill」的保護,需自行註冊 plugin。流程:

1. 建立 `<repo>/.opencode/plugins/pre-change-guard.ts` (59 行,內容見下方)
2. 在 `<repo>/.opencode/opencode.json` 的 `plugin` 欄位加入路徑:

```json
{
  "plugin": ["./plugins/pre-change-guard.ts"]
}
```

**plugin source** (複製貼上即可):

```typescript
// .opencode/plugins/pre-change-guard.ts
import type { Plugin } from "@opencode-ai/plugin"

const EDIT_TOOLS = new Set(["edit", "write", "patch"])
const PROTECTED = /\/(internal|cmd)\/[^/]+$/

export const PreChangeGuard: Plugin = async () => {
  const warned = new Set<string>()
  return {
    "tool.execute.before": async (input, output) => {
      if (!EDIT_TOOLS.has(input.tool)) return
      const filePath = output?.args?.filePath
      if (typeof filePath !== "string") return
      if (!PROTECTED.test(filePath)) return
      const key = `${input.tool}:${filePath}`
      if (warned.has(key)) return
      warned.add(key)
      console.warn(
        `\n⚠️  [pre-change-guard] ${input.tool} on protected path:\n   ${filePath}\n` +
        `   BEFORE this edit, invoke: skill(name="atlas-pre-change-protocol") ` +
        `(Step 0 OVERLAP DETECTION)\n`,
      )
    },
  }
}
export default PreChangeGuard
```

**為什麼建議裝**:AI agent 每天都在 internal/ 和 cmd/ 內寫出已存在的 function / type / module,造成浪費的 PR (PR #799 是最近的案例)。這個 plugin 不會阻擋開發,只在 AI 即將犯錯時印顯眼警告。

### 2.3 Set up environment variables

```bash
# Create the env file (one-time per machine)
touch ~/.config/atlas-go/.env
chmod 600 ~/.config/atlas-go/.env

# Edit and add your keys (see Section 3 for what each key is)
$EDITOR ~/.config/atlas-go/.env
```

The `config.Load()` function (in `internal/config/config.go`) reads this file automatically on every backend start.

### 2.4 Install frontend dependencies (2 directories)

```bash
make install     # = install-frontend + go mod download
```

Or per-directory:

```bash
cd admin_web && npm ci
cd client_web && npm ci
```

### 2.5 Start the full stack (Docker compose)

```bash
docker compose up -d   # postgres, redis, atlas-go, prism worker, swarm, grafana, prometheus
```

> **First time?** Docker will build the `atlas-go` image from `Dockerfile` (multi-stage: node → go). Takes ~5-10 minutes.

### 2.5a Native dev workflow with `make dev` (recommended for iteration)

如果你主要在改 Go backend(orchestrator / portfolio / marketdata / experiment 等),用 `make dev` 比 docker compose 迭代快 5-10 分鐘(rebuild docker image)→ 2-3 秒(go run 直接 reload)。

**原理**(2026-06-28 從「用戶每次重複踩同一個坑」事件抽出):

- **postgre / redis 用 docker**(隔離、init scripts、auto-restart)
- **atlas-go 用 native `go run`**(原生 debugger、fast reload)
- **fubon-proxy 由 ProcessManager 自動 spawn**(`internal/fubonproxy/manager.go` + `shouldStartFubonProxy` — 已有完整 lifecycle,不用再寫新的 orchestration)
- **`docker compose stop atlas`** 釋出 port 18080 給 native 進程

**用法**:

```bash
make dev          # 起 docker deps(postgres+redis)+ stop atlas container + go run ./cmd/atlas -api
                  # CTRL+C 結束;postgres/redis 留 docker 跑
make dev-status   # 看容器狀態 + port 18080/18081 占用 + native process
make dev-logs     # tail atlas-go 啟動 log(若 go run 在背景)
make dev-stop     # 收尾:停 docker deps(注意:不會 kill native atlas-go,用 CTRL+C 或 kill <pid>)
```

**前置**:
- `~/.config/atlas-go/.env` 存在(同 2.3)
- `~/.config/atlas-go/.fubon-env/bin/python` 存在(若 `.env` 有 `FUBON_API_KEY`,ProcessManager 會用此 venv 的 Python spawn fubon-proxy)
- Port 18080 在 host 沒被其他程式佔用(`make dev-status` 會查;若被佔用會 error 讓你手動處理 — 因為 ProcessManager 也不 auto-kill 18080 衝突,可能誤殺 Chrome devtools / IDE)

**不該做的事**:
- **不要** `docker compose up -d fubon-proxy` 跟 `make dev` 同時跑 → ProcessManager 看到 18081 healthy 跳過 spawn 看似 OK,但若 docker fubon-proxy 在 restart 中(暫時 unhealthy),ProcessManager 會 spawn 撞 18081 EADDRINUSE 進入 supervisor loop。dev 時讓 ProcessManager 唯一管 fubon-proxy。
- **不要**修改 `internal/fubonproxy/manager.go` 試圖加「auto-kill port 18080 佔用者」邏輯 — port 18080 可能被 Chrome devtools / IDE LISTEN,自動 kill 風險太高。改由 user 手動 `docker compose stop atlas` 即可。

**驗證**(2026-06-28 實測):
```
$ make dev
✅ postgres + redis healthy
✅ port 18080 free
🚀 go run ./cmd/atlas -api
... (啟動 ~5s)
dashboard api listening on :18080
fubonproxy.process_started (spawn native fubon-proxy subprocess)
fubonproxy.health_check_passed (login Fubon SDK 成功)
/health → 200 ✓
```

**為什麼這之前是踩坑**:
- 第一個版本考慮寫新的 wrapper script / 新 Makefile target 起 fubon-proxy → 查 codebase 才發現 ProcessManager + `shouldStartFubonProxy` 已經在做這件事 → 改用 `make dev` 串接,沒寫任何新 Go code
- 詳見 `docs/reference/traps.md` § Dev Workflow / 造輪子陷阱

### 2.6 Verify the stack is healthy

```bash
# Liveness
curl -fsS http://localhost:18080/health
# Expected: {"status":"ok",...}

# LLM provider readiness (deep, requires running pipeline)
curl -fsS http://localhost:18080/api/llm/health
# Expected: {"providers":{"deepseek":{...},"minimax":{...}},"router_version":"..."}

# Container status
docker compose ps
# Expected: atlas-go, atlas-postgres, atlas-redis, atlas-prism-worker all "healthy"
```

### 2.7 Frontend hot-reload (optional, dev only)

```bash
make watch-frontend-admin_web
# (or client_web)

# In another terminal, mount the rebuilt dist/ into the running container
docker compose restart atlas-go
```

This is faster than rebuilding the full Docker image (28-48ms vs 5-10 min).

---

## 3. Environment Variables

The backend reads from `~/.config/atlas-go/.env`. Required variables:

| Variable | Required | Source | Notes |
|---|---|---|---|
| `LLM_DEEPSEEK_API_KEY` | ✅ Yes (for any LLM feature) | https://platform.deepseek.com → API Keys | Used for: narrative, regime, sector agents |
| `LLM_MINIMAX_API_KEY` | ⚠️ Optional | MiniMax M3 coding plan | `sk-cp-` prefix required. If unset, router falls back to DeepSeek. |
| `ATLAS_API_KEY` | ✅ Yes (production) | **Self-generated** (see below) | Atlas-go's own API auth, NOT an LLM key. |
| `ATLAS_ENV` | ⚠️ Auto | Defaults to `development` | Set to `production` only when deploying live trading. |
| `DATABASE_URL` | ⚠️ Auto | Defaults to compose-network URL | Override only for non-Docker deployments. |
| `REDIS_URL` | ⚠️ Auto | Defaults to compose-network URL | Override only for non-Docker deployments. |
| `LLM_ANNOTATOR_API_KEY` | ❌ Deprecated | (legacy compat) | If set, used as fallback for `LLM_MINIMAX_API_KEY`. |

### Generate `ATLAS_API_KEY`

Atlas-go's `internal/monitoring/api/shared/handler.go` requires this when `ATLAS_ENV=production`. It accepts the value via `Authorization: Bearer <key>` or `X-API-Key: <key>` header.

```bash
# 32-byte random key (64 hex chars)
openssl rand -hex 32
# Example output: e4f8c2a9b1d3e5f7...64 chars
```

**Do NOT use the development placeholder** `ATLAS_API_KEY=e2e-test-key-not-for-prod` in production. See [Pre-Deployment Checklist](#5-pre-deployment-checklist).

> **⚠️ Critical gotcha — `env_file` vs `environment` precedence** (見 `docs/reference/traps.md` § Deploy/Docker 完整說明)
>
> `docker-compose.yml` 的 `env_file` 跟 `environment` **兩個 section 對同一個變數會衝突**:`environment` 段優先。
> `environment: ATLAS_API_KEY=${ATLAS_API_KEY}` 是 **shell 變數展開**(讀 host shell 的 `ATLAS_API_KEY`,**不是從 .env 讀**)。
> 如果 host shell 沒設 → 展開成空字串 → 把 `env_file` 段從 .env 讀到的值**覆蓋掉** → AuthMiddleware 永遠回 503。
>
> **正確做法**:`docker-compose.yml` 的 `environment` 段**不要放**需要從 .env 讀的變數,完全依賴 `env_file` 段提供。
> CI/CD 也應該把 secrets 寫進 .env(透過 secret mount),不要靠 shell env(會跟 env_file 機制衝突)。

---

## 4. Deployment

> **部署真相（2026-08-15 方案二定案）**：開發在 MacBook、production 在 iMac。
> iMac 用**本地 build image**（`atlas-atlas:latest`），**不使用 ghcr.io pull**（舊模式已淘汰）。
> 完整部署流程見 [`docs/operations/local-deploy.md`](../operations/local-deploy.md)。

### 4.1 Local dev (MacBook — 本機驗證)

```bash
# 1. 本地 build + 起容器（完整 stack）
make rebuild-all

# 2. 驗證
docker compose ps
curl -fsS http://localhost:18080/health
```

> 本機容器與 iMac production 不同機器、不同 port 空間，**互不影響**。驗證完可 `docker compose down` 停止。

### 4.2 Production (iMac — 唯一部署機)

**流程**：MacBook push → iMac `git pull` → iMac `make rebuild-all` → 驗證。

```bash
# On the production host (iMac)
cd ~/workspace/atlas
git pull origin main
make rebuild-all

# Verify
docker compose ps  # all containers "healthy"
curl -fsS http://localhost:18080/health
curl -fsS http://localhost:18080/api/llm/health
```

> **hermes 代勞**：部署是 hermes（iMac 運維員）的職責，可透過 hermes-dispatch skill 派她執行
> `git pull → make rebuild-all → 驗證 → 回報`。
>
> **Note**: atlas-go is a **single-host Docker deployment**, not a Kubernetes cluster. For multi-host / cloud-managed, you'd need to refactor `docker-compose.yml` to a Helm chart or similar.

---

## 5. Pre-Deployment Checklist

Before any production deployment, ALL of these must be true:

### ✅ 1. `ATLAS_API_KEY` is the production value, NOT the e2e placeholder

```bash
grep ^ATLAS_API_KEY ~/.config/atlas-go/.env
# ❌ WRONG: ATLAS_API_KEY=e2e-test-key-not-for-prod
# ✅ CORRECT: ATLAS_API_KEY=<your-64-char-random-hex>
```

If you deployed with the placeholder, the `AuthMiddleware` will still work in dev (returns 200 because `isProduction=false`), but **anyone who reads this guide could access your API** in production. Fix immediately.

### ✅ 2. `ATLAS_ENV=production` is set on production hosts

```bash
# In docker-compose.yml on production host:
environment:
  - ATLAS_ENV=production
```

When `ATLAS_ENV=production` AND `ATLAS_API_KEY=""`, the backend returns:
```json
HTTP 503 {"error":"server misconfigured: ATLAS_API_KEY required in production"}
```

### ✅ 3. `.git/hooks/pre-commit` is aligned with the current `.githooks/pre-commit`

```bash
diff .githooks/pre-commit .git/hooks/pre-commit
# (empty = aligned)

# If not aligned:
bash scripts/install-hooks.sh
```

The pre-commit hook has 5 phases including the **go generate drift check** (added in PR #796) that physically blocks commits of manual edits to `admin_web/static/js/shared/field_types.ts` and `valid_fields.json` (plus the identical copies in `client_web/` and `shared_web/`). Skipping this step means AI coding agents (or humans) can silently break CI.

### ✅ 4. Don't manually edit `admin_web/static/js/shared/field_types.ts` or `valid_fields.json`

These two files are **auto-generated by `cmd/gentags`** from `internal/*/*.go` struct JSON tags. Manual edits:
- Get overwritten on next `go generate ./...`
- Cause `quality.yml` `generate` job to fail with "uncommitted changes" → block every frontend PR

**If you need to add / change / remove a frontend field**, the correct flow is:

```bash
# 1. Edit the Go struct in internal/<pkg>/<file>.go
#    e.g., add: type Foo struct { NewField string `json:"new_field"` }
$EDITOR internal/domain/foo.go

# 2. Run go generate to regenerate the 2 files
go generate ./...

# 3. Stage the regenerated files (NOT manual edits)
git add admin_web/static/js/shared/field_types.ts admin_web/static/js/shared/valid_fields.json \
        client_web/static/js/shared/field_types.ts client_web/static/js/shared/valid_fields.json \
        shared_web/static/js/shared/field_types.ts shared_web/static/js/shared/valid_fields.json
git commit -m "feat(domain): add NewField to Foo"
```

For full details see `shared_web/AGENTS.md` "Generated Files" 章節 and `docs/reference/traps.md` "Build Pipeline / 程式碼生成".

---

## Troubleshooting

### LLM provider returns 401 / 429

Check the env file:
```bash
grep -E "LLM_.*_API_KEY" ~/.config/atlas-go/.env
# Verify each key is non-empty and starts with the correct prefix
```

### Frontend shows "載入中…" indefinitely

1. Open DevTools → Network tab
2. Check if `/api/...` requests are returning 200 or timing out
3. If 200 but stuck loading: check browser console for JS errors (the page's `init` function may not be wired)
4. If 503: check `docker compose logs atlas-go` for the AuthMiddleware rejection (you may need to set `X-API-Key` header in dev)

### `go generate` produces changes that won't go away

You're trying to manually edit a generated file. The `pre-commit` Phase 5 will block this. Read `shared_web/AGENTS.md` "Generated Files" and edit the corresponding Go struct instead.

### `make ci` hangs on a particular script

Some scripts (e.g., `check_data_naming.sh`, `check_layer3_benchmarks.sh`) take 30+ seconds. `make ci` has a 30s per-script timeout — if a script exceeds that, it's skipped. Use `make ci-slow` to run them individually with no timeout.

### `docker compose up` fails with "port already in use"

```bash
lsof -ti :18080 | xargs kill -9   # kill anything on 18080
docker compose down              # stop compose-managed containers
docker compose up -d
```

---

## Reference: File Locations

| What | Where |
|---|---|
| Root `Makefile` (build/test/lint/ci targets) | `./Makefile` |
| Backend entry | `./cmd/atlas/main.go` |
| Active frontend apps | `./admin_web/`, `./client_web/` |
| Shared frontend resources | `./shared_web/` |
| Environment config loader | `./internal/config/config.go` |
| Auth middleware (checks `ATLAS_API_KEY`) | `./internal/monitoring/api/shared/handler.go` |
| Docker stack | `./docker-compose.yml` |
| Git hooks source | `./.githooks/` |
| Pre-commit drift check (Phase 5) | `./.githooks/pre-commit` line 80+ |
| CI workflow | `./.github/workflows/ci-cd.yml` |
| Test cmd/atlas job (20min timeout) | `./.github/workflows/ci-cd.yml` line ~150 |

---

## Related Guides

- [`new-workspace-startup.md`](new-workspace-startup.md) — 3-minute workspace bootstrap SOP
- [`git-tool-cache-policy.md`](git-tool-cache-policy.md) — which tool caches to never commit
- [`ai-productivity.md`](ai-productivity.md) — execution tips for AI agents
- [`opencode-oh-my-openagent-tuning.md`](opencode-oh-my-openagent-tuning.md) — token injection防护
- [`../reference/traps.md`](../reference/traps.md) — high-stakes traps reference
- [`../environment.md`](../environment.md) — this dev environment's status
