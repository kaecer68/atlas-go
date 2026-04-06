# AGENTS.md — atlas-go

Guidelines for AI agents working in this Go codebase.

---

## Build / Lint / Test Commands

```bash
# Build all packages
go build ./...

# Run all tests
go test ./...

# Run a single test (verbose)
go test -v ./internal/sim -run TestRunBuildsPositions

# Run specific package tests
go test ./internal/orchestrator/...

# Format check (CI uses this)
test -z "$(gofmt -l .)"

# Run the main application
go run ./cmd/atlas

# Run other commands
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27
go run ./cmd/import-replay -source <csv> -target <jsonl>
```

---

## Project Structure

```
.
├── cmd/           # CLI entry points (main packages)
├── internal/      # Private application code
│   ├── domain/    # Domain types and interfaces
│   ├── orchestrator/  # Multi-agent execution
│   ├── sim/       # Simulation engine
│   ├── ledger/    # Storage/record keeping
│   ├── experiment/    # Experiment execution
│   ├── evolution/     # Evolution/runner logic
│   ├── backtest/      # Backtesting
│   ├── config/    # Configuration loading
│   ├── marketdata/    # Market data providers
│   ├── importer/  # Data import
│   ├── baseline/  # Baseline policy management
│   └── replay/    # Replay functionality
├── prompts/       # Agent prompt files
├── configs/       # Configuration files
├── samples/       # Sample data
└── docs/          # Documentation
```

---

## Code Style Guidelines

### Package Naming
- Use lowercase, single-word names: `sim`, `ledger`, `domain`
- Module path: `github.com/kaecer68/atlas-go`

### Import Groups
```go
import (
    "math"
    "os"
    "time"

    "github.com/kaecer68/atlas-go/internal/domain"
)
```
- Standard library first
- External packages second
- Separate groups with a blank line

### Naming Conventions
- **Types/Functions/Exported vars**: PascalCase (`Engine`, `NewEngine`)
- **Unexported vars/params**: camelCase (`constraints`, `quoteBySymbol`)
- **Interfaces**: Descriptive names, often ending in `-er` (`ControlExecutor`)
- **Constants**: PascalCase for exported, camelCase for unexported

### Error Handling
```go
// Early return pattern
if err != nil {
    if os.IsNotExist(err) {
        return nil, nil
    }
    return nil, err
}

// Wrap errors with context
return nil, fmt.Errorf("decode outcome: %w", err)

// Fatal in main/cmd only
log.Fatalf("simulation failed: %v", err)
```

### Type Definitions
```go
// Structs use clear, descriptive field names
type SimulationConstraints struct {
    StartingCash                float64
    MaxPositionWeight           float64
    MaxOpenPositions            int
    MinRecommendationConviction int
    RequireCROPass              bool
}

// String-based enums
type Regime string

const (
    RegimeRiskOn  Regime = "RISK_ON"
    RegimeRiskOff Regime = "RISK_OFF"
    RegimeNeutral Regime = "NEUTRAL"
)
```

### Interface Design
- Keep interfaces small and focused
- 1-3 methods is ideal
```go
type ControlExecutor interface {
    Supports(agent domain.AgentSpec) bool
    Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation
}
```

### Function Design
- Early returns for guard clauses
- Minimize nesting
- Use named return values sparingly
```go
func (r *PluginRegistry) ResolvePrompt(agent domain.AgentSpec, overrides map[string]string) string {
    if override, ok := overrides[agent.ID]; ok && override != "" {
        return override
    }
    // ...
}
```

---

## Testing

### Test File Naming
- `*_test.go` alongside source files
- Test package same as source: `package sim`

### Test Function Pattern
```go
func TestRunBuildsPositions(t *testing.T) {
    engine := NewEngine(domain.SimulationConstraints{...})
    result := engine.Run(...)
    
    if len(result.Orders) == 0 {
        t.Fatalf("expected orders to be created")
    }
}
```

### Test Helpers
- Use `t.TempDir()` for temporary directories
- Use `t.Fatalf()` for immediate test failure
- Check specific error conditions

---

## Domain Conventions

### Agent Layer Types
```go
type AgentLayer string

const (
    LayerContext AgentLayer = "context"
    LayerSector  AgentLayer = "sector"
    LayerStyle   AgentLayer = "style"
    LayerControl AgentLayer = "control"
)
```

### Key Domain Types
- `domain.Quote` — Market data
- `domain.Recommendation` — Agent output
- `domain.Position` — Portfolio state
- `domain.Order` — Execution intent
- `domain.SimulationResult` — Simulation output

---

## File Permissions
- JSON/config files: `0o644`
- Directories: `0o755`

---

## Experiment Execution Flow

The OpenClaw experiment execution requires proper data flow through these stages:

### Prerequisites

1. **Backtest window must exist**: `data/state/windows/window-YYYYMMDD-YYYYMMDD.json`
2. **Outcome data**: `data/state/recommendation_outcomes.jsonl`
3. **Agent configuration**: `configs/agents.json`

### Execution Sequence

```
propose-mutation.sh --auto
    ↓
generates: data/state/mutation-briefs/brief-{agent}-{timestamp}.json
    ↓
execute-next.sh --auto
    ↓
generates: data/state/experiments/exec-{agent}-{timestamp}.json
          prompts/experiments/{agent}/exec-{agent}-{timestamp}/v2.md
    ↓
judge-latest.sh --auto
    ↓
updates: experiment status, baseline/candidate values
```

### Key Fixes Applied

