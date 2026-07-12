# Phase 3 Implementation: Agent Spawning, PRISM, Reflexivity & Swarm

## Overview

Phase 3 of the Atlas system upgrade introduces four advanced capabilities inspired by Atlas-GIC architecture:

1. **Agent Spawning**: Automated agent creation based on detected knowledge gaps
2. **PRISM Training**: Parallel Regime-Specific Independent Systems with 5 training queues
3. **Soros Reflexivity Engine**: Market feedback loop detection and modeling
4. **MiroFish Swarm**: Parallel simulated futures training

## Implementation Status

### ✅ 1. Agent Spawning Automation (`internal/spawning/`)

**Components:**
- **GapDetector** (`gap_detector.go`): Identifies knowledge gaps in system coverage
  - Sector coverage gaps
  - Style coverage gaps  
  - Market cap coverage gaps
  - Regime-specific performance gaps
  - High-correlation agent pairs
  - Gap prioritization by severity and impact

- **AgentFactory** (`agent_factory.go`): Creates new agent specifications
  - Generates unique agent IDs
  - Creates appropriate skill assignments
  - Builds specialized prompts for each gap type
  - Supports agent variations (conservative, aggressive, contrarian, etc.)
  - Produces collaboration guidelines

- **SpawningManager** (`spawning_manager.go`): Orchestrates full spawning lifecycle
  - Daily gap detection cycles
  - Maximum concurrent spawns (default: 3)
  - Training period tracking (30 days)
  - Validation gate (20+ signals required)
  - Acceptance/rejection workflow
  - Cleanup of rejected agents

**Usage:**
```go
import "atlas/internal/spawning"

// Create spawning manager
manager := spawning.NewSpawningManager(&registry, spawning.DefaultSpawningConfig())

// Start automated spawning
manager.Start()

// Manual spawn for specific gap
spawned, err := manager.ManualSpawn(spawning.GapTypeSector, "biotech", "")

// Accept/reject candidates
manager.AcceptAgent(spawned.AgentID)
manager.RejectAgent(spawned.AgentID, "Poor performance")
```

### ✅ 2. PRISM Multi-Regime Training (`internal/prism/`)

**Components:**
- **5 Independent Training Queues**:
  - `RegimeRiskOn`: Bull market, expansion phase
  - `RegimeRiskOff`: Bear market, contraction phase
  - `RegimeHighVolatility`: Crisis mode, high vol
  - `RegimeLowVolatility`: Range-bound, complacent
  - `RegimeTransition`: Regime change, uncertainty

- **TrainingTask**: Individual training unit with:
  - Agent ID and skill
  - Time window (start/end)
  - Regime classification
  - Priority scoring
  - Training results tracking

- **PRISMManager**: Central coordinator
  - Queue management for all 5 regimes
  - Parallel workers per queue (default: 2)
  - Auto-balancing across queues
  - Priority boost for new agents
  - Training result aggregation

**Configuration:**
```go
config := prism.DefaultPRISMConfig()
config.QueueSize = 100
config.WorkersPerQueue = 2
config.AutoBalance = true
config.PrioritizeNewAgents = true

manager := prism.NewPRISMManager(config)
manager.Start()

// Schedule training
windows := []prism.TrainingWindow{
    {Start: time.Now().Add(-90*24*time.Hour), End: time.Now()},
}
manager.ScheduleTraining(agent, windows)
```

**Metrics:**
- Hit rate per regime
- Sharpe ratio per regime
- Max drawdown per regime
- Queue utilization
- Task completion rates

### ✅ 3. Soros Reflexivity Engine (`internal/reflexivity/`)

**Theory Implementation:**
George Soros' Theory of Reflexivity posits a two-way connection between:
- **Market Bias**: Participants' cognitive biases (trend-following, herding, etc.)
- **Market Reality**: Actual market conditions and prices

**Components:**
- **MarketBias**: Tracks participant sentiment
  - Types: TrendFollowing, Contrarian, Anchoring, Recency, Confirmation, Herding, Overconfidence, FearGreed
  - Magnitude: -1.0 (bearish) to 1.0 (bullish)
  - Confidence: 0.0 to 1.0
  - Source tracking (which agents)

- **MarketReality**: Current market conditions
  - Price, trend, volatility
  - Volume, liquidity
  - Timestamp

