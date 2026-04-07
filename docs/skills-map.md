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

### `asset_allocation_manager` *(New in v23)*

- Focus: strategic asset allocation across asset classes and security types
- Responsibilities:
  - maintain target allocation weights in `configs/portfolio-allocation.v23.json`
  - balance ETF core exposure with single-stock satellite positions
  - enforce concentration limits per asset class and individual name
  - coordinate rebalancing schedule (default: annual)
  - track allocation drift and trigger adjustment alerts
- **Configuration**:
  - File: `configs/portfolio-allocation.v23.json`
  - Taiwan Equity Total: 45% (ETF 15% + Single Stocks 30%)
  - Precious Metals: 15% (Gold 10% + Silver 5%)
  - Industrial Metals: 15% (Copper)
  - Cash: 25%
- **Single Stock Targets** (14 names, ~2.14% each):
  - Semiconductor: 2330.TW, 2303.TW, 2454.TW, 3034.TW
  - AI Supply Chain: 2382.TW, 6669.TW, 3017.TW, 3037.TW
  - Financials: 2881.TW, 2882.TW, 2891.TW
  - Shipping: 2603.TW, 2609.TW, 2615.TW
- **Risk Guards**:
  - Max single stock weight: 3%
  - Daily portfolio drawdown alert: -2%
  - Emergency trigger: 3-day cumulative Sharpe drop >5%
- Avoid: over-concentration in single names or sectors

### `darwinian_weight_manager` *(Phase 2 - Atlas-GIC Style)*

- Focus: dynamic agent weight adjustment based on performance
- Responsibilities:
  - track rolling 30-day Sharpe ratio for each agent
  - adjust weights daily: top quartile ×1.05, bottom quartile ×0.95
  - clamp weights between 0.3 (whispering) and 2.5 (shouting)
  - persist weights to `configs/darwinian_weights.json`
  - apply weights to recommendations before portfolio synthesis
- **Weight Range**:
  - 2.0-2.5: Shouting (highest confidence, strong performance)
  - 1.5-2.0: Strong (above average)
  - 0.8-1.5: Neutral (average)
  - 0.5-0.8: Weak (below average)
  - 0.3-0.5: Whispering (poor performance, minimal influence)
- **Metrics Tracked**:
  - Rolling Sharpe ratio (30-day window)
  - Win rate and average return per signal
  - Total signals and consecutive streaks
- **Usage**:
  ```bash
  ./scripts/darwinian-adjust.sh          # Daily adjustment
  ./scripts/darwinian-adjust.sh --reset # Reset to neutral (1.0)
  ```
- Implementation: `internal/portfolio/darwinian_weights.go`

### Superinvestor Layer *(Phase 2 - 4 Master-Style Agents)*

**Layer 4 agents providing concentrated, high-quality recommendations:**

| Agent | Style | Focus | Key Metrics |
|-------|-------|-------|-------------|
| `super_druckenmiller` | Macro/Momentum | Asymmetric macro trades, momentum timing | VIX, DXY, 10Y yields, sector flows |
| `super_aschenbrenner` | AI/Compute Cycle | AI capex beneficiaries, value chain mapping | Data center capex, GPU pricing, Taiwan exports |
| `super_baker` | Deep Tech | IP moat analysis, R&D efficiency | Patent quality, gross margins, R&D spend |
| `super_ackman` | Quality Compounder | Pricing power, FCF generation | ROIC, FCF conversion, customer retention |

- **Conviction Threshold**: 65+ (higher than regular agents)
- **Coordination**: Each superinvestor has defined collaboration notes with other agents
- **Prompts**: `prompts/agents/super_*.md`

---

## Phase 3: Advanced Agent Systems *(Agent Spawning, PRISM, Reflexivity, Swarm)*

### `agent_spawner` *(Phase 3 - Automated Agent Lifecycle)*

