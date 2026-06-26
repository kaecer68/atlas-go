# Atlas Risk Module Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 6 phases of risk module gaps — live trading gate, parameter provenance, decision chain transparency, industry integration, portfolio risk adjustment, and stress test CLI.

**Architecture:** Three-layer risk architecture (macro assessment → structural trend → dynamic drawdown) extended with: (1) pre-order risk gate in live trading, (2) ParameterMetadata wrapping for drawdown config, (3) DrawdownBreakdown for dashboard transparency, (4) IndustryRiskAssessment bridging industry cycle data to risk, (5) RiskAdjuster interface for portfolio weight modulation, (6) stress-test CLI for validation.

**Tech Stack:** Go 1.25.0, PostgreSQL 15, Redis 7, existing `internal/risk/`, `internal/live/`, `internal/config/`, `internal/portfolio/`, `internal/industry/`, `internal/monitoring/`

**Dependency Graph:**
```
Phase 1 (CRITICAL) → independent, can start immediately
Phase 2 (HIGH) → independent, can start immediately  
Phase 3 (HIGH) → depends on Phase 2 (uses ParameterMetadata pattern)
Phase 4 (MEDIUM) → independent, can start immediately
Phase 5 (MEDIUM) → depends on Phase 3 (uses DrawdownBreakdown) + Phase 4
Phase 6 (LOW) → depends on Phase 1 + Phase 3 + Phase 5
```

**Parallel Execution Strategy:**
- Wave 1: Phase 1 + Phase 2 + Phase 4 (all independent)
- Wave 2: Phase 3 (depends on Phase 2)
- Wave 3: Phase 5 (depends on Phase 3 + Phase 4)
- Wave 4: Phase 6 (depends on Phase 1 + Phase 3 + Phase 5)

---

### Task 1: Phase 1 — Live Risk Gate (`live/risk_gate.go`)

**Complexity:** Medium
**Priority:** CRITICAL — blocks live trading safety

**Files:**
- Create: `internal/live/risk_gate.go`
- Create: `internal/live/risk_gate_test.go`
- Modify: `internal/live/order_manager.go:93-164` (inject gate into `Run()`)
- Modify: `internal/live/orchestrator.go` (wire gate into orchestrator)

**Context:** `OrderManager.Run()` at line 93 calls `m.broker.SubmitOrder()` directly with zero risk checks. We need a gate that checks: (1) `ShouldHaltTrading()` from drawdown engine, (2) daily loss limit, (3) portfolio VaR limit, (4) circuit breaker state.

- [ ] **Step 1: Write failing test for RiskGate.Check()**

```go
// internal/live/risk_gate_test.go
package live

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func TestRiskGate_Check_AllowsOrder(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct: 0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.currentDailyLoss = 0.01
	gate.currentVaR95 = 0.02
	gate.haltTrading = false

	order := domain.Order{Symbol: "2330", Side: "buy", Quantity: 1000, Price: 900}
	err := gate.Check(context.Background(), order)
	if err != nil {
		t.Errorf("expected order allowed, got error: %v", err)
	}
}

func TestRiskGate_Check_HaltsOnDrawdown(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct: 0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.haltTrading = true

	order := domain.Order{Symbol: "2330", Side: "buy", Quantity: 1000, Price: 900}
	err := gate.Check(context.Background(), order)
	if err == nil {
		t.Error("expected error when trading halted, got nil")
	}
}

func TestRiskGate_Check_RejectsOnDailyLoss(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct: 0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.currentDailyLoss = 0.035 // exceeds 3%

	order := domain.Order{Symbol: "2330", Side: "buy", Quantity: 1000, Price: 900}
	err := gate.Check(context.Background(), order)
	if err == nil {
		t.Error("expected error when daily loss exceeded, got nil")
	}
}

func TestRiskGate_Check_RejectsOnVaR(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct: 0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.currentVaR95 = 0.06 // exceeds 5%

	order := domain.Order{Symbol: "2330", Side: "buy", Quantity: 1000, Price: 900}
	err := gate.Check(context.Background(), order)
	if err == nil {
		t.Error("expected error when VaR exceeded, got nil")
	}
}

func TestRiskGate_UpdateDailyLoss(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{MaxDailyLossPct: 0.03})
	gate.UpdateDailyLoss(-0.01)
	if gate.currentDailyLoss != 0.01 {
		t.Errorf("expected daily loss 0.01, got %f", gate.currentDailyLoss)
	}
}

func TestRiskGate_ResetDaily(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{MaxDailyLossPct: 0.03})
	gate.currentDailyLoss = 0.02
	gate.ResetDaily()
	if gate.currentDailyLoss != 0 {
		t.Errorf("expected daily loss reset to 0, got %f", gate.currentDailyLoss)
	}
}
```

Run: `go test ./internal/live/... -run TestRiskGate -v`
Expected: FAIL — `NewRiskGate` and `RiskGate` not defined

- [ ] **Step 2: Implement RiskGate**

```go
// internal/live/risk_gate.go
package live

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// RiskGateConfig holds configuration for the risk gate.
type RiskGateConfig struct {
	MaxDailyLossPct      float64
	VaRCriticalThreshold float64
}

// RiskGate is a pre-order risk check that blocks orders when risk limits are breached.
type RiskGate struct {
	cfg                  RiskGateConfig
	mu                   sync.RWMutex
	currentDailyLoss     float64
	currentVaR95         float64
	haltTrading          bool
	haltReason           string
	lastResetDate        time.Time
}

// NewRiskGate creates a new risk gate with the given configuration.
func NewRiskGate(cfg RiskGateConfig) *RiskGate {
	return &RiskGate{
		cfg:           cfg,
		lastResetDate: time.Now(),
	}
}

// Check validates whether an order can proceed through risk checks.
// Returns nil if allowed, or an error explaining why the order is blocked.
func (g *RiskGate) Check(ctx context.Context, order domain.Order) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Auto-reset daily loss at midnight
	now := time.Now()
	if now.YearDay() != g.lastResetDate.YearDay() {
		// Release read lock, acquire write lock for reset
		g.mu.RUnlock()
		g.mu.Lock()
		if now.YearDay() != g.lastResetDate.YearDay() {
			g.currentDailyLoss = 0
			g.lastResetDate = now
		}
		g.mu.Unlock()
		g.mu.RLock()
	}

	// Check 1: Trading halted by drawdown engine
	if g.haltTrading {
		return fmt.Errorf("risk gate: trading halted — %s", g.haltReason)
	}

	// Check 2: Daily loss limit
	if g.currentDailyLoss >= g.cfg.MaxDailyLossPct {
		return fmt.Errorf("risk gate: daily loss limit exceeded (%.2f%% >= %.2f%%)",
			g.currentDailyLoss*100, g.cfg.MaxDailyLossPct*100)
	}

	// Check 3: Portfolio VaR critical threshold
	if g.currentVaR95 >= g.cfg.VaRCriticalThreshold {
		return fmt.Errorf("risk gate: portfolio VaR critical (%.2f%% >= %.2f%%)",
			g.currentVaR95*100, g.cfg.VaRCriticalThreshold*100)
	}

	return nil
}

// SetHaltTrading sets the halt state from the drawdown engine.
func (g *RiskGate) SetHaltTrading(halted bool, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.haltTrading = halted
	g.haltReason = reason
}

// UpdateVaR updates the current portfolio VaR95 value.
func (g *RiskGate) UpdateVaR(var95 float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.currentVaR95 = var95
}

// UpdateDailyLoss adds a loss amount (negative PnL) to the daily running total.
func (g *RiskGate) UpdateDailyLoss(lossPct float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if lossPct > 0 {
		g.currentDailyLoss += lossPct
	}
}

// ResetDaily resets daily counters. Called at market open.
func (g *RiskGate) ResetDaily() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.currentDailyLoss = 0
	g.lastResetDate = time.Now()
}

// Status returns the current risk gate status for monitoring.
func (g *RiskGate) Status() map[string]interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return map[string]interface{}{
		"halt_trading":      g.haltTrading,
		"halt_reason":       g.haltReason,
		"daily_loss_pct":    g.currentDailyLoss,
		"daily_loss_limit":  g.cfg.MaxDailyLossPct,
		"var_95":            g.currentVaR95,
		"var_critical":      g.cfg.VaRCriticalThreshold,
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./internal/live/... -run TestRiskGate -v`
Expected: All 6 tests PASS

