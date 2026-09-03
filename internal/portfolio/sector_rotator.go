package portfolio

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// SectorAllocation represents target allocation for a sector
type SectorAllocation struct {
	Sector     string  `json:"sector"`
	TargetPct  float64 `json:"target_pct"`
	CurrentPct float64 `json:"current_pct"`
	Delta      float64 `json:"delta"`
	Rationale  string  `json:"rationale"`
}

// SectorRotationPlan represents the plan for rotating between sectors
type SectorRotationPlan struct {
	Allocations           []SectorAllocation                 `json:"allocations"`
	PrimaryFlow           string                             `json:"primary_flow"`
	CapitalFlowAssessment *capitalflow.CapitalFlowAssessment `json:"capital_flow_assessment,omitempty"`
	Rationale             string                             `json:"rationale"`
	Timestamp             time.Time                          `json:"timestamp"`
	ConfigSource          string                             `json:"config_source,omitempty"`
}

// SectorRotator executes sector rotation based on macro conditions.
//
// MIGRATION (2026-06-14): The base allocations used here were one of three
// independent sources of hard-coded weights (orchestrator.base_allocations:
// semiconductor=19%) that caused the "三個不同半導體權重" bug. New code
// should:
//
//  1. Call sectorallocation.WeightEngine.ComputeWeights(ctx, now) to obtain
//     the unified SectorAllocationPlan.
//  2. Feed that plan's adjusted_weight values into NewSectorRotatorWithTargets
//     (or a similar constructor that accepts a target map) as the
//     "current_pct" basis.
//  3. Let the macro-driven min/max/rebalance logic run unchanged on top.
//
// SectorRotator itself is retained because the macro rotation logic is
// orthogonal to weight derivation; only the input source changes.
type SectorRotator struct {
	baseAllocations    map[string]float64
	minAllocation      float64
	maxAllocation      float64
	rebalanceThreshold float64
}

// NewSectorRotator creates a new sector rotator with base allocations from ParametersConfig.
func NewSectorRotator() *SectorRotator {
	params := config.GetParametersConfig()
	ba := params.SectorAllocation.BaseWeights
	engineCfg := params.Engine.SectorRotation.ToConfig()
	return &SectorRotator{
		baseAllocations:    ba,
		minAllocation:      engineCfg.MinAllocation,
		maxAllocation:      engineCfg.MaxAllocation,
		rebalanceThreshold: engineCfg.RebalanceThreshold,
	}
}

// NewSectorRotatorWithConfig creates a new sector rotator with custom config
func NewSectorRotatorWithConfig(cfg config.SectorRotationConfig) *SectorRotator {
	return &SectorRotator{
		baseAllocations:    cfg.BaseAllocations,
		minAllocation:      cfg.MinAllocation,
		maxAllocation:      cfg.MaxAllocation,
		rebalanceThreshold: cfg.RebalanceThreshold,
	}
}

