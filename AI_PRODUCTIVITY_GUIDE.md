# Atlas-Go AI Productivity Guide

**Purpose**: Actionable insights for AI agents working on this project. Focuses on what makes development faster, builds stronger, and avoids common mistakes.

## Scope & Source of Truth

This guide is an execution productivity reference. For current OpenClaw mutation lifecycle semantics, treat the following as canonical first:

- `docs/skills-map.md`
- `docs/iteration-playbook.md`
- `docs/operations-playbook.md`
- `scripts/openclaw/today-start.sh`

Historical reports may contain point-in-time findings and should not override the current guard and acceptance behavior documented above.

---

## 1. CRITICAL BUILD/TEST WORKFLOWS

### Primary Build & Test Commands

```bash
# Format check (CI blocker - MUST pass)
gofmt -l .                  # Check format violations
gofmt -w .                  # Auto-fix formatting

# Build verification
go build ./...              # Build all packages

# Standard test run (start here)
go test ./...               # Run all tests
go test -v ./...            # Verbose output

# Focused test runs (faster iteration)
go test ./internal/sim/...              # Simulator logic
go test ./internal/orchestrator/...     # Coordination layer
go test ./internal/portfolio/...        # Risk & weights
go test ./internal/experiment/...       # Mutation lifecycle
go test -run TestRunBuildsPositions ./internal/sim/...  # Single test by name

# Quality gates (full validation before PR)
go vet ./...                # Static analysis
staticcheck ./...           # Additional lint checks

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -n 1  # Summary line
```

### CLI Entry Points (Most Frequent Operations)

| Command | Use Case | Execution |
|---------|----------|-----------|
| `go run ./cmd/atlas` | Daily simulation replay, next experiment candidate selection | ~30s-2m |
| `go run ./cmd/backtest-window -start YYYY-MM-DD -end YYYY-MM-DD` | Multi-session evaluation, baseline comparison | ~2-5m |
| `go run ./cmd/execute-experiment -brief <brief-file>` | Run proposed mutation experiment | ~1-2m per session |
| `go run ./cmd/judge-experiment` | Evaluate experiment vs baseline using metrics | ~30s |
| `go run ./cmd/promote-baseline` or `./scripts/openclaw/decide.sh` | Accept and promote mutated policy | Immediate (state update) |
| `go run ./cmd/revert-baseline` | Roll back failed promotion | Immediate (state update) |
| `go run ./cmd/import-replay -source <csv> -target <jsonl>` | Normalize TWSE/TPEX CSV → replay format | ~10-30s |
| `./enhanced_experiment_runner.go` | Automated evolution loop with mutation, execute, judge, promote | ~10-30m |

### Non-Obvious Build Steps

- **No external build system required**: Pure Go, single `go.mod` with minimal dependencies (only `golang.org/x/time`)
- **Config path resolution**: Always checks both env vars and `.env` file; `.env` is loaded silently if missing. Quoted values in `.env` (`KEY="value"`) are automatically unwrapped, and invalid integers in env vars trigger a warning log while falling back to defaults.
- **Test isolation**: Tests run in-process; no Docker spin-up needed for basic testing
- **Ledger directory auto-setup**: Simulation creates `data/state/sessions/<id>/` automatically; no pre-creation needed
- **Registry loading fallback**: If `configs/agents.json` is missing or corrupt, system falls back to `SeedRegistry()` default
- **Replay data must be CSV or JSONL**: Format auto-detected; import step is required to normalize raw TWSE/TPEX CSVs

---

## 2. KEY ARCHITECTURAL PATTERNS

### Plugin Registry Pattern (Core Execution Flow)

Architecture: Market Data → Plugin Registry → Layer Executors → Control Filters → Simulation Engine → Ledger