- [ ] **Step 4: Integrate RiskGate into OrderManager.Run()**

Modify `internal/live/order_manager.go` — add `riskGate` field to `OrderManager` struct and check before `SubmitOrder`:

```go
// Add to OrderManager struct (after line 57):
type OrderManager struct {
	broker       Broker
	eventBus     *ChannelEventBus
	riskGate     *RiskGate  // ADD THIS LINE
	maxRetries   int
	retryBackoff time.Duration
	mu           sync.RWMutex
	orders       map[string]OrderRecord
}

// Update NewOrderManager signature (add riskGate parameter):
func NewOrderManager(broker Broker, eventBus *ChannelEventBus, riskGate *RiskGate, maxRetries int, retryBackoff time.Duration) *OrderManager {
	// ... existing validation ...
	return &OrderManager{
		broker:       broker,
		eventBus:     eventBus,
		riskGate:     riskGate,  // ADD THIS LINE
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
		orders:       make(map[string]OrderRecord),
	}
}

// Add risk gate check at start of Run() method (after line 96, before the retry loop):
func (m *OrderManager) Run(ctx context.Context, order domain.Order) error {
	if m.broker == nil {
		m.broker = NewDryRunBroker()
	}

	// ADD: Risk gate check before any order submission
	if m.riskGate != nil {
		if err := m.riskGate.Check(ctx, order); err != nil {
			if m.eventBus != nil {
				_ = m.eventBus.PublishOrderError(
					"", order.Symbol, string(order.Side),
					order.Price, order.Quantity,
					"risk_gate_blocked", err.Error(), 0, "rejected",
				)
			}
			return fmt.Errorf("risk gate blocked order: %w", err)
		}
	}

	// ... existing retry loop continues unchanged ...
```

- [ ] **Step 5: Update all callers of NewOrderManager**

Search for all `NewOrderManager(` calls and add `nil` or actual `*RiskGate` parameter:

```bash
# Find all callers
grep -rn "NewOrderManager(" internal/ cmd/
```

Expected locations to update:
- `internal/live/orchestrator.go` — wire the actual risk gate
- `internal/live/order_manager_test.go` — pass `nil` for existing tests

- [ ] **Step 6: Wire RiskGate into Orchestrator**

Modify `internal/live/orchestrator.go`:
- Add `riskGate *RiskGate` field to `Orchestrator` struct
- Create risk gate in `NewOrchestrator()` using config values:
  ```go
  riskGate := NewRiskGate(RiskGateConfig{
      MaxDailyLossPct:      cfg.Risk.MaxDailyLossPct.Value,
      VaRCriticalThreshold: cfg.Risk.VaRCriticalThreshold.Value,
  })
  ```
- Connect drawdown engine's `ShouldHaltTrading()` to `riskGate.SetHaltTrading()`
- Connect VaR calculator output to `riskGate.UpdateVaR()`

- [ ] **Step 7: Write integration test for OrderManager + RiskGate**

```go
// internal/live/order_manager_test.go — add new test
func TestOrderManager_Run_BlockedByRiskGate(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.SetHaltTrading(true, "drawdown emergency")

	om := NewOrderManager(NewDryRunBroker(), nil, gate, 0, 0)
	order := domain.Order{Symbol: "2330", Side: "buy", Quantity: 1000, Price: 900}
	err := om.Run(context.Background(), order)
	if err == nil {
		t.Error("expected order blocked by risk gate, got nil")
	}
	if !strings.Contains(err.Error(), "risk gate") {
		t.Errorf("expected risk gate error, got: %v", err)
	}
}
```

Run: `go test ./internal/live/... -run TestOrderManager_Run_BlockedByRiskGate -v`
Expected: PASS

- [ ] **Step 8: Run full CI checks**

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./internal/live/...
go vet ./internal/live/...
```

- [ ] **Step 9: Commit**

```bash
git add internal/live/risk_gate.go internal/live/risk_gate_test.go internal/live/order_manager.go internal/live/order_manager_test.go internal/live/orchestrator.go
git commit -m "feat(risk): add pre-order risk gate to live trading

