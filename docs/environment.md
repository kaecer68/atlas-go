# Environment Status

> **AI MUST READ THIS FIRST** before assuming any external dependency is missing.
>
> Last verified: 2026-07-12 (Wave 11 / v0.0.0.32).
>
> **Story so far**: v0.0.0.17's 5-second dry-run smoke test confirmed the Wave 9 observability wire powered on; v0.0.0.18 (PR #704) closed the integration-test gaps (simulation-only SSE subscriptions, partial-start detector leaks, dropped parallel-start errors) and added v2 + chain integration tests plus idempotency for `risk.NewAuditSubscriber`. Subsequent Waves (9→11) shipped subscription/JWT (PR #972), capital-flow/eventdriven tools, and the 91-tool atlas-mcp catalog.
> All items marked "✅ SET UP" have been verified working in this dev environment.
> For older sessions or different machines, re-run the verification commands in each section.

This document exists because the Wave 9 observability wire work repeatedly hit the issue:
**AI sees code paths referencing external systems (Fubon SDK, PostgreSQL, etc.) and
assumes they need to be set up from scratch, wasting time and risking re-creation of
existing infrastructure.**

The truth: in this dev environment (`kaecer68`'s machine), most external dependencies
are pre-configured at known paths. This document records their current state.

## TL;DR — Quick Status Table

| Dependency | Status | Path / Command |
|---|---|---|
| Fubon SDK (Python venv + cert + .env) | ✅ SET UP | `~/.config/atlas-go/.fubon-env/` |
| PostgreSQL 15 | ✅ SET UP | `/opt/homebrew/bin/psql` |
| Redis 8 | ✅ SET UP | `/opt/homebrew/bin/redis-cli` |
| Go 1.26 | ✅ SET UP | `go version` |
| Python 3.14 (system) | ✅ SET UP | `/opt/homebrew/bin/python3` |
| Python 3.13 (Fubon venv) | ✅ SET UP | `~/.config/atlas-go/.fubon-env/bin/python` |
| macOS Keychain (secrets) | ✅ IN USE | `security find-generic-password ...` |
| Atlas data dir | ✅ SET UP | `/Users/kaecer/workspace/atlas/data/` |

**If any of the above shows "❌ NOT SET UP" when you verify, do not recreate — consult
the user first. These were carefully configured.**

---

## Environment Isolation Contract

> **IRON RULE:** `ATLAS_ENV=production` must only exist in a dedicated, isolated
> worktree or deployment target. Never mix production configuration with the same
> worktree used for development, experiments, backtests, or `go run` debugging.

### `ATLAS_ENV` semantics

| Value | Meaning | Worktree type |
|---|---|---|
| `development` (default) | Local dev, safe to `go run`, experiment, debug | Any feature/dev worktree |
| `staging` | Pre-production validation, may touch real data but read-only or sandboxed | Dedicated staging worktree |
| `production` | Live deployment, real accounts, real money / real market impact | **Isolated production worktree ONLY** |

### Agent contract

When working in this repo:

1. **Verify `ATLAS_ENV` before any state-changing command.**
   ```bash
   echo "ATLAS_ENV=${ATLAS_ENV:-development}"
   ```
2. **If `ATLAS_ENV=production`:**
   - Do NOT run `go run`, `go test`, `make dev`, `make test`, `make watch-frontend`,
     `make setup-mcp*`, or any experiment/backfill CLI (`run-experiment`,
     `judge-experiment`, `promote-baseline`, `backtest-window`, etc.).
   - Do NOT run `docker compose build/up` from a local worktree.
   - Do NOT edit `.env` files directly; use the deployment runbook.
   - Production commands are limited to built artifacts and deployment-specific
     scripts documented in `docs/deployment/`.
3. **If the current branch is `main` and `ATLAS_ENV=production`:**
   - STOP. Production deploys must come from a tagged release or deployment
     pipeline, not a local `main` checkout.
4. **Cross-environment confusion is a known failure mode.** Agents must not assume
   that a command safe in `development` is safe in `production`. The deny-dangerous
   hook (`.agent-hooks/deny-dangerous.sh`) enforces these rules when
   `ATLAS_ENV=production`.

### Why this matters

Historical incidents include agents switching between "dev" and "production"
contexts within the same CLI session, running dev-only experiment commands
against production credentials, and pushing local `.env` overrides to production
worktrees. This contract exists to make the environment boundary explicit and
machine-enforceable.

---

## Fubon SDK (the recurring question)

**Path**: `~/.config/atlas-go/.fubon-env/`

**SDK 來源(非公開 PyPI)**:富邦新一代 API 的官方 Python SDK,**從未放在公開 PyPI**。必須於 [`https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk`](https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk) 簽署 API 服務申請書後,從 `https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/` 手動下載 wheel。Dockerfile 用 `curl` + `pip install <wheel>` 在 build 階段處理。

**Contents** (verified 2026-06-28):

| File | Purpose |
|---|---|
| `bin/python` (3.13.7) | Python interpreter for Fubon proxy |
| `bin/pip` + fubon_neo 2.2.8 | Official Fubon Python SDK(從官方 wheel 安裝) |
| `M120628569_20270620.p12` | Personal certificate for DMA login |
| `lib/python3.13/site-packages/fubon_neo/` | SDK source(`_fubon_neo.abi3.so` + Python 模組) |
| `pyvenv.cfg` | venv config (system-site-packages enabled) |

**⚠️ 重要:`fubon-neo` 不是公開 PyPI 套件(不是下架,是從未在上面)**

- `https://pypi.org/pypi/fubon-neo` 回 404 — 因為這個 package **本來就不在 PyPI**,它是富邦證券內部 SDK
- `https://pypi.org/simple/fubon-neo/` 同樣 404
- 因此 **`pip install fubon-neo==2.2.8` 一定會失敗** — 不是版本問題,是來源問題
- 唯一可用來源:官方 wheel 下載頁
- **不要** 假裝 SDK 「下架了」或試圖用「舊版」從 PyPI 裝 — 從來就沒有 PyPI 版本

**Wheel 平台分發(官方提供)**

| 平台 | wheel 檔名 | Python 支援 |
|------|------------|------------|
| Windows x64 | `fubon_neo-2.2.8-cp37-abi3-win_amd64.zip` | 3.8-3.13 |
| macOS arm64 | `fubon_neo-2.2.8-cp37-abi3-macosx_11_0_arm64.zip` | 3.8-3.13 |
| macOS x86_64 | `fubon_neo-2.2.8-cp37-abi3-macosx_10_12_x86_64.zip` | 3.8-3.13 |
| Linux x64 | `fubon_neo-2.2.8-cp37-abi3-manylinux_2_17_x86_64.manylinux2014_x86_64.zip` | 3.8-3.13 |

「cp37-abi3」是 wheel tag,意思是 stable ABI from Python 3.7+;不是 Python 3.7 only。所以 manylinux_2_17 wheel 跟 Python 3.13 是相容的(因為 3.13 >= 3.7,abi3 向前相容)。

**Docker 部署的 build 機制**(`Dockerfile`):

```dockerfile
ARG FUBON_NEO_WHEEL_URL=https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/fubon_neo-2.2.8-cp37-abi3-manylinux_2_17_x86_64.manylinux2014_x86_64.zip
RUN curl -fsSL "${FUBON_NEO_WHEEL_URL}" -o /tmp/fubon_neo.zip \
    && unzip -j /tmp/fubon_neo.zip '*.whl' -d /tmp/fubon_wheel \
    && pip install --no-cache-dir /tmp/fubon_wheel/*.whl
```

Dockerfile 必須用 `python:3.13-slim`(對齊官方支援範圍 3.8-3.13),不要用 3.12(manylinux wheel 雖然 abi3 向前相容,但 3.13 是官方文件列的支援版本)。

**`.p12` 憑證 mount**(`docker-compose.yml`):

```yaml
fubon-proxy:
  volumes:
    # mount 整個 .fubon-env(只為 .p12),路徑對齊 main.py:_find_cert 搜尋路徑
    - ~/.config/atlas-go/.fubon-env:/home/appuser/.config/atlas-go/.fubon-env:ro
```

SDK 本身由 Dockerfile 從官方 wheel install(不靠 mount);只有 `.p12` 憑證需要 mount。

**升級 SDK 版本**:
1. 到 [官方下載頁](https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk) 確認新版本 + wheel URL
2. 改 `docker-compose.yml` 的 `args:` (FUBON_NEO_VERSION + FUBON_NEO_WHEEL_URL)
3. `docker compose build fubon-proxy` — 重新從官方拉 wheel install

**Account** (from `.env` + Keychain):
- Name: 詹博凱
- Branch: 20909
- Account: 1835440
- Type: stock (DMA)

**Verify setup**:
```bash
ls ~/.config/atlas-go/.fubon-env/bin/python  # should exist
~/.config/atlas-go/.fubon-env/bin/pip list | grep fubon  # should show fubon_neo 2.2.8
ls ~/.config/atlas-go/.fubon-env/*.p12  # should show certificate file
```

**Verify runtime** (start atlas -api, look for):
```bash
/tmp/atlas-bin -api 2>&1 | grep "venv_python_found\|fubon_proxy_not_reachable\|Login successful"
# Expected: "venv_python_found path=~/.config/atlas-go/.fubon-env/bin/python"
# Then either "fubon_proxy_not_reachable" (proxy not running yet) or "Login successful"
```

**Use in this dev environment**:
```bash
# Dashboard mode (safe, dry-run broker)
/tmp/atlas-bin -api

# Live mode (requires explicit flags per AGENTS.md:95)
/tmp/atlas-bin -live -allow-live-broker -allow-real-signer -allow-http-broker
```

**DO NOT**:
- ❌ Recreate the venv — `fubon_neo-2.2.8-cp37-abi3-macosx_11_0_arm64.zip` is
  already in `~/.config/atlas-go/` for re-installation if needed, but do not
  rebuild unless user explicitly requests
- ❌ Ask "do you have Fubon SDK?" — it IS set up; verify with commands above
- ❌ Try to `pip install fubon-neo` from PyPI — it's NOT on PyPI (never was);
  download the wheel from the official Fubon site
- ❌ Mount the host's macOS venv into a Linux container — the macOS .so
  is Mach-O, will not load on Linux. The Dockerfile downloads the
  proper manylinux wheel from the official site instead
- ❌ Treat `-allow-live-broker` as dangerous-by-default in this env — it is
  safe because the SDK + cert + .env are all present (AGENTS.md:95 warning
  is for environments WITHOUT these pre-installed)

---

## PostgreSQL

**Path**: `/opt/homebrew/bin/psql` (v15.17)

**Connection** (used by atlas api mode, verified by Wave 9 smoke test):
- Default config via `internal/db/` package
- Migration files in `sql/migrations/`

**Verify**:
```bash
psql --version  # should show 15.x
psql -l  # list databases (requires running server)
```

**Atlas startup log** (when working):
```
[PostgreSQL] connecting...
[PostgreSQL] connected
```

If connection fails, check `internal/db/` config and `sql/migrations/` state.

---

## Redis

**Path**: `/opt/homebrew/bin/redis-cli` (v8.4.1)

**Usage**: Caching layer (channel health, etc.)

**Verify**:
```bash
redis-cli --version  # should show 8.x
redis-cli ping  # PONG if running
```

---

## Go toolchain

**Version**: 1.26.2 (darwin/arm64)

**Verify**:
```bash
go version
```

**Required version**: see `go.mod` (`go 1.25` directive).

---

## Python

**System Python**: 3.14.4 (`/opt/homebrew/bin/python3`)
**Fubon venv Python**: 3.13.7 (`~/.config/atlas-go/.fubon-env/bin/python`)

The system Python is for general use. The Fubon venv Python 是 **3.13** 因為:
- `fubon_neo 2.2.8` 官方支援 Python 3.8-3.13(v2.0.1 後停止支援 3.7,**也不支援 3.14**)
- 「cp37-abi3」是 wheel tag 表示 stable ABI from Python 3.7+,所以 3.13 可用(manylinux wheel 跟 3.13 abi3 相容)
- macOS 跟 Linux 都有對應 wheel(分開下載,不要 cross-mount)

> 注意:此限制意味著 `fubon-proxy` 的 Dockerfile 必須用 `python:3.13-slim`(官方支援範圍內,不要用 3.12 或 3.14)。

---

## Secrets (macOS Keychain)

**Storage location**: macOS Keychain, service `com.kaecer68.atlas-go`

**Access**:
```bash
security find-generic-password -a "FINMIND_API_KEY" -s com.kaecer68.atlas-go -w
security find-generic-password -a "FUBON_API_KEY" -s com.kaecer68.atlas-go -w
security find-generic-password -a "FUBON_PERSONAL_ID" -s com.kaecer68.atlas-go -w
# ... etc per configs/allowed_env_vars.md
```

**`.env` file** at `~/.config/atlas-go/.env` is a fallback (4.8 KB) with non-secret
config. The first comment in `.env` says:
> "Secrets (API keys, passwords) are stored in macOS Keychain. Use Keychain
> Access.app or: `security find-generic-password -a "KEY_NAME" -s
> com.kaecer68.atlas-go -w`"

**Documented env vars**: see `configs/allowed_env_vars.md`.

---

## v0.0.0.31 Subscription / JWT Secret (Wave 11 PR #972)

`internal/subscription/jwt.go` 需要 HS256 secret 簽發 token。透過 `config.GetSecret("ATLAS_JWT_SECRET")` 解析（不直接 `os.Getenv`，符合 Constitution）。

| Env Var | 用途 | 預設 |
|---------|------|------|
| `ATLAS_JWT_SECRET` | JWT 簽章 secret（HS256，必須 ≥ 32 字元） | dev fallback: `"atlas-dev-secret-do-not-use-in-prod-32chars"` |
| `ATLAS_SUBSCRIPTION_DB_PATH` | SQLite store 路徑（users + subscription_events） | `${ATLAS_WORK_DIR}/data/subscriptions.db` |
| `ATLAS_PREMIUM_TRIAL_DAYS` | 新用戶免費試用天數 | `7` |

**dev 設定（macOS Keychain）**：

```bash
# 寫入 Keychain（一次性）
security add-generic-password -a ATLAS_JWT_SECRET -s com.kaecer68.atlas-go \
  -w "$(openssl rand -hex 32)" -U

# 透過 ~/.config/atlas-go/.env 引用
echo 'ATLAS_JWT_SECRET=$(security find-generic-password -a ATLAS_JWT_SECRET -s com.kaecer68.atlas-go -w)' >> ~/.config/atlas-go/.env
```

**production 設定**：用 secret manager（GCP Secret Manager / AWS SSM / Vault），**禁止** hardcode 或 commit。

⚠️ **旋轉策略**：secret 變更會讓所有現有 JWT token 失效，使用者被強制重新登入。建議在低流量時段（盤前 8:30 之前）輪換。

---

## Local data directory

**Path**: `/Users/kaecer/workspace/atlas/data/`

**Subdirectories** (per `docs/data-directory-standard.md`):
- `data/state/` — runtime state (baseline_policy.json, circuit_breaker_state.json, etc.)
- `data/state/{finmind,fubon,fugle}/` — API response cache
- `data/replay/` — historical replay data (JSONL)
- `data/fundamentals.json` — company fundamentals

**Verify**:
```bash
ls /Users/kaecer/workspace/atlas/data/  # should show subdirectories
cat /Users/kaecer/workspace/atlas/data/README.md  # if exists, has more details
```

**Re-creation**: data is generated by `cmd/import-replay` and other tools. Do NOT
delete without user confirmation — some files (e.g., baseline_policy.json) take
significant effort to recreate.

---

## If something is broken

Before assuming "X is missing and I should install it":

1. **Check this document's TL;DR table** — most likely it's marked "✅ SET UP"
2. **Run the verification command** in the relevant section
3. **If verification passes** — the issue is elsewhere (config, network, logic), not
   the dependency itself
4. **If verification fails** — DO NOT recreate, ask the user. Recreating may overwrite
   existing credentials or break working setups

Common false-alarm scenarios:
- "Fubon SDK not found" → actually at `~/.config/atlas-go/.fubon-env/`, not in venv
- "PostgreSQL connection failed" → check if local server is running, not whether
  PostgreSQL is installed
- "Cert file missing" → check `~/.config/atlas-go/.fubon-env/`, not `/etc/ssl/`

---

## Update procedure

When external setup changes (e.g., SDK update, cert renewal, account switch):

1. Update this document with new status + verification commands
2. Update `configs/allowed_env_vars.md` if env var names change
3. Update `AGENTS.md` if any cross-cutting rules change
4. Commit via PR (label: `chore: env-status-update`)

This is a contract between the developer and AI: keeping this document accurate
prevents the recurring "AI assumes it's not there" issue.

---

## See also

- `AGENTS.md` — cross-module pitfalls, quick commands
- `configs/allowed_env_vars.md` — full env var reference
- `docs/quickstart.md` — quick build/test reference (44 lines, not env-focused)
- `README.md` — overview + Fubon proxy architecture
- `internal/apigateway/CONSTITUTION.md` — data source governance
- `.claude/skills/atlas-data-visibility/SKILL.md` — silent failure detection
- `.claude/skills/atlas-pre-change-protocol/SKILL.md` — session-start env recording
- `.agent-hooks/deny-dangerous.sh` — machine-enforced env isolation rules
- 內部根因調查（`.omo/investigations/`，gitignored）
- `docs/reference/traps.md` § Deploy/Docker — 跨模組部署陷阱(ENTRYPOINT 衝突、env_file precedence、hardcoded healthcheck)