// GeneratePlan computes the sector rotation plan from the macro risk
// assessment and current allocations.
//
// capitalFlowAssessment is stored on the plan for downstream consumers
// (ApplySectorRotation → capitalFlowActionFromPlan in
// internal/orchestrator/strategy_evolver.go) but is NOT consumed by the
// allocation math in this function.
// TODO(capital-flow Phase 3): not consumed here — the E07 branch of
// capitalFlowActionFromPlan is short-circuited by M1 (CalibrationStatus
// is hardcoded "calibrating", so EligibleForAutomation() is always
// false). Wiring the assessment into decisions is planned (see
// .omo/plans/2026-09-04-capital-flow-model-plan.md B1 / Phase 3);
// until then this parameter is carried for observation only.
func (r *SectorRotator) GeneratePlan(
	macroAssessment *narrative.MacroRiskAssessment,
	currentAllocations map[string]float64,
	capitalFlowAssessment *capitalflow.CapitalFlowAssessment,
) *SectorRotationPlan {
	plan := &SectorRotationPlan{
		PrimaryFlow:           macroAssessment.PrimaryFlow,
		CapitalFlowAssessment: capitalFlowAssessment,
		Timestamp:             time.Now(),
		ConfigSource:          sectorRotationConfigSource(),
	}

	// Start with base allocations
	targetAllocations := make(map[string]float64)
	maps.Copy(targetAllocations, r.baseAllocations)

	// Apply macro-driven adjustments
	r.applyMacroAdjustments(targetAllocations, macroAssessment)

	// Apply sector rotation based on primary flow
	r.applyFlowAdjustments(targetAllocations, macroAssessment.PrimaryFlow)

	// Normalize to ensure sum = 1.0
	r.normalizeAllocations(targetAllocations)

	// Build allocation list
	for sector, target := range targetAllocations {
		current := currentAllocations[sector]
		plan.Allocations = append(plan.Allocations, SectorAllocation{
			Sector:     sector,
			TargetPct:  target,
			CurrentPct: current,
			Delta:      target - current,
			Rationale:  r.getAllocationRationale(sector, macroAssessment),
		})
	}

	// Sort by target percentage (descending)
	sort.Slice(plan.Allocations, func(i, j int) bool {
		return plan.Allocations[i].TargetPct > plan.Allocations[j].TargetPct
	})

	plan.Rationale = r.buildPlanRationale(macroAssessment)

	return plan
}

func (r *SectorRotator) macroLevelKey(level narrative.MacroRiskLevel) string {
	switch level {
	case narrative.MacroRiskGreen:
		return "green"
	case narrative.MacroRiskYellow:
		return "yellow"
	case narrative.MacroRiskOrange:
		return "orange"
	case narrative.MacroRiskRed:
		return "red"
	default:
		return "green"
	}
}

// sectorRotationMacroAdjustments returns the single source of truth for macro
// risk level → sector allocation adjustments. Values come from
// ParametersConfig (Orchestrator.SectorRotationMacroAdjustments); the defaults
// live in internal/config/defaults_engine.go. No code-level fallback map is
// kept here on purpose: an empty config is rejected by Validate()
// ("orchestrator.sector_rotation_macro_adjustments must not be empty").
func sectorRotationMacroAdjustments() map[string]map[string]float64 {
	return config.GetParametersConfig().Orchestrator.SectorRotationMacroAdjustments.Value
}

func sectorRotationConfigSource() string {
	return "config"
}

func (r *SectorRotator) applyMacroAdjustments(allocations map[string]float64, macro *narrative.MacroRiskAssessment) {
	adjustments := sectorRotationMacroAdjustments()

	levelKey := r.macroLevelKey(macro.Level)
	deltas, ok := adjustments[levelKey]
	if !ok {
		return
	}
	for sector, delta := range deltas {
		if _, exists := allocations[sector]; exists || delta > 0 {
			allocations[sector] += delta
		} else {
			allocations[sector] = delta
		}
	}
}

// sectorRotationFlowAdjustments returns the single source of truth for capital
// flow pattern → sector allocation adjustments. Values come from
// ParametersConfig (Orchestrator.SectorRotationFlowAdjustments); the defaults
// live in internal/config/defaults_engine.go. No code-level fallback map is
// kept here on purpose: an empty config is rejected by Validate()
// ("orchestrator.sector_rotation_flow_adjustments must not be empty").
func sectorRotationFlowAdjustments() map[string]map[string]float64 {
	return config.GetParametersConfig().Orchestrator.SectorRotationFlowAdjustments.Value
}

func (r *SectorRotator) applyFlowAdjustments(allocations map[string]float64, flow string) {
	adjustments := sectorRotationFlowAdjustments()

	deltas, ok := adjustments[flow]
	if !ok {
		return
	}
	for sector, delta := range deltas {
		if _, exists := allocations[sector]; exists || delta > 0 {
			allocations[sector] += delta
		} else {
			allocations[sector] = delta
		}
	}
}