- New RiskGate with daily loss, VaR, and halt trading checks
- Integrated into OrderManager.Run() before broker submission
- Auto-reset daily counters at midnight
- Event bus publishing for blocked orders"
```

---

### Task 2: Phase 2 — DrawdownConfig Parameter Provenance

**Complexity:** Medium
**Priority:** HIGH — enables audit trail for all drawdown parameters

**Files:**
- Modify: `internal/config/parameters.go` (add `DrawdownParameters` struct)
- Modify: `internal/config/parameters_defaults.go` (add `defaultDrawdownParameters()`)
- Modify: `internal/config/parameters.go:678+` (add validation in `Validate()`)
- Modify: `internal/config/parameters.go:653+` (add to `ParametersConfig`)
- Modify: `internal/config/engine_config.go` (align `DrawdownConfig` with new pattern)
- Modify: `internal/risk/macro_aware_drawdown.go` (use ParameterMetadata values)

**Context:** `DrawdownConfig` in `engine_config.go` is a plain struct with no `ParameterMetadata[T]` wrapping. All other parameter groups (Darwinian, Factor, Risk, etc.) use `ParameterMetadata[T]` with Rationale/Source/Todo. This phase adds the same pattern.

- [ ] **Step 1: Add DrawdownParameters to parameters.go**

Add to `internal/config/parameters.go` after `RiskParameters` struct (around line 231):

```go
// DrawdownParameters holds tunable values for the macro-aware drawdown engine.
type DrawdownParameters struct {
	// Drawdown level thresholds
	NoneMaxExposure     ParameterMetadata[float64] `json:"none_max_exposure"`
	LightPercentage     ParameterMetadata[float64] `json:"light_percentage"`
	LightMaxExposure    ParameterMetadata[float64] `json:"light_max_exposure"`
	ModeratePercentage  ParameterMetadata[float64] `json:"moderate_percentage"`
	ModerateMaxExposure ParameterMetadata[float64] `json:"moderate_max_exposure"`
	SeverePercentage    ParameterMetadata[float64] `json:"severe_percentage"`
	SevereMaxExposure   ParameterMetadata[float64] `json:"severe_max_exposure"`
	EmergencyPercentage ParameterMetadata[float64] `json:"emergency_percentage"`
	EmergencyMaxExposure ParameterMetadata[float64] `json:"emergency_max_exposure"`

	// Structural override thresholds
	OrangeOverrideMinScore ParameterMetadata[float64] `json:"orange_override_min_score"`
	RedOverrideMinScore    ParameterMetadata[float64] `json:"red_override_min_score"`

	// Sector constraints by flow pattern
	SectorConstraintsRiskOff          ParameterMetadata[map[string]float64] `json:"sector_constraints_risk_off"`
	SectorConstraintsCarryTradeUnwind ParameterMetadata[map[string]float64] `json:"sector_constraints_carry_trade_unwind"`
	SectorConstraintsSectorRotation   ParameterMetadata[map[string]float64] `json:"sector_constraints_sector_rotation"`
}
```

Add `Drawdown` field to `ParametersConfig` struct (around line 674):

```go
type ParametersConfig struct {
	// ... existing fields ...
	Drawdown          DrawdownParameters          `json:"drawdown"`
	// ... remaining fields ...
}
```

- [ ] **Step 2: Add defaultDrawdownParameters() to parameters_defaults.go**

Add to `internal/config/parameters_defaults.go`:

```go
func defaultDrawdownParameters() DrawdownParameters {
	return DrawdownParameters{
		NoneMaxExposure: ParameterMetadata[float64]{
			Value:     1.0,
			Rationale: "No drawdown: full exposure allowed",
			Source:    SourceHeuristic,
		},
		LightPercentage: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "15% reduction for elevated macro risk (yellow)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [0.10, 0.20] range",
		},
		LightMaxExposure: ParameterMetadata[float64]{
			Value:     0.85,
			Rationale: "85% max exposure under light drawdown",
			Source:    SourceHeuristic,
		},
		ModeratePercentage: ParameterMetadata[float64]{
			Value:     0.35,
			Rationale: "35% reduction for orange risk without structural override",
			Source:    SourceHeuristic,
		},
		ModerateMaxExposure: ParameterMetadata[float64]{
			Value:     0.65,
			Rationale: "65% max exposure under moderate drawdown",
			Source:    SourceHeuristic,
		},
		SeverePercentage: ParameterMetadata[float64]{
			Value:     0.60,
			Rationale: "60% reduction for red risk or severe macro events",
			Source:    SourceHeuristic,
		},
		SevereMaxExposure: ParameterMetadata[float64]{
			Value:     0.40,
			Rationale: "40% max exposure under severe drawdown",
			Source:    SourceHeuristic,
		},
		EmergencyPercentage: ParameterMetadata[float64]{
			Value:     0.90,
			Rationale: "90% reduction for systemic crisis (halt trading threshold)",
			Source:    SourceHeuristic,
		},
		EmergencyMaxExposure: ParameterMetadata[float64]{
			Value:     0.10,
			Rationale: "10% max exposure in emergency — triggers halt",
			Source:    SourceHeuristic,
		},
		OrangeOverrideMinScore: ParameterMetadata[float64]{
			Value:     0.55,
			Rationale: "Minimum structural override score to reduce orange drawdown",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from structural trend backtest",
		},
		RedOverrideMinScore: ParameterMetadata[float64]{
			Value:     0.75,
			Rationale: "Minimum structural override score to reduce red drawdown (very high bar)",
			Source:    SourceHeuristic,
		},
		SectorConstraintsRiskOff: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"ai_supply_chain": 0.3,
				"small_cap":       0.2,
				"emerging_market": 0.1,
				"gold":            1.5,
				"utilities":       1.2,
			},
			Rationale: "Reduce risk assets, increase defensive during risk_off flow",
			Source:    SourceHeuristic,
		},
		SectorConstraintsCarryTradeUnwind: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"all_equities": 0.1,
				"tech":         0.05,
				"financials":   0.1,
				"cash":         2.0,
			},
			Rationale: "Exit equities, move to cash during carry trade unwind",
			Source:    SourceHeuristic,
		},
		SectorConstraintsSectorRotation: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"energy":              1.8,
				"oil_services":        1.5,
				"high_valuation_tech": 0.3,
				"rate_sensitive":      0.4,
			},
			Rationale: "Rotate to energy, reduce tech during sector rotation",
			Source:    SourceHeuristic,
		},
	}
}
```

- [ ] **Step 3: Wire into DefaultParametersConfig()**

Add `Drawdown: defaultDrawdownParameters(),` to `DefaultParametersConfig()` in `parameters_defaults.go`.

- [ ] **Step 4: Add validation in ParametersConfig.Validate()**

Add to `internal/config/parameters.go` in the `Validate()` method (after Risk constraints, around line 805):

```go
// Drawdown constraints
if p.Drawdown.NoneMaxExposure.Value < 0 || p.Drawdown.NoneMaxExposure.Value > 1 {
	return fmt.Errorf("drawdown.none_max_exposure (%.3f) must be in [0,1]", p.Drawdown.NoneMaxExposure.Value)
}
if p.Drawdown.LightPercentage.Value < 0 || p.Drawdown.LightPercentage.Value > 1 {
	return fmt.Errorf("drawdown.light_percentage (%.3f) must be in [0,1]", p.Drawdown.LightPercentage.Value)
}
if p.Drawdown.LightMaxExposure.Value < 0 || p.Drawdown.LightMaxExposure.Value > 1 {
	return fmt.Errorf("drawdown.light_max_exposure (%.3f) must be in [0,1]", p.Drawdown.LightMaxExposure.Value)
}
if p.Drawdown.ModeratePercentage.Value < p.Drawdown.LightPercentage.Value {
	return fmt.Errorf("drawdown.moderate_percentage (%.3f) must be >= light_percentage (%.3f)",
		p.Drawdown.ModeratePercentage.Value, p.Drawdown.LightPercentage.Value)
}
if p.Drawdown.SeverePercentage.Value < p.Drawdown.ModeratePercentage.Value {
	return fmt.Errorf("drawdown.severe_percentage (%.3f) must be >= moderate_percentage (%.3f)",
		p.Drawdown.SeverePercentage.Value, p.Drawdown.ModeratePercentage.Value)
}
if p.Drawdown.EmergencyPercentage.Value < p.Drawdown.SeverePercentage.Value {
	return fmt.Errorf("drawdown.emergency_percentage (%.3f) must be >= severe_percentage (%.3f)",
		p.Drawdown.EmergencyPercentage.Value, p.Drawdown.SeverePercentage.Value)
}
if p.Drawdown.OrangeOverrideMinScore.Value < 0 || p.Drawdown.OrangeOverrideMinScore.Value > 1 {
	return fmt.Errorf("drawdown.orange_override_min_score (%.3f) must be in [0,1]", p.Drawdown.OrangeOverrideMinScore.Value)
}
if p.Drawdown.RedOverrideMinScore.Value < p.Drawdown.OrangeOverrideMinScore.Value {
	return fmt.Errorf("drawdown.red_override_min_score (%.3f) must be >= orange_override_min_score (%.3f)",
		p.Drawdown.RedOverrideMinScore.Value, p.Drawdown.OrangeOverrideMinScore.Value)
}
```

- [ ] **Step 5: Update MacroAwareDrawdownEngine to use ParameterMetadata**

Modify `internal/risk/macro_aware_drawdown.go`:
- `NewMacroAwareDrawdownEngineWithConfig()` should read from `ParametersConfig.Drawdown` when `engine_config.go` values are not available
- Add a new constructor `NewMacroAwareDrawdownEngineFromParameters()` that reads from `ParametersConfig`

```go
// NewMacroAwareDrawdownEngineFromParameters creates engine from ParametersConfig
func NewMacroAwareDrawdownEngineFromParameters() *MacroAwareDrawdownEngine {
	cfg := config.GetParametersConfig()
	dp := cfg.Drawdown
	return &MacroAwareDrawdownEngine{
		levels: map[DrawdownAction]DrawdownLevel{
			DrawdownNone:      {Action: DrawdownNone, Percentage: 0.0, MaxExposure: dp.NoneMaxExposure.Value},
			DrawdownLight:     {Action: DrawdownLight, Percentage: dp.LightPercentage.Value, MaxExposure: dp.LightMaxExposure.Value},
			DrawdownModerate:  {Action: DrawdownModerate, Percentage: dp.ModeratePercentage.Value, MaxExposure: dp.ModerateMaxExposure.Value},
			DrawdownSevere:    {Action: DrawdownSevere, Percentage: dp.SeverePercentage.Value, MaxExposure: dp.SevereMaxExposure.Value},
			DrawdownEmergency: {Action: DrawdownEmergency, Percentage: dp.EmergencyPercentage.Value, MaxExposure: dp.EmergencyMaxExposure.Value},
		},
		cfg: config.DrawdownConfig{
			OrangeOverrideMinScore:            dp.OrangeOverrideMinScore.Value,
			RedOverrideMinScore:               dp.RedOverrideMinScore.Value,
			SectorConstraintsRiskOff:          dp.SectorConstraintsRiskOff.Value,
			SectorConstraintsCarryTradeUnwind: dp.SectorConstraintsCarryTradeUnwind.Value,
			SectorConstraintsSectorRotation:   dp.SectorConstraintsSectorRotation.Value,
		},
	}
}
```

- [ ] **Step 6: Run full CI checks**

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./internal/risk/...
go test ./internal/config/...
go vet ./...
staticcheck ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/config/parameters.go internal/config/parameters_defaults.go internal/risk/macro_aware_drawdown.go
git commit -m "feat(risk): add DrawdownParameters with ParameterMetadata provenance

- New DrawdownParameters struct with Rationale/Source/Todo for all drawdown levels
- Default values in parameters_defaults.go aligned with existing engine_config.go
- Validation in ParametersConfig.Validate() for monotonic drawdown levels
- New constructor NewMacroAwareDrawdownEngineFromParameters()"
```

