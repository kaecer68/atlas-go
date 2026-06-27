# Atlas-Go Install / Deploy Guide

> **Audience**: developers onboarding to atlas-go + operators deploying it.
> **Related**: [`../../AGENTS.md`](../../AGENTS.md), [`../../CLAUDE.md`](../../CLAUDE.md), [`../ENVIRONMENT.md`](../ENVIRONMENT.md), [`../TRAPS.md`](../TRAPS.md)

This guide covers:
1. **Prerequisites** — 3rd party dependencies you must install
2. **Local Development Setup** — clone → run → debug cycle
3. **Environment Variables** — all keys + their sources
4. **Deployment** — local Docker, production via ghcr.io
5. **Pre-Deployment Checklist** — the 4 things you MUST do before going live

If any step fails, check the [Troubleshooting](#troubleshooting) section at the bottom.

---

## 1. Prerequisites

Atlas-go is a Go backend + 3-frontend (esbuild) + Docker infra stack. You need:

| Dependency | Version | Why | Install |
|---|---|---|---|
| **Go** | 1.26+ | Backend `cmd/atlas` + all `internal/*` packages | `brew install go` (macOS) or [go.dev/dl](https://go.dev/dl) |
| **Node.js** | 20+ | `npm run build` for `web/` + `admin_web/` + `client_web/` | `brew install node@20` or [nodejs.org](https://nodejs.org) |
| **Docker** + compose | 24+ | Local dev: postgres, redis, prometheus, grafana, atlas-go, prism worker, swarm runner | [docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop) |
| **PostgreSQL** | 15 | Persistent storage (`atlas-go` database) | (optional) local install; docker-compose 內含 |
| **Redis** | 8 | Event bus + session state | (optional) local install; docker-compose 內含 |
| **Python** | 3.13+ | Fubon SDK proxy (live trading only) | macOS 預設 3.14; Fubon venv 用 3.13 |
| **git** | 2.30+ | Worktree workflow + pre-commit hooks | macOS 預設 |
| **gh** CLI | 2.0+ | PR / issue workflows | `brew install gh` + `gh auth login` |

> **Why these versions?** Go 1.26 is the minimum for `slices.Concat` + `maps.Collect` used in `internal/orchestrator`. Node 20+ for esbuild 0.25+. PostgreSQL 15 + Redis 8 match `docker-compose.yml`.

**Verify your setup**:

```bash
go version       # go1.26 or higher
node --version   # v20.x or higher
docker --version # 24.x or higher
gh --version     # 2.x
```

If any are missing, install before continuing.

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

This installs `.githooks/pre-commit` (5 phases: binary detection, PID files, coverage output, frontend imports, **go generate drift check**) into `.git/hooks/`. Without this, the pre-commit Phase 5 won't block manual edits to `web/static/js/shared/field_types.ts` + `valid_fields.json`.

> **如果略過這步,未來 AI Coding agent 可能手動改這兩個 generated 檔案,被 CI `generate` job 擋下,造成 PR 整批 fail。** 詳見 `web/AGENTS.md` "Generated Files" 章節。

### 2.3 Set up environment variables

```bash
# Create the env file (one-time per machine)
touch ~/.config/atlas-go/.env
chmod 600 ~/.config/atlas-go/.env

# Edit and add your keys (see Section 3 for what each key is)
$EDITOR ~/.config/atlas-go/.env
```

The `config.Load()` function (in `internal/config/config.go`) reads this file automatically on every backend start.

### 2.4 Install frontend dependencies (3 directories)

```bash
make install     # = install-frontend + go mod download
```

Or per-directory:

```bash
cd web        && npm ci
cd admin_web && npm ci
cd client_web && npm ci
```

### 2.5 Start the full stack (Docker compose)

```bash
docker compose up -d   # postgres, redis, atlas-go, prism worker, swarm, grafana, prometheus
```

> **First time?** Docker will build the `atlas-go` image from `Dockerfile` (multi-stage: node → go). Takes ~5-10 minutes.

### 2.6 Verify the stack is healthy

```bash
# Liveness
curl -fsS http://localhost:8080/health
# Expected: {"status":"ok",...}

# LLM provider readiness (deep, requires running pipeline)
curl -fsS http://localhost:8080/api/llm/health
# Expected: {"providers":{"deepseek":{...},"minimax":{...}},"router_version":"..."}

# Container status
docker compose ps
# Expected: atlas-go, atlas-postgres, atlas-redis, atlas-prism-worker, atlas-swarm-runner all "healthy"
```

### 2.7 Frontend hot-reload (optional, dev only)

```bash
make watch-frontend-admin_web
# (or client_web / web)

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

---

## 4. Deployment

### 4.1 Local (Docker compose)

For local dev / staging on the same machine:

```bash
# 1. Pull latest image
docker compose pull atlas-go

# 2. Restart
docker compose up -d atlas-go

# 3. Verify
docker compose ps
curl -fsS http://localhost:8080/health
```

The image tag in `docker-compose.yml` follows `ghcr.io/kaecer68/atlas-go:<git-sha>`. Update by:
```bash
# Edit docker-compose.yml to set the desired image tag
docker compose pull atlas-go
docker compose up -d atlas-go
```

### 4.2 Production (ghcr.io)

The CI (`ci-cd.yml` main branch) auto-builds and pushes to `ghcr.io/kaecer68/atlas-go:<sha>`. To deploy:

```bash
# On the production host
cd atlas-go
git pull origin main
docker compose pull atlas-go
docker compose up -d atlas-go

# Verify
docker compose ps  # all containers "healthy"
curl -fsS http://<host>:8080/health
curl -fsS http://<host>:8080/api/llm/health
```

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

The pre-commit hook has 5 phases including the **go generate drift check** (added in PR #796) that physically blocks commits of manual edits to `web/static/js/shared/field_types.ts` and `valid_fields.json`. Skipping this step means AI coding agents (or humans) can silently break CI.

### ✅ 4. Don't manually edit `web/static/js/shared/field_types.ts` or `valid_fields.json`

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
git add web/static/js/shared/field_types.ts web/static/js/shared/valid_fields.json
git commit -m "feat(domain): add NewField to Foo"
```

For full details see `web/AGENTS.md` "Generated Files" 章節 and `docs/TRAPS.md` "Build Pipeline / 程式碼生成".

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

You're trying to manually edit a generated file. The `pre-commit` Phase 5 will block this. Read `web/AGENTS.md` "Generated Files" and edit the corresponding Go struct instead.

### `make ci` hangs on a particular script

Some scripts (e.g., `check_data_naming.sh`, `check_layer3_benchmarks.sh`) take 30+ seconds. `make ci` has a 30s per-script timeout — if a script exceeds that, it's skipped. Use `make ci-slow` to run them individually with no timeout.

### `docker compose up` fails with "port already in use"

```bash
lsof -ti :8080 | xargs kill -9   # kill anything on 8080
docker compose down              # stop compose-managed containers
docker compose up -d
```

---

## Reference: File Locations

| What | Where |
|---|---|
| Root `Makefile` (build/test/lint/ci targets) | `./Makefile` |
| Backend entry | `./cmd/atlas/main.go` |
| Frontend configs | `./web/`, `./admin_web/`, `./client_web/` |
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
- [`../TRAPS.md`](../TRAPS.md) — high-stakes traps reference
- [`../ENVIRONMENT.md`](../ENVIRONMENT.md) — this dev environment's status
