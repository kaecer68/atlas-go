# atlas-go

`atlas-go` is a Taiwan-stock adaptation of the public `atlas-gic` architecture. It is designed as a simulation and training platform for OpenClaw-driven strategy research, prompt iteration, and paper-trading evaluation.

## Design Goal

Build a safe, auditable system for:

- Taiwan stock market research and simulation
- multi-agent market debate and synthesis
- paper trading with realistic constraints
- score-based strategy iteration and prompt evolution

## Why Taiwan First

This project is optimized for the Taiwan equity market:

- TWSE and TPEX as the baseline market data sources
- Fugle as the preferred near-real-time provider
- Yahoo data only as an optional fallback or validation source

## Current MVP Scope

- Go project scaffold for a Taiwan-market ATLAS system
- simulation engine with risk constraints
- market data provider abstraction
- registry-driven multi-agent orchestration for daily replay training
- sessionized replay and window backtesting
- ledger-backed weakest-agent selection
- GitHub workflow, issue templates, and contribution structure

## Initial Architecture

1. Layer 1: Taiwan macro and flow agents
2. Layer 2: sector desks
3. Layer 3: style and portfolio review agents
4. Layer 4: CRO, execution simulator, CIO

## Recommended Data Strategy

- **Primary real-time provider**: Hybrid (Fugle + TWSE fallback) — automatically switches to free TWSE OpenAPI when Fugle fails
- **TWSE OpenAPI**: Free, covers 1335 listed stocks, rate limit 3 req/5s — used as default fallback
- **Fugle**: Preferred for real-time trading (paid), demo key limited to symbol 1476
- **Historical/validation**: TWSE + TPEX for backtesting

### Provider Configuration

Set in `.env`:
```bash
# Default: Hybrid mode (recommended)
ATLAS_MARKET_DATA_PROVIDER=hybrid
FUGLE_API_KEY=your_api_key_here

# Or use TWSE only (free)
ATLAS_MARKET_DATA_PROVIDER=twse

# Or use Fugle only (requires valid API key)
ATLAS_MARKET_DATA_PROVIDER=fugle
```

## Quick Start

### Daily Simulation

```bash
go run ./cmd/atlas
```

### OpenClaw Experiment Loop (Recommended)

```bash
# One-command validated experiment round (90-day window, auto-promote if accepted)
./scripts/openclaw/run-validated-round.sh

# Or step by step:
./scripts/openclaw/propose-mutation.sh --auto              # Step 1: Generate mutation brief
./scripts/openclaw/execute-next.sh --auto                # Step 2: Execute experiment
./scripts/openclaw/judge-latest.sh --auto                # Step 3: Judge results
```

### Advanced: Mutation Type Selection

```bash
# Test different mutation strategies
./scripts/openclaw/run-validated-round.sh --type risk_rule_change           # Aggressive entry rules (+40% avg)
./scripts/openclaw/run-validated-round.sh --type portfolio_constraint       # Position sizing & cash rules (+26% avg)
./scripts/openclaw/run-validated-round.sh --type prompt_tightening          # Prompt refinement (limited effect)

# Specify agent
./scripts/openclaw/run-validated-round.sh --agent growth-momentum-01 --type risk_rule_change
```

### Manual Commands

```bash
go test ./...
go run ./cmd/import-replay -source samples/replay/twse_stock_day_all_sample.csv -target data/replay/tw_open_data.jsonl
go run ./cmd/backtest-window -start 2026-01-01 -end 2026-03-31  # Create 90-day window
go run ./cmd/execute-experiment --brief data/state/mutation-briefs/brief-xxx.json
go run ./cmd/judge-experiment -result data/state/experiments/exec-xxx.json
go run ./cmd/promote-baseline -result data/state/experiments/exec-xxx.json
```

## Operating Logic

The intended operating loop is:

1. Import or prepare replay-ready TWSE/TPEX data
2. Run registry-driven multi-agent replay execution for one session or one backtest window
3. Record outcomes, experiments, and session summaries
4. Aggregate scorecards
5. Select one weakest agent
6. Design one bounded mutation
7. Re-run on a defined evaluation window
8. Keep or revert based on evidence

## Operating Commands

### Single replay session

```bash
go run ./cmd/atlas
```

### Import raw replay data

```bash
go run ./cmd/import-replay -source samples/replay/twse_stock_day_all_sample.csv -target data/replay/tw_open_data.jsonl
```

### Run a backtest window

```bash
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27
```

### Execute a mutation brief

```bash
go run ./cmd/execute-experiment -brief data/state/windows/window-20260326-20260327-mutation-brief.json
```

### Judge an experiment

```bash
go run ./cmd/judge-experiment -result data/state/experiments/exec-growth-momentum-01-1774800459.json
```

### Promote an accepted experiment into the baseline

```bash
go run ./cmd/promote-baseline -result data/state/experiments/exec-growth-momentum-01-1774800459.json
```

## Multi-Agent Executor

Replay decision-making is now routed through a registry-driven executor instead of one hardcoded recommendation function.

- Layer 1 context agents infer regime
- Layer 2 and 3 agents emit symbol-level recommendations
- Layer 4 control plugins apply CRO-style filtering and CIO-style aggregation

The currently implemented skill plugins cover:

- Context: `taiwan_macro`, `foreign_flow`
- Sector desks: `semiconductor_desk`, `ai_supply_chain_desk`, `etf_rotation_desk`, `financials_desk`, `shipping_desk`
- Style filters: `growth_momentum`, `value_yield`, `earnings_quality`, `technical_breakout`
- Controls: `cro_risk`, `cio_portfolio`

Execution is now split into two layers on purpose:

- `raw recommendations`: emitted by context, sector, and style agents, and used for replay scoring, scorecards, and weakest-agent selection
- `final recommendations`: passed through CRO and CIO controls, and used by the simulator for portfolio decisions

This separation matters because it lets the system keep attribution at the skill level while still enforcing portfolio governance at execution time.

Weakest-agent selection is also now `layer-aware` and `window-aware`:

- sector and style agents are prioritized ahead of control agents for mutation work
- context agents come next
- control agents are still scored, but they are de-prioritized as default mutation targets
- scorecards track how many replay windows contributed evidence before an agent is picked for refinement

Mutation briefs are also evidence-aware now. Each brief carries:

- target layer
- observed replay window count
- maturity level
- iteration guidance tuned to the agent layer

That means later AI operators do not just receive a prompt path and a hypothesis. They also receive the operating posture for the mutation: how cautious to be, whether the evidence is still exploratory, and what kind of change is appropriate for that layer.

Experiment judging is now maturity-aware too:

- `level_1_exploratory` can accept modest improvements with lighter evidence
- `level_2_window_validated` requires a clearer replay delta and stronger check coverage
- `level_3_regime_aware` uses the strictest improvement threshold before promotion

Acceptance is also mutation-type-aware:

- `prompt_tightening` uses the lightest default threshold
- `risk_rule_change` requires stronger replay improvement and more check coverage
- `portfolio_constraint_revision` is treated as the strictest class because it can reshape portfolio behavior system-wide

Mutation type now also changes the generated candidate artifact:

- `prompt_tightening` produces a prompt-style `v2` candidate
- `risk_rule_change` produces a risk-rule proposal artifact with candidate rule patch content
- `portfolio_constraint_revision` produces a portfolio-governance proposal artifact with constraint patch content

Judge checks are artifact-aware too:

- risk-rule mutations must look like structured rule proposals
- portfolio-constraint mutations must expose structured governance fields
- prompt mutations are still checked for tighter signal qualification language

Constraint proposals now also feed replay scoring:

- `conviction_floor` and `liquidity_floor` can be parsed into candidate execution constraints
- `max_position_weight` and `reserve_cash_fraction` can be parsed into candidate portfolio constraints
- replay compare can score these candidates against the baseline engine instead of treating them as text only

Those policy candidates now route through a shared execution policy model:

- runtime execution defaults to `RequireCROPass: true`
- replay compare can derive a candidate execution policy from the proposal artifact
- CRO conviction filtering and governance routing now read the same policy surface instead of separate ad-hoc rules

## Resource Monitoring & Round Management (New in v0.5)

The system now includes automated resource monitoring and round tracking to prevent overloading and ensure stable long-running experiments.

### Resource Guard

Monitor system resources before each experiment round:

```bash
# Check CPU, memory, and disk usage
./scripts/monitor/resource-guard.sh check

# JSON output for automation
./scripts/monitor/resource-guard.sh check --json
```

**Default Thresholds (Balanced Mode)**:
- CPU: 75% (warn if exceeded)
- Memory: 80% (warn if exceeded)  
- Disk: 85% (warn if exceeded)

Configure in `configs/monitor-limits.json`.

### Round Tracker

Track experiment rounds and enforce stop conditions:

```bash
# Check if stop conditions are met
./scripts/monitor/round-tracker.sh check

# View statistics
./scripts/monitor/round-tracker.sh stats

# Reset tracker (with backup)
./scripts/monitor/round-tracker.sh reset
```

**Stop Conditions (Balanced Mode)**:
- Maximum 20 total rounds
- 3 consecutive rejections
- Acceptance rate below 15%
- All 7 agents optimized

### Automatic Integration

`run-validated-round.sh` now automatically:
1. Checks resource status before execution
2. Validates round limits
3. Records results after completion

Example output:
```
[Resource Check] Checking system resources... ✓
[Round Check] Checking round limits...
[Round Tracker] Current round: 10/20
...
[Round Recorded] Round 10: accepted (improvement: 0.0025)
```

## Skill System

`atlas-go` now treats skills as a full operating system, not only prompt roles.

- Domain skills: market context, sector desks, style filters
- Operating skills: replay, import, ledger, backtest discipline
- Control skills: CRO, CIO, audit, guardrails
- Evolution skills: weakest-agent selection, mutation, experiment design, experiment judgment

The authoritative references are:

- [`docs/skills-map.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/skills-map.md)
- [`docs/operations-playbook.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/operations-playbook.md)
- [`docs/iteration-playbook.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/iteration-playbook.md)

The system also has machine-readable policy in:

- [`configs/agents.json`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/configs/agents.json)
- [registry.go](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/internal/domain/registry.go)

These policy fields define:

- required skills
- forbidden actions
- operating notes
- mutation type
- acceptance gates

This means later AI agents can read both the human-facing skill doctrine and the machine-readable execution policy before they mutate prompts or change code paths.

## Intelligent Improvement Techniques

The preferred improvement style in this repository is:

- optimize one agent at a time
- mutate one meaningful behavior at a time
- evaluate on replay windows, not anecdotes
- promote repeated lessons from prompt lore into docs, config, or code
- keep execution truth in the engine, not hidden in prompts
- enforce mutation discipline through machine-readable policy, not only freeform judgment

## Repository Plan

- [`docs/architecture.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/architecture.md)
- [`docs/data-sources.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/data-sources.md)
- [`docs/skills-map.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/skills-map.md)
- [`docs/operations-playbook.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/operations-playbook.md)
- [`docs/iteration-playbook.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/iteration-playbook.md)
- [`docs/evolution-loop.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/evolution-loop.md)
- [`docs/roadmap.md`](/Users/kaecer/.openclaw/workspace/agents/finance/atlas/docs/roadmap.md)

## Notes

This repository does not attempt to reproduce proprietary parts of `atlas-gic`. Instead, it applies the public architectural ideas to a Taiwan-stock simulation environment suitable for iterative agent training.