---

### Task 3: Phase 3 — Decision Chain Breakdown

**Complexity:** Complex
**Priority:** HIGH — enables dashboard transparency for risk decisions

**Files:**
- Create: `internal/risk/drawdown_breakdown.go`
- Create: `internal/risk/drawdown_breakdown_test.go`
- Modify: `internal/risk/macro_aware_drawdown.go` (add breakdown to Evaluate)
- Modify: `internal/monitoring/dashboard_api.go` (expose breakdown in API)
- Modify: `internal/domain/types.go` (add DrawdownBreakdown struct if needed for JSON serialization)

**Context:** `FactorScoreBreakdown` pattern in `internal/portfolio/factor_engine.go` provides transparent factor scoring. `MacroAwareDrawdownEngine.Evaluate()` currently returns only `Action`, `Percentage`, `MaxExposure`, `Rationale` — no step-by-step breakdown of how the decision was reached.

- [ ] **Step 1: Define DrawdownBreakdown struct**

Create `internal/risk/drawdown_breakdown.go`:

```go
package risk

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

// DrawdownStep represents a single step in the drawdown decision chain.
type DrawdownStep struct {
	Rule   string  `json:"rule"`   // e.g., "macro_risk_level", "structural_override", "flow_pattern"
	Delta  float64 `json:"delta"`  // impact on exposure (negative = reduce)
	Reason string  `json:"reason"` // human-readable explanation
}

// DrawdownBreakdown provides transparent breakdown of a drawdown decision.
type DrawdownBreakdown struct {
	MacroRiskLevel       narrative.MacroRiskLevel `json:"macro_risk_level"`
	ForeignOutflowProb   float64                  `json:"foreign_outflow_prob"`
	StructuralTrendName  string                   `json:"structural_trend_name,omitempty"`
	StructuralScore      float64                  `json:"structural_score"`
	CanWithstand         bool                     `json:"can_withstand"`
	StructuralOverride   bool                     `json:"structural_override"`
	PrimaryFlow          string                   `json:"primary_flow"`
	Steps                []DrawdownStep           `json:"steps"`
	FinalAction          DrawdownAction           `json:"final_action"`
	FinalMaxExposure     float64                  `json:"final_max_exposure"`
	Timestamp            time.Time                `json:"timestamp"`
}

// DrawdownDecisionWithBreakdown wraps the existing decision with full breakdown.
type DrawdownDecisionWithBreakdown struct {
	Decision  *MacroAwareDrawdownDecision `json:"decision"`
	Breakdown *DrawdownBreakdown          `json:"breakdown"`
}
```

- [ ] **Step 2: Write tests for DrawdownBreakdown generation**

```go
// internal/risk/drawdown_breakdown_test.go
package risk

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestMacroAwareDrawdownEngine_EvaluateWithBreakdown_GreenRisk(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()
	macro := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskGreen,
		ForeignOutflowProb: 15.0,
		PrimaryFlow:        "normal",
	}
	structural := &narrative.StructuralTrendAssessment{
		OverrideScore: 0.8,
	}

	result := engine.EvaluateWithBreakdown(macro, structural)
	if result.Breakdown.FinalAction != DrawdownNone {
		t.Errorf("expected DrawdownNone, got %v", result.Breakdown.FinalAction)
	}
	if len(result.Breakdown.Steps) == 0 {
		t.Error("expected at least one step in breakdown")
	}
}

func TestMacroAwareDrawdownEngine_EvaluateWithBreakdown_RedRiskWithOverride(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()
	macro := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskRed,
		ForeignOutflowProb: 85.0,
		PrimaryFlow:        "risk_off",
	}
	structural := &narrative.StructuralTrendAssessment{
		OverrideScore:     0.80, // exceeds RedOverrideMinScore (0.75)
		ShouldOverrideRisk: true,
	}

	result := engine.EvaluateWithBreakdown(macro, structural)
	if !result.Breakdown.StructuralOverride {
		t.Error("expected structural override to be true")
	}
	if result.Breakdown.FinalAction != DrawdownModerate {
		t.Errorf("expected DrawdownModerate with override, got %v", result.Breakdown.FinalAction)
	}
}

func TestDrawdownBreakdown_StepsAreOrdered(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()
	macro := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskOrange,
		ForeignOutflowProb: 60.0,
		PrimaryFlow:        "sector_rotation",
	}
	structural := &narrative.StructuralTrendAssessment{
		OverrideScore:     0.40, // below OrangeOverrideMinScore (0.55)
		ShouldOverrideRisk: false,
	}

	result := engine.EvaluateWithBreakdown(macro, structural)
	// Steps should be: macro assessment → structural check → flow pattern → final action
	if len(result.Breakdown.Steps) < 3 {
		t.Errorf("expected at least 3 steps, got %d", len(result.Breakdown.Steps))
	}
}
```

Run: `go test ./internal/risk/... -run TestMacroAwareDrawdownEngine_EvaluateWithBreakdown -v`
Expected: FAIL — `EvaluateWithBreakdown` not defined

- [ ] **Step 3: Implement EvaluateWithBreakdown**

Add to `internal/risk/macro_aware_drawdown.go`:

```go
// EvaluateWithBreakdown makes a drawdown decision and returns full step-by-step breakdown.
func (e *MacroAwareDrawdownEngine) EvaluateWithBreakdown(
	macroAssessment *narrative.MacroRiskAssessment,
	structuralAssessment *narrative.StructuralTrendAssessment,
) *DrawdownDecisionWithBreakdown {
	decision := e.Evaluate(macroAssessment, structuralAssessment)

	breakdown := &DrawdownBreakdown{
		MacroRiskLevel:     macroAssessment.Level,
		ForeignOutflowProb: macroAssessment.ForeignOutflowProb,
		PrimaryFlow:        macroAssessment.PrimaryFlow,
		Timestamp:          decision.Timestamp,
		FinalAction:        decision.Action,
		FinalMaxExposure:   decision.MaxExposure,
	}

	// Step 1: Macro risk assessment
	breakdown.Steps = append(breakdown.Steps, DrawdownStep{
		Rule:   "macro_risk_level",
		Delta:  0.0,
		Reason: fmt.Sprintf("Macro risk level: %s (foreign outflow probability: %.1f%%)",
			macroAssessment.Level.String(), macroAssessment.ForeignOutflowProb),
	})

	// Step 2: Structural trend check
	canWithstand := e.canWithstandMacroRisk(macroAssessment.Level, structuralAssessment)
	breakdown.CanWithstand = canWithstand
	if structuralAssessment != nil && structuralAssessment.DominantTrend != nil {
		breakdown.StructuralTrendName = structuralAssessment.DominantTrend.Name
		breakdown.StructuralScore = structuralAssessment.OverrideScore
	}
	breakdown.StructuralOverride = canWithstand && structuralAssessment != nil && structuralAssessment.ShouldOverrideRisk

	if breakdown.StructuralOverride {
		breakdown.Steps = append(breakdown.Steps, DrawdownStep{
			Rule:   "structural_override",
			Delta:  -0.20, // override reduces drawdown by one level
			Reason: fmt.Sprintf("Structural trend '%s' (score: %.2f) overrides macro risk",
				breakdown.StructuralTrendName, breakdown.StructuralScore),
		})
	} else if structuralAssessment != nil {
		breakdown.Steps = append(breakdown.Steps, DrawdownStep{
			Rule:   "structural_check",
			Delta:  0.0,
			Reason: fmt.Sprintf("No structural override (score: %.2f, canWithstand: %v)",
				breakdown.StructuralScore, canWithstand),
		})
	}

	// Step 3: Flow pattern / sector constraints
	if macroAssessment.PrimaryFlow != "" {
		breakdown.Steps = append(breakdown.Steps, DrawdownStep{
			Rule:   "flow_pattern",
			Delta:  0.0,
			Reason: fmt.Sprintf("Capital flow pattern: %s — sector constraints applied",
				macroAssessment.PrimaryFlow),
		})
	}

	// Step 4: Final action
	level := e.levels[decision.Action]
	breakdown.Steps = append(breakdown.Steps, DrawdownStep{
		Rule:   "final_action",
		Delta:  level.Percentage,
		Reason: fmt.Sprintf("Drawdown action: %s (%.0f%% reduction, max exposure %.0f%%)",
			decision.Action.String(), level.Percentage*100, level.MaxExposure*100),
	})

	return &DrawdownDecisionWithBreakdown{
		Decision:  decision,
		Breakdown: breakdown,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/risk/... -run TestMacroAwareDrawdownEngine_EvaluateWithBreakdown -v`
Expected: All 3 tests PASS

- [ ] **Step 5: Expose breakdown in Dashboard API**

Modify `internal/monitoring/dashboard_api.go` — find the risk metrics handler and add breakdown field:

```go
// In the risk metrics API response struct, add:
type RiskMetricsResponse struct {
	// ... existing fields ...
	DrawdownBreakdown *risk.DrawdownBreakdown `json:"drawdown_breakdown,omitempty"`
}
```

- [ ] **Step 6: Run full CI checks**

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./internal/risk/...
go test ./internal/monitoring/...
go vet ./...
staticcheck ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/risk/drawdown_breakdown.go internal/risk/drawdown_breakdown_test.go internal/risk/macro_aware_drawdown.go internal/monitoring/dashboard_api.go
git commit -m "feat(risk): add DrawdownBreakdown for transparent decision chain

- New DrawdownBreakdown struct with step-by-step decision trace
- EvaluateWithBreakdown() method on MacroAwareDrawdownEngine
- 4-step breakdown: macro risk → structural check → flow pattern → final action
- Dashboard API exposure for drawdown_breakdown field"
```

---

### Task 4: Phase 4 — Industry Risk Integration

**Complexity:** Medium
**Priority:** MEDIUM — enriches risk with industry cycle data

**Files:**
- Create: `internal/risk/industry_risk.go`
- Create: `internal/risk/industry_risk_test.go`
- Modify: `internal/risk/macro_aware_drawdown.go` (accept industry assessment in Evaluate)

**Context:** `internal/industry/cycle.go` provides `CycleTracker` with business cycle phases, inventory cycles, and capex cycles per industry. This data should feed into the risk module to adjust drawdown decisions based on industry health.

- [ ] **Step 1: Define IndustryRiskAssessment struct**

Create `internal/risk/industry_risk.go`:

```go
package risk

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// IndustryRiskAssessment aggregates industry-level risk signals.
type IndustryRiskAssessment struct {
	RecessionIndustryCount int                `json:"recession_industry_count"`
	ExpansionIndustryCount int                `json:"expansion_industry_count"`
	WeightedCycleScore     float64            `json:"weighted_cycle_score"` // -1.0 to 1.0
	TopRiskIndustries      []IndustryRiskItem `json:"top_risk_industries"`
	Timestamp              time.Time          `json:"timestamp"`
}

// IndustryRiskItem represents risk data for a single industry.
type IndustryRiskItem struct {
	IndustryID    string             `json:"industry_id"`
	BusinessCycle industry.CyclePhase `json:"business_cycle"`
	Confidence    float64            `json:"confidence"`
	PhaseScore    float64            `json:"phase_score"` // from GetPhaseScore()
	Weight        float64            `json:"weight"`      // market cap weight
}

// IndustryRiskProvider is the interface for getting industry risk data.
type IndustryRiskProvider interface {
	Assess() *IndustryRiskAssessment
}

// CycleTrackerRiskProvider wraps industry.CycleTracker as a risk provider.
type CycleTrackerRiskProvider struct {
	tracker      *industry.CycleTracker
	sectorWeights map[string]float64
}

// NewCycleTrackerRiskProvider creates a risk provider from a CycleTracker.
func NewCycleTrackerRiskProvider(tracker *industry.CycleTracker, sectorWeights map[string]float64) *CycleTrackerRiskProvider {
	return &CycleTrackerRiskProvider{
		tracker:       tracker,
		sectorWeights: sectorWeights,
	}
}

// Assess computes the industry risk assessment from cycle data.
func (p *CycleTrackerRiskProvider) Assess() *IndustryRiskAssessment {
	positions := p.tracker.GetAllPositions()
	assessment := &IndustryRiskAssessment{
		Timestamp: time.Now(),
	}

	var totalWeight float64
	var weightedScore float64

	for id, pos := range positions {
		weight := p.sectorWeights[id]
		if weight == 0 {
			weight = 0.05 // default weight for unknown industries
		}

		item := IndustryRiskItem{
			IndustryID:    id,
			BusinessCycle: pos.BusinessCycle,
			Confidence:    pos.Confidence,
			PhaseScore:    pos.GetPhaseScore(),
			Weight:        weight,
		}
		assessment.TopRiskIndustries = append(assessment.TopRiskIndustries, item)

		switch pos.BusinessCycle {
		case industry.CycleRecession:
			assessment.RecessionIndustryCount++
		case industry.CycleExpansion, industry.CycleRecovery:
			assessment.ExpansionIndustryCount++
		}

		weightedScore += pos.GetPhaseScore() * weight
		totalWeight += weight
	}

	if totalWeight > 0 {
		assessment.WeightedCycleScore = weightedScore / totalWeight
	}

	return assessment
}
```

- [ ] **Step 2: Write tests for IndustryRiskAssessment**

```go
// internal/risk/industry_risk_test.go
package risk

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

func TestCycleTrackerRiskProvider_Assess(t *testing.T) {
	tracker := industry.NewCycleTracker()
	weights := map[string]float64{
		"semiconductor":   0.19,
		"ai_supply_chain": 0.15,
		"financials":      0.11,
	}

	provider := NewCycleTrackerRiskProvider(tracker, weights)
	assessment := provider.Assess()

	if assessment.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if len(assessment.TopRiskIndustries) == 0 {
		t.Error("expected at least one industry in assessment")
	}
	// Default positions have semiconductor in expansion
	if assessment.ExpansionIndustryCount == 0 {
		t.Error("expected at least one expansion industry from defaults")
	}
}