**Executor Interfaces** (all in `internal/orchestrator/`):
```go
// Regime scoring (market context)
type RegimeExecutor interface {
    Supports(agent domain.AgentSpec) bool
    Score(agent domain.AgentSpec, quotes map[string]domain.Quote, prompt string) int
}

// Recommendation generation (agent logic)
type AgentExecutor interface {
    Supports(agent domain.AgentSpec) bool
    Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool)
}

// Risk filtering (post-execution controls)
type ControlExecutor interface {
    Supports(agent domain.AgentSpec) bool
    Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation
}
```

**Key Pattern Rules**:
- Executors are **small and focused**: one `Supports()` check + one operation method
- Executors are **composable**: CRO filter + CIO aggregation both implement same interface
- Registry iterates executors in order; **first match wins** for Regime & Agent, **all matches run sequentially** for Control
- **Error handling**: Always wrap context using `fmt.Errorf("operation: %w", err)`

### Layer Architecture (4 Tiers + Decision)

```
Layer 1: Context       (Market environment: regime, risk budget)
  ├─ Taiwan Macro
  ├─ Foreign Flow
  └─ FX & Liquidity

Layer 2: Sector        (Opportunity discovery in specific sectors)
  ├─ Semiconductor
  ├─ AI Supply Chain
  ├─ Financials
  ├─ Shipping
  └─ ETF Rotation

Layer 3: Style         (Apply filters using style-specific lenses)
  ├─ Growth Momentum
  ├─ Value & Yield
  ├─ Earnings Quality
  └─ Technical Breakout

Layer 4: Superinvestor (Concentrated master-strategy ideas)
  ├─ Druckenmiller Macro
  ├─ Aschenbrenner AI/Compute
  ├─ Baker Deep Tech IP
  └─ Ackman Quality Compound

Decision Controls:
  ├─ CRO Risk Filter    (conviction floor, drawdown limits)
  └─ CIO Aggregation    (symbol deduplication, final ranking)
```

**Why This Matters**: Each layer output feeds the next. Control layer applies **after** all layers complete. Never modify layer ordering without explicit requirement.

### Domain Type Hierarchy (in `internal/domain/types.go`)

Core immutable types:
- `Quote`: Latest tradable price, volume, liquidity flag
- `Recommendation`: Agent → Symbol opinion with conviction (0-100)
- `SimulationResult`: Orders + Positions + Regime + Cash
- `RecommendationOutcome`: Post-execution feedback (hit/miss, forward return)
- `Position`, `Order`: Portfolio state tracking

**Agent Specification** (`AgentSpec`):
```go
type AgentSpec struct {
    ID            string        // unique identifier
    Skill         string        // capability category
    Layer         AgentLayer    // execution position
    Universe      []string      // allowed symbols (empty = unrestricted)
    PromptFile    string        // path to prompt markdown
    RequiredSkills []string     // precondition checks
    ForbiddenActions []string   // boundary enforcement
    DarwinianWeight float64     // performance multiplier (0.3-2.5)
}
```

### Darwinian Weight System (Performance-Based Agent Scaling)

Located: [internal/portfolio/darwinian_weights.go](internal/portfolio/darwinian_weights.go)

- **Range**: [0.3 (whisper) ... 1.0 (neutral) ... 2.5 (shout)]
- **Adjustment**: Top 25% agents get +5% multiplier, bottom 25% get -5%
- **Cooldown**: Min 20 hours between adjustments per agent
- **Application**: CIOPortfolioExecutorWithWeights weights aggregated conviction by agent multiplier
- **Safety Rail**: Weights are clamped to bounds; out-of-range values silently normalized

### State Management Hierarchy

```
Config               (environment variables + .env file)
  ↓
Registry             (agents.json: agents, skills, policies)
  ↓
Baseline Policy      (baseline_policy.json: execution rules, prompt overrides)
  ↓
Replay Dataset       (JSONL/CSV: historical OHLCV bars by date)
  ↓
Ledger Store         (data/state/: outcomes, experiments, scorecards)
  ↓
Session              (ephemeral: current run's ID, date, results)
```

**Load Order Dependency**: Always load config → registry → baseline → replay → initialize ledger before running simulation.

---

## 3. COMMON PITFALLS & GOTCHAS