- **FeedbackLoop**: The reflexive cycle
  - Direction: Positive (self-reinforcing) or Negative (self-correcting)
  - Strength: 0.0 to 1.0
  - Status: Emerging → Active → Maturing → Exhausting → Completed

**Usage:**
```go
import "atlas/internal/reflexivity"

engine := reflexivity.NewReflexivityEngine()

// Register bias from recommendations
bias := &reflexivity.MarketBias{
    Type: reflexivity.Herding,
    Target: "2330.TW",
    Magnitude: 0.75,
    Confidence: 0.8,
}
engine.RegisterBias(bias)

// Update market reality
reality := &reflexivity.MarketReality{
    Target: "2330.TW",
    Price: 850.0,
    Trend: 0.02,
    Volatility: 0.18,
}
engine.UpdateReality(reality)

// Get feedback loops
loops := engine.GetActiveLoops()
for _, loop := range loops {
    prediction, conf := engine.PredictLoopOutcome(loop.ID)
    fmt.Printf("Loop %s: %s (%.0f%% confidence)\n", loop.ID, prediction, conf*100)
}

// Apply reflexivity adjustments
adjusted := engine.ApplyReflexivityAdjustment(recommendations)
```

**Alerts:**
- Bubble formation (positive feedback + high bullish bias)
- Crash/capitulation (positive feedback + high bearish bias)
- Mean reversion (negative feedback)
- High disagreement (many opposing views)

### ✅ 4. MiroFish Swarm Simulation (`internal/swarm/`)

**Concept:**
Parallel simulation of multiple possible market futures ("fish"), each swimming through a different scenario. Fish that make better predictions survive and reproduce their strategies.

**Components:**
- **MiroFish**: Single simulation unit
  - Assigned to specific market scenario
  - Maintains market state history
  - Generates predictions
  - Tracks performance (accuracy, Sharpe, drawdown)

- **MarketScenario**: Possible future
  - Bull trend, Bear trend, High volatility, Low volatility, Transition
  - Volatility and trend parameters
  - Scheduled market events (earnings, crashes, rallies)

- **MiroFishSwarm**: Fleet coordinator
  - 100 fish distributed across 5 scenarios (20 per scenario)
  - Parallel simulation workers
  - Consensus aggregation
  - Anomaly detection

**Usage:**
```go
import "atlas/internal/swarm"

config := swarm.DefaultSwarmConfig()
swarm := swarm.NewMiroFishSwarm(config)

// Initialize with base market state
baseState := swarm.MarketState{
    Prices: map[string]float64{
        "2330.TW": 850.0,
        "2317.TW": 105.0,
    },
    Volumes: map[string]float64{
        "2330.TW": 50000000,
        "2317.TW": 20000000,
    },
}
swarm.InitializeScenarios(baseState)

// Run simulation
swarm.Start()
time.Sleep(5 * time.Minute) // Let it run
swarm.Stop()

// Get results
result, ok := swarm.GetLatestResult()
if ok {
    fmt.Printf("Consensus: %s (confidence: %.1f%%)\n", 
        result.Consensus["2330.TW"].ConsensusDirection,
        result.Confidence*100)
}

// Export training data for agents
trainingData := swarm.ExportTrainingData()
```

**Training Data Export:**
Each fish's journey through a scenario becomes training material for real agents:
- Market states at each timestep
- Fish predictions and rationales
- Performance outcomes
- Scenario characteristics

## Architecture Integration

### Phase 2 + Phase 3 Combined Flow

```
┌─────────────────────────────────────────────────────────────┐
│                        Layer 1: Macro                        │
│                    (Taiwan Macro Agent)                      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Layer 2: Sector Desks                     │
│  (Semiconductor, Financial, Shipping, Biotech, etc.)        │
│                    + Darwinian Weights                       │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Layer 3: Style Agents                       │
│   (Value, Growth, Momentum, Quality, etc.)                │
│              + Superinvestor Layer (4 agents)               │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Layer 4: Control Layer                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Reflexivity │  │    PRISM    │  │    Swarm    │     │
│  │    Engine    │  │   Training   │  │  Simulation │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│              ↓              ↓              ↓               │
│  ┌────────────────────────────────────────────────────┐   │
│  │           Agent Spawning (Gap Detection)            │   │
│  └────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      CIO Portfolio Synthesis                  │
│              (Darwinian-weighted aggregation)               │
└─────────────────────────────────────────────────────────────┘
```

