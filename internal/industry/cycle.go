package industry

import (
	"fmt"
	"maps"
	"math"
	"sync"
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
	positions          map[string]*CyclePosition
	mu                 sync.RWMutex
	history            map[string][]CyclePosition
	seasonalEngine     *SeasonalEngine
	linkageAnalyzer    *LinkageAnalyzer
	narrativeHitRate   func() float64
	narrativeAdjust    func(industryID string) NarrativeAdjustment
	lastNarrativeTheme map[string]string // active narrative theme per industry (updated during detectBusinessCycle)
}

// NewCycleTracker creates a new cycle tracker.
func NewCycleTracker() *CycleTracker {
	ct := &CycleTracker{
		positions:          make(map[string]*CyclePosition),
		history:            make(map[string][]CyclePosition),
		lastNarrativeTheme: make(map[string]string),
	}
	ct.initializeDefaultPositions()
	return ct
}

// SetExternalValidators wires optional external data sources for
// multi-dimensional confidence. Nil args disable that dimension.
func (ct *CycleTracker) SetExternalValidators(seasonal *SeasonalEngine, linkage *LinkageAnalyzer) {
	ct.seasonalEngine = seasonal
	ct.linkageAnalyzer = linkage
}

// SetNarrativeProvider sets a function that returns a global narrative hit rate
// used as an independent signal in multi-dimensional confidence scoring.
func (ct *CycleTracker) SetNarrativeProvider(fn func() float64) {
	ct.narrativeHitRate = fn
}

func (ct *CycleTracker) SetNarrativeAdjuster(fn func(industryID string) NarrativeAdjustment) {
	ct.narrativeAdjust = fn
}

func (ct *CycleTracker) HasEmpiricalData(industryID string) bool {
	return len(ct.history[industryID]) > 1
}

