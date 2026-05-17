package industry

import (
	"fmt"
	"maps"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// CyclePhase represents the current phase of an industry business cycle.
type CyclePhase string

const (
	CycleRecovery  CyclePhase = "recovery"  // 復甦期
	CycleExpansion CyclePhase = "expansion" // 擴張期
	CycleMature    CyclePhase = "mature"    // 成熟期
	CycleRecession CyclePhase = "recession" // 衰退期
)

// InventoryCycle represents the inventory cycle state.
type InventoryCycle string

const (
	InvRestockingActive  InventoryCycle = "active_restocking"  // 主動補庫存
	InvRestockingPassive InventoryCycle = "passive_restocking" // 被動補庫存
	InvDestockingActive  InventoryCycle = "active_destocking"  // 主動去庫存
	InvDestockingPassive InventoryCycle = "passive_destocking" // 被動去庫存
)

// CapexCycle represents the capital expenditure cycle state.
type CapexCycle string

const (
	CapexExpansion   CapexCycle = "expansion"   // 資本支出擴張
	CapexMaintenance CapexCycle = "maintenance" // 維護性支出
	CapexContraction CapexCycle = "contraction" // 資本支出緊縮
)

// CyclePosition holds the complete cycle positioning for an industry.
type CyclePosition struct {
	IndustryID          string         `json:"industry_id"`
	BusinessCycle       CyclePhase     `json:"business_cycle"`
	InventoryCycle      InventoryCycle `json:"inventory_cycle"`
	CapexCycle          CapexCycle     `json:"capex_cycle"`
	Confidence          float64        `json:"confidence"` // 0.0 to 1.0
	LeadingIndicators   []Indicator    `json:"leading_indicators"`
	LaggingIndicators   []Indicator    `json:"lagging_indicators"`
	CycleDurationDays   int            `json:"cycle_duration_days"` // Days in current phase
	ExpectedPhaseChange *time.Time     `json:"expected_phase_change,omitempty"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// Indicator represents a single economic indicator.
type Indicator struct {
	Name        string    `json:"name"`
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	Trend       string    `json:"trend"` // "up", "down", "stable"
	Threshold   float64   `json:"threshold"`
	IsLeading   bool      `json:"is_leading"`
	Weight      float64   `json:"weight"`
	LastUpdated time.Time `json:"last_updated"`
}

// CycleTracker monitors and tracks cycle positions for industries.
type CycleTracker struct {
	positions map[string]*CyclePosition
	history   map[string][]CyclePosition
}

// NewCycleTracker creates a new cycle tracker.
func NewCycleTracker() *CycleTracker {
	ct := &CycleTracker{
		positions: make(map[string]*CyclePosition),
		history:   make(map[string][]CyclePosition),
	}
	ct.initializeDefaultPositions()
	return ct
}

// initializeDefaultPositions populates the tracker with default cycle positions
// for known industries. This ensures the API returns meaningful data even before
// real-time metrics are available.
func (ct *CycleTracker) initializeDefaultPositions() {
	defaults := map[string]IndustryMetrics{
		"semiconductor": {
			IndustryID:          "semiconductor",
			RevenueGrowthYoY:    0.25,
			ProfitGrowthYoY:     0.30,
			InventoryTurnover:   5.5,
			CapacityUtilization: 0.85,
			DataFreshness:       FreshFallback,
		},
		"ai_supply_chain": {
			IndustryID:          "ai_supply_chain",
			RevenueGrowthYoY:    0.45,
			ProfitGrowthYoY:     0.50,
			InventoryTurnover:   6.0,
			CapacityUtilization: 0.90,
			DataFreshness:       FreshFallback,
		},
		"robotics": {
			IndustryID:          "robotics",
			RevenueGrowthYoY:    0.15,
			ProfitGrowthYoY:     0.12,
			InventoryTurnover:   4.0,
			CapacityUtilization: 0.70,
			DataFreshness:       FreshFallback,
		},
		"financials": {
			IndustryID:          "financials",
			RevenueGrowthYoY:    0.08,
			ProfitGrowthYoY:     0.10,
			InventoryTurnover:   0.0,
			CapacityUtilization: 0.75,
			DataFreshness:       FreshFallback,
		},
		"shipping": {
			IndustryID:          "shipping",
			RevenueGrowthYoY:    -0.05,
			ProfitGrowthYoY:     -0.10,
			InventoryTurnover:   3.0,
			CapacityUtilization: 0.65,
			DataFreshness:       FreshFallback,
		},
		"energy": {
			IndustryID:          "energy",
			RevenueGrowthYoY:    0.05,
			ProfitGrowthYoY:     0.03,
			InventoryTurnover:   4.5,
			CapacityUtilization: 0.70,
			DataFreshness:       FreshFallback,
		},
		"electronics": {
			IndustryID:          "electronics",
			RevenueGrowthYoY:    0.12,
			ProfitGrowthYoY:     0.15,
			InventoryTurnover:   5.0,
			CapacityUtilization: 0.75,
			DataFreshness:       FreshFallback,
		},
		"consumer": {
			IndustryID:          "consumer",
			RevenueGrowthYoY:    0.03,
			ProfitGrowthYoY:     0.05,
			InventoryTurnover:   6.0,
			CapacityUtilization: 0.70,
			DataFreshness:       FreshFallback,
		},
		"industrial": {
			IndustryID:          "industrial",
			RevenueGrowthYoY:    0.06,
			ProfitGrowthYoY:     0.08,
			InventoryTurnover:   4.0,
			CapacityUtilization: 0.68,
			DataFreshness:       FreshFallback,
		},
	}

	for id, metrics := range defaults {
		ct.UpdatePosition(id, metrics)
	}
}

// UpdatePosition updates the cycle position for an industry.
func (ct *CycleTracker) UpdatePosition(industryID string, metrics IndustryMetrics) *CyclePosition {
	position := ct.detectCyclePosition(industryID, metrics)
	position.UpdatedAt = time.Now()

	// Store historical data
	ct.history[industryID] = append(ct.history[industryID], *position)
	if len(ct.history[industryID]) > 90 { // Keep 90 days of history
		ct.history[industryID] = ct.history[industryID][1:]
	}

	ct.positions[industryID] = position
	return position
}

// GetPosition returns the current cycle position for an industry.
func (ct *CycleTracker) GetPosition(industryID string) (*CyclePosition, bool) {
	pos, ok := ct.positions[industryID]
	return pos, ok
}

// GetAllPositions returns all current cycle positions.
func (ct *CycleTracker) GetAllPositions() map[string]*CyclePosition {
	result := make(map[string]*CyclePosition)
	maps.Copy(result, ct.positions)
	return result
}

// GetHistory returns the historical cycle positions for an industry.
func (ct *CycleTracker) GetHistory(industryID string) []CyclePosition {
	return ct.history[industryID]
}

// detectCyclePosition determines the cycle phase based on industry metrics.
func (ct *CycleTracker) detectCyclePosition(industryID string, metrics IndustryMetrics) *CyclePosition {
	position := &CyclePosition{
		IndustryID: industryID,
		UpdatedAt:  time.Now(),
	}

	// Detect business cycle phase
	position.BusinessCycle = ct.detectBusinessCycle(metrics)

	// Detect inventory cycle
	position.InventoryCycle = ct.detectInventoryCycle(metrics)

	// Detect capex cycle
	position.CapexCycle = ct.detectCapexCycle(metrics)

	// Calculate confidence based on data quality
	position.Confidence = ct.calculateConfidence(metrics)

	// Set leading and lagging indicators
	position.LeadingIndicators = ct.getLeadingIndicators(industryID, metrics)
	position.LaggingIndicators = ct.getLaggingIndicators(industryID, metrics)

	// Calculate cycle duration
	if history := ct.history[industryID]; len(history) > 0 {
		lastPhase := history[len(history)-1].BusinessCycle
		if lastPhase == position.BusinessCycle {
			position.CycleDurationDays = history[len(history)-1].CycleDurationDays + 1
		} else {
			position.CycleDurationDays = 1
		}
	}

	return position
}

// detectBusinessCycle determines the business cycle phase.
func (ct *CycleTracker) detectBusinessCycle(metrics IndustryMetrics) CyclePhase {
	params := config.GetParametersConfig().Industry
	thresholds, ok := params.CycleThresholds.Value[metrics.IndustryID]
	if !ok {
		thresholds = config.CycleThresholdConfig{
			ExpansionRevenuePct: 0.20,
			ExpansionProfitPct:  0.20,
			RecoveryRevenuePct:  0.05,
			RecoveryProfitPct:   0.05,
			MatureRevenuePct:    -0.05,
			MatureProfitPct:     -0.05,
		}
	}

	revenueGrowth := metrics.RevenueGrowthYoY
	profitGrowth := metrics.ProfitGrowthYoY

	switch {
	case revenueGrowth > thresholds.ExpansionRevenuePct && profitGrowth > thresholds.ExpansionProfitPct:
		return CycleExpansion
	case revenueGrowth > thresholds.RecoveryRevenuePct && profitGrowth > thresholds.RecoveryProfitPct:
		return CycleRecovery
	case revenueGrowth > thresholds.MatureRevenuePct && profitGrowth > thresholds.MatureProfitPct:
		return CycleMature
	default:
		return CycleRecession
	}
}

// detectInventoryCycle determines the inventory cycle state.
func (ct *CycleTracker) detectInventoryCycle(metrics IndustryMetrics) InventoryCycle {
	inventoryTurnover := metrics.InventoryTurnover
	capacityUtilization := metrics.CapacityUtilization

	switch {
	case inventoryTurnover > 6.0 && capacityUtilization > 0.80:
		return InvRestockingActive
	case inventoryTurnover > 4.0 && capacityUtilization > 0.70:
		return InvRestockingPassive
	case inventoryTurnover < 3.0 && capacityUtilization < 0.60:
		return InvDestockingActive
	default:
		return InvDestockingPassive
	}
}

// detectCapexCycle determines the capex cycle state.
func (ct *CycleTracker) detectCapexCycle(metrics IndustryMetrics) CapexCycle {
	// Based on capacity utilization and revenue growth
	capacityUtilization := metrics.CapacityUtilization
	revenueGrowth := metrics.RevenueGrowthYoY

	switch {
	case capacityUtilization > 0.85 && revenueGrowth > 0.15:
		return CapexExpansion
	case capacityUtilization > 0.70 && revenueGrowth > 0.05:
		return CapexMaintenance
	default:
		return CapexContraction
	}
}

// calculateConfidence calculates the confidence level of cycle detection.
func (ct *CycleTracker) calculateConfidence(metrics IndustryMetrics) float64 {
	confidence := 0.5 // Base confidence

	// Increase confidence if we have all key metrics
	if metrics.RevenueGrowthYoY != 0 {
		confidence += 0.15
	}
	if metrics.ProfitGrowthYoY != 0 {
		confidence += 0.15
	}
	if metrics.InventoryTurnover != 0 {
		confidence += 0.10
	}
	if metrics.CapacityUtilization != 0 {
		confidence += 0.10
	}

	if confidence > 1.0 {
		confidence = 1.0
	}
	return confidence
}

// getLeadingIndicators returns leading indicators for an industry.
func (ct *CycleTracker) getLeadingIndicators(industryID string, metrics IndustryMetrics) []Indicator {
	var indicators []Indicator

	// Common leading indicators
	if metrics.RevenueGrowthYoY != 0 {
		indicators = append(indicators, Indicator{
			Name:      "Revenue Growth YoY",
			Value:     metrics.RevenueGrowthYoY,
			Unit:      "%",
			Trend:     ct.getTrend(metrics.RevenueGrowthYoY, 0.10),
			IsLeading: true,
			Weight:    0.30,
		})
	}

	if metrics.InventoryTurnover != 0 {
		indicators = append(indicators, Indicator{
			Name:      "Inventory Turnover",
			Value:     metrics.InventoryTurnover,
			Unit:      "x",
			Trend:     ct.getTrend(metrics.InventoryTurnover, 4.0),
			IsLeading: true,
			Weight:    0.25,
		})
	}

	return indicators
}

// getLaggingIndicators returns lagging indicators for an industry.
func (ct *CycleTracker) getLaggingIndicators(industryID string, metrics IndustryMetrics) []Indicator {
	var indicators []Indicator

	if metrics.ProfitGrowthYoY != 0 {
		indicators = append(indicators, Indicator{
			Name:      "Profit Growth YoY",
			Value:     metrics.ProfitGrowthYoY,
			Unit:      "%",
			Trend:     ct.getTrend(metrics.ProfitGrowthYoY, 0.10),
			IsLeading: false,
			Weight:    0.35,
		})
	}

	if metrics.CapacityUtilization != 0 {
		indicators = append(indicators, Indicator{
			Name:      "Capacity Utilization",
			Value:     metrics.CapacityUtilization,
			Unit:      "%",
			Trend:     ct.getTrend(metrics.CapacityUtilization, 0.75),
			IsLeading: false,
			Weight:    0.30,
		})
	}

	return indicators
}

// getTrend determines the trend direction based on value and threshold.
func (ct *CycleTracker) getTrend(value, threshold float64) string {
	if value > threshold*1.1 {
		return "up"
	} else if value < threshold*0.9 {
		return "down"
	}
	return "stable"
}

func (cp *CyclePosition) IsFavorable() bool {
	return cp.BusinessCycle == CycleRecovery || cp.BusinessCycle == CycleExpansion
}

func (cp *CyclePosition) IsFavorablePhase() bool {
	return cp.IsFavorable()
}

func (cp *CyclePosition) GetTrend() string {
	switch cp.BusinessCycle {
	case CycleExpansion:
		return "up"
	case CycleRecovery:
		return "up"
	case CycleMature:
		return "stable"
	case CycleRecession:
		return "down"
	default:
		return "stable"
	}
}

// GetPhaseScore returns a numerical score for the cycle phase (-1 to 1).
func (cp *CyclePosition) GetPhaseScore() float64 {
	switch cp.BusinessCycle {
	case CycleExpansion:
		return 1.0
	case CycleRecovery:
		return 0.5
	case CycleMature:
		return 0.0
	case CycleRecession:
		return -1.0
	default:
		return 0.0
	}
}

// String returns a human-readable summary of the cycle position.
func (cp *CyclePosition) String() string {
	return fmt.Sprintf("%s: Business=%s, Inventory=%s, Capex=%s, Confidence=%.0f%%",
		cp.IndustryID,
		cp.BusinessCycle,
		cp.InventoryCycle,
		cp.CapexCycle,
		cp.Confidence*100,
	)
}

// CyclePhaseTransition represents a transition from one cycle phase to another.
type CyclePhaseTransition struct {
	FromPhase           CyclePhase `json:"from_phase"`
	ToPhase             CyclePhase `json:"to_phase"`
	Probability         float64    `json:"probability"` // 0.0 to 1.0
	Triggers            []string   `json:"triggers"`    // Events that trigger transition
	TypicalDurationDays int        `json:"typical_duration_days"`
}

// GetTypicalTransitions returns typical cycle transitions.
func GetTypicalTransitions() []CyclePhaseTransition {
	return []CyclePhaseTransition{
		{FromPhase: CycleRecession, ToPhase: CycleRecovery, Probability: 0.70, Triggers: []string{"inventory_depletion", "demand_stabilization"}, TypicalDurationDays: 180},
		{FromPhase: CycleRecovery, ToPhase: CycleExpansion, Probability: 0.80, Triggers: []string{"revenue_acceleration", "capex_increase"}, TypicalDurationDays: 270},
		{FromPhase: CycleExpansion, ToPhase: CycleMature, Probability: 0.60, Triggers: []string{"growth_deceleration", "margin_compression"}, TypicalDurationDays: 360},
		{FromPhase: CycleMature, ToPhase: CycleRecession, Probability: 0.50, Triggers: []string{"inventory_buildup", "demand_contraction"}, TypicalDurationDays: 180},
	}
}