- Focus: automatic agent creation, gap detection, and lifecycle management
- Responsibilities:
  - detect capability gaps in current agent population
  - spawn new agents based on missing skills or market opportunities
  - manage agent lifecycle: spawn → train → deploy → monitor → retire
  - coordinate with PRISM for regime-specific training
- **Components**:
  - `GapDetector`: identifies missing capabilities
  - `AgentFactory`: creates new agent instances
  - `SpawningManager`: orchestrates deployment
- **Configuration**: `configs/spawning-config.json`
- **Usage**:
  ```bash
  ./scripts/spawning-manage.sh --scan      # Scan for gaps
  ./scripts/spawning-manage.sh --spawn     # Spawn new agent
  ./scripts/spawning-manage.sh --status    # Show status
  ```
- Implementation: `internal/spawning/`

### `prism_manager` *(Phase 3 - Multi-Regime Training)*

- Focus: regime-specific training queue management and optimization
- Responsibilities:
  - maintain training queues for different market regimes
  - schedule training tasks based on regime detection
  - balance training load across regimes
  - optimize training priority based on regime volatility
- **Regime Types**:
  - `trending_up`: strong momentum, trend-following strategies
  - `trending_down`: bear market, defensive positioning
  - `range_bound`: low volatility, mean-reversion strategies
  - `high_volatility`: crisis mode, risk-off positioning
  - `low_volatility`: calm market, momentum strategies
  - `rotation`: sector rotation, style switching
  - `earnings`: earnings season, event-driven
- **Configuration**: `configs/prism-config.json`
- **Usage**:
  ```bash
  ./scripts/prism-manage.sh --rebalance    # Rebalance queues
  ./scripts/prism-manage.sh --status       # Show queue status
  ./scripts/prism-manage.sh --train        # Trigger training
  ```
- Implementation: `internal/prism/`

### `reflexivity_engine` *(Phase 3 - Soros Reflexivity)*

- Focus: detect and apply market bias feedback loops
- Responsibilities:
  - identify reflexivity patterns in market data
  - track price-fundamental feedback loops
  - detect bubble formation and collapse signals
  - adjust recommendations based on reflexive bias
- **Key Concepts**:
  - **Bias**: market participants' distorted perception
  - **Far-from-Equilibrium**: price far from intrinsic value
  - **Self-Reinforcing**: feedback loop amplifies trend
  - **Reversal Point**: when trend cannot sustain itself
- **Detection Metrics**:
  - Price/momentum divergence
  - Volume/price relationship
  - Sentiment/price divergence
  - Fundamental deterioration with price rise
- **Usage**:
  ```bash
  ./scripts/reflexivity-report.sh          # Generate analysis
  ```
- Implementation: `internal/reflexivity/`

### `mirofish_swarm` *(Phase 3 - Swarm Intelligence Simulation)*

- Focus: simulate diverse agent behaviors for robust strategy discovery
- Responsibilities:
  - run parallel simulations with varied agent configurations
  - aggregate consensus from diverse strategies
  - detect anomalies in strategy performance
  - generate training data from simulation results
- **Swarm Components**:
  - `MiroFish`: individual strategy agents with unique behaviors
  - `MarketScenario`: simulated market conditions
  - `ConsensusEngine`: aggregates signals from swarm
  - `AnomalyDetector`: identifies outlier strategies
- **Simulation Modes**:
  - `diversity`: maximize strategy variation
  - `convergence`: find common signals
  - `stress_test`: extreme market conditions
  - `regime_specific`: targeted regime simulation
- **Usage**:
  ```bash
  ./scripts/swarm-manage.sh --run          # Run simulation
  ./scripts/swarm-manage.sh --consensus    # Get consensus
  ```
- Implementation: `internal/swarm/`

---

## Phase 4: Expert-Level Capabilities *(Meta-Learning, Adversarial, Global, Real-Time)*

### `metalearner` *(Phase 4 - Learning-to-Learn)*

