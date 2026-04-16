# Architecture

## Product Intent

`atlas-go` is a simulation-first investment research system for Taiwan stocks. The system is built to let OpenClaw run strategy experiments, evaluate agent performance, and iterate prompts or rules without placing real orders.

## Core Principles

- Simulation before execution
- Audit trail over intuition
- Structured messages between layers
- Replaceable data providers
- Risk controls at the engine layer, not only in prompts

## Layered Agent Model

### Layer 1: Taiwan Market Context

Purpose: determine regime, risk budget, and sector bias.

Initial agents:

- 台灣總經
- 外資流向
- TWD / USD FX
- US Tech Spillover
- Semiconductor Cycle
- Index Breadth
- Futures and Options Sentiment

### Layer 2: Sector Desks

Purpose: select opportunities inside Taiwan-specific sectors.

Initial desks:

- 半導體產業桌
- AI 供應鏈產業桌
- PCB and Thermal
- 金融產業桌
- 航運產業桌
- Consumer and Tourism
- High Dividend and ETF Rotation
- Small Cap Momentum

### Layer 3: Style Filters

Purpose: filter raw ideas using style-specific lenses.

Initial styles:

- 成長動能
- 價值股息
- 獲利品質
- 技術突破
- Chip and Flow Confirmation

### Layer 4: Decision Layer

Purpose: enforce risk, simulate execution, and record final actions.

Decision agents:

- 風控長
- Execution Simulator
- 投資長
- Research Auditor

## Data Flow

```text
Market Data -> Layer 1 -> Screener -> Layer 2 -> Layer 3 -> 風控長 -> 投資長 -> Simulation Engine -> Scorecard
```

**Screener** (`internal/screener/`) runs before Layer 2/3 executors generate recommendations. It filters symbols using declarative criteria (P/E, P/B, dividend yield, momentum, volume, and total factor score) so that only qualifying stocks reach the sector desks and style filters.

## Modes

### Daily Replay

- Uses daily bars
- End-of-day decision
- Next-session simulated fill
- Best first target for MVP

### Intraday Replay

- Uses minute bars or snapshots
- Event-driven decision points
- Requires more robust data and clock simulation

### Near-Real-Time Paper Trading

- Uses live snapshots or websockets
- No real orders
- Full audit trail required

## Simulation Constraints

- long-only for first release
- per-position max allocation
- portfolio gross exposure cap
- minimum liquidity filter
- transaction cost and slippage model
- cooldown after stop exit

## Autoresearch Loop

The system should eventually:

1. score each agent's recommendations
2. identify weak agents by rolling metrics
3. propose one prompt or rule change
4. replay on unseen periods
5. keep or discard the change by objective performance

## Go Package Intent

- `internal/domain`: canonical types
- `internal/marketdata`: provider abstraction and adapters
- `internal/orchestrator`: layered workflow
- `internal/screener`: declarative stock screening (fundamentals + technicals)
- `internal/portfolio`: Darwinian weights and multi-factor engine
- `internal/sim`: portfolio and execution engine
- `internal/config`: runtime configuration
- `cmd/atlas`: entrypoint for local simulation runs

