# Environment Status

> **AI MUST READ THIS FIRST** before assuming any external dependency is missing.
>
> Last verified: 2026-06-25 (Wave 9 observability wire smoke test → v0.0.0.17; integration-test audit → v0.0.0.18 gap fixes pending in PR #704 on `feat/wave-10-l1-l2-iteration`).
>
> **Story so far**: v0.0.0.17's 5-second dry-run smoke test could only confirm the wire powered on (dry-run produces no symbols). A follow-up integration-test review then caught three real production bugs in the v0.0.0.17 wire that the smoke test could not exercise: (1) the 15 dashboard buffer subscriptions (including 5 Wave 9 outputs) were wired against the simulation bus only, so SSE catchup was permanently empty in `runLiveTrading`; (2) `Wave9Observability.Start` left partially-started detectors running on partial failure; (3) the parallel-start `errs` channel dropped all errors after the first. v0.0.0.18 (PR #704) closes all three plus adds v2 + chain integration tests, and adds idempotency to `risk.NewAuditSubscriber` to prevent duplicate JSONL records on double-registration.
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

## Fubon SDK (the recurring question)

**Path**: `~/.config/atlas-go/.fubon-env/`

**Contents** (verified 2026-06-25):

| File | Purpose |
|---|---|
| `bin/python` (3.13.7) | Python interpreter for Fubon proxy |
| `bin/pip` + fubon_neo 2.2.8 | Official Fubon Python SDK |
| `M120628569_20270620.p12` | Personal certificate for DMA login |
| `lib/python3.13/site-packages/fubon_neo/...` | SDK source |
| `pyvenv.cfg` | venv config (system-site-packages enabled) |

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

The system Python is for general use. The Fubon venv Python is locked to 3.13
because `fubon_neo 2.2.8` is built for `cp37-abi3` (Python 3.7+).

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

## Local data directory

**Path**: `/Users/kaecer/workspace/atlas/data/`

**Subdirectories** (per `docs/DATA_DIRECTORY_STANDARD.md`):
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
- `docs/QUICKSTART.md` — quick build/test reference (44 lines, not env-focused)
- `README.md` — overview + Fubon proxy architecture
- `internal/apigateway/CONSTITUTION.md` — data source governance
- `.claude/skills/atlas-data-visibility/SKILL.md` — silent failure detection
