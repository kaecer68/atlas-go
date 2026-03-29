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

- Taiwan Macro
- Foreign Flow
- TWD / USD FX
- US Tech Spillover
- Semiconductor Cycle
- Index Breadth
- Futures and Options Sentiment

### Layer 2: Sector Desks

Purpose: select opportunities inside Taiwan-specific sectors.

Initial desks:

- Semiconductor
- AI Supply Chain
- PCB and Thermal
- Financials
- Shipping
- Consumer and Tourism
- High Dividend and ETF Rotation
- Small Cap Momentum

### Layer 3: Style Filters

Purpose: filter raw ideas using style-specific lenses.

Initial styles:

- Growth Momentum
- Value and Yield
- Earnings Quality
- Technical Breakout
- Chip and Flow Confirmation

### Layer 4: Decision Layer

Purpose: enforce risk, simulate execution, and record final actions.

Decision agents:

- CRO
- Execution Simulator
- CIO
- Research Auditor

## Data Flow

```text
Market Data -> Layer 1 -> Layer 2 -> Layer 3 -> CRO -> CIO -> Simulation Engine -> Scorecard
```

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
- `internal/sim`: portfolio and execution engine
- `internal/config`: runtime configuration
- `cmd/atlas`: entrypoint for local simulation runs

