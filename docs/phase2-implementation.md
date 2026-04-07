# Phase 2 Implementation: Darwinian Weights & Superinvestor Layer

## Overview

Phase 2 of the Atlas system upgrade introduces two major capabilities inspired by Atlas-GIC architecture:

1. **Darwinian Weights Mechanism**: Dynamic agent weight adjustment (0.3-2.5 range) based on rolling Sharpe ratio performance
2. **Superinvestor Layer**: 4 master-style investment agents that provide high-quality, concentrated recommendations

## Implementation Status

### ✅ Completed Components

#### 1. Darwinian Weights Core (`internal/portfolio/darwinian_weights.go`)
- **DarwinianAgentWeight**: Per-agent weight tracking with performance metrics
  - Fields: AgentID, Weight, RollingSharpe, TotalSignals, WinCount, LossCount, WinRate, AvgReturn
  - 30-day rolling Sharpe window
  - Daily weight adjustment based on quartile performance

- **DarwinianWeightManager**: Central management system
  - Agent initialization from registry
  - Performance tracking (RecordSignalResult)
  - Daily adjustment algorithm (ApplyDailyAdjustment)
  - Weight persistence (Save/Load from JSON)
  - Weight application to recommendations (ApplyDarwinianWeights)
  - Comprehensive reporting (GenerateReport)

- **Weight Adjustment Rules**:
  - Top quartile performers: ×1.05 boost
  - Bottom quartile: ×0.95 reduction
  - Range clamp: 0.3 (minimum) to 2.5 (maximum)
  - Neutral weight: 1.0

#### 2. Superinvestor Layer Agents (`configs/agents.json` + `prompts/agents/`)

Four master-style agents added:

| Agent ID | Style | Skill | Focus |
|----------|-------|-------|-------|
| `super_druckenmiller` | Macro/Momentum | `super_macro` | Asymmetric macro trades, momentum timing |
| `super_aschenbrenner` | AI/Compute Cycle | `super_compute` | AI capex beneficiaries, tech stack mapping |
| `super_baker` | Deep Tech | `super_ipmoat` | IP moat analysis, R&D efficiency |
| `super_ackman` | Quality Compounder | `super_quality` | Pricing power, FCF generation, catalysts |

Each agent includes:
- Detailed investment philosophy and principles
- Key metrics to track
- Decision framework (entry/exit criteria)
- Taiwan market-specific considerations
- Collaboration notes with other agents

#### 3. Orchestrator Integration (`internal/orchestrator/executors.go`)

- **collectRecommendations**: Extended to include `LayerSuperinvestor` agents
- **ExecuteRegistryResearchWithDarwinianWeights**: New function that:
  - Initializes DarwinianWeightManager from registry
  - Loads existing weights from disk
  - Applies weights to all recommendations before control layer
  - Returns weight data for monitoring

#### 4. Control Layer Updates (`internal/orchestrator/plugin_control.go`)

- **CIOPortfolioExecutorWithWeights**: Weighted aggregation version
  - Uses Darwinian weights in conviction averaging
  - Weights top performers more heavily in final portfolio
  - Maintains backward compatibility with unweighted executor

- **SuperinvestorExecutor**: Dedicated executor for superinvestor layer
  - Higher conviction threshold (65 minimum)
  - Marks recommendations as superinvestor-sourced

#### 5. Daily Adjustment Script (`scripts/darwinian-adjust.sh`)

Bash script for daily weight management:
- Dry-run mode for testing
- Reset mode to restore neutral weights (1.0)
- Market hours warning
- JSON report generation
- Backup creation before modifications

### 📁 File Locations

