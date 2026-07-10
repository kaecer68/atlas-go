# atlas-go

`atlas-go` is a simulation-first investment research system focused on Taiwan equities.

It provides an auditable workflow for:

- orchestrating layered research agents
- replaying market data
- running bounded simulations
- evaluating mutations (prompt/risk/constraint)
- accepting or rejecting candidates with explicit gates

## Recent updates

- **v0.0.0.21 (Wave 10 L2.3 PoC complete + Wave 11 L2.1 doc audit closure)** — `SemiconductorLLMAgent` wired behind `UseLLMSectorAgents` feature flag (PR #733), `LLMDriver` split into `PlanDriver` + `ReflectDriver` (PR #726), `LLM_OPENCODE_GO_API_KEY` and routing chain demoted to 3-tier effective fallback (PR #723), `llm_annotator` deprecation boundary documented (PR #730), and LLM sector agent plugin wired (PR #734). See [CHANGELOG.md](CHANGELOG.md).
- **v0.0.0.18 (Wave 9 gap fixes)** — Closed 3 production bugs (SSE catchup dead in `runLiveTrading`, `Start` partial-failure cleanup without defer, `errs` channel dropping errors after first), added v2/chain integration tests, and made `risk.NewAuditSubscriber` idempotent. See [CHANGELOG.md](CHANGELOG.md).
- **v0.0.0.17 (Wave 9 wire)** — Completed Wave 9 observability wiring (`Wave9Observability`, five detectors, `ChannelHealthSynthesizer`), `BaselineTrigger` policy enforcement, and `docs/ENVIRONMENT.md`. See [CHANGELOG.md](CHANGELOG.md).
- **v0.0.0.16 (Wave 9 trigger)** — Baseline runtime policy enforcement (`baseline.Trigger`), StopLoss/TakeProfit/MaxHoldingDays violations, and Layer 3 baseline tests. See [CHANGELOG.md](CHANGELOG.md).
- **v0.0.0.8 (Wave 9 schema)** — Wave 9 event schema: `EventPositionUpdate`, `EventRegimeChangeConfirmed`, `EventFactorWeightRegression`, `EventIngestionLagSpike`, `EventDriftDetected`, `EventChannelIndividualHealth`. See [CHANGELOG.md](CHANGELOG.md).
- **v0.0.0.7 (Wave 8 events)** — Event bus integration, regime debouncing, and DriftDetector v2. See [CHANGELOG.md](CHANGELOG.md).
- **v0.0.0.6 (Wave 7.5)** — Risk gate safety wiring, orphan config rejection, `AutoJudgePromoter` scheduler integration, promotion-recorded SSE events, and dashboard channel fetch log. See [CHANGELOG.md](CHANGELOG.md).

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
# Fubon proxy URL is fixed: host.docker.internal:18081 in containers,
# 127.0.0.1:18081 when running natively (ProcessManager).
# Env override removed 2026-06 (PR #572).
```

Main packages:

- `internal/domain`: canonical types
- `internal/orchestrator`: routing and plugin execution
- `internal/sim`: portfolio/execution simulation
- `internal/experiment`: mutation execution and judging
- `internal/baseline`: baseline policy management; `baseline.Trigger` enforces StopLoss/TakeProfit/MaxHoldingDays in live trading
- `internal/marketdata`: provider abstraction and adapters
- `internal/ledger`: outcomes and scorecard persistence
- `internal/monitoring`: Dashboard API, Wave 9 observability runtime, and channel health synthesis
- `internal/eventbus`: pub/sub event bus wiring for Wave 8/9 events
- `internal/risk`: RiskManager, VaR, and macro drawdown guards
- `internal/llm`: LLM capability-based multi-provider router with effective 3-tier fallback chain (Primary → Backup1 → LastResort; Backup2 reserved for [PLANNED] OpenCode providers), DataClass governance gate, and 12 capability handlers (see [LLM Framework](#llm-framework))

## LLM Framework

`internal/llm/` 是 capability-based 多 Provider 路由層，提供 fallback chain 與 DataClass 治理閘門。

> **Effective routing chain**: RoutingChain 結構保留 4 層（Primary/Backup1/Backup2/LastResort）以維持向後相容，但 `defaultRoutingTable()` 與 `configs/llm_router.yaml` 預設把 Backup2 設為空字串（等效於 3 層 fallback）。`ProviderOpenCodeGo`/`ProviderOpenCodeZen` 標記為 `[PLANNED]` 常數，等未來 client 實作後可重用。參見 Issue #720（Wave 11 L2.1 doc audit）。

- `internal/llm`: LLM capability-based multi-provider router with effective 3-tier fallback chain (Primary → Backup1 → LastResort; Backup2 reserved for [PLANNED] OpenCode providers), DataClass governance gate, and 12 capability handlers (see [LLM Framework](#llm-framework))

## LLM Framework

`internal/llm/` 是 capability-based 多 Provider 路由層，提供 fallback chain 與 DataClass 治理閘門。

> **Effective routing chain**: RoutingChain 結構保留 4 層（Primary/Backup1/Backup2/LastResort）以維持向後相容，但 `defaultRoutingTable()` 與 `configs/llm_router.yaml` 預設把 Backup2 設為空字串（等效於 3 層 fallback）。`ProviderOpenCodeGo`/`ProviderOpenCodeZen` 標記為 `[PLANNED]` 常數，等未來 client 實作後可重用。參見 Issue #720（Wave 11 L2.1 doc audit）。
>>>>>>> 62e7a3bc (fix(llm): remove OpenCode-Go/Zen from routing chain (Issue #720))

**架構分層**：

- `provider.go` — `ProviderImpl` 介面、`Capability`（能力描述）、`DataClass`（資料分級）、`RoutingChain`（備援鏈）
- `router.go` — `DefaultRouter` 實作 Primary → Backup1 → Backup2 → LastResort（Backup2 預設空字串，等效 3 層）；強制執行 DataClass 閘門（ADR-010：MiniMax M3 對 regulated 資料強制 skip）
- `clients/` — 3 個 Provider HTTP 客戶端（DeepSeek V4、MiniMax M3、Kimi K2.7）+ 共享 `BaseClient`（retry / rate-limit / circuit breaker）
- `capabilities/` — 12 個 capability handlers（failure_attribution、code_review_annotation、prompt_lint、rationale_generation、strategy_summary、risk_surface_extraction、regime_explanation、scenario_simulation、sentiment_explanation、performance_forensics、contra_attribution、confidence_commentary）
- `schemas/` — typed I/O contract（JSON-serialized, Zod-compatible JSON Schema）
- `adapters/` — Annotator / Router 整合層

**配置**：`configs/llm_router.yaml` 為 runtime 來源（`TryLoadRouterConfig()` 載入）；fallback 預設見 `router.go:defaultRoutingTable()`。

**健康端點**：`GET /api/llm/health` 暴露所有 Provider 的 `HealthStatus` 與 circuit breaker 狀態。

**Sector Agent LLM**：`internal/orchestrator/sector_agent_llm.go` 定義 `SectorAgentLLM` 骨架（plan → tool_call → reflect loop）。在觀察窗口內 `LLM == nil` 回 `ErrNotImplemented`，deterministic 路徑保留以保證 backtest 可重現。Feature flag `UseLLMSectorAgents` 控制啟用。

**熱路徑護欄**：`internal/sim/` 與 `internal/experiment/` 不可 import `internal/llm` 做同步呼叫（見 `internal/llm/AGENTS.md` §2）。

> 設計權威：`docs/llm-integration-strategy-framework.md` · `internal/llm/AGENTS.md`

## Atlas as MCP Server

atlas-go 同時也是 **MCP (Model Context Protocol) server**，提供 **91 個 tool** 給外部 AI agent（Hermes / OpenClaw / Claude Desktop / Cursor / OpenCode）調用。三種 transport 全部已 ship：stdio（預設）、SSE、streamable-HTTP。

### 你的場景

- **本機開發 / 個人 agent**：atlas-go backend 與 agent 在同一台 → 見 [`docs/mcp-integration-LOCAL.md`](docs/mcp-integration-LOCAL.md)（完整指南）+ [`.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md`](.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md)（50 行 SOP）
- **雲端接入**（規劃中，2026 Q4）：atlas-go 部署到雲端、外部 agent 透過 reverse proxy → 見 [`docs/mcp-integration-CLOUD.md`](docs/mcp-integration-CLOUD.md)（scaffold，待雲端部署穩定後補細節）

### 5 分鐘接入（任選 1 種 client）

```bash
# 1. 編譯 MCP server binary
make build-mcp

# 2. 確認 atlas-go backend 在 :18080
curl -fsS http://127.0.0.1:18080/health

# 3a. 自動設定（推薦，需先合併 PR #3）
make setup-mcp        # 互動式 wizard

# 3b. 手刻設定（見 AGENT_QUICKSTART.md §3 的 5 種範例）
# Hermes:     ~/.hermes/config.yaml
# OpenClaw:   ~/.openclaw/mcp.json
# Claude:     ~/Library/Application Support/Claude/claude_desktop_config.json
# Cursor:     ~/.cursor/mcp.json
# OpenCode:   ~/.config/opencode/opencode.json

# 4. 驗證
hermes mcp test atlas-mcp    # 應列出 91 個 tool
```

### Tool 速覽

| 分類 | 代表 tool | 用途 |
|------|----------|------|
| 市場總覽 | `mcp_quickstart`、`macro_get_snapshot_latest`、`crossmarket_get_us_indices` | 一次拿到當前市場快照 |
| 策略 | `strategy_ranker`、`strategy_list_active`、`strategy_get` | 策略排名與定義 |
| 風險 | `risk_get_metrics`、`risk_get_drawdown`、`risk_get_commentary` | VaR、回撤、風險評論 |
| 事件 | `event_calendar`、`event_flow_prediction`、`narrative_get_events` | 事件日曆 + 5 日預測 |
| 個股 | `stock_get_quote`、`stock_get_fundamentals`、`stock_get_chips`、`stock_get_technical` | 報價 / 基本面 / 籌碼 / 技術 |
| 系統 | `system_get_health`、`llm_get_health`、`data_get_channels` | 服務健康 |

完整 91 tool 決策樹見 [`docs/AGENT_TOOLS.md`](docs/AGENT_TOOLS.md)。

> **重要 env var**（不要再用舊版）：`ATLAS_BASE_URL`、`ATLAS_API_KEY`、`ATLAS_MCP_TOKEN`（取代已廢棄的 `ATLAS_WORK_DIR` / `ATLAS_DATABASE_URL` / `ATLAS_REDIS_URL` / `ATLAS_API_TOKEN`）。完整清單見 [`cmd/atlas-mcp/README.md`](cmd/atlas-mcp/README.md) §配置。

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
- `docs/ENVIRONMENT.md` — 外部依賴與開發環境狀態（PR #700）

Current hand-written `atlas-*` skills:

- `.claude/skills/atlas-pre-change-protocol/SKILL.md` — 修改前 7 步驟強制檢查清單
- `.claude/skills/atlas-data-visibility/SKILL.md` — 四層資料可見性防護
- `.claude/skills/atlas-macro-narrative/SKILL.md` — 宏觀敘事
- `.claude/skills/atlas-risk-management/SKILL.md` — 風險管理
- `.claude/skills/atlas-strategy-evolution/SKILL.md` — 策略進化
- `.claude/skills/atlas-strategy-techniques/SKILL.md` — 投資心法庫
- `.claude/skills/atlas-multi-strategy/SKILL.md` — 多策略框架
- `.claude/skills/atlas-event-driven-weights/SKILL.md` — 事件驅動權重
- `.claude/skills/atlas-swarm-analyst/SKILL.md` — Swarm 分析
- `.claude/skills/atlas-taiwan-leading-indicators/SKILL.md` — 短線領先指標

Operational playbooks:

- `docs/operations_playbook.md` — 操作手冊
- `docs/iteration_playbook.md` — 迭代指南
- `docs/evolution_loop.md` — 演化循環

## Web Dashboard

The frontend has been split into two SPAs served by Go's `http.FileServer`:

- **`client_web/`** — Investor-facing dashboard (`/client/`), default landing page.
- **`admin_web/`** — Operator/admin dashboard (`/admin/`).
- **`shared_web/`** — Shared CSS, JS pages, components, and services used by both apps.

Root `/` redirects to `/client/` so general users land on the investor interface by default.

### Navigation

Each SPA uses the **History API** for clean URL routing (e.g., `/client/portfolio`, `/admin/agents`).
The Go backend serves `index.html` for unmatched paths under `/client/` and `/admin/`,
enabling direct URL access and refresh on any route. Legacy hash URLs (`#page-overview`) are
automatically redirected.

### CSS Architecture

Styles are modularized into 50+ files under `shared_web/static/css/` and bundled by each app:

```text
shared_web/static/css/
|-- main.css                # @import aggregator (cascade-order)
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

Key JS files live in `shared_web/static/js/` and are consumed by `admin_web/` and `client_web/`:

| File | Purpose |
|------|---------|
| `main.js` (per app) | SPA router (`switchPage()`), navigation, auto-refresh |
| `bootstrap-utils.js` | Utility imports and `window.*` assignments |
| `component-init.js` (admin) | CircuitBreaker, PerformanceReport, SimHealth panel init |
| `event-listeners.js` (admin) | `DOMContentLoaded`-bound event delegation (~80 handlers) |
| `pages/*.js` | Page-specific data loading modules |

All inline `onclick` handlers have been extracted to `event-listeners.js` using `addEventListener`.

## Repository Structure

```text
.
|-- cmd/                    # CLI entrypoints
|-- internal/               # Core system packages
|-- configs/                # Agent and runtime configuration
|-- prompts/                # Agent and experiment prompts
|-- data/                   # Runtime state and replay data
|-- admin_web/              # Admin/operator SPA
|-- client_web/             # Investor-facing SPA
|-- shared_web/             # Shared frontend assets
|-- web/                    # Legacy monolithic SPA (deprecated, not served)
|-- docs/                   # Architecture and operations docs
`-- scripts/                # Operational helper scripts
```
