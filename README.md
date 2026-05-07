# atlas-go

`atlas-go` is a simulation-first investment research system focused on Taiwan equities.

It provides an auditable workflow for:

- orchestrating layered research agents
- replaying market data
- running bounded simulations
- evaluating mutations (prompt/risk/constraint)
- accepting or rejecting candidates with explicit gates

## Current-First Readme

This README is an operational entrypoint, not a frozen performance report.

- If prose conflicts with code or experiment artifacts, treat code and runtime outputs as source of truth.
- Primary truth sources:
	- `configs/agents.json`
	- `internal/experiment/*`
	- `internal/orchestrator/*`
	- `internal/sim/*`
	- `data/state/experiments/*.json`

## Architecture

Core path:

`market data -> orchestrator -> layered executors -> control filters (CRO/CIO) -> simulator -> ledger`

### Data Providers (Priority Order)

1. **TWSE OpenAPI** (Free, no auth) - Primary
2. **FinMind** (Free, API key) - Historical data
3. **Fubon** (Free, account required) - Real-time via Python proxy
4. **Fugle** (Paid, circuit breaker protected) - Last resort

**Fubon Integration**: Since Fubon's Go SDK does not support market data APIs, we use a Python FastAPI microservice (`services/fubon-proxy/`) that wraps the official Python SDK. The Go application communicates with this proxy via HTTP.

**Configuration**:
```bash
# .env
FUBON_API_KEY=your_api_key
FUBON_PERSONAL_ID=your_id_number  # Required for DMA login
FUBON_PROXY_URL=http://localhost:8081  # Optional, defaults to localhost:8081
```

Main packages:

- `internal/domain`: canonical types
- `internal/orchestrator`: routing and plugin execution
- `internal/sim`: portfolio/execution simulation
- `internal/experiment`: mutation execution and judging
- `internal/baseline`: baseline policy management
- `internal/marketdata`: provider abstraction and adapters
- `internal/ledger`: outcomes and scorecard persistence

## Quick Start

Run application simulation entrypoint:

```bash
go run ./cmd/atlas
```

Run experiment flow:

```bash
go run ./cmd/run-experiment -brief <brief-file>
go run ./cmd/judge-experiment              # auto-discovers latest experiment
# or: go run ./cmd/judge-experiment -result <experiment-result-file>
```

Run baseline operations:

```bash
go run ./cmd/promote-baseline              # auto-discovers latest experiment
# or: go run ./cmd/promote-baseline -result <accepted-result-file>
go run ./cmd/revert-baseline --list
```

## Validation

CI-aligned checks:

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./...
go vet ./...
staticcheck ./...
```

Focused checks:

```bash
go test ./internal/experiment/...
go test ./internal/orchestrator/...
go test ./internal/sim/...
```

## Data Notes

- Default judge replay path comes from config (`ATLAS_REPLAY_DATA_PATH`).
- Small replay files can cause low-observation outcomes; recent judge logic now records:
	- `BaselineObservations`
	- `CandidateObservations`
	- `UsedFallbackWindow`
- Acceptance now distinguishes:
	- insufficient observations
	- no improvement over baseline

## Agent and Skill Mapping

For the complete operating skill map and guardrails, see:

- `.claude/SKILLS-MAP.md` — **統一技能地圖入口**
- `.claude/skills/atlas-core-architecture/SKILL.md` — 核心架構
- `.claude/skills/atlas-macro-narrative/SKILL.md` — 宏觀敘事
- `.claude/skills/atlas-risk-management/SKILL.md` — 風險管理
- `.claude/skills/atlas-strategy-evolution/SKILL.md` — 策略進化
- `.claude/skills/atlas-operations-guide/SKILL.md` — 操作指南
- `docs/operations_playbook.md` — 操作手冊
- `docs/iteration_playbook.md` — 迭代指南
- `docs/evolution_loop.md` — 演化循環

## Repository Structure

```text
.
|-- cmd/                    # CLI entrypoints
|-- internal/               # Core system packages
|-- configs/                # Agent and runtime configuration
|-- prompts/                # Agent and experiment prompts
|-- data/                   # Runtime state and replay data
|-- docs/                   # Architecture and operations docs
`-- scripts/                # Operational helper scripts
```