func TestIndustryRiskAssessment_WeightedScore(t *testing.T) {
	tracker := industry.NewCycleTracker()
	weights := map[string]float64{
		"semiconductor": 1.0, // single industry for simplicity
	}
	provider := NewCycleTrackerRiskProvider(tracker, weights)
	assessment := provider.Assess()

	// Score should be within [-1, 1] range
	if assessment.WeightedCycleScore < -1.0 || assessment.WeightedCycleScore > 1.0 {
		t.Errorf("weighted score out of range: %f", assessment.WeightedCycleScore)
	}
}
```

Run: `go test ./internal/risk/... -run TestCycleTrackerRiskProvider -v`
Expected: PASS

- [ ] **Step 3: Integrate industry assessment into drawdown engine**

Add optional industry assessment parameter to `EvaluateWithBreakdown`:

```go
// Modify EvaluateWithBreakdown signature:
func (e *MacroAwareDrawdownEngine) EvaluateWithBreakdown(
	macroAssessment *narrative.MacroRiskAssessment,
	structuralAssessment *narrative.StructuralTrendAssessment,
	industryAssessment *IndustryRiskAssessment, // NEW
) *DrawdownDecisionWithBreakdown {
	// ... existing logic ...

	// Add industry risk step if assessment provided
	if industryAssessment != nil {
		breakdown.Steps = append(breakdown.Steps, DrawdownStep{
			Rule:   "industry_cycle_risk",
			Delta:  industryAssessment.WeightedCycleScore * 0.1, // up to ±10% adjustment
			Reason: fmt.Sprintf("Industry cycle score: %.2f (%d recession, %d expansion)",
				industryAssessment.WeightedCycleScore,
				industryAssessment.RecessionIndustryCount,
				industryAssessment.ExpansionIndustryCount),
		})
	}
}
```

- [ ] **Step 4: Run full CI checks**

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./internal/risk/...
go vet ./...
staticcheck ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/risk/industry_risk.go internal/risk/industry_risk_test.go internal/risk/macro_aware_drawdown.go
git commit -m "feat(risk): add IndustryRiskAssessment from cycle tracker data

- New IndustryRiskAssessment struct aggregating industry cycle signals
- CycleTrackerRiskProvider wrapping industry.CycleTracker
- Weighted cycle score computation using sector weights
- Integration into drawdown breakdown as industry_cycle_risk step"
```

---

### Task 5: Phase 5 — Portfolio Risk Adjustment

**Complexity:** Complex
**Priority:** MEDIUM — connects risk decisions to portfolio weights

**Files:**
- Create: `internal/portfolio/risk_adjuster.go`
- Create: `internal/portfolio/risk_adjuster_test.go`
- Modify: `internal/portfolio/darwinian_weights.go` (apply risk adjustment)
- Modify: `internal/portfolio/optimizer.go` (use risk-adjusted constraints)

**Context:** Darwinian weights (`internal/portfolio/darwinian_weights.go`) and factor weights operate independently from risk levels. When drawdown is active, portfolio weights should be modulated.

- [ ] **Step 1: Define RiskAdjuster interface**

Create `internal/portfolio/risk_adjuster.go`:

```go
package portfolio

import (
	"github.com/kaecer68/atlas-go/internal/risk"
)

// RiskAdjuster modulates portfolio parameters based on risk level.
type RiskAdjuster interface {
	// AdjustPositionSize returns the risk-adjusted position size multiplier.
	AdjustPositionSize(baseSize float64, decision *risk.MacroAwareDrawdownDecision) float64

	// AdjustMaxPosition returns the risk-adjusted max position percentage.
	AdjustMaxPosition(baseMaxPct float64, decision *risk.MacroAwareDrawdownDecision) float64

	// AdjustCashReserve returns the risk-adjusted cash reserve target.
	AdjustCashReserve(baseReserve float64, decision *risk.MacroAwareDrawdownDecision) float64

	// AdjustDarwinianWeights returns risk-modulated Darwinian weight bounds.
	AdjustDarwinianWeights(weightMin, weightMax float64, decision *risk.MacroAwareDrawdownDecision) (newMin, newMax float64)
}

// DefaultRiskAdjuster implements RiskAdjuster with the decision matrix from the risk skill.
type DefaultRiskAdjuster struct{}

// NewDefaultRiskAdjuster creates the default risk adjuster.
func NewDefaultRiskAdjuster() *DefaultRiskAdjuster {
	return &DefaultRiskAdjuster{}
}

func (r *DefaultRiskAdjuster) AdjustPositionSize(baseSize float64, decision *risk.MacroAwareDrawdownDecision) float64 {
	return baseSize * decision.MaxExposure
}

func (r *DefaultRiskAdjuster) AdjustMaxPosition(baseMaxPct float64, decision *risk.MacroAwareDrawdownDecision) float64 {
	// Decision matrix from risk skill:
	// Green/HOLD: 22% → no change
	// Yellow/REDUCE: 15% → scale down
	// Orange/REDUCE: 12% → scale down more
	// Red/LIQUIDATE: 0% → zero
	switch decision.Action {
	case risk.DrawdownNone:
		return baseMaxPct
	case risk.DrawdownLight:
		return baseMaxPct * 0.85
	case risk.DrawdownModerate:
		return baseMaxPct * 0.65
	case risk.DrawdownSevere:
		return baseMaxPct * 0.40
	case risk.DrawdownEmergency:
		return 0.0
	default:
		return baseMaxPct
	}
}

func (r *DefaultRiskAdjuster) AdjustCashReserve(baseReserve float64, decision *risk.MacroAwareDrawdownDecision) float64 {
	// Decision matrix: Green=8%, Yellow=15%, Orange=25%, Red=50%+
	switch decision.Action {
	case risk.DrawdownNone:
		return baseReserve
	case risk.DrawdownLight:
		return max(baseReserve, 0.15)
	case risk.DrawdownModerate:
		return max(baseReserve, 0.25)
	case risk.DrawdownSevere:
		return max(baseReserve, 0.50)
	case risk.DrawdownEmergency:
		return 1.0
	default:
		return baseReserve
	}
}

func (r *DefaultRiskAdjuster) AdjustDarwinianWeights(weightMin, weightMax float64, decision *risk.MacroAwareDrawdownDecision) (newMin, newMax float64) {
	// In drawdown states, narrow the weight range to reduce agent influence variance
	switch decision.Action {
	case risk.DrawdownNone:
		return weightMin, weightMax
	case risk.DrawdownLight:
		return weightMin * 1.1, weightMax * 0.9
	case risk.DrawdownModerate:
		return weightMin * 1.2, weightMax * 0.8
	case risk.DrawdownSevere:
		return 0.8, 1.2 // narrow to [0.8, 1.2] — near-neutral
	case risk.DrawdownEmergency:
		return 1.0, 1.0 // completely neutral
	default:
		return weightMin, weightMax
	}
}
```

- [ ] **Step 2: Write tests for RiskAdjuster**

```go
// internal/portfolio/risk_adjuster_test.go
package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/risk"
)

func TestDefaultRiskAdjuster_AdjustPositionSize(t *testing.T) {
	ra := NewDefaultRiskAdjuster()

	tests := []struct {
		action risk.DrawdownAction
		want   float64
	}{
		{risk.DrawdownNone, 100000},
		{risk.DrawdownLight, 85000},
		{risk.DrawdownModerate, 65000},
		{risk.DrawdownSevere, 40000},
		{risk.DrawdownEmergency, 10000},
	}

	for _, tt := range tests {
		decision := &risk.MacroAwareDrawdownDecision{
			Action:      tt.action,
			MaxExposure: risk.DefaultDrawdownLevels[tt.action].MaxExposure,
		}
		got := ra.AdjustPositionSize(100000, decision)
		if got != tt.want {
			t.Errorf("action %v: got %f, want %f", tt.action, got, tt.want)
		}
	}
}

func TestDefaultRiskAdjuster_AdjustMaxPosition(t *testing.T) {
	ra := NewDefaultRiskAdjuster()

	tests := []struct {
		action risk.DrawdownAction
		want   float64
	}{
		{risk.DrawdownNone, 0.18},
		{risk.DrawdownLight, 0.153},
		{risk.DrawdownModerate, 0.117},
		{risk.DrawdownSevere, 0.072},
		{risk.DrawdownEmergency, 0.0},
	}

	for _, tt := range tests {
		decision := &risk.MacroAwareDrawdownDecision{Action: tt.action}
		got := ra.AdjustMaxPosition(0.18, decision)
		if got != tt.want {
			t.Errorf("action %v: got %f, want %f", tt.action, got, tt.want)
		}
	}
}

func TestDefaultRiskAdjuster_AdjustDarwinianWeights(t *testing.T) {
	ra := NewDefaultRiskAdjuster()

	// Emergency should clamp to neutral
	decision := &risk.MacroAwareDrawdownDecision{Action: risk.DrawdownEmergency}
	min, max := ra.AdjustDarwinianWeights(0.3, 2.5, decision)
	if min != 1.0 || max != 1.0 {
		t.Errorf("emergency: got [%f, %f], want [1.0, 1.0]", min, max)
	}

	// None should not change
	decision = &risk.MacroAwareDrawdownDecision{Action: risk.DrawdownNone}
	min, max = ra.AdjustDarwinianWeights(0.3, 2.5, decision)
	if min != 0.3 || max != 2.5 {
		t.Errorf("none: got [%f, %f], want [0.3, 2.5]", min, max)
	}
}
```