### Data & Configuration Gotchas

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Empty replay window** | Backtest returns no outcomes | Check `data/replay/` file exists; dates may have sparse trading data |
| **CSV format wrong** | Import fails w/ "missing column X" | Ensure headers match TWSE/TPEX: `Date`, `Code`, `Name`, `Open`, `High`, `Low`, `Close`, `TradeVolume` |
| **Baseline policy corrupt or missing** | Simulation uses default constraints | Regenerate via `promote-baseline` or delete file to reset |
| **agents.json has disabled agent** | No recommendations from that agent | Check `"enabled": true` flag; verify promptFile path is valid |
| **Missing prompt file** | Agent returns empty recommendation | Verify `prompts/agents/<skill>.md` exists; path must match `promptFile` in config |

### Execution Flow Gotchas

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Recommendation slice is mutable** | Cross-session contamination | Never reuse recommendation slices across multiple `Engine.Run()` calls; copy or rebuild each time |
| **Conviction scores outside [0,100]** | Silent filtering/invalid outcomes | Validate agent conviction before returning; clamp to [0,100] if needed |
| **Quotes missing for symbols** | Simulation skips unquoted recommendations | Check symbol format (must be `XXXX.TW`); verify quote source and date |
| **Control layer order matters** | CRO filters before CIO aggregates | Don't apply control layer before upper layers complete; violates architecture |
| **Empty position allocations** | All orders rejected by simulator | Check `MaxPositionWeight`, `MinRecommendationConviction`, `ReserveCashFraction` constraints |

### Configuration & Validation Gotchas

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Darwinian weight clamping** | Expected 3.0 multiplier becomes 2.5 | Weights auto-clamp to [0.3, 2.5]; document expected behavior, don't hardcode boundary values |
| **Universe filter bypassed** | Agent produces recommendations outside declared universe | Always check agent.Universe; empty = no restriction; non-empty = strict enforcement |
| **ForbiddenActions unenforced** | Agent produces exactly what it shouldn't | Executor code doesn't check ForbiddenActions; treat as documentation-only guidance for prompt engineering |
| **Transaction costs hard to find** | Slippage surprises in P&L | Look for `TransactionCostBPS` and `SlippageBPS` in `SimulationConstraints`; applied via `applyBPS()` helper |

### Mutation & Experiment Gotchas

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Never mutate multiple independent things at once** | Can't identify what caused result change | Break into separate experiments; one prompt + one rule + one constraint = failure |
| **Baseline not loaded before experiment** | Experiment baseline_value=0 or invalid | Always call `baseline.Load()` before running experiment validator; check policy exists |
| **Sparse replay window low confidence** | Made decision on 2 data points | Skip periods with <10 trading days; mark as exploratory only if n<5 |
| **Reusing old recommendation objects** | Data corruption across simulations | Copy recommendations when building outcomes; don't mutate array elements |
| **Prompt mutations hidden in code** | Audit trail lost | Keep mutations in `PromptOverrides` map in baseline policy, not in executor code |

### Testing Pitfalls

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **gofmt not checked before commit** | CI fails on formatting | Always run `gofmt -w .` before pushing; make it pre-commit habit |
| **Test file in wrong package** | Import cycles or missing discovery | Use `*_test.go` suffix in same directory and package as code under test |
| **Forgetting error wrapping** | Stack trace useless when debugging** | Always use `fmt.Errorf("context: %w", err)` not `fmt.Errorf("error: %v", err)` |
| **Hardcoding paths in tests** | Tests fail in CI/different machines | Use relative paths from project root or read from environment |

---

## 4. PROJECT IDIOMS & CODING STYLE

### Go Style Conventions (from project practice)

**Error Handling**:
```go
// ✅ GOOD: Context-wrapped errors
return nil, fmt.Errorf("load registry from %s: %w", path, err)

// ❌ AVOID: Bare error returns
return nil, err

// ❌ AVOID: Printf-style errors
return nil, fmt.Errorf("error: %v", err)
```

