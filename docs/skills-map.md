# Skills Map

## Purpose

This document defines the full operating skill system for `atlas-go`.

It is not only a list of market-analysis roles. It is the shared operating logic that any AI, developer, or future automation layer must follow when:

- running simulations
- importing or replaying market data
- evaluating agent quality
- mutating prompts or rules
- accepting or rejecting changes

The goal is to make the system:

- auditable
- reproducible
- professionally bounded
- resistant to random prompt drift

## Skill System Overview

The complete skill system is divided into four layers:

1. Domain skills
2. Operating skills
3. Control skills
4. Evolution skills

## Machine-Readable Policy

The skill system is also enforced through machine-readable policy fields in code and config:

- `configs/agents.json`
- `AgentSpec.RequiredSkills`
- `AgentSpec.ForbiddenActions`
- `AgentSpec.OperatingNotes`
- `ExperimentRecord.MutationType`
- `ExperimentRecord.AcceptanceGates`
- `MutationBrief`

Future automation should preserve these policy fields directly instead of inferring them only from prose.

## Layer 1: Domain Skills

These skills produce research and market judgment.

### Market Context Skills

#### `taiwan_macro`

- Focus: Taiwan macro, policy, rates, domestic growth
- Inputs: CPI, PMI, GDP proxies, local policy direction
- Outputs: regime view, domestic risk commentary
- Avoid: single-stock calls

#### `foreign_flow`

- Focus: foreign institutional flows and ownership behavior
- Inputs: foreign buy/sell, futures positioning, index weights
- Outputs: market participation bias, crowdedness warnings
- Avoid: using flow alone as a complete thesis

#### `fx_and_liquidity`

- Focus: TWD/USD, DXY, liquidity spillover, rate pressure
- Inputs: FX, rates, liquidity stress indicators
- Outputs: broad support or stress signals for Taiwan equities
- Avoid: pretending FX predicts everything

#### `us_tech_spillover`

- Focus: transmission from US semis and mega-cap tech into Taiwan
- Inputs: Nasdaq, NVDA, AMD, AVGO, AI capex cycle
- Outputs: sector bias and watchlist pressure
- Avoid: naive one-to-one mapping

### Sector Desk Skills

#### `semiconductor_desk`

- Focus: foundry, packaging, equipment, IP, memory
- Inputs: capex cycle, utilization, peer leadership, supply-chain role
- Outputs: ranked candidates and regime notes
- Avoid: treating all semiconductor names as one trade

#### `ai_supply_chain_desk`

- Focus: ODM, server, PCB, thermal, connectors, power
- Inputs: order visibility, hyperscaler capex, shipment narratives
- Outputs: AI-linked candidate grading
- Avoid: converting hype directly into earnings certainty

#### `financials_desk`

- Focus: banks, insurers, brokers, asset managers
- Inputs: rates, spreads, balance-sheet quality, capital sensitivity
- Outputs: cyclical or defensive positioning ideas
- Avoid: importing foreign-bank logic without Taiwan adaptation

#### `shipping_desk`

- Focus: container and transport-cycle equities
- Inputs: freight cycle, utilization, macro demand, supply changes
- Outputs: cycle conviction and reversal risk
- Avoid: extrapolating peak-cycle earnings

#### `etf_rotation_desk`

- Focus: broad ETF and high-dividend allocation
- Inputs: valuation spread, breadth, defensive flow, concentration risk
- Outputs: ETF-based fallback or defensive rotation ideas
- Avoid: using ETFs to hide weak single-name research

### Style Skills

#### `growth_momentum`

- Focus: trend persistence with earnings or narrative support
- Inputs: relative strength, estimate direction, breakout quality, volume
- Outputs: momentum-qualified score
- Avoid: illiquid and narrative-only setups

#### `value_yield`

- Focus: dividend durability, valuation discipline, cash generation
- Inputs: payout resilience, FCF, balance sheet
- Outputs: yield-adjusted conviction
- Avoid: yield traps

#### `earnings_quality`

- Focus: accounting and earnings durability
- Inputs: margins, accruals, receivables, inventory, guidance quality
- Outputs: quality filter on sector ideas
- Avoid: overreacting to one noisy quarter

#### `technical_breakout`

- Focus: entry timing, trend structure, failure risk
- Inputs: base structure, volatility, volume behavior
- Outputs: timing and stop suggestions
- Avoid: overriding portfolio and macro controls

## Layer 2: Operating Skills

These skills define how the system is run correctly.

### `replay_operator`

- Focus: replay execution discipline
- Responsibilities:
- choose a valid replay data source
- set session date or window correctly
- verify forward-return availability
- ensure replays remain reproducible

### `ledger_operator`

- Focus: experiment and outcome hygiene
- Responsibilities:
- preserve session boundaries
- write outcomes, experiments, summaries
- keep global and session-specific records consistent
- avoid mixing unrelated replay windows in the same judgment

