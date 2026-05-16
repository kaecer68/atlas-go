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

### 決策鏈透明度（Audit Trail）

系統同時輸出完整的計算過程供人工稽核：

1. **個股因子** → `FactorScores` 含四因子（動能/價值/品質/Agent）與總分，每因子含公式、原始輸入、是否 fallback
2. **信念增減** → `ConvictionBreakdown` 含 base/floor/final 與每步 rule/delta/reason
3. **宏觀事件** → `NarrativeEvent` 含 `confidence_source` 與 `historical_hit_rate`

前端 `web/static/index.html` 的「決策鏈」頁面以五層卡片呈現（宏觀→行業→個股篩選→控制→績效），每層皆可展開查看計算明細。

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
- `internal/industry`: industry ecosystem analysis (supply chain linkage, seasonal patterns, business cycle compass)
- `internal/narrative`: macro narrative event detection, causal chains, and SeasonalBridge for industry correlation modulation
- `internal/replay`: historical market data loading and forward return calculation
- `internal/reporting`: Markdown report generation and performance tables
- `cmd/atlas`: entrypoint for local simulation runs
- `cmd/calibrate-seasonal`: CLI for calibrating seasonal patterns from replay data

## Industry Ecosystem

### Supply Chain Linkage (`internal/industry/linkage.go`)

Models upstream/downstream relationships between Taiwan industries with configurable correlation matrices:
- `CorrelationMatrix` supports three initialization modes: hardcoded defaults, config-driven (parameters.json), and empirical recalculation from returns
- `ShockPropagation` propagates impact through the supply chain with narrative-aware correlation multipliers
- `LinkageAnalyzer` exposes scores, graphs, and shock simulation via `GET /api/industry/linkage`
- Graph topology is defined in `configs/supply_chain_graph.json` (hot-reloadable)
- Narrative themes from `SeasonalBridge` dynamically adjust pairwise correlations

### Seasonal Patterns (`internal/industry/seasonality.go`)

Calendar effect detection and calibration:
- `SeasonalEngine` manages per-industry patterns with start/end months and adjustment factors
- `cmd/calibrate-seasonal` CLI supports synthetic data, replay-based calibration, and automatic parameter update
- Evidence quality badges (`heuristic_awaiting_data` / `low` / `medium` / `high`) displayed in frontend
- `GetAdjustmentBreakdown()` provides four-layer decomposition: seasonal x narrative x cycle x environment

### Business Cycle Compass (`internal/industry/cycle.go`)

Multi-phase industry cycle tracking:
- `CycleTracker` manages five phases (expansion/recovery/mature/recession) with confidence scoring
- `DynamicEnvModulator` ingests macro data (oil, BDI, DXY) for real-time cycle adjustment
- External validators integrate seasonal engine and linkage analyzer for multi-dimensional confidence

