# Warmup → Auto-Calibration → Auto-Evolution Design

**Status**: Design Complete  
**Date**: 2026-06-01  
**Branch**: `feat/warmup-auto-evolution` (proposed)

---

## 1. Problem Statement

After fixing sample-size thresholds (Sharpe≥60, VaR≥252, t-test≥63), the system now requires a **warmup period** before statistical engines become reliable. Without explicit warmup management:

- Day 0-59: Darwinian weights are frozen (Sharpe=0), but the system pretends they're valid
- Day 0-251: VaR returns 0, silently disabling tail-risk checks
- Experiments cannot be statistically judged until ≥63 observations per arm
- Calibration tasks run regardless of data sufficiency, producing noisy results

**Goal**: Build a maturity-gated system that is **auto-execution, auto-correction, and auto-evolution first** at every phase. Human intervention is the exception, not the rule.

**Core Principles:**
- **Auto-execution first**: The machine runs continuously, making decisions autonomously
- **Auto-correction first**: When performance degrades, the system self-heals (rollback, disable, fallback)
- **Auto-evolution first**: Improvements are detected, validated, and promoted without human gatekeeping
- **Conservative in BURN_IN, not frozen**: Reduced adjustment magnitude, longer cooldowns, but still active
- **Full auto in CALIBRATING**: No human approval gates; safety limits (cooldown, max loss) are hard-coded
- **Auto-rollback always active**: Any promoted change that degrades is automatically reverted

---

## 2. SystemMaturity State Machine (Auto-First Design)

```
BURN_IN ──60d──► CALIBRATING ──192d──► FULL_AUTO
   │                  │                     │
   │                  │                     │
   ▼                  ▼                     ▼
 Darwinian         Darwinian            Darwinian
 conservative      full auto            full auto
 auto (50% mag)    auto-adjust          auto-adjust
 Risk: static      Risk: static+warn    Risk: dynamic VaR
 Judge: record     Judge: auto-judge    Judge: auto-judge
 Calib: skip       Calib: partial       Calib: full
 AutoRollback:     AutoRollback:        AutoRollback:
   monitor only    ✅ active             ✅ active
 HealthMonitor:    HealthMonitor:       HealthMonitor:
   ✅ active       ✅ active             ✅ active
```

**Key difference from v1**: BURN_IN is no longer frozen. All auto-systems run at every phase, with phase-appropriate conservative parameters.

### 2.1 Maturity Thresholds

| Phase | Min Days | Trigger Condition | Rationale |
|-------|----------|-------------------|-----------|
| `BURN_IN` | 0 | `systemAge < 60` | Sharpe requires 60 samples |
| `CALIBRATING` | 60 | `60 ≤ systemAge < 252` | Most stats valid; VaR still warming |
| `FULL_AUTO` | 252 | `systemAge ≥ 252` | VaR (1yr history) + all calibrations |

### 2.2 Domain Type

```go
// SystemMaturity represents the statistical readiness phase of the system.
type SystemMaturity string

const (
    MaturityBurnIn     SystemMaturity = "burn_in"     // < 60d: conservative defaults
    MaturityCalibrating SystemMaturity = "calibrating" // 60-251d: partial auto
    MaturityFullAuto   SystemMaturity = "full_auto"   // ≥ 252d: all systems go
)

// MaturityThresholds maps maturity phases to their minimum day requirements.
var MaturityThresholds = map[SystemMaturity]int{
    MaturityBurnIn:     0,
    MaturityCalibrating: 60,
    MaturityFullAuto:   252,
}
```

---

## 3. MaturityTracker

Responsibility: Track system age, compute current maturity, publish transitions.

### 3.1 Design

```go
type MaturityTracker struct {
    startDate    time.Time      // system first-start date (persisted)
    current      SystemMaturity // cached current maturity
    mu           sync.RWMutex
    subs         []func(SystemMaturity, SystemMaturity) // transition callbacks
}

func (t *MaturityTracker) Current() SystemMaturity
func (t *MaturityTracker) DaysSinceStart() int
func (t *MaturityTracker) OnTransition(fn func(old, new SystemMaturity))
```