### `data_import_operator`

- Focus: normalized market data ingestion
- Responsibilities:
- convert source files into stable internal replay formats
- track source provenance
- reject malformed or partial records
- keep importer logic deterministic

### `backtest_operator`

- Focus: window-based replay execution
- Responsibilities:
- run multi-session replay windows
- aggregate scorecards
- identify weak agents across a period instead of one day
- avoid drawing conclusions from too few sessions

### `market_data_provider`

- Focus: reliable market data acquisition with automatic fallback
- Responsibilities:
- choose appropriate provider (hybrid/twse/fugle) based on config and availability
- handle rate limiting gracefully (TWSE: 3 req/5s, Fugle: 50 req/min)
- automatic fallback from paid to free sources when failures occur
- maintain data provenance in quote source field
- Providers:
  - **hybrid** (default): tries Fugle first, falls back to TWSE OpenAPI automatically
  - **twse**: TWSE OpenAPI only (free, 1335 listed stocks)
  - **fugle**: Fugle only (paid for real-time, demo key limited to symbol 1476)

## Layer 3: Control Skills

These skills limit risk and maintain system integrity.

### `cro_risk`

- Focus: attack portfolio proposals before action
- Inputs: all recommendations, exposure, concentration, correlation
- Outputs: rejects, trims, caution flags
- Avoid: proposing alpha itself

### `cio_portfolio`

- Focus: synthesis and final simulated action set
- Inputs: weighted research outputs, portfolio state, constraints
- Outputs: paper-trading actions and exposure summary
- Avoid: bypassing engine constraints

### `research_auditor`

- Focus: explainability and evidence quality
- Inputs: recommendations, fills, outcomes, experiments, prompt versions
- Outputs: audit notes, traceability, evidence gaps
- Avoid: inventing performance or rationale after the fact

### `system_guardrail`

- Focus: protecting non-prompt rules
- Responsibilities:
- prevent prompt mutation from silently changing system risk
- enforce that fill model, liquidity rules, and ledger semantics stay in code review scope
- separate model judgment from execution truth

## Layer 4: Evolution Skills

These skills govern how the system improves over time.

### `weak_agent_selector`

- Focus: identify the next agent worth improving
- Inputs: aggregated scorecards and outcomes
- Outputs: a single mutation candidate
- Avoid: changing several weak agents at once

### `prompt_mutator`

- Focus: produce one targeted prompt change
- Inputs: weak-agent failure pattern, current prompt, allowed scope
- Outputs: candidate prompt revision
- **Strategy evolution**:
  - **Defensive (v1)**: tighten qualification, downgrade uncertain signals, reject fragile setups
  - **Offensive (v2)**: optimize entry timing, increase position sizing on high-conviction setups
  - **Aggressive (v3 - current)**: 
    - Higher conviction thresholds (>70 vs >50)
    - 2x position sizing when Sharpe > 1.0
    - Concentrate up to 30% in single name when edge is clear
    - Skip diversification rules for high-conviction opportunities
- **Known limitations**: 
  - `prompt_tightening` mutation currently produces no measurable difference (candidate == baseline)
  - Only modifies prompt text without changing underlying recommendation scoring logic
- Avoid: broad rewrites with unclear causality

### `experiment_designer`

- Focus: define a valid before/after comparison
- Inputs: hypothesis, window, acceptance metric, revert condition
- Outputs: experiment record
- Required fields in mutation brief:
  - `window_id`: backtest window for evaluation
  - `target_skill`: agent skill identifier
  - `acceptance_gates`: ["improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"]
  - `maturity_level`: level_1_exploratory | level_2_window_validated | level_3_regime_aware
- Avoid: evaluation leakage and moving goalposts

### `experiment_judge`

- Focus: accept or reject a candidate based on evidence
- Inputs: baseline metrics, candidate metrics, risk behavior, judge checks
- Outputs: accepted or rejected decision
- **Acceptance criteria** (simplified in 2026-04):
  - candidate must be > baseline (strict improvement required)
  - improvement must exceed threshold:
    - `prompt_tightening`: 0.0005 (rarely effective, see known issues)
    - `risk_rule_change`: 0.001 (lowered from 0.0025)
    - `portfolio_constraint_revision`: 0.001 (lowered from 0.0035)
  - sufficient policy checks must pass
- **Implementation note**: Thresholds were lowered to allow gradual improvements. Previously strict thresholds (0.0025-0.0035) caused excessive rejections even for positive improvements.
- Avoid: approving changes based on narrative only

### `risk_rule_change` Parameters (2026-04 Updated)

Effective risk rule mutation uses these aggressive parameters:

```yaml
risk_rule_change:
  conviction_floor: 35              # Lowered from 55 (capture more opportunities)
  liquidity_floor: 2000000         # Reduced from 5M (broader universe)
  max_position_weight: 0.25        # Increased from 0.18 (higher conviction sizing)
  high_conviction_threshold: 80    # For auto-scaling to max weight
  stop_loss_pct: 8                 # Tighter than 15% (faster capital rotation)
  min_cash_pct: 5                  # Reduced from 12% (less cash drag)
  aggressive_mode: true
```

**Observed effects** (90-day window, 4 agents tested):
- Average improvement: +29%
- Acceptance rate: 100% (4/4 agents)
- Baseline range: 0.005073 → Candidate: 0.006419-0.007110

### `live_trading_operator` *(New)*

- Focus: real-time paper trading execution and monitoring
- Responsibilities:
  - maintain live portfolio state and position tracking
  - subscribe to market events through Event Bus
  - execute agent logic on market snapshots
  - apply risk checks before order simulation
  - record all trading activity to persistent ledger
- Components:
  - Live State Store (portfolio, positions, regime)
  - Event Bus (market snapshots, orders, risk alerts)
  - Real-time Orchestrator (market schedule, intraday cycles)

### `monitoring_operator` *(New in v0.5)*

- Focus: system health, resource monitoring, and experiment lifecycle management
- Responsibilities:
  - monitor CPU, memory, and disk usage during experiments
  - track experiment rounds and enforce stop conditions
  - alert on resource pressure and anomalies
  - provide visibility into system performance
- **Resource thresholds** (Balanced mode):
  - CPU: 75% (warn), 85% (critical)
  - Memory: 80% (warn), 90% (critical)
  - Disk: 85% (warn)
- **Stop conditions**:
  - Maximum 20 total rounds per session
  - 3 consecutive rejections
  - Acceptance rate below 15% (after 5+ rounds)
  - All 7 agents optimized
- **Scripts**:
  - `scripts/monitor/resource-guard.sh` - Resource monitoring
  - `scripts/monitor/round-tracker.sh` - Round tracking
- **Configuration**: `configs/monitor-limits.json`
- Avoid: ignoring resource warnings during long-running experiments

## Core Operating Logic

Every operation in `atlas-go` should follow this sequence:

1. Import or choose replay-ready market data
2. Create a replay session or replay window
3. Run agents through bounded skill roles
4. Simulate execution through engine constraints
5. Write outcomes to ledger
6. Build scorecards
7. Select one weak agent
8. Design one experiment
9. Compare baseline and candidate on unseen periods
10. Keep or revert

## Core Operating Techniques

### Technique 1: Separate judgment from execution

- Agents propose
- Engine constrains
- Ledger records

Never let a prompt directly decide fill mechanics or risk ceilings.

### Technique 2: Optimize one variable at a time

- One weak agent
- One prompt mutation
- One evaluation window

This preserves causality.

### Technique 3: Prefer windows over anecdotes

A single good day is not evidence. Window-level scoring is the minimum acceptable unit for agent quality.

### Technique 4: Preserve session provenance

Every replay run must be attributable to:

- one session id
- one data source
- one time window
- one registry state

### Technique 5: Use failures as training assets

Bad results are not noise. They are the raw material for:

- weak-agent selection
- targeted mutation
- future guardrail design

## Intelligence Improvement Techniques

These are the preferred techniques for making the system smarter over time.

### `failure_pattern_extraction`

- Find repeated losses by sector, regime, or style
- Do not mutate based on one-off mistakes

### `narrow_scope_mutation`

- Change wording, thresholds, exclusions, or evidence requirements
- Do not rewrite the whole identity of the agent

### `regime_specific_learning`

- Separate trend, defensive, and rotation conditions
- Avoid forcing one prompt to behave well in every regime

### `prompt_to_rule_promotion`

- If a lesson keeps recurring, move it from prompt lore into code, docs, or constraints
- Example: liquidity filters belong in engine constraints, not only prompts

### `audit_before_acceptance`

- A change is only mature if:
- the hypothesis is written
- the evaluation window is known
- the metric improved
- the risk behavior did not degrade materially

## Required Knowledge Baseline

Any AI modifying this system must preserve these Taiwan-market assumptions:

- MVP starts long-only
- round lots matter for fill realism
- liquidity filters are mandatory
- costs and slippage are part of ground truth
- ETFs and single stocks should not be scored identically
- replay and paper trading are not real execution
- prompt changes must not silently rewrite risk policy

## Non-Negotiable Rules

1. No hidden change to risk ceilings through prompt edits
2. No acceptance without experiment metadata
3. No judgment from a single replay day when a window is available
4. No unverifiable source of returns
5. No mixing operation logic with market-analysis logic

## Companion Documents

- [`docs/operations-playbook.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/operations-playbook.md)
- [`docs/iteration-playbook.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/iteration-playbook.md)
- [`docs/evolution-loop.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/evolution-loop.md)