Run: `go test ./internal/portfolio/... -run TestDefaultRiskAdjuster -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./internal/portfolio/... -run TestDefaultRiskAdjuster -v`
Expected: All 3 tests PASS

- [ ] **Step 4: Integrate RiskAdjuster into Darwinian weight system**

Modify `internal/portfolio/darwinian_weights.go` — add optional `RiskAdjuster` field and apply adjustment in the weight calculation loop.

- [ ] **Step 5: Run full CI checks**

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./internal/portfolio/...
go vet ./...
staticcheck ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/portfolio/risk_adjuster.go internal/portfolio/risk_adjuster_test.go internal/portfolio/darwinian_weights.go
git commit -m "feat(portfolio): add RiskAdjuster interface for risk-modulated weights

- RiskAdjuster interface with position size, max position, cash reserve, Darwinian weight methods
- DefaultRiskAdjuster implementing the decision matrix from risk skill
- Integration with Darwinian weight system to narrow ranges during drawdown"
```

---

### Task 6: Phase 6 — Stress Test CLI

**Complexity:** Medium
**Priority:** LOW — validation tool for extreme scenarios

**Files:**
- Create: `cmd/stress-test-risk/main.go`
- Create: `cmd/stress-test-risk/scenarios.go`
- Create: `cmd/stress-test-risk/main_test.go`

**Context:** Need a CLI tool to simulate extreme market conditions and verify risk decisions are correct.

- [ ] **Step 1: Define stress test scenarios**

Create `cmd/stress-test-risk/scenarios.go`:

```go
package main