**Interface Design**:
```go
// ✅ GOOD: Small, focused interfaces
type Executor interface {
    Supports(agent domain.AgentSpec) bool
    Execute(agent domain.AgentSpec, ...) Result
}

// ❌ AVOID: God interfaces with 10+ methods
type DoEverything interface {
    LoadData() error
    Validate() error
    Execute() error
    // ... many more
}
```

**Package Organization**:
- `internal/domain/` — shared types only, no logic
- `internal/orchestrator/` — flow coordination
- `internal/sim/` — execution engine
- `internal/*/` (other) — domain-specific implementations
- No circular imports; domain is "bottom" layer

**Naming Conventions**:
- Enum-like constants as strings: `Regime`, `Side`, `AgentLayer`
- Suffix `Executor` for plugin implementations
- Suffix `Manager` for stateful coordinators
- Field names match JSON tags exactly (for unmarshal)

### Test Pattern

```go
// ✅ GOOD: Clear setup → act → assert
func TestCRORiskFilter(t *testing.T) {
    // Setup
    registry := SeedRegistry()
    recs := []domain.Recommendation{
        {Agent: "a", Conviction: 40},  // Below floor
        {Agent: "b", Conviction: 80},  // Above floor
    }
    
    // Act
    filtered := applyControlLayer(registry, recs, policy)
    
    // Assert
    if len(filtered) != 1 {
        t.Fatalf("expected 1 filtered rec, got %d", len(filtered))
    }
}
```

### Log Output Style

Project uses `fmt.Printf()` for user-facing output, logging is minimal. When adding logs:
- Use `log.Printf()` for errors/warnings
- Avoid noisy debug logs; prefer structured data (JSON) to files
- Session artifacts in `data/state/sessions/<id>/` are the primary audit trail, not stdout

### Strings vs Constants

- Domain "enums" (Regime, Side, AgentLayer, Layer) kept as strings, **not** iota constants
- Reason: better JSON readability, easier prompt engineering guidance
- Always use named constants: `domain.RegimeRiskOn` not `"RISK_ON"`

---

## 5. DATA FLOW & DEPENDENCIES

### Complete Data Flow Pipeline

```
┌─ Market Data Sourcing ─────────────────────────────────────┐
│                                                              │
│  Replay Mode:        Live Mode:                              │
│  CSV/JSONL ──────→  TWSE/Fugle API ──────┐                 │
│  (preloaded)        (periodic fetch)      │                 │
└─────────────────────────────────────────────────────────────┘
                        ↓
            ┌─ Quote Normalization ───────┐
            │ (Symbol, Price, Volume,     │
            │  IsTradable, AsOf)          │
            └─────────────────────────────┘
                        ↓
         ┌──── Orchestrator Coordination ────────┐
         │                                        │
         │  1. Load Registry (agents.json)        │
         │  2. Load Baseline Policy               │
         │  3. Execute Layers:                    │
         │     - Layer 1: Regime Sync             │
         │     - Layer 2-3: Recommendations      │
         │     - Layer 4: Control Filters        │
         │  4. Apply Execution Policy             │
         │                                        │
         └────────────────────────────────────────┘
                        ↓
         ┌──── Simulation Engine ─────┐
         │ (Order Sizing, Constraints)│
         └──────────────────┬──────────┘
                            ↓
         ┌──────── Ledger Storage ────────────┐
         │ - Outcomes (hit/miss, return)       │
         │ - Experiments (metadata)            │
         │ - Scorecards (per-agent metrics)    │
         │ - Session Summary                   │
         └────────────────────────────────────┘
                            ↓
         ┌──── Evolution Loop ──────────────┐
         │ 1. Select weakest agent          │
         │ 2. Propose mutation              │
         │ 3. Execute on new window         │
         │ 4. Judge vs baseline             │
         │ 5. Promote or revert             │
         └──────────────────────────────────┘
```

### Replay vs Live Mode Branching