### 3.2 Persistence

Stored at `data/state/maturity_tracker.json`:
```json
{
  "first_start_date": "2026-03-01T09:00:00Z",
  "last_checked": "2026-06-01T10:00:00Z"
}
```

**Important**: `first_start_date` is immutable after first write. If file is missing on startup, set to `time.Now()`.

---

## 4. Maturity-Aware Modules

### 4.1 DarwinianWeightManager

| Maturity | Behavior |
|----------|----------|
| `BURN_IN` | `PerformDailyAdjustment()` is a no-op; all agents remain at `WeightNeutral`. Log: `"darwinian: burn_in mode, skipping adjustment"` |
| `CALIBRATING` | ✅ Normal adjustment with 60-day Sharpe |
| `FULL_AUTO` | ✅ Normal adjustment |

### 4.2 RiskGate / PreTradeGate

| Maturity | Behavior |
|----------|----------|
| `BURN_IN` | Use **static conservative thresholds** from `ParametersConfig.RiskGate`. VaR checks return `PASS` (data insufficient) with warning log. |
| `CALIBRATING` | Use static thresholds. VaR still returns 0 (<252d), so `ruleVaRLimit` logs `"var_warming: N days until full_auto"`. |
| `FULL_AUTO` | ✅ Dynamic VaR/CVaR from historical simulation. All gates fully active. |

### 4.3 Experiment Judge

| Maturity | Behavior |
|----------|----------|
| `BURN_IN` | `passesAcceptance()` always returns `(false, "burn_in: insufficient statistical power")`. Experiments are recorded but never promoted. |
| `CALIBRATING` | ✅ Full Welch t-test, volatility checks, downside protection gates. Auto-promote eligible. |
| `FULL_AUTO` | ✅ Same as CALIBRATING |

### 4.4 CalibrationEngine

| Maturity | Behavior |
|----------|----------|
| `BURN_IN` | `CalibrateAll()` returns empty reports with note `"skipped: burn_in mode"`. |
| `CALIBRATING` | ✅ Run calibration for executors with ≥`minimumSamples` (60). Skip VaR-based calibrations. |
| `FULL_AUTO` | ✅ Full calibration including risk thresholds. |

---

## 5. BackgroundCalibrationScheduler

A `BackgroundTaskFunc` registered with `apigateway.BackgroundTaskManager`, runs daily.

### 5.1 Responsibilities

1. Check `MaturityTracker.Current()`
2. In `BURN_IN`: log maturity status, skip all calibration
3. In `CALIBRATING` / `FULL_AUTO`: trigger maturity-appropriate calibrations

### 5.2 Calibration Tasks

| Task | Min Maturity | Interval | Description |
|------|-------------|----------|-------------|
| `risk_threshold_calib` | `CALIBRATING` | Weekly | `risk.SelfCalibrate()` — update VaR alert thresholds |
| `industry_cycle_calib` | `CALIBRATING` | Weekly | `industry.CycleCalibration()` — update cycle compass weights |
| `factor_weight_calib` | `CALIBRATING` | Weekly | `portfolio.FactorWeightCalibrator()` — update factor weights |
| `conviction_calib` | `CALIBRATING` | Weekly | `orchestrator.CalibrationEngine.CalibrateAll()` — executor thresholds |
| `seasonal_calib` | `FULL_AUTO` | Monthly | `industry.SeasonalCalibrator()` — needs 1yr+ data |
| `macro_risk_calib` | `FULL_AUTO` | Monthly | `config.MacroRiskCalibrator()` — needs long history |

### 5.3 Implementation Sketch

```go
type BackgroundCalibrationScheduler struct {
    tracker    *MaturityTracker
    engine     *orchestrator.CalibrationEngine
    riskCalib  *risk.SelfCalibrator
    cycleCalib *industry.CycleCalibrator
    factorCalib *portfolio.FactorWeightCalibrator
    // ...
}

func (s *BackgroundCalibrationScheduler) RunDaily(ctx context.Context) error {
    maturity := s.tracker.Current()
    switch maturity {
    case MaturityBurnIn:
        logging.Info("scheduler", "burn_in_skip", "days_until_calibrating", 60-s.tracker.DaysSinceStart())
        return nil
    case MaturityCalibrating, MaturityFullAuto:
        return s.runCalibrations(ctx, maturity)
    }
}
```