import (
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// StressScenario defines a market scenario for stress testing.
type StressScenario struct {
	Name               string
	Description        string
	MacroRisk          narrative.MacroRiskLevel
	ForeignOutflowProb float64
	PrimaryFlow        string
	StructuralScore    float64
	ShouldOverride     bool
	RecessionCount     int
	ExpansionCount     int
	WantAction         risk.DrawdownAction
	WantHalt           bool
}

// DefaultScenarios returns predefined stress test scenarios.
func DefaultScenarios() []StressScenario {
	return []StressScenario{
		{
			Name:               "normal_market",
			Description:        "Normal market conditions, no risk signals",
			MacroRisk:          narrative.MacroRiskGreen,
			ForeignOutflowProb: 15.0,
			PrimaryFlow:        "normal",
			StructuralScore:    0.8,
			WantAction:         risk.DrawdownNone,
			WantHalt:           false,
		},
		{
			Name:               "2022_russia_ukraine",
			Description:        "Systemic risk: war + oil shock + Fed hiking",
			MacroRisk:          narrative.MacroRiskRed,
			ForeignOutflowProb: 85.0,
			PrimaryFlow:        "risk_off",
			StructuralScore:    0.3,
			ShouldOverride:     false,
			RecessionCount:     5,
			WantAction:         risk.DrawdownSevere,
			WantHalt:           true,
		},
		{
			Name:               "2024_ai_structural",
			Description:        "Yellow macro but strong AI structural trend",
			MacroRisk:          narrative.MacroRiskYellow,
			ForeignOutflowProb: 40.0,
			PrimaryFlow:        "normal",
			StructuralScore:    0.85,
			ShouldOverride:     true,
			ExpansionCount:     4,
			WantAction:         risk.DrawdownLight,
			WantHalt:           false,
		},
		{
			Name:               "2026_us_iran_sector_rotation",
			Description:        "Orange macro with sector rotation to energy",
			MacroRisk:          narrative.MacroRiskOrange,
			ForeignOutflowProb: 65.0,
			PrimaryFlow:        "sector_rotation",
			StructuralScore:    0.40,
			ShouldOverride:     false,
			WantAction:         risk.DrawdownModerate,
			WantHalt:           false,
		},
		{
			Name:               "carry_trade_unwind",
			Description:        "JPY carry trade unwind (Aug 2024 style)",
			MacroRisk:          narrative.MacroRiskRed,
			ForeignOutflowProb: 90.0,
			PrimaryFlow:        "carry_trade_unwind",
			StructuralScore:    0.2,
			ShouldOverride:     false,
			RecessionCount:     3,
			WantAction:         risk.DrawdownSevere,
			WantHalt:           true,
		},
	}
}
```

- [ ] **Step 2: Implement CLI main.go**

Create `cmd/stress-test-risk/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func main() {
	engine := risk.NewMacroAwareDrawdownEngine()
	scenarios := DefaultScenarios()

	passed := 0
	failed := 0

	fmt.Println("=== Atlas Risk Module Stress Test ===")
	fmt.Println()

	for _, s := range scenarios {
		macro := &narrative.MacroRiskAssessment{
			Level:              s.MacroRisk,
			ForeignOutflowProb: s.ForeignOutflowProb,
			PrimaryFlow:        s.PrimaryFlow,
		}
		structural := &narrative.StructuralTrendAssessment{
			OverrideScore:      s.StructuralScore,
			ShouldOverrideRisk: s.ShouldOverride,
		}

		result := engine.EvaluateWithBreakdown(macro, structural, nil)

		actionOK := result.Decision.Action == s.WantAction
		haltOK := engine.ShouldHaltTrading(result.Decision) == s.WantHalt

		status := "PASS"
		if !actionOK || !haltOK {
			status = "FAIL"
			failed++
		} else {
			passed++
		}

		fmt.Printf("[%s] %s\n", status, s.Name)
		fmt.Printf("  %s\n", s.Description)
		fmt.Printf("  Macro: %s, Outflow: %.1f%%, Flow: %s\n",
			macro.Level.String(), macro.ForeignOutflowProb, macro.PrimaryFlow)
		fmt.Printf("  Structural: score=%.2f, override=%v\n",
			s.StructuralScore, s.ShouldOverride)
		fmt.Printf("  Decision: action=%v (want=%v), halt=%v (want=%v)\n",
			result.Decision.Action, s.WantAction,
			engine.ShouldHaltTrading(result.Decision), s.WantHalt)

		if !actionOK || !haltHalt {
			fmt.Printf("  Breakdown steps:\n")
			for _, step := range result.Breakdown.Steps {
				fmt.Printf("    - [%s] %s (delta: %.2f)\n", step.Rule, step.Reason, step.Delta)
			}
		}
		fmt.Println()
	}

	fmt.Printf("=== Results: %d passed, %d failed ===\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
```

Note: Fix the typo `haltHalt` → `haltOK` in the actual implementation.

- [ ] **Step 3: Write unit test for CLI scenarios**

Create `cmd/stress-test-risk/main_test.go`:

```go
package main

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func TestDefaultScenarios_AllPass(t *testing.T) {
	engine := risk.NewMacroAwareDrawdownEngine()

	for _, s := range DefaultScenarios() {
		t.Run(s.Name, func(t *testing.T) {
			macro := &narrative.MacroRiskAssessment{
				Level:              s.MacroRisk,
				ForeignOutflowProb: s.ForeignOutflowProb,
				PrimaryFlow:        s.PrimaryFlow,
			}
			structural := &narrative.StructuralTrendAssessment{
				OverrideScore:      s.StructuralScore,
				ShouldOverrideRisk: s.ShouldOverride,
			}

			result := engine.EvaluateWithBreakdown(macro, structural, nil)

			if result.Decision.Action != s.WantAction {
				t.Errorf("action = %v, want %v", result.Decision.Action, s.WantAction)
			}
			if engine.ShouldHaltTrading(result.Decision) != s.WantHalt {
				t.Errorf("halt = %v, want %v", engine.ShouldHaltTrading(result.Decision), s.WantHalt)
			}
		})
	}
}
```

Run: `go test ./cmd/stress-test-risk/... -v`
Expected: All 5 scenario tests PASS

- [ ] **Step 4: Verify CLI runs successfully**

```bash
go run ./cmd/stress-test-risk/
```

Expected output: All 5 scenarios PASS, exit code 0

- [ ] **Step 5: Run full CI checks**

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./cmd/stress-test-risk/...
go vet ./...
staticcheck ./...
```

- [ ] **Step 6: Commit**

```bash
git add cmd/stress-test-risk/
git commit -m "feat(cli): add stress-test-risk CLI for extreme scenario validation

- 5 predefined scenarios: normal, 2022 Russia-Ukraine, 2024 AI structural, 2026 US-Iran, carry trade unwind
- Full breakdown output on failure for debugging
- Exit code 1 on any scenario failure for CI integration"
```

---

## Summary

### Phase Dependencies & Parallelism

| Phase | Complexity | Depends On | Can Run In Parallel With |
|-------|-----------|------------|------------------------|
| Phase 1: Live Risk Gate | Medium | None | Phase 2, Phase 4 |
| Phase 2: DrawdownConfig | Medium | None | Phase 1, Phase 4 |
| Phase 3: Breakdown | Complex | Phase 2 | — |
| Phase 4: Industry Risk | Medium | None | Phase 1, Phase 2 |
| Phase 5: Portfolio Adjust | Complex | Phase 3, Phase 4 | — |
| Phase 6: Stress CLI | Medium | Phase 1, Phase 3, Phase 5 | — |

### Execution Waves

**Wave 1** (parallel): Phase 1 + Phase 2 + Phase 4
**Wave 2**: Phase 3 (after Phase 2)
**Wave 3**: Phase 5 (after Phase 3 + Phase 4)
**Wave 4**: Phase 6 (after Phase 1 + Phase 3 + Phase 5)

### Total Estimated Effort

| Phase | Tasks | Estimated Time |
|-------|-------|---------------|
| Phase 1 | 9 steps | 45 min |
| Phase 2 | 7 steps | 35 min |
| Phase 3 | 7 steps | 50 min |
| Phase 4 | 5 steps | 30 min |
| Phase 5 | 6 steps | 45 min |
| Phase 6 | 6 steps | 30 min |
| **Total** | **40 steps** | **~4 hours** |

### Files Created/Modified Summary

| Action | File | Phase |
|--------|------|-------|
| Create | `internal/live/risk_gate.go` | 1 |
| Create | `internal/live/risk_gate_test.go` | 1 |
| Modify | `internal/live/order_manager.go` | 1 |
| Modify | `internal/live/orchestrator.go` | 1 |
| Modify | `internal/live/order_manager_test.go` | 1 |
| Modify | `internal/config/parameters.go` | 2 |
| Modify | `internal/config/parameters_defaults.go` | 2 |
| Modify | `internal/config/engine_config.go` | 2 |
| Modify | `internal/risk/macro_aware_drawdown.go` | 2, 3, 4 |
| Create | `internal/risk/drawdown_breakdown.go` | 3 |
| Create | `internal/risk/drawdown_breakdown_test.go` | 3 |
| Modify | `internal/monitoring/dashboard_api.go` | 3 |
| Create | `internal/risk/industry_risk.go` | 4 |
| Create | `internal/risk/industry_risk_test.go` | 4 |
| Create | `internal/portfolio/risk_adjuster.go` | 5 |
| Create | `internal/portfolio/risk_adjuster_test.go` | 5 |
| Modify | `internal/portfolio/darwinian_weights.go` | 5 |
| Create | `cmd/stress-test-risk/main.go` | 6 |
| Create | `cmd/stress-test-risk/scenarios.go` | 6 |
| Create | `cmd/stress-test-risk/main_test.go` | 6 |

### QA/Acceptance Criteria (Agent-Executable)

**Phase 1 — Live Risk Gate:**
```bash
# Test 1: Risk gate blocks order when halted
go test ./internal/live/... -run TestRiskGate_Check_HaltsOnDrawdown -v
# Expected: PASS

# Test 2: Risk gate allows order when within limits
go test ./internal/live/... -run TestRiskGate_Check_AllowsOrder -v
# Expected: PASS

# Test 3: Integration — OrderManager respects risk gate
go test ./internal/live/... -run TestOrderManager_Run_BlockedByRiskGate -v
# Expected: PASS

# Test 4: Full build + test
go build ./... && go test ./internal/live/...
# Expected: exit 0
```

**Phase 2 — DrawdownConfig:**
```bash
# Test: Validation catches invalid drawdown levels
go test ./internal/config/... -run TestParametersConfig_Validate -v
# Expected: PASS

# Test: Default values are monotonic
go run -exec '' - <<'EOF'
package main
import (
	"fmt"
	"github.com/kaecer68/atlas-go/internal/config"
)
func main() {
	cfg := config.DefaultParametersConfig()
	dp := cfg.Drawdown
	if dp.LightPercentage.Value >= dp.ModeratePercentage.Value {
		fmt.Println("FAIL: light >= moderate")
	} else if dp.ModeratePercentage.Value >= dp.SeverePercentage.Value {
		fmt.Println("FAIL: moderate >= severe")
	} else {
		fmt.Println("PASS: drawdown levels are monotonic")
	}
}
EOF
# Expected: "PASS: drawdown levels are monotonic"
```

**Phase 3 — Breakdown:**
```bash
# Test: Breakdown has at least 3 steps
go test ./internal/risk/... -run TestDrawdownBreakdown_StepsAreOrdered -v
# Expected: PASS

# Test: Green risk produces DrawdownNone with breakdown
go test ./internal/risk/... -run TestMacroAwareDrawdownEngine_EvaluateWithBreakdown_GreenRisk -v
# Expected: PASS
```

**Phase 4 — Industry Risk:**
```bash
# Test: Industry assessment produces valid weighted score
go test ./internal/risk/... -run TestIndustryRiskAssessment_WeightedScore -v
# Expected: PASS
```

**Phase 5 — Portfolio Risk Adjuster:**
```bash
# Test: Emergency clamps Darwinian weights to neutral
go test ./internal/portfolio/... -run TestDefaultRiskAdjuster_AdjustDarwinianWeights -v
# Expected: PASS

# Test: Position size scales with MaxExposure
go test ./internal/portfolio/... -run TestDefaultRiskAdjuster_AdjustPositionSize -v
# Expected: PASS
```

**Phase 6 — Stress CLI:**
```bash
# Test: All 5 predefined scenarios pass
go test ./cmd/stress-test-risk/... -v
# Expected: 5 PASS, 0 FAIL

# Test: CLI runs and exits 0
go run ./cmd/stress-test-risk/
# Expected: "=== Results: 5 passed, 0 failed ==="
```