func (ct *CycleTracker) NarrativeTheme(industryID string) string {
	if ct.lastNarrativeTheme == nil {
		return ""
	}
	return ct.lastNarrativeTheme[industryID]
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
		},
		"ai_supply_chain": {
			IndustryID:          "ai_supply_chain",
			RevenueGrowthYoY:    0.45,
			ProfitGrowthYoY:     0.50,
			InventoryTurnover:   6.0,
			CapacityUtilization: 0.90,
		},
		"robotics": {
			IndustryID:          "robotics",
			RevenueGrowthYoY:    0.15,
			ProfitGrowthYoY:     0.12,
			InventoryTurnover:   4.0,
			CapacityUtilization: 0.70,
		},
		"financials": {
			IndustryID:          "financials",
			RevenueGrowthYoY:    0.08,
			ProfitGrowthYoY:     0.10,
			InventoryTurnover:   0.0,
			CapacityUtilization: 0.75,
		},
		"shipping": {
			IndustryID:          "shipping",
			RevenueGrowthYoY:    -0.05,
			ProfitGrowthYoY:     -0.10,
			InventoryTurnover:   3.0,
			CapacityUtilization: 0.65,
		},
		"energy": {
			IndustryID:          "energy",
			RevenueGrowthYoY:    0.05,
			ProfitGrowthYoY:     0.03,
			InventoryTurnover:   4.5,
			CapacityUtilization: 0.70,
		},
		"electronics": {
			IndustryID:          "electronics",
			RevenueGrowthYoY:    0.12,
			ProfitGrowthYoY:     0.15,
			InventoryTurnover:   5.0,
			CapacityUtilization: 0.75,
		},
		"consumer": {
			IndustryID:          "consumer",
			RevenueGrowthYoY:    0.03,
			ProfitGrowthYoY:     0.05,
			InventoryTurnover:   6.0,
			CapacityUtilization: 0.70,
		},
		"industrial": {
			IndustryID:          "industrial",
			RevenueGrowthYoY:    0.06,
			ProfitGrowthYoY:     0.08,
			InventoryTurnover:   4.0,
			CapacityUtilization: 0.68,
		},
		"foundry": {
			IndustryID:          "foundry",
			RevenueGrowthYoY:    0.22,
			ProfitGrowthYoY:     0.28,
			InventoryTurnover:   5.0,
			CapacityUtilization: 0.88,
		},
		"server_assembly": {
			IndustryID:          "server_assembly",
			RevenueGrowthYoY:    0.40,
			ProfitGrowthYoY:     0.45,
			InventoryTurnover:   6.5,
			CapacityUtilization: 0.85,
		},
		"cooling": {
			IndustryID:          "cooling",
			RevenueGrowthYoY:    0.20,
			ProfitGrowthYoY:     0.22,
			InventoryTurnover:   5.5,
			CapacityUtilization: 0.80,
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

// GetPhase returns the current business cycle phase for an industry.
// Implements CycleProvider.
func (ct *CycleTracker) GetPhase(industryID string) (CyclePhase, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	pos, ok := ct.positions[industryID]
	if !ok {
		return "", false
	}
	return pos.BusinessCycle, true
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

	position.Confidence = ct.calculateConfidence(industryID, metrics)

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

	if ct.narrativeAdjust != nil {
		adj := ct.narrativeAdjust(metrics.IndustryID)
		revenueGrowth += adj.RevenueBias * adj.Confidence
		profitGrowth += adj.ProfitBias * adj.Confidence
		ct.lastNarrativeTheme[metrics.IndustryID] = adj.ActiveTheme
	} else {
		delete(ct.lastNarrativeTheme, metrics.IndustryID)
	}

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
	params := config.GetParametersConfig().Industry
	t := params.InventoryCycleThresholds.Value

	switch {
	case metrics.InventoryTurnover > t.ActiveRestockingInventoryMin && metrics.CapacityUtilization > t.ActiveRestockingCapacityMin:
		return InvRestockingActive
	case metrics.InventoryTurnover > t.PassiveRestockingInventoryMin && metrics.CapacityUtilization > t.PassiveRestockingCapacityMin:
		return InvRestockingPassive
	case metrics.InventoryTurnover < t.ActiveDestockingInventoryMax && metrics.CapacityUtilization < t.ActiveDestockingCapacityMax:
		return InvDestockingActive
	default:
		return InvDestockingPassive
	}
}

// detectCapexCycle determines the capex cycle state.
func (ct *CycleTracker) detectCapexCycle(metrics IndustryMetrics) CapexCycle {
	params := config.GetParametersConfig().Industry
	t := params.CapexCycleThresholds.Value

	switch {
	case metrics.CapacityUtilization > t.ExpansionCapacityMin && metrics.RevenueGrowthYoY > t.ExpansionRevenueMin:
		return CapexExpansion
	case metrics.CapacityUtilization > t.MaintenanceCapacityMin && metrics.RevenueGrowthYoY > t.MaintenanceRevenueMin:
		return CapexMaintenance
	default:
		return CapexContraction
	}
}

func (ct *CycleTracker) calculateConfidence(industryID string, metrics IndustryMetrics) float64 {
	cfgSignal := config.GetParametersConfig().Industry.ConfidenceSignal.Value
	cfgMix := config.GetParametersConfig().Industry.ConfidenceMix.Value

	hasData := metrics.RevenueGrowthYoY != 0 || metrics.ProfitGrowthYoY != 0 ||
		metrics.InventoryTurnover != 0 || metrics.CapacityUtilization > 0
	if !hasData {
		return cfgSignal.ConfidenceFloor
	}

	signal := cfgSignal.SignalBase
	if metrics.RevenueGrowthYoY != 0 {
		signal += math.Min(math.Abs(metrics.RevenueGrowthYoY)/cfgSignal.RevenueNormDenom, 1.0) * cfgSignal.RevenueWeight
	}
	if metrics.ProfitGrowthYoY != 0 {
		signal += math.Min(math.Abs(metrics.ProfitGrowthYoY)/cfgSignal.ProfitNormDenom, 1.0) * cfgSignal.ProfitWeight
	}
	if metrics.InventoryTurnover != 0 {
		signal += math.Min(metrics.InventoryTurnover/cfgSignal.InventoryNormDenom, 1.0) * cfgSignal.InventoryWeight
	}
	if metrics.CapacityUtilization > 0 {
		signal += math.Min(metrics.CapacityUtilization, 1.0) * cfgSignal.UtilizationWeight
	}

	boundary := ct.boundaryConfidence(industryID, metrics)
	// When revenue or profit is negative, the industry is in contraction.
	// High boundary confidence (far from positive thresholds) should not boost
	// overall confidence — it just means the decline is unambiguous, not that
	// the industry data is strong. Halve boundary contribution in that case.
	if metrics.RevenueGrowthYoY < 0 || metrics.ProfitGrowthYoY < 0 {
		boundary *= 0.5
	}
	confidence := signal*cfgSignal.SignalBoundaryMix + boundary*(1.0-cfgSignal.SignalBoundaryMix)

	seasonalScore := 0.0
	hasSeasonal := false
	if ct.seasonalEngine != nil {
		accuracy := ct.seasonalEngine.GetHistoricalAccuracy(time.Now())
		if accuracy > 0.5 {
			seasonalScore = accuracy
			hasSeasonal = true
		}
	}

	linkageScore := 0.0
	hasLinkage := false
	if ct.linkageAnalyzer != nil && cfgMix.WeightLinkage > 0 {
		ls := ct.computeLinkageConfidence(industryID)
		if ls > 0 {
			linkageScore = ls
			hasLinkage = true
		}
	}

	narrativeScore := 0.0
	hasNarrative := false
	if ct.narrativeHitRate != nil && cfgMix.WeightNarrative > 0 {
		ns := ct.narrativeHitRate()
		if ns > 0 {
			narrativeScore = ns
			hasNarrative = true
		}
	}

	if hasSeasonal || hasLinkage || hasNarrative {
		baseW := cfgMix.WeightBoundary + cfgMix.WeightFreshness
		seasonalW := cfgMix.WeightSeasonal
		linkageW := cfgMix.WeightLinkage
		narrativeW := cfgMix.WeightNarrative

		activeW := baseW
		if hasSeasonal {
			activeW += seasonalW
		}
		if hasLinkage {
			activeW += linkageW
		}
		if hasNarrative {
			activeW += narrativeW
		}

		if activeW > 0 {
			confidence = confidence*baseW/activeW +
				seasonalScore*seasonalW/activeW +
				linkageScore*linkageW/activeW +
				narrativeScore*narrativeW/activeW
		}
	}

	if confidence > cfgSignal.ConfidenceCeiling {
		confidence = cfgSignal.ConfidenceCeiling
	}
	if confidence < cfgSignal.ConfidenceFloor {
		confidence = cfgSignal.ConfidenceFloor
	}
	return confidence
}

func (ct *CycleTracker) computeLinkageConfidence(industryID string) float64 {
	if ct.linkageAnalyzer == nil {
		return 0
	}
	score := ct.linkageAnalyzer.CalculateLinkageScore(industryID)
	if score == nil {
		return 0
	}
	related := ct.linkageAnalyzer.GetCorrelationMatrix().GetCorrelatedIndustries(industryID, 0.3)
	if len(related) == 0 {
		return 0
	}
	pos, ok := ct.GetPosition(industryID)
	if !ok {
		return 0
	}
	consistent := 0.0
	total := 0.0
	for relatedID, corr := range related {
		if rp, rok := ct.GetPosition(relatedID); rok {
			total += math.Abs(corr)
			if rp.BusinessCycle == pos.BusinessCycle {
				consistent += math.Abs(corr)
			}
		}
	}
	if total == 0 {
		return 0
	}
	return (consistent / total) * score.SystemicImportance
}

// boundaryConfidence returns 0–1: 0 = metric at a phase threshold (ambiguous),
// 1 = far from any threshold (strong conviction in detected phase).
func (ct *CycleTracker) boundaryConfidence(industryID string, metrics IndustryMetrics) float64 {
	params := config.GetParametersConfig().Industry
	thresholds, ok := params.CycleThresholds.Value[industryID]
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

	revMinDist := math.Abs(metrics.RevenueGrowthYoY - thresholds.ExpansionRevenuePct)
	for _, t := range []float64{thresholds.RecoveryRevenuePct, thresholds.MatureRevenuePct} {
		if d := math.Abs(metrics.RevenueGrowthYoY - t); d < revMinDist {
			revMinDist = d
		}
	}

	profitMinDist := math.Abs(metrics.ProfitGrowthYoY - thresholds.ExpansionProfitPct)
	for _, t := range []float64{thresholds.RecoveryProfitPct, thresholds.MatureProfitPct} {
		if d := math.Abs(metrics.ProfitGrowthYoY - t); d < profitMinDist {
			profitMinDist = d
		}
	}

	tr := thresholds.ExpansionRevenuePct - thresholds.MatureRevenuePct
	if tr <= 0 {
		tr = 0.25
	}
	denom := tr * config.GetParametersConfig().Industry.ConfidenceSignal.Value.BoundaryDenomFactor

	revScore := math.Min(revMinDist/denom, 1.0)
	profitScore := math.Min(profitMinDist/denom, 1.0)

	return (revScore + profitScore) / 2.0
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

// GetWeightModulator returns a weight multiplier based on cycle phase and confidence.
// The multiplier scales from the configured phase multiplier toward 1.0 based on
// confidence level. High confidence in expansion yields a larger boost; low
// confidence dampens the adjustment toward neutral.
func (ct *CycleTracker) GetWeightModulator(industryID string) float64 {
	pos, ok := ct.GetPosition(industryID)
	if !ok {
		return 1.0
	}

	cfg := config.GetParametersConfig().Industry.CycleWeightMultipliers.Value
	var phaseMultiplier float64
	switch pos.BusinessCycle {
	case CycleExpansion:
		phaseMultiplier = cfg.ExpansionMultiplier
	case CycleRecovery:
		phaseMultiplier = cfg.RecoveryMultiplier
	case CycleMature:
		phaseMultiplier = cfg.MatureMultiplier
	case CycleRecession:
		phaseMultiplier = cfg.RecessionMultiplier
	default:
		return 1.0
	}

	deviation := phaseMultiplier - 1.0
	return 1.0 + deviation*pos.Confidence
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
	cfg := config.GetParametersConfig().Industry.PhaseScores.Value
	switch cp.BusinessCycle {
	case CycleExpansion:
		return cfg.ScoreExpansion
	case CycleRecovery:
		return cfg.ScoreRecovery
	case CycleMature:
		return cfg.ScoreMature
	case CycleRecession:
		return cfg.ScoreRecession
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

// GetContinuousPhaseScore returns a probability-weighted continuous phase score
// for an industry, ranging from -1.0 (deep recession) to +1.0 (full expansion).
// Uses the position confidence as a weight to blend adjacent phase scores,
// producing a smooth transition between discrete cycle phases.
func (ct *CycleTracker) GetContinuousPhaseScore(industryID string) float64 {
	pos, ok := ct.positions[industryID]
	if !ok {
		return 0.0
	}

	cfg := config.GetParametersConfig().Industry.PhaseScores.Value

	// Map each phase to its discrete score
	phaseScores := map[CyclePhase]float64{
		CycleExpansion: cfg.ScoreExpansion,
		CycleRecovery:  cfg.ScoreRecovery,
		CycleMature:    cfg.ScoreMature,
		CycleRecession: cfg.ScoreRecession,
	}

	// Use confidence as a blend factor toward the next phase.
	// Low confidence means the industry is "between" phases.
	confidence := pos.Confidence

	// Determine adjacent phases (forward/backward in the cycle)
	transitions := GetTypicalTransitions()

	// Find transitions from the current phase
	transProb := 0.0
	nextPhaseScore := pos.GetPhaseScore()
	for _, t := range transitions {
		if t.FromPhase == pos.BusinessCycle {
			transProb = t.Probability
			if ns, ok := phaseScores[t.ToPhase]; ok {
				nextPhaseScore = ns
			}
			break
		}
	}

	// Blend: when confidence is low (<0.5), pull toward the next likely phase.
	// When confidence is high (>=0.5), stay close to the discrete score.
	// This creates a smooth S-curve transition rather than a step function.
	blend := 1.0 - (confidence * confidence) // quadratic: at 0% confidence → 100% blend, at 100% → 0%
	continuousScore := pos.GetPhaseScore()*(1.0-blend*transProb) + nextPhaseScore*(blend*transProb)

	return continuousScore
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
	configs := config.GetParametersConfig().Industry.CycleTransitions.Value
	if len(configs) > 0 {
		transitions := make([]CyclePhaseTransition, len(configs))
		for i, c := range configs {
			transitions[i] = CyclePhaseTransition{
				FromPhase:           CyclePhase(c.FromPhase),
				ToPhase:             CyclePhase(c.ToPhase),
				Probability:         c.Probability,
				Triggers:            c.Triggers,
				TypicalDurationDays: c.TypicalDurationDays,
			}
		}
		return transitions
	}
	return []CyclePhaseTransition{
		{FromPhase: CycleRecession, ToPhase: CycleRecovery, Probability: 0.70, Triggers: []string{"inventory_depletion", "demand_stabilization"}, TypicalDurationDays: 180},
		{FromPhase: CycleRecovery, ToPhase: CycleExpansion, Probability: 0.80, Triggers: []string{"revenue_acceleration", "capex_increase"}, TypicalDurationDays: 270},
		{FromPhase: CycleExpansion, ToPhase: CycleMature, Probability: 0.60, Triggers: []string{"growth_deceleration", "margin_compression"}, TypicalDurationDays: 360},
		{FromPhase: CycleMature, ToPhase: CycleRecession, Probability: 0.50, Triggers: []string{"inventory_buildup", "demand_contraction"}, TypicalDurationDays: 180},
	}
}