**Replay Mode** (default for testing):
- All data pre-loaded from `data/replay/*.jsonl` or imported from CSV
- Deterministic: same input → same output every time
- Used by `cmd/backtest-window`, `cmd/atlas` (with env var)
- Forward returns calculated from future bars in dataset

**Live Mode** (partial):
- Fetches from Fugle or TWSE REST API
- Non-deterministic: depends on API latency, current market state
- Status: marked as TODO in `internal/live/`; use replay as reliable path

### Configuration Resolution Order

```go
// internal/config/config.go
1. Check environment variable (highest priority)
   ATLAS_MARKET_DATA_PROVIDER=twse
   ATLAS_REPLAY_SESSION_DATE=2026-03-26

2. Load .env file (fallback, silent if missing)
   # .env
   ATLAS_LEDGER_DIR=data/state
   
3. Apply code defaults (lowest priority)
   AgentRegistryPath: "configs/agents.json"
   ReplaySessionDate: "2026-03-26"
```

### Ledger Structure

```
data/state/
├── baseline_policy.json          # Current active policy (promoted)
├── recommendation_outcomes.jsonl  # All recommendation outcomes
├── experiments.jsonl              # All experiment metadata
├── sessions/
│   ├── <session-id-1>/
│   │   ├── summary.json           # Session-scoped statistics
│   │   ├── recommendation_outcomes.jsonl
│   │   └── experiments.jsonl
│   ├── <session-id-2>/
│   └── ...
└── scorecards/
    ├── <scorecard-1>.json         # Per-agent metrics
    └── ...
```

**Key**: Session artifacts are the **source of truth** for why a mutation was proposed. Don't rely only on memory or notes.

---

## 6. CONFIGURATION MANAGEMENT

### agents.json Structure (Machine-Readable Policy)

```json
{
  "version": 1,
  "agents": [
    {
      "id": "semi-desk-01",
      "name": "Semiconductor Desk",
      "layer": "sector",
      "skill": "semiconductor_desk",
      "promptFile": "prompts/agents/semiconductor_desk.md",
      "enabled": true,
      "universe": ["2330.TW", "2303.TW", "2454.TW"],
      "primaryMetrics": ["alpha_hit_rate", "risk_adjusted_return"],
      "requiredSkills": ["semiconductor_desk", "earnings_quality"],
      "forbiddenActions": ["illiquid_name_selection", "macro_override"],
      "operatingNotes": ["Differentiate foundry, packaging roles."],
      "darwininWeight": 1.0
    }
  ]
}
```

**Validation Rules**:
- `id` must be unique
- `layer` must be one of: `context`, `sector`, `style`, `superinvestor`, `control`
- `enabled: false` completely disables agent (no calls, no scoring)
- `universe` empty = no restriction; non-empty = only those symbols allowed
- `promptFile` path must exist and be readable
- `forbiddenActions` checked in prompt design, **not** enforced by code

### Baseline Policy (baseline_policy.json)

```json
{
  "version": 1,
  "policyName": "baseline-v23",
  "promotedAt": "2026-04-09T14:30:00Z",
  "constraints": {
    "startingCash": 1000000,
    "maxPositionWeight": 0.05,
    "maxOpenPositions": 10,
    "minTradableVolume": 1000000,
    "transactionCostBPS": 10,
    "slippageBPS": 5,
    "reserveCashFraction": 0.1
  },
  "executionPolicy": {
    "convictionFloor": 50,
    "requireCROPass": true
  },
  "promptOverrides": {
    "semi-desk-01": "custom prompt for this agent..."
  }
}
```

**Why It Matters**:
- Baseline is the **source of truth** for what policies were active during an experiment
- Promotion = overwrite baseline_policy.json
- Revert = restore previous baseline from git history (manual)
- Prompt overrides in baseline, not in code = auditable + no rebuild

### .env File Behavior