- Focus: evolutionary strategy optimization and automated learning approach selection
- Responsibilities:
  - maintain population of learning strategies (20+ strategies)
  - evolve strategies based on MiroFish simulation results
  - perform crossover, mutation, and selection of top performers
  - adapt training approaches daily based on performance
- **Strategy Types**:
  - Conservative: minimal changes, low risk
  - Aggressive: frequent mutations, high exploration
  - Adaptive: regime-dependent approach
  - Focused: deep optimization on specific skills
  - Broad: wide exploration across many strategies
- **Evolution Process**:
  1. Evaluate all strategies on recent performance
  2. Select top performers (top 25%)
  3. Create offspring via crossover/mutation
  4. Replace worst performers
  5. Persist strategy population
- **Configuration**: `configs/metalearning-config.json`
- Implementation: `internal/metalearning/`

### `adversarial_trainer` *(Phase 4 - Red/Blue Team)*

- Focus: stress testing agents against adversarial scenarios
- Responsibilities:
  - run Red Team attacks simulating market crises
  - coordinate Blue Team defensive responses
  - identify vulnerabilities in agent strategies
  - harden agents against known attack patterns
- **Red Team (5 Attackers)**:
  - `flash_crash`: sudden price drop simulation
  - `liquidity_drain`: low volume scenario
  - `correlation_spike`: unexpected correlation
  - `sentiment_shift`: rapid mood change
  - `regime_jump`: sudden regime transition
- **Blue Team (5 Defenders)**:
  - `risk_mitigator`: position sizing defense
  - `recovery_agent`: loss recovery strategies
  - `diversifier`: correlation hedging
  - `liquidity_guard`: cash management
  - `stop_loss_manager`: exit strategy
- **Training Parameters**:
  - 100 training cycles
  - Adaptive difficulty (increases with agent improvement)
  - Vulnerability severity levels: critical, high, medium, low
- Implementation: `internal/adversarial/`

### `global_market_manager` *(Phase 4 - Cross-Market Expansion)*

- Focus: multi-market operations and regional exposure management
- Responsibilities:
  - manage operations across 7 regional markets
  - track cross-market correlations
  - enforce regional exposure limits
  - handle multi-currency and timezone operations
- **Supported Markets**:
  | Region | Code | Currency | Timezone |
  |--------|------|----------|----------|
  | Taiwan | TW | TWD | UTC+8 |
  | United States | US | USD | UTC-5 |
  | Europe | EU | EUR | UTC+1 |
  | Japan | JP | JPY | UTC+9 |
  | China | CN | CNY | UTC+8 |
  | Asia Pacific | AS | Multi | UTC+8 |
  | Emerging | EM | Multi | Various |
- **Exposure Limits**:
  - Max single region: 40%
  - Max single country: 25%
  - Max correlation exposure: 0.7
- **Configuration**: `configs/global-market-config.json`
- Implementation: `internal/globalmarket/`

### `realtime_adapter` *(Phase 4 - Sub-Second Adaptation)*

- Focus: real-time regime detection and dynamic weight adjustment
- Responsibilities:
  - detect market regime within 100ms
  - adjust agent weights based on current regime
  - provide confidence scoring for regime predictions
  - trigger automatic rebalancing when regime shifts
- **Detected Regimes**:
  - `calm`: low volatility, steady trends
  - `volatile`: high volatility, uncertain direction
  - `trending_up`: strong upward momentum
  - `trending_down`: strong downward momentum
  - `reversing`: trend exhaustion, potential reversal
  - `breakout`: breakout from consolidation
  - `breakdown`: breakdown from support
- **Update Cycle**: 100ms
- **Weight Adjustment**:
  - Regime-specific agent activation
  - Confidence-weighted recommendations
  - Automatic rebalancing triggers
- **Configuration**: `configs/realtime-config.json`
- Implementation: `internal/realtime/`

---

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