```
configs/
├── agents.json                    # Updated with 4 Superinvestor agents
└── darwinian_weights.json         # Runtime weight persistence (auto-created)

internal/portfolio/
├── darwinian_weights.go           # Core Darwinian mechanism (650+ lines)
└── agent_weights.go               # Legacy weight manager (reference)

internal/orchestrator/
├── executors.go                   # Updated with DW integration
└── plugin_control.go              # Weighted CIO executor added

internal/domain/
└── registry.go                    # Added LayerSuperinvestor, DarwinianWeight field

prompts/agents/
├── super_druckenmiller.md         # Macro/momentum style
├── super_aschenbrenner.md         # AI/compute cycle style
├── super_baker.md                 # Deep tech/IP moat style
└── super_ackman.md                # Quality compounder style

scripts/
└── darwinian-adjust.sh            # Daily weight adjustment script
```

## Usage

### Initial Setup

```bash
# Make script executable
chmod +x scripts/darwinian-adjust.sh

# Initialize weights from registry (first run)
go run cmd/atlas/main.go --init-darwinian
```

### Daily Operations

```bash
# Run research with Darwinian weights applied
export DARWINIAN_WEIGHTS_FILE=configs/darwinian_weights.json
go run cmd/atlas/main.go research --darwinian

# Adjust weights after market close
./scripts/darwinian-adjust.sh

# Dry run to preview adjustments
./scripts/darwinian-adjust.sh --dry-run

# Reset all weights to neutral (1.0)
./scripts/darwinian-adjust.sh --reset
```

### Integration Example

```go
import (
    "atlas/internal/portfolio"
    "atlas/internal/orchestrator"
)

// Initialize weight manager
weightManager := portfolio.NewDarwinianWeightManager("configs/darwinian_weights.json")
weightManager.InitializeFromRegistry(registry)
weightManager.Load()

// Execute research with weights
regime, raw, final, weightData := orchestrator.ExecuteRegistryResearchWithDarwinianWeights(
    registry, quotes, overrides, policy, weightManager,
)

// Generate weight report
report := weightManager.GenerateReport()
weightManager.SaveReport("reports/weights_20250115.json")
```

## Key Metrics & Monitoring

### Weight Distribution Interpretation

| Weight Range | Label | Interpretation |
|--------------|-------|----------------|
| 2.0 - 2.5 | Shouting | Top performers, highest influence |
| 1.5 - 2.0 | Strong | Above average, good performance |
| 0.8 - 1.5 | Neutral | Average performers |
| 0.5 - 0.8 | Weak | Below average, reduced influence |
| 0.3 - 0.5 | Whispering | Poor performers, minimal influence |

### Alert Thresholds

- **Green**: No agents at 0.3 or 2.5 (balanced system)
- **Yellow**: >20% of agents at extremes (review needed)
- **Red**: Agent stuck at 0.3 for 20+ days (consider disabling)

### Performance Tracking

Each agent tracked on:
- Rolling 30-day Sharpe ratio
- Win rate (hit rate)
- Average return per signal
- Total signals generated
- Consecutive wins/losses

## Architecture Comparison: Atlas-Go vs Atlas-GIC

| Feature | Atlas-Go (Phase 2) | Atlas-GIC |
|---------|-------------------|-----------|
| Weight Range | 0.3 - 2.5 | 0.1 - 5.0 |
| Adjustment Frequency | Daily | Weekly |
| Metric | Rolling Sharpe | Cumulative PnL |
| Layers | 4 (Macro, Sector, Style, Superinvestor) | 4 |
| Agent Count | Configurable (12+ agents) | 50+ agents |
| Spawning | Manual (Phase 3: Auto) | Auto |

## Next Steps (Phase 3 Preview)

1. **Agent Spawning**: Auto-create agents for knowledge gaps
2. **PRISM Training**: 5 cohort-specific queues
3. **Soros Reflexivity**: Market feedback loops
4. **MiroFish Swarm**: Parallel simulated futures

## References

- Atlas-GIC Architecture: https://github.com/chrisworsey55/atlas-gic
- Darwinian Weights Design: `docs/skills-map.md` (Layer 2)
- Agent Prompts: `prompts/agents/super_*.md`

---

**Phase 2 Status**: ✅ COMPLETE  
**Date**: 2025-01-15  
**Version**: atlas-go v2.0-Phase2