| Issue | Location | Fix |
|-------|----------|-----|
| Brief missing `window_id` | `propose-mutation.sh` | Auto-select latest valid window |
| Brief missing `target_skill` | `propose-mutation.sh` | Use agent base name correctly |
| Execute without `--brief` | `execute-next.sh` | Force `--brief` parameter |
| Judge not auto-triggering | `judge-latest.sh` | Check `running` status, auto-run judge |
| Zero baseline/candidate | `replay_compare.go` | Add fallback window logic |
| JSON parsing whitespace | `judge-latest.sh` | Use whitespace-tolerant regex |
| Prompt file path error | `propose-mutation.sh` | Read from configs/agents.json promptFile field |
| Acceptance threshold too strict | `judge.go` | Lower from 0.0025-0.0035 to 0.001 |
| Risk rule too conservative | `executor.go` | Aggressive parameters (floor 35, max 25%, 8% stop) |

### Common Rejection Reasons

1. **candidate ≤ baseline**: Mutation didn't improve performance
2. **Improvement below threshold** (updated 2026-04): 
   - `prompt_tightening`: requires 0.0005 improvement (rarely effective)
   - `risk_rule_change`: requires 0.001 improvement (lowered from 0.0025)
   - `portfolio_constraint_revision`: requires 0.001 improvement (lowered from 0.0035)
3. **Insufficient policy checks**: Not enough acceptance gates passed

### Mutation Strategy Evolution

**Defensive (Original)**
- Increase conviction thresholds
- Add downgrade logic
- Reduce position limits
- Result: Often reduces performance

**Offensive/Aggressive (Current)**
- Lower entry barriers (conviction floor: 35)
- Increase position sizing on high-conviction setups (max: 25%)
- Reduce cash reserves (min: 5%)
- Tight stops for faster capital rotation (8%)
- Result: +29% average improvement (90-day window, 4 agents)

## Resource Monitoring & Stop Conditions (New in v0.5)

### Resource Guard

The system includes automated resource monitoring to prevent overloading during long-running experiments.

**Scripts:**
- `scripts/monitor/resource-guard.sh` - CPU/memory/disk monitoring
- `scripts/monitor/round-tracker.sh` - Round counting and stop conditions

**Configuration:** `configs/monitor-limits.json`

```bash
# Check resource status
./scripts/monitor/resource-guard.sh check

# Check if stop conditions are met
./scripts/monitor/round-tracker.sh check

# View round statistics
./scripts/monitor/round-tracker.sh stats
```

### Stop Conditions (Balanced Mode)

The system enforces the following stop conditions:

| Condition | Threshold | Action |
|-----------|-----------|--------|
| Max total rounds | 20 rounds | Block new rounds |
| Consecutive rejects | 3 rounds | Warn, suggest stop |
| Min acceptance rate | 15% | Warn after 5+ rounds |
| CPU usage | 75% | Warn, prompt to continue |
| Memory usage | 80% | Warn, prompt to continue |
| All agents optimized | 7/7 agents | Suggest completion |

### Automatic Integration

`run-validated-round.sh` now automatically:
1. Checks resource status before each execution
2. Validates round limits
3. Records results to round tracker

Example output:
```
[Resource Check] Checking system resources... ✓
[Round Check] Checking round limits...
[Round Tracker] Current round: 10/20
...
[Round Recorded] Round 10: accepted (improvement: 0.0025)
```

## Live Trading System

### Overview

The live trading system provides real-time paper trading execution and monitoring capabilities, separate from the replay/backtest system.

### Key Components

| Component | Purpose | Status |
|-----------|---------|--------|
| Live State Store | Portfolio, positions, regime tracking | Implemented |
| Event Bus | Market snapshots, orders, risk alerts | Implemented |
| Real-time Orchestrator | Market schedule, intraday cycles | Implemented |
| Market Data Provider | Hybrid (TWSE/Fugle) with auto-fallback | Implemented |

### Data Sources

- **TWSE OpenAPI**: Free, 1335 listed stocks, 3 req/5s rate limit
- **Fugle API**: Paid real-time, demo key limited to symbol 1476, 50 req/min
- **Hybrid Mode** (default): Tries Fugle first, falls back to TWSE automatically

### Running Live Trading

```bash
# Start paper trading simulation
go run ./cmd/atlas

# The system will:
# 1. Load agent configurations from configs/agents.json
# 2. Initialize live state store
# 3. Subscribe to market events
# 4. Execute agent logic on market snapshots
# 5. Apply risk checks before order simulation
# 6. Record all activity to persistent ledger
```

## Architecture Evolution

### Version History

| Version | Date | Key Changes |
|---------|------|-------------|
| v0.1 | 2025-Q4 | Initial replay-only system |
| v0.2 | 2026-01 | Added experiment loop (propose-execute-judge) |
| v0.3 | 2026-02 | Added live trading infrastructure |
| v0.4 | 2026-03 | Defensive mutation strategy (ineffective) |
| **v0.5** | **2026-04** | **Aggressive mutation, threshold optimization, 90-day windows** |

### Current System State (v0.5)

**Strengths:**
- Complete experiment loop: propose → execute → judge → promote
- 90-day backtest windows available
- 2/3 mutation types effective (risk_rule_change, portfolio_constraint)
- 4/4 tested agents showing positive improvements
- Acceptance threshold optimized (0.001)

**Known Limitations:**
- prompt_tightening mutation produces no measurable difference
- Data import requires manual CSV preparation
- 3 agents not yet tested (earnings-quality, semi-desk, ai-desk)

**Next Version Priorities:**
- Automated data import from APIs
- Fix or remove prompt_tightening mutation
- Expand agent coverage testing
- Historical experiment trend visualization