## File Locations

```
internal/
├── spawning/
│   ├── gap_detector.go        # Knowledge gap detection (450+ lines)
│   ├── agent_factory.go       # Agent creation (350+ lines)
│   └── spawning_manager.go    # Lifecycle orchestration (400+ lines)
├── prism/
│   └── prism_manager.go       # 5-queue training system (400+ lines)
├── reflexivity/
│   └── reflexivity_engine.go  # Soros feedback loops (350+ lines)
└── swarm/
    └── mirofish_swarm.go      # Parallel simulation (650+ lines)
```

## Configuration

### Agent Spawning Config
```go
type SpawningConfig struct {
    MaxActiveSpawns      int           // Max concurrent spawns (default: 3)
    TrainingWindowDays   int           // Training period (default: 30)
    ValidationMinSignals int           // Min signals to evaluate (default: 20)
    AcceptanceThreshold  float64       // Sharpe threshold (default: 0.5)
    CheckInterval        time.Duration // Gap check frequency (default: 24h)
}
```

### PRISM Config
```go
type PRISMConfig struct {
    QueueSize         int           // Per-queue capacity (default: 100)
    WorkersPerQueue   int           // Parallel workers (default: 2)
    AutoBalance       bool          // Rebalance queues (default: true)
    PrioritizeNewAgents bool        // Boost new agents (default: true)
}
```

### MiroFish Swarm Config
```go
type SwarmConfig struct {
    FishCount           int           // Total fish (default: 100)
    SimulationHorizon   time.Duration // Horizon (default: 30 days)
    TimeStep            time.Duration // Granularity (default: 1 hour)
    ConvergenceThreshold float64      // Consensus threshold (default: 0.7)
    Parallelism         int           // Worker threads (default: 10)
}
```

## Daily Operations

### Morning Routine
```bash
# 1. Check overnight gap detection results
cat reports/gap_detection_$(date +%Y%m%d).json

# 2. Review new spawned agents
./scripts/spawning-status.sh

# 3. Run PRISM training for new agents
./scripts/prism-train.sh --new-agents-only

# 4. Check reflexivity alerts
./scripts/reflexivity_report.sh

# 5. Run swarm simulation for market scenarios
./scripts/swarm-run.sh --duration=1h
```

### Evening Routine
```bash
# 1. Darwinian weight adjustment
./scripts/darwinian_adjust.sh

# 2. Spawn agents for detected gaps (if any)
./scripts/spawning-cycle.sh

# 3. Update PRISM training queues
./scripts/prism-update.sh

# 4. Generate reflexivity report
./scripts/reflexivity_report.sh --save

# 5. Export swarm training data
./scripts/swarm-export.sh
```

## Comparison: Atlas-Go vs Atlas-GIC

| Feature | Atlas-Go (Phase 3) | Atlas-GIC |
|---------|-------------------|-----------|
| Agent Spawning | Automated with validation gates | Automated with 40-week training |
| Training Queues | 5 regime-specific (PRISM) | 5 cohort-specific |
| Reflexivity | Bias-reality loop detection | Full reflexivity modeling |
| Swarm Simulation | 100 fish, 5 scenarios | 1000s of fish, continuous |
| Gap Detection | Sector, style, regime, correlation | Multi-dimensional with LLM analysis |
| Validation | 30-day training, 20 signals | 40-week with multiple gates |

## Next Steps (Phase 4 Preview)

1. **Meta-Learning**: Agents learn to learn from swarm results
2. **Adversarial Training**: Red team vs Blue team agent battles
3. **Cross-Market Intelligence**: Taiwan → Global market extension
4. **Real-Time Adaptation**: Sub-second regime detection
5. **Explainable AI**: Full decision traceability

## References

- Atlas-GIC Architecture: https://github.com/chrisworsey55/atlas-gic
- Soros Reflexivity Theory: "The Alchemy of Finance"
- Agent Spawning Design: `docs/skills-map.md` (Layer 4)
- Phase 2 Documentation: `docs/2026-06-15-phase2-implementation.md`

---

**Phase 3 Status**: ✅ COMPLETE  
**Date**: 2025-01-15  
**Version**: atlas-go v3.0-Phase3
