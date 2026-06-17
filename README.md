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
# Fubon proxy URL is fixed at 127.0.0.1:8081 (env override removed 2026-06).
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

## Web Dashboard

The web dashboard (`web/static/`) is a vanilla JS SPA served by Go's `http.FileServer`.
It provides real-time simulation monitoring, agent configuration, and experiment management.

### Navigation

The SPA uses the **History API** for clean URL routing (e.g., `/overview`, `/portfolio`).
The Go backend includes a catch-all fallback in both `runSimulation()` and `runLiveTrading()`
that rewrites unmatched paths to serve `index.html`, enabling direct URL access and refresh on any route.
Legacy hash URLs (`#page-overview`) are automatically redirected.

### CSS Architecture

Styles are modularized into 50+ files under `web/static/css/`:

```text
web/static/css/
|-- main.css                # @import aggregator (42 imports, cascade-order)
|-- base/                   # Design tokens and resets
|   |-- variables.css       # CSS custom properties (theme)
|   |-- reset.css           # Element resets
|   |-- tables.css          # Table base styles
|   `-- typography.css      # Font and text utilities
|-- layout/                 # Structural layout
|   |-- animations.css, grid.css, header.css, page-shell.css
|   |-- responsive.css, sidebar.css, topbar.css
|-- components/             # Reusable UI components
|   |-- badge, chain, circuit-breaker, controls, empty-state
|   |-- error-banner, filter-panel, inbox-card, live-progress
|   |-- loading-bar, metric, misc, modal, notification
|   |-- notification-colors, panel, performance-report, pipeline
|   |-- refresh, refresh-pill, sse-status, table-pagination
|   |-- tabs, tool-events, utilities, view-controls, workflow
`-- pages/                  # Page-specific styles
    |-- decision-chain.css, evolution-panel.css, industry.css
    |-- overview.css, parameters.css
```

### JavaScript Modules

Key JS files in `web/static/js/`:

| File | Purpose |
|------|---------|
| `main.js` | SPA router (`switchPage()`), navigation, auto-refresh |
| `bootstrap-utils.js` | Utility imports and `window.*` assignments |
| `component-init.js` | CircuitBreaker, PerformanceReport, SimHealth panel init |
| `event-listeners.js` | `DOMContentLoaded`-bound event delegation (~80 handlers) |
| `dashboard.js`, `pipeline.js`, `portfolio.js`, etc. | Page-specific data loading modules |

All inline `onclick` handlers have been extracted to `event-listeners.js` using `addEventListener`.

## Repository Structure

```text
.
|-- cmd/                    # CLI entrypoints
|-- internal/               # Core system packages
|-- configs/                # Agent and runtime configuration
|-- prompts/                # Agent and experiment prompts
|-- data/                   # Runtime state and replay data
|-- web/                    # Web dashboard (SPA + static assets)
|-- docs/                   # Architecture and operations docs
`-- scripts/                # Operational helper scripts
```