```bash
# .env (optional, silent if missing)
ATLAS_MARKET_DATA_PROVIDER=twse
ATLAS_REPLAY_DATA_PATH=data/replay/tw_combined_2024_2026.jsonl
ATLAS_REPLAY_SESSION_DATE=2026-03-29

# Environment vars override .env
export ATLAS_REPLAY_SESSION_DATE=2026-03-30  # Takes precedence
go run ./cmd/atlas  # Uses 2026-03-30, not value from .env
```

### What Can Break Silently

| Configuration | Default If Missing | Consequence |
|----------------|-------------------|-------------|
| agents.json | SeedRegistry() (hardcoded agents) | Uses built-in agents, config ignored |
| baseline_policy.json | DefaultPolicy() (1M cash, 5% pos weight, etc.) | Constraints very conservative |
| Prompt files | Empty string | Agent returns empty recommendation (no skill execution) |
| Replay data | Nil | Live mode attempted; fails if no API key |
| .env file | Skipped silently | Env vars must be set manually |

**Defense**: Always validate config load succeeded; log what was actually activated.

---

## 7. PROMPT ENGINEERING & AGENT CUSTOMIZATION

### Prompt File Location & Naming

All prompts stored in `prompts/agents/` with skill-based naming:
```
prompts/agents/
├── taiwan_macro.md
├── foreign_flow.md
├── semiconductor_desk.md
├── ai_supply_chain_desk.md
├── growth_momentum.md
├── cro_risk.md
└── ...
```

**Rule**: Filename must match `promptFile` entry in `agents.json` exactly.

### Prompt-to-Executor Routing

```
Config specifies promptFile → Orchestrator resolves to string → Executor receives prompt string
```

- Prompt is loaded **only when needed** (lazy, during execution)
- Prompt overrides from baseline_policy.json take precedence over file
- Empty prompt returns empty recommendation

### Mutation Constraints (from agents.json policy)

Each agent declares:
- **requiredSkills**: Must be demonstrated in prompt
- **forbiddenActions**: Explicitly tell prompt to avoid
- **operatingNotes**: Additional constraints

These are **documentation for prompt design**, not enforced by code.

### Prompt Design Anti-Patterns (Common Failures)

| Anti-Pattern | Effect | Fix |
|---------|--------|-----|
| Prompt tries to override risk limits | Ignored by simulator | Keep risk rules in config, not prompt |
| Prompt decides execution order | Ignored; fixed by architecture | Work within layer position; don't drift |
| Prompt conflates market view with execution | Confusion in testing | Separate "what to buy" from "how much" |
| No universe guardrails | Agent may recommend illiquid names | Explicitly list approved symbols in prompt |
| Single-point-failure narratives | Over-conviction | Require multiple evidence types in prompt |

---

## 8. EVOLUTION LOOP & MUTATION WORKFLOW

### Standard Mutation Lifecycle (4 Steps)

```
1. SELECT weakest agent from scorecard
   └─ Run: go run ./cmd/atlas  →  outputs: next_experiment_agent

2. PROPOSE mutation brief
   └─ Modify prompt or rule  →  outputs: MutationBrief

3. EXECUTE on new replay window
   └─ Run: `./scripts/openclaw/execute-next.sh`  →  outputs: experiment record

4. JUDGE against baseline
   └─ Run: `./scripts/openclaw/judge-latest.sh`  →  decision: ACCEPT or REJECT

5. PROMOTE (if accepted) or SKIP (if rejected)
   └─ Run: `./scripts/openclaw/decide.sh --promote EXP-ID --reason "improved X%"`
```

### Mutation Types & Acceptance Rigor

| Mutation Type | Modification | Evidence Bar | Time to Decide |
|---------------|--------------|--------------|-----------------|
| Prompt tightening | Add conviction guard; tighten exclusion | 1 replay session OK | Same day |
| Rule change | Adjust conviction floor; change filter logic | 3-5 sessions min | 1-2 days |
| Constraint revision | Portfolio weight, drawdown limit | 10+ sessions over regimes | 1 week |

### Acceptance Metrics (in order of importance)

