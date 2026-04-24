package portfolio

import (
	"fmt"
	"sort"
	"time"

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
	Allocations []SectorAllocation `json:"allocations"`
	PrimaryFlow string             `json:"primary_flow"`
	Rationale   string             `json:"rationale"`
	Timestamp   time.Time          `json:"timestamp"`
}

// SectorRotator executes sector rotation based on macro conditions
type SectorRotator struct {
	baseAllocations    map[string]float64
	minAllocation      float64
	maxAllocation      float64
	rebalanceThreshold float64
}

// NewSectorRotator creates a new sector rotator with default base allocations
func NewSectorRotator() *SectorRotator {
	return NewSectorRotatorWithConfig(config.GetEngineConfig().SectorRotation)
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

// GeneratePlan creates a sector rotation plan based on macro assessment
func (r *SectorRotator) GeneratePlan(
	macroAssessment *narrative.MacroRiskAssessment,
	currentAllocations map[string]float64,
) *SectorRotationPlan {
	plan := &SectorRotationPlan{
		PrimaryFlow: macroAssessment.PrimaryFlow,
		Timestamp:   time.Now(),
	}

	// Start with base allocations
	targetAllocations := make(map[string]float64)
	for sector, alloc := range r.baseAllocations {
		targetAllocations[sector] = alloc
	}

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

func (r *SectorRotator) applyMacroAdjustments(allocations map[string]float64, macro *narrative.MacroRiskAssessment) {
	switch macro.Level {
	case narrative.MacroRiskGreen:
		// Maintain base allocations

	case narrative.MacroRiskYellow:
		// Slight defensive tilt
		allocations["defensive"] += 0.05
		allocations["cash"] += 0.03
		allocations["ai_supply_chain"] -= 0.04
		allocations["semiconductor"] -= 0.04

	case narrative.MacroRiskOrange:
		// Moderate defensive positioning
		allocations["defensive"] += 0.10
		allocations["cash"] += 0.08
		allocations["gold"] = 0.05
		allocations["ai_supply_chain"] -= 0.08
		allocations["semiconductor"] -= 0.08
		allocations["financials"] -= 0.05

	case narrative.MacroRiskRed:
		// Severe risk-off
		allocations["cash"] += 0.25
		allocations["defensive"] += 0.15
		allocations["gold"] = 0.10
		allocations["ai_supply_chain"] -= 0.15
		allocations["semiconductor"] -= 0.15
		allocations["financials"] -= 0.10
		allocations["shipping"] -= 0.05
	}
}

func (r *SectorRotator) applyFlowAdjustments(allocations map[string]float64, flow string) {
	switch flow {
	case "risk_off":
		allocations["gold"] += 0.10
		allocations["utilities"] = 0.08
		allocations["high_dividend"] = 0.07
		allocations["ai_supply_chain"] -= 0.10
		allocations["small_cap"] = 0.0

	case "carry_trade_unwind":
		allocations["cash"] += 0.30
		allocations["short_term_bonds"] = 0.15
		allocations["jpy"] = 0.05
		allocations["ai_supply_chain"] = 0.02
		allocations["semiconductor"] = 0.03
		allocations["financials"] -= 0.10

	case "sector_rotation":
		allocations["energy"] += 0.15
		allocations["oil_services"] = 0.08
		allocations["alternative_energy"] = 0.05
		allocations["shipping"] += 0.05
		allocations["high_valuation_tech"] = 0.02
		allocations["rate_sensitive"] -= 0.08
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
	for i := 0; i < 10; i++ {
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
	for _, favored := range macro.FavoredSectors {
		if favored == sector {
			return fmt.Sprintf("Favored sector during %s conditions", macro.PrimaryFlow)
		}
	}

	// Check if sector is in avoided list
	for _, avoided := range macro.AvoidedSectors {
		if avoided == sector {
			return fmt.Sprintf("Avoided sector during %s conditions", macro.PrimaryFlow)
		}
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
