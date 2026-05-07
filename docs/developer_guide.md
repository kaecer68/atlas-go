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
- **Coverage**: ≥40% overall; critical paths should have tests

## Key Gotchas (AGENTS.md traps)
1. Enabled agents in `configs/agents.json` must have matching `prompts/agents/<name>.md`
2. Darwinian weights silently clamp to [0.3, 2.5]
3. Control layer outputs must preserve original AgentID
4. Session dates use SessionID, NOT RecordedAt
5. Replay data is JSONL (one JSON object per line), not JSON array
6. JSON tags are snake_case everywhere — PascalCase unmarshaling silently produces nil