1. **Sharpe-like score**: Risk-adjusted return
2. **Max drawdown**: Peak-to-trough loss
3. **Hit rate**: % of recs with positive 1-day return
4. **Average return**: Mean outcome per rec
5. **Turnover**: Churn cost (if applicable)

### Gotchas in Evolution Loop

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Tiny sample window** | Data n=2, promoted anyway | Reject unless n≥10; mark n<5 as exploratory |
| **Cherry-picked period** | Good backtest, bad live | Test across multiple regimes (trending, range, drawdown) |
| **Hidden risk change** | Drawdown exploded, not in brief | Review mutation brief; track all config changes |
| **Regression missed** | Metric A improved, metric B collapsed | Always check Sharpe, drawdown, hit rate together |
| **Baseline drifted** | Comparing against old baseline | Always reload baseline before judging |

---

## 9. QUICK REFERENCE: FILE LOCATIONS & PATTERNS

### Critical Files to Know

| Path | Purpose | Who Edits |
|------|---------|-----------|
| `configs/agents.json` | Agent registry & policies | AI (mutation briefs) or DevOps (config) |
| `data/state/baseline_policy.json` | Current policy rules | Promotion system (automated via promote-baseline) |
| `prompts/agents/*.md` | Agent logic (skill-specific) | AI (prompt mutations) |
| `internal/orchestrator/` | Executor implementations | Dev (rarely; plugin system is stable) |
| `internal/sim/engine.go` | Order sizing logic | Dev (rarely; architecture-critical) |
| `internal/portfolio/darwinian_weights.go` | Agent weight adjustment | Dev (rarely; tuning via config) |
| `AGENTS.md` | Developer operations guide | Dev (when changing build/test process) |
| `docs/evolution-loop.md` | Mutation acceptance policy | Dev (when changing acceptance criteria) |

### Key Utility Scripts

```bash
# Status check (always start here)
./scripts/openclaw/status.sh

# Execute mutation experiment (from proposal)
./scripts/openclaw/execute-next.sh --auto

# Judge latest experiment
./scripts/openclaw/judge-latest.sh --auto --json

# Promote (after human decision)
./scripts/openclaw/decide.sh --promote EXP-ID --reason "reason text"

# Revert (if promotion caused issues)
./scripts/openclaw/decide.sh --revert --reason "reason text"

# Integrated runner (full evolution cycle)
go run enhanced_experiment_runner.go
```

### Test Execution Quick Map

```bash
# Any code touch → these must pass
go fmt ./...
go test ./...

# Specific subsystem testing
go test ./internal/orchestrator/...      # Executor logic
go test ./internal/sim/...               # Simulator
go test ./internal/portfolio/...         # Darwinian weights
go test ./internal/experiment/...        # Mutation logic
go test ./internal/evolution/...         # Acceptance gates

# Full system integration
go run ./cmd/atlas                       # Single session replay
go run ./cmd/backtest-window -start <date> -end <date>  # Multi-session
```

---

## 10. PRODUCTIVITY SHORTCUTS FOR AI AGENTS

### Before Writing Code

1. **Always check the instruction files first**:
   - [AGENTS.md](AGENTS.md) — Build, test, architecture boundaries
   - `.github/instructions/go-core.instructions.md` — Go coding rules
   - `.github/instructions/experiments-guardrails.instructions.md` — Mutation safety
   - `.github/instructions/live-trading.guardrails.instructions.md` — Live path constraints

2. **Verify config & baseline exist**:
   ```bash
   ls -la configs/agents.json data/state/baseline_policy.json
   ```

3. **Run existing tests first** (understand what should pass):
   ```bash
   go test ./internal/orchestrator/... -v
   ```

### When Changing Orchestrator/Sim Logic

- **Never modify** `plugin_host.go` / `system.go` routing without updating tests
- **Always test** the specific executor type individually before merge
- **Check** that control layer still runs **after** all open layers
- **Validate** that quotes are passed correctly (symbol matching, IsTradable flag)

### When Proposing Mutations