---

## 6. AutoExperimentProposer

Responsibility: Monitor agent health and auto-generate mutation briefs when performance degrades.

### 6.1 Trigger Conditions

A mutation brief is auto-generated when **ANY** of the following hold:

1. **Rolling Sharpe Drop**: Agent's 60d Sharpe drops below `NegativeSharpeThreshold` (-0.5) for 5 consecutive days
2. **Hit Rate Collapse**: Agent hit rate drops from >60% to <40% over a 20-day window
3. **Weight Trap**: Agent stuck at `WeightMin` (0.3) for >30 days after auto-reset
4. **Factor Decay**: Factor IC (information coefficient) for a factor drops below 0.05 for 20 days

### 6.2 Brief Generation

```go
type AutoProposer struct {
    dwManager    *portfolio.DarwinianWeightManager
    factorEngine *portfolio.FactorEngine
    store        ledger.ExperimentStore
}

func (p *AutoProposer) CheckAndPropose(ctx context.Context) (*domain.MutationBrief, error)
```

Generated brief includes:
- `mutation_type`: `"auto_prompt_optimization"` or `"auto_rule_tightening"`
- `target_agent`: underperforming agent ID
- `trigger_reason`: which condition fired
- `maturity_level`: derived from `MaturityTracker`

### 6.3 Maturity Gating

| Maturity | Behavior |
|----------|----------|
| `BURN_IN` | ❌ No auto-proposal. Too early to judge agent skill. |
| `CALIBRATING` | ✅ Propose, but require manual approval before execution |
| `FULL_AUTO` | ✅ Propose and auto-execute (subject to capital phase limits) |

---

## 7. AutoJudgePromoter

Responsibility: Automatically judge experiments when observations reach statistical thresholds, then auto-promote or revert.

### 7.1 Trigger: Observation Threshold

An experiment becomes "judgeable" when:
- Both baseline and candidate have ≥`MinObservationsForJudge` (63) observations
- AND experiment is at least 3 calendar days old (prevents same-day noise)

### 7.2 Auto-Judge Flow

```
Daily scan of pending experiments
    │
    ▼
Check observations ≥ 63?
    │
    ├── No → skip, log "pending: N/63 observations"
    │
    └── Yes → run Judge.passesAcceptance()
                │
                ├── Rejected → auto-revert (if not already reverted)
                │             log rejection reason
                │             notify dashboard
                │
                └── Accepted → auto-promote
                               write to baseline_policy.json
                               archive experiment
                               notify dashboard
```

### 7.3 Safety Limits

Even in `FULL_AUTO`, these hard limits apply:
1. **Max 1 auto-promote per week** — prevents rapid mutation accumulation
2. **Rollback window**: Auto-promoted experiments can be reverted within 7 days via dashboard
3. **Capital phase gate**: Never auto-promote if `CapitalPhase == PhaseSimulation` (requires `PhasePaper` minimum)

### 7.4 Implementation

```go
type AutoJudgePromoter struct {
    judge     *experiment.Judge
    baseline  *baseline.Manager
    tracker   *MaturityTracker
    store     ledger.ExperimentStore
}

func (a *AutoJudgePromoter) RunDaily(ctx context.Context) error {
    if a.tracker.Current() == MaturityBurnIn {
        return nil // no auto-judge during burn-in
    }
    pending := a.store.ListPending()
    for _, exp := range pending {
        if a.isJudgeable(exp) {
            result := a.judge.Evaluate(exp)
            if result.Accepted {
                a.autoPromote(exp, result)
            } else {
                a.autoRevert(exp, result)
            }
        }
    }
}
```

---

## 8. Integration with Existing Infrastructure

### 8.1 BackgroundTaskManager Registration

