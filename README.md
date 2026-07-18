# atlas-go

`atlas-go` is a simulation-first, audit-driven investment research system for Taiwan equities.
It orchestrates layered research agents, replays market data, runs bounded simulations,
and evaluates mutations with explicit acceptance gates.

**For Taiwan retail investors (散戶)**: the web dashboard (`/client/`) is the primary interface — plain-language capital-flow observation, money-tide prediction and strategy comparison, no setup needed. AI agents (Hermes / OpenClaw) act as a supplementary layer via atlas-mcp for anything the web UI cannot explain well. See [`docs/reference/product-positioning.md`](docs/reference/product-positioning.md).

## Quick Start

```bash
# Run the application
go run ./cmd/atlas

# Run experiments
go run ./cmd/run-experiment -brief <brief-file>
go run ./cmd/judge-experiment

# Baseline management
go run ./cmd/promote-baseline
go run ./cmd/revert-baseline --list
```

Full setup guide: [`docs/quickstart.md`](docs/quickstart.md)

## Atlas as MCP Server

atlas-go doubles as a **MCP (Model Context Protocol) server** with **111 tools**,
allowing external AI agents to query Taiwan stock market data, strategies, risks, and more.
MCP is the **Agent Support** layer that complements the web-first dashboard — the web UI
comes first; agents explain what the UI cannot.

```bash
# One-line installer (no Go toolchain needed)
curl -fsSL https://raw.githubusercontent.com/kaecer68/atlas-go/main/scripts/install-atlas-mcp-from-release.sh | bash
```

See: [`cmd/atlas-mcp/README.md`](cmd/atlas-mcp/README.md) · [`.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md`](.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md)
Tool catalog: [`docs/reference/tool-catalog.md`](docs/reference/tool-catalog.md)

## Architecture

```
market data → orchestrator → layered executors → control filters → simulator → ledger
```

- **Language**: Go 1.26 · **DB**: PostgreSQL 15 + Redis 8
- **CI**: `gofmt` / `go vet` / `staticcheck` / `golangci-lint` / `gosec` · coverage ≥ 60%
- **Data Providers** (priority): TWSE OpenAPI → FinMind → Fubon (Python proxy) → Fugle
- **Key packages**: `internal/domain` (types), `internal/orchestrator` (routing), `internal/sim` (simulation), `internal/experiment` (mutations), `internal/risk` (VaR/guards), `internal/llm` (multi-provider router)

Deep dive: [`docs/architecture.md`](docs/architecture.md) · [`docs/llm-integration-strategy-framework.md`](docs/llm-integration-strategy-framework.md)

## Frontend

Three-directory SPA with History API routing:
- **`client_web/`** — Investor dashboard (`/client/`, default landing)
- **`admin_web/`** — Operator dashboard (`/admin/`)
- **`shared_web/`** — Shared CSS/JS assets (dark/light theme, components)

Root `/` redirects to `/client/`. Full frontend architecture: [`CLAUDE.md` §前端架構](CLAUDE.md)

## Validation

```bash
test -z "$(gofmt -l .)"       # format check
go build ./...                 # build
go test ./...                  # full test suite
go vet ./...                   # vet
staticcheck ./...              # static analysis
```

Focused: `go test ./internal/experiment/... ./internal/orchestrator/... ./internal/sim/...`

## Where to Go Next

| You are... | Start here |
|------------|-----------|
| Retail investor (散戶) | Open `/client/` web dashboard, then [`docs/investor/README.md`](docs/investor/README.md) |
| External AI agent connecting to atlas | [`docs/investor/README.md`](docs/investor/README.md) (5-min overview) |
| Developer modifying code | `AGENTS.md` (cross-tool AI guide) + [`docs/reference/traps.md`](docs/reference/traps.md) |
| Debugging or troubleshooting | [`CLAUDE.md`](CLAUDE.md) (deploy, frontend, token rules) |
| Understanding module maturity | [`internal/MATURITY.md`](internal/MATURITY.md) → [`internal/AGENTS_INDEX.md`](internal/AGENTS_INDEX.md) |
| Learning about MCP integration | [`cmd/atlas-mcp/README.md`](cmd/atlas-mcp/README.md) |
| Exploring architecture details | [`docs/architecture.md`](docs/architecture.md) |

## Repository Structure

```text
.
├── cmd/                    # CLI entrypoints (atlas, atlas-mcp, run-experiment, …)
├── internal/               # ~70 Go packages (domain, orchestrator, sim, risk, llm, …)
├── configs/                # Agent config, LLM routing table
├── admin_web/ client_web/ shared_web/  # Frontend SPAs
├── docs/                   # Architecture, specs, playbooks, audit trails
├── scripts/                # CI helpers, installer, smoke tests
├── .claude/skills/         # AI coding standard operating procedures
└── services/               # Python microservices (fubon-proxy)
```

---

*[CHANGELOG](CHANGELOG.md) · [AGENTS.md](AGENTS.md) · [SECURITY.md](SECURITY.md) · [CONTRIBUTING.md](CONTRIBUTING.md)*
