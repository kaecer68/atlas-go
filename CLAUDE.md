# CLAUDE.md

> **語言**: 所有回覆、總結、分析與建議請使用繁體中文。

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`atlas-go` is a simulation-first, audit-driven investment research system for Taiwan equities. It orchestrates layered AI agents over historical market data, evaluates mutations via experiments, and manages baseline policies through an explicit promote/revert gate.

- **Language**: Go 1.25.0
- **Module**: `github.com/kaecer68/atlas-go`
- **No Makefile**: use native Go toolchain and shell scripts directly

## Common Commands

### Build & Test

```bash
# CI-aligned checks (run these before submitting)
test -z "$(gofmt -l .)"
go build ./...
go test ./...
go vet ./...
staticcheck ./...
```

### Formatting

```bash
gofmt -w .
```

### Focused Testing

```bash
# Run a single test
go test -v ./internal/sim -run TestRunBuildsPositions

# Test specific packages
go test ./internal/orchestrator/...
go test ./internal/experiment/...
go test ./internal/sim/...
go test ./internal/portfolio/...
```

### Coverage

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -n 1
```

CI requires total coverage **≥ 40%**.

### Application Entrypoints

```bash
# Main HTTP server (default port 8080)
go run ./cmd/atlas

# Backtest a date window
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27

# Experiment lifecycle
go run ./cmd/run-experiment -brief <brief-file>
go run ./cmd/judge-experiment              # auto-discovers latest experiment
go run ./cmd/promote-baseline              # auto-discovers latest accepted experiment
go run ./cmd/revert-baseline --list

# Data import (CSV → JSONL)
go run ./cmd/import-replay -source <csv> -target <jsonl>
```

### Docker

```bash
# Full local stack (atlas + postgres + redis + prometheus + grafana + cron + prism-worker + swarm-runner)
docker-compose up -d

# Core services only
docker-compose up -d atlas redis postgres
```

## High-Level Architecture

### Core Data Flow

```
Market Data (replay/*.jsonl or live provider)
         ↓
Orchestrator (internal/orchestrator/)
    ├─ RegimeExecutor    (context layer: macro regime scoring)
    ├─ AgentExecutor     (sector/style/superinvestor layers: generate recommendations)
    └─ ControlExecutor   (control layer: CRO/CIO risk filters)
         ↓
Simulator (internal/sim/)
    ├─ RunSymbol() per ticker
    ├─ ApplyRecommendations() + ApplyControl()
    └─ Produces Position mutations / Orders
         ↓
Ledger (internal/ledger/)
    └─ Persists outcomes and scorecards for judgment
```

### SystemCore + PluginHost Architecture

The runtime is structured around `SystemCore` (essential simulation state and services) and `PluginHost` (plugin lifecycle and recommendation chain). See:

- `internal/orchestrator/system.go` — `SystemCore` holds the provider, engine, registry, policy, ledger, replay dataset, optimizer, alpha discovery, and narrative engine.
- `internal/orchestrator/plugin_host.go` — `PluginHost` registers `Plugin` implementations and delegates `Attach`, `ProcessRecommendations`, and `PostSimulation` hooks in registration order.

New executors that need lifecycle hooks implement `Plugin` from `internal/orchestrator/plugin.go` and are registered via the factory.

### Agent Layers

Defined in `internal/domain/types.go`:

- `context` — macro regime assessment
- `sector` — industry desk recommendations
- `style` — style filters
- `superinvestor` — high-conviction layer
- `control` — CRO / CIO risk gates

### Experiment & Baseline Lifecycle

1. **Propose**: generate a mutation brief
2. **Execute**: `cmd/run-experiment` generates a candidate prompt and records an experiment
3. **Judge**: `cmd/judge-experiment` replays candidate vs. baseline; requires sufficient observations (n≥10 preferred)
4. **Promote/Revert**: `cmd/promote-baseline` advances accepted experiments; `cmd/revert-baseline` rolls back

Baseline policy is loaded from `data/state/baseline_policy.json`. The experiment result artifacts live under `data/state/experiments/`.

## Code Conventions

- **Interface style**: small and focused, typically `Supports(...) bool` plus one operation method. See `internal/orchestrator/plugin.go` and `plugin_registry.go`.
- **Early return**: preferred over deep nesting.
- **Error wrapping**: always use `fmt.Errorf("context: %w", err)`.
- **Domain enums**: string-based (e.g., `type Regime string`) for JSON roundtrip clarity.
- **Import grouping**: stdlib → external packages → internal module (`github.com/kaecer68/atlas-go/...`).
- **Tests**: same directory, same package, `*_test.go` naming.
- **No global mutable state** for execution coordination.

## Critical Pitfalls

| Issue | Mitigation |
|-------|------------|
| **Missing prompt files** | Every `enabled: true` agent in `configs/agents.json` must have a matching file in `prompts/agents/`. |
| **Sparse replay window** | Judge needs n≥10 observations for meaningful results; small replay files cause low-confidence rejection. |
| **Silent Darwinian clipping** | Weights are clamped to `[0.3, 2.5]` without error; do not assume out-of-range values propagate. |
| **Mutable recommendation slices** | Do not reuse a mutable `[]Recommendation` across multiple simulation runs. Rebuild or copy per run. |
| **Live trading incomplete** | `internal/live/` has TODO boundaries. Default safe path is replay/simulation. |
| **Replay format** | Replay data is **JSONL** (one JSON object per line), not a JSON array. |

## Configuration

- `configs/agents.json` (and `agents.yaml`) define the agent registry.
- `.env` is auto-loaded by `internal/config/config.go` without overriding existing environment variables.
- Environment variables use `ATLAS_*` prefix (e.g., `ATLAS_MARKET_DATA_PROVIDER`, `ATLAS_REPLAY_DATA_PATH`, `ATLAS_BASELINE_POLICY_PATH`).

## Key File References

- `agents.md` — detailed agent boundaries, build commands, and operational scripts
- `.github/instructions/go-core.instructions.md` — Go coding rules
- `.github/instructions/experiments-guardrails.instructions.md` — experiment safety rules
- `.github/instructions/live-trading.guardrails.instructions.md` — live trading guardrails
- `docs/architecture.md` — layered architecture details
- `docs/operations_playbook.md` — day-to-day workflows

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (37047 symbols, 85964 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/atlas-go/context` | Codebase overview, check index freshness |
| `gitnexus://repo/atlas-go/clusters` | All functional areas |
| `gitnexus://repo/atlas-go/processes` | All execution flows |
| `gitnexus://repo/atlas-go/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
