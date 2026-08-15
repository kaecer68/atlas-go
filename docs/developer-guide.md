# Atlas-Go Developer Guide

## Quick Start

```bash
# Build & test
go build ./... && go test ./...

# Format check (must pass)
test -z "$(gofmt -l .)"

# Pre-commit hooks
cp scripts/hooks/pre-commit .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit
```

## Binary Freshness 檢查（make check-binaries）

Binary freshness 將「宣稱完成」與「正在執行的 binary 是否來自目前 `HEAD`」綁在一起。修改 Go binary 來源、frontend build、Dockerfile/build args、image tag，或重建 host binary 後，應在 push、promotion 與 release 前執行；純文件修改不需要執行。

### 檢查命令與觸發時機

```bash
make check-binaries
```

檢查目前主 image 內的 `/app/atlas-go`、`/app/atlas-mcp`、`/app/daily-replay-sync`、`/app/calibrate-seasonal`，cron image 的 `/app/macro-ingest`，以及存在時的 host `bin/atlas-mcp`。exit `0` 表示全部通過，stale／missing buildinfo／image unavailable 為 exit `1`，不在 Git repo 則為 exit `2`。

### 過期 binary 判定與 freshness 閾值

- 對 host binary 使用 `strings` 讀取第一個 `Commit=...`；對 image binary 則從暫存 container 複製後以相同方式解析。
- freshness 的唯一門檻是 binary 的 `buildinfo.Commit` 與 `git rev-parse HEAD` **完整相等**；沒有 7 碼容許值，也沒有以小時或天數計算的時間窗。
- `Commit=unknown`、找不到 `Commit=`、image 無法建立／copy binary 都是失敗。host `bin/atlas-mcp` 不存在時腳本會跳過並標示 warning；若部署契約要求該 host binary，必須另外重建，不能只看 `ALL BINARIES FRESH` 就視為完整。
- Go build 與 Dockerfile 的 ldflags 必須注入同一個 source commit；`Dockerfile`／`Dockerfile.cron` 也明確拒絕 `GIT_COMMIT=unknown`。

### 更新／重建流程

一律在 Git repo 根目錄的主 worktree 執行，並依下列順序，避免先部署舊 image／host binary：

```bash
# 1. 先更新兩套 embedded frontend assets
make build-frontend

# 2. 重建 atlas main image 與 cron image，注入目前 HEAD 的 buildinfo
make rebuild-atlas
make rebuild-cron

# 3. 重建 host MCP binary
make rebuild-host-bin

# 4. 對 Docker + host 產物做最後一致性檢查
make check-binaries
```

需要一鍵收斂時可跑 `make build-frontend && make rebuild-all`；`rebuild-all` 的內部順序是 host binary → atlas image → cron image，最後自動呼叫 `make check-binaries`。`rebuild-atlas`／`rebuild-cron` 使用 `Dockerfile.atlas.local`／`Dockerfile.cron.local`，只封裝 host-built binaries，不會把兩套 frontend `dist` 放進 image；若目標是 embedded frontend 的完整 image，必須在前端完成後使用正式 `Dockerfile`／`Dockerfile.cron` 另行確認 image 內容。不要用手工 `go build` 取代 Make targets，否則容易漏掉 `internal/buildinfo` 的 commit ldflags。若環境無法連線 `proxy.golang.org`，可設定 `GOPROXY=https://goproxy.cn,direct` 後再執行同一套 local rebuild targets。

### 常見失敗與排除