func (r *SectorRotator) normalizeAllocations(allocations map[string]float64) {
	// Ensure no negative allocations
	for sector := range allocations {
		if allocations[sector] < 0 {
			allocations[sector] = 0
		}
	}

	// Iteratively normalize and clamp until stable
	for range 10 {
		// Calculate total
		total := 0.0
		for _, alloc := range allocations {
			total += alloc
		}

		if total == 0 {
			return
		}

		// Normalize to 1.0
		for sector := range allocations {
			allocations[sector] = allocations[sector] / total
		}

		// Apply min/max constraints
		clamped := false
		for sector := range allocations {
			if allocations[sector] < r.minAllocation && allocations[sector] > 0 {
				allocations[sector] = r.minAllocation
				clamped = true
			}
			if allocations[sector] > r.maxAllocation {
				allocations[sector] = r.maxAllocation
				clamped = true
			}
		}

		// If no clamping occurred, we're done
		if !clamped {
			break
		}
	}
}

func (r *SectorRotator) getAllocationRationale(sector string, macro *narrative.MacroRiskAssessment) string {
	// Check if sector is in favored list
	if slices.Contains(macro.FavoredSectors, sector) {
		return fmt.Sprintf("Favored sector during %s conditions", macro.PrimaryFlow)
	}

	// Check if sector is in avoided list
	if slices.Contains(macro.AvoidedSectors, sector) {
		return fmt.Sprintf("Avoided sector during %s conditions", macro.PrimaryFlow)
	}

	return "Base allocation"
}

func (r *SectorRotator) buildPlanRationale(macro *narrative.MacroRiskAssessment) string {
	return fmt.Sprintf(
		"Sector rotation based on macro risk level %s with primary flow %s. "+
			"Favored sectors: %v. Avoided sectors: %v.",
		macro.Level.String(),
		macro.PrimaryFlow,
		macro.FavoredSectors,
		macro.AvoidedSectors,
	)
}

// GetRebalancingTrades returns the trades needed to execute the rotation plan
func (r *SectorRotator) GetRebalancingTrades(
	plan *SectorRotationPlan,
	portfolioValue float64,
) []RebalancingTrade {
	var trades []RebalancingTrade

	for _, alloc := range plan.Allocations {
		if absFloat64(alloc.Delta) < r.rebalanceThreshold {
			continue // Skip small adjustments
		}

		trades = append(trades, RebalancingTrade{
			Sector:     alloc.Sector,
			DeltaPct:   alloc.Delta,
			DeltaValue: alloc.Delta * portfolioValue,
			Rationale:  alloc.Rationale,
		})
	}

	// Sort by absolute delta (largest trades first)
	sort.Slice(trades, func(i, j int) bool {
		return absFloat64(trades[i].DeltaPct) > absFloat64(trades[j].DeltaPct)
	})

	return trades
}

// RebalancingTrade represents a single rebalancing trade
type RebalancingTrade struct {
	Sector     string  `json:"sector"`
	DeltaPct   float64 `json:"delta_pct"`
	DeltaValue float64 `json:"delta_value"`
	Rationale  string  `json:"rationale"`
}

// CanExecuteRotation checks if rotation should be executed based on drawdown decision
func (r *SectorRotator) CanExecuteRotation(
	decision *risk.MacroAwareDrawdownDecision,
) (bool, string) {
	switch decision.Action {
	case risk.DrawdownEmergency:
		return false, "Emergency drawdown - all rotation halted"
	case risk.DrawdownSevere:
		return false, "Severe drawdown - rotation to defensive assets only"
	case risk.DrawdownModerate:
		return true, "Moderate drawdown - selective rotation allowed"
	case risk.DrawdownLight, risk.DrawdownNone:
		return true, "Normal conditions - rotation allowed"
	default:
		return true, "Unknown condition - proceeding with caution"
	}
}

func absFloat64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