```go
// In bootstrap/background.go or main.go:
calibrationTask := &apigateway.ScheduledTask{
    Name:     "auto_calibration",
    Interval: 24 * time.Hour,
    Task:     sched.RunDaily,
    Enabled:  true,
}
bgManager.Register(calibrationTask)

autoJudgeTask := &apigateway.ScheduledTask{
    Name:     "auto_judge_promote",
    Interval: 24 * time.Hour,
    Task:     promoter.RunDaily,
    Enabled:  true,
}
bgManager.Register(autoJudgeTask)
```

### 8.2 Monitoring API Endpoints

New dashboard endpoints:
- `GET /api/system/maturity` → `{ "maturity": "calibrating", "days_since_start": 95, "days_until_full_auto": 157 }`
- `GET /api/system/calibration/status` → list of last calibration runs per task
- `GET /api/experiments/auto-proposals` → pending auto-generated briefs awaiting approval
- `POST /api/experiments/auto-proposals/{id}/approve` → manual approval in CALIBRATING phase

### 8.3 Event Bus Integration

New events:
- `MaturityChanged` — published when system transitions between phases
- `AutoCalibrationCompleted` — published after each calibration task
- `AutoExperimentProposed` — published when proposer generates a brief
- `AutoPromoted` / `AutoReverted` — published after auto-judge decisions

---

## 9. Files to Create / Modify

### 9.1 New Files

| File | Purpose |
|------|---------|
| `internal/domain/maturity.go` | `SystemMaturity` enum, `MaturityTracker` |
| `internal/orchestrator/maturity_plugin.go` | Orchestrator plugin wiring maturity to executors |
| `internal/scheduler/auto_calibration.go` | `BackgroundCalibrationScheduler` |
| `internal/scheduler/auto_calibration_test.go` | Tests |
| `internal/experiment/auto_proposer.go` | `AutoExperimentProposer` |
| `internal/experiment/auto_proposer_test.go` | Tests |
| `internal/experiment/auto_judge_promoter.go` | `AutoJudgePromoter` |
| `internal/experiment/auto_judge_promoter_test.go` | Tests |

### 9.2 Modified Files

| File | Change |
|------|--------|
| `internal/domain/types.go` | Add `SystemMaturity` type |
| `internal/portfolio/darwinian_weights.go` | Gate `PerformDailyAdjustment` on maturity |
| `internal/risk/gate.go` / `pre_trade.go` | Gate VaR on maturity |
| `internal/experiment/judge.go` | Gate `passesAcceptance` on maturity |
| `internal/orchestrator/calibration_engine.go` | Gate `CalibrateAll` on maturity |
| `internal/apigateway/background.go` | (already exists, just register new tasks) |
| `cmd/atlas/main.go` | Wire all components together |

---

## 10. Rollout Plan

| Phase | PR | Scope |
|-------|-----|-------|
| **P0** | `feat/maturity-state-machine` | `SystemMaturity` + `MaturityTracker` + gating in Darwinian/Risk/Judge/Calib |
| **P1** | `feat/auto-calibration-scheduler` | `BackgroundCalibrationScheduler` + daily task registration |
| **P2** | `feat/auto-experiment-proposer` | `AutoExperimentProposer` + trigger logic |
| **P3** | `feat/auto-judge-promoter` | `AutoJudgePromoter` + auto-promote/revert |
| **P4** | `feat/maturity-dashboard` | API endpoints + dashboard UI |

---

## 11. Open Questions

1. **Maturity start date**: Should `first_start_date` be per-deployment (new date on each restart) or persisted across deployments? → **Decision**: Persist across deployments. Deleting `data/state/maturity_tracker.json` resets to burn-in.

2. **Auto-promote in CALIBRATING**: Should we allow auto-promote with a stricter threshold (e.g., t > 3.0 vs t > 2.0)? → **Decision**: Same threshold, but require manual approval via dashboard in CALIBRATING. Auto-promote only in FULL_AUTO.

3. **Backfill for existing deployments**: If a system already has 200 days of history, should it skip BURN_IN? → **Decision**: Yes. On first run, check ledger for earliest record. If `days_earliest_record_to_now ≥ 60`, start in CALIBRATING (or FULL_AUTO if ≥ 252).

---

*End of Design Document*