| 現象 | 原因 | 排除方式 |
|---|---|---|
| `not in a git repo` | 在錯誤目錄執行 | `cd` 到包含 `.git` 的 atlas-go repo root |
| `STALE ...` | binary、image 或 host build 使用的 commit 較舊 | 先確認 `git status` 與目前 `HEAD`，再執行 `make rebuild-all`，不要直接用舊 image 重新 tag |
| `buildinfo.Commit NOT FOUND` | binary 未注入 ldflags、截斷或不是預期 ELF/Mach-O 產物 | 依序跑 `make build-frontend`、Docker rebuild、`make rebuild-host-bin`；確認 build log 含 `internal/buildinfo.Commit=<完整 HEAD>` |
| `image unavailable` | `atlas-atlas:latest` 或 `atlas-cron-rebuilt:local` 不存在／不是預期 tag | 執行 `make rebuild-atlas`／`make rebuild-cron`；部署前用 `docker image inspect` 確認 binary 與 tag 存在 |
| frontend build 失敗 | 任一 frontend dependency 或 build error | 先修正 `npm ci`／`npm run build` 錯誤，再重做後續 image；不要把 stale image 當成替代品 |
| linked worktree 拒絕 rebuild | Docker target 必須在主 worktree 執行 | 切回主要 checkout／worktree 後重跑，保留 linked worktree 的隔離修改 |
| host `bin/atlas-mcp` 缺失但檢查仍綠 | 現行檢查腳本把 host 檔案缺失列為 skip | 執行 `make rebuild-host-bin` 後重跑；若流程要求 host MCP 存在，應把此情況視為未完成 |

修復後以 `make check-binaries` 顯示 `✓ ALL BINARIES FRESH` 為必要條件，再進行 API smoke／integration regression；freshness 只證明產物對應目前 source commit，不替代功能測試。

## Conventions

### File Naming
- **snake_case** for all new files: `my_module.go`, `data_export.json`
- **Exceptions** (tool-enforced): `README.md`, `CLAUDE.md`, `Dockerfile`, `SKILL.md`
- **Shell scripts**: snake_case only (hyphens banned by CI)

### Case-Sensitive Renames (CRITICAL — macOS APFS)
macOS APFS is case-insensitive. Case-only renames MUST use intermediate temp name:
```bash
# ❌ Broken on APFS
git mv AGENTS.md agents.md

# ✅ Correct on APFS
git mv AGENTS.md _agents_temp.md && git mv _agents_temp.md agents.md
```

### Go Code
- `gofmt` before commit (CI blocks otherwise)
- `fmt.Errorf("context: %w", err)` — always wrap errors
- Early returns preferred over deep nesting
- Import order: stdlib → external → `github.com/kaecer68/atlas-go/...`

### Commits
- Conventional commit format: `type(scope): description`
- Types: `feat`, `fix`, `refactor`, `test`, `chore`, `docs`, `ci`
- CI runs `go build`, `go test`, `go vet`, `gofmt -l`

## Project Structure (Key Paths)

```
internal/
├── orchestrator/     # Core orchestration (SystemCore, PluginHost, executors)
├── experiment/       # Mutation → Execute → Judge lifecycle
├── sim/              # Portfolio simulation engine
├── live/             # Live trading (gated behind -allow-live-broker flag)
├── monitoring/       # Dashboard API (36 handlers)
├── ledger/           # JSONL append-only audit trail
├── portfolio/        # Darwinian weights, FactorEngine
├── narrative/        # Macro events, causal chains, stress index
├── risk/             # VaR, drawdown, circuit breaker
├── baselne/          # Baseline policy promotion/revert
└── domain/           # Canonical types (no logic, no I/O)
```

## Branch Strategy
- Active branches must be rebased onto main weekly
- Stale branches: tag as `archive/<name>` before deletion
- Push branches to origin for backup
- Feature branches get worktrees

## Safety Gates
- **Live trading**: `-allow-live-broker` flag required (default false)
- **Secrets**: never hardcode API keys; use `.env` / environment variables
- **Panic**: zero panics in production code
- **Coverage**: ≥60% overall; critical paths should have tests

## Key Gotchas (AGENTS.md traps)
1. Enabled agents in `configs/agents.json` must have matching `prompts/agents/<name>.md`
2. Darwinian weights silently clamp to [0.3, 2.5]
3. Control layer outputs must preserve original AgentID
4. Session dates use SessionID, NOT RecordedAt
5. Replay data is JSONL (one JSON object per line), not JSON array
6. JSON tags are snake_case everywhere — PascalCase unmarshaling silently produces nil