- **Read the weakest agent's skill definition** from `docs/skills-map.md`
- **Check requiredSkills and forbiddenActions** in `agents.json`
- **Limit to one change**: one prompt OR one rule OR one constraint, never mixed
- **Document expected improvement** in mutation brief (reference scorecard evidence)

### When Testing Locally

```bash
# Start small → expand scope
go test -run TestSmallest ./internal/orchestrator/...
go test ./internal/orchestrator/...
go test ./...

# Check format before commit
gofmt -l .  # Should output nothing
gofmt -w .  # Fix all violations
```

### Performance Debugging

- Ledger queries slow? → Check session count in `data/state/sessions/`
- Backtest slow? → Use `-start` and `-end` window to narrow range
- Replay import slow? → File size too big; split into date ranges
- Registry load slow? → agents.json is huge; split by layer or market

---

## APPENDIX: Command Cheat Sheet

```bash
# === DAILY OPERATIONS ===
go run ./cmd/atlas                              # Run one-off session, suggest next mutation

go run ./cmd/backtest-window \                  # Evaluate over date range
  -start 2026-03-20 -end 2026-03-29

./scripts/openclaw/status.sh                    # View system status & recommendations

# === MUTATION WORKFLOW ===
./scripts/openclaw/execute-next.sh --auto      # Execute the proposed mutation

./scripts/openclaw/judge-latest.sh --auto      # Judge experiment vs baseline
  --json                                        # Output structured results

./scripts/openclaw/decide.sh \                  # Promote (accept) mutation
  --promote EXP-ID --reason "improved Sharpe"

./scripts/openclaw/decide.sh \                  # Reject (revert) mutation
  --revert --reason "degraded drawdown"

# === DATA IMPORT ===
go run ./cmd/import-replay \                    # Normalize CSV to JSONL
  -source samples/replay/twse_stock_day_all_sample.csv \
  -target data/replay/tw_combined.jsonl

# === BUILD & TEST ===
gofmt -w .                                      # Auto-format all Go files

go build ./...                                  # Verify compilation

go test ./...                                   # Run all tests

go test ./internal/sim/... -v                   # Verbose test output

go test ./internal/orchestrator/... \           # Single test by name
  -run TestCRORiskFilter

go vet ./...                                    # Static analysis

staticcheck ./...                               # Additional lint

# Governance deep verification gates (G2/G3/G4 + M5 + M7)
./scripts/openclaw/verify-governance-gates.sh

# M5 parallel simulation verification (base/stress/shock + determinism)
./scripts/openclaw/verify-parallel-scenarios.sh

# M5 strict mode: require scenario diversity
./scripts/openclaw/verify-parallel-scenarios.sh --require-diversity

# Unified governance strict mode: fail if M5 has no scenario diversity
./scripts/openclaw/verify-governance-gates.sh --require-scenario-diversity

# Required CI status checks for branch protection
# - ci / governance
# - ci / operations

# Guided branch protection setup (safe default: dry-run)
./scripts/openclaw/setup-branch-protection.sh

# Apply branch protection after interactive confirmation
./scripts/openclaw/setup-branch-protection.sh --apply

# Note: --apply creates a pre-change snapshot in data/state/branch-protection-snapshots/

# Human-in-the-loop approval/reject/revert entrypoint (M7)
./scripts/openclaw/human-approval.sh --approve --experiment <EXP-ID> --reason "..."
./scripts/openclaw/human-approval.sh --reject --experiment <EXP-ID> --reason "..."
./scripts/openclaw/human-approval.sh --revert --reason "..."

# Approval event schema + replayability verification
./scripts/openclaw/verify-human-approval-event.sh

# Replay one stored approval event (safe default: dry-run)
./scripts/openclaw/replay-approval-event.sh --event data/state/approvals/<decision-file>.json --dry-run

# === FULL AUTOMATION ===
go run enhanced_experiment_runner.go            # End-to-end evolution: mutate → execute → judge → promote
```

---

**Last Updated**: 2026-04-15

**Document Purpose**: Reference for AI agents; updated when new patterns or gotchas are confirmed.

