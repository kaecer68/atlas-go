package industry

import (
	"fmt"
	"maps"
	"math"
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
	IndustryID          string               `json:"industry_id"`
	BusinessCycle       CyclePhase           `json:"business_cycle"`
	InventoryCycle      InventoryCycle       `json:"inventory_cycle"`
	CapexCycle          CapexCycle           `json:"capex_cycle"`
	Confidence          float64              `json:"confidence"` // 0.0 to 1.0 (backward-compatible composite)
	ConfidenceBreakdown *ConfidenceBreakdown `json:"confidence_breakdown,omitempty"`
	LeadingIndicators   []Indicator          `json:"leading_indicators"`
	LaggingIndicators   []Indicator          `json:"lagging_indicators"`
	CycleDurationDays   int                  `json:"cycle_duration_days"` // Days in current phase
	ExpectedPhaseChange *time.Time           `json:"expected_phase_change,omitempty"`
	UpdatedAt           time.Time            `json:"updated_at"`
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
	positions        map[string]*CyclePosition
	history          map[string][]CyclePosition
	seasonalEngine   *SeasonalEngine
	linkageAnalyzer  *LinkageAnalyzer
	narrativeHitRate func(industryID string) float64
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

// SetExternalValidators wires optional external data sources for
// multi-dimensional confidence. Nil args disable that dimension.
func (ct *CycleTracker) SetExternalValidators(seasonal *SeasonalEngine, linkage *LinkageAnalyzer) {
	ct.seasonalEngine = seasonal
	ct.linkageAnalyzer = linkage
}

func (ct *CycleTracker) SetNarrativeProvider(fn func(industryID string) float64) {
	ct.narrativeHitRate = fn
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
	if len(ct.history[industryID]) > config.GetParametersConfig().Industry.HistoryRetentionDays.Value {
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

	position.Confidence = ct.calculateConfidence(industryID, metrics)
	position.ConfidenceBreakdown = ct.calculateConfidenceBreakdown(industryID, metrics)

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
	cfg := config.GetParametersConfig().Industry
	s := cfg.ConfidenceSignal.Value

	hasData := metrics.RevenueGrowthYoY != 0 || metrics.ProfitGrowthYoY != 0 ||
		metrics.InventoryTurnover != 0 || metrics.CapacityUtilization > 0
	if !hasData {
		return s.SignalBase
	}

	signal := s.SignalBase
	if metrics.RevenueGrowthYoY != 0 {
		signal += math.Min(math.Abs(metrics.RevenueGrowthYoY)/s.RevenueNormDenom, 1.0) * s.RevenueWeight
	}
	if metrics.ProfitGrowthYoY != 0 {
		signal += math.Min(math.Abs(metrics.ProfitGrowthYoY)/s.ProfitNormDenom, 1.0) * s.ProfitWeight
	}
	if metrics.InventoryTurnover != 0 {
		signal += math.Min(metrics.InventoryTurnover/s.InventoryNormDenom, 1.0) * s.InventoryWeight
	}
	if metrics.CapacityUtilization > 0 {
		signal += math.Min(metrics.CapacityUtilization, 1.0) * s.UtilizationWeight
	}

	boundary := ct.boundaryConfidence(industryID, metrics)

	confidence := signal*s.SignalBoundaryMix + boundary*(1.0-s.SignalBoundaryMix)

	if confidence > s.ConfidenceCeiling {
		confidence = s.ConfidenceCeiling
	}
	if confidence < s.ConfidenceFloor {
		confidence = s.ConfidenceFloor
	}
	return confidence
}

func (ct *CycleTracker) calculateConfidenceBreakdown(industryID string, metrics IndustryMetrics) *ConfidenceBreakdown {
	cfg := config.GetParametersConfig().Industry
	mix := cfg.ConfidenceMix.Value

	signal := cfg.ConfidenceSignal.Value
	hasData := metrics.RevenueGrowthYoY != 0 || metrics.ProfitGrowthYoY != 0 ||
		metrics.InventoryTurnover != 0 || metrics.CapacityUtilization > 0

	var boundaryScore float64
	if hasData {
		s := signal.SignalBase
		if metrics.RevenueGrowthYoY != 0 {
			s += math.Min(math.Abs(metrics.RevenueGrowthYoY)/signal.RevenueNormDenom, 1.0) * signal.RevenueWeight
		}
		if metrics.ProfitGrowthYoY != 0 {
			s += math.Min(math.Abs(metrics.ProfitGrowthYoY)/signal.ProfitNormDenom, 1.0) * signal.ProfitWeight
		}
		if metrics.InventoryTurnover != 0 {
			s += math.Min(metrics.InventoryTurnover/signal.InventoryNormDenom, 1.0) * signal.InventoryWeight
		}
		if metrics.CapacityUtilization > 0 {
			s += math.Min(metrics.CapacityUtilization, 1.0) * signal.UtilizationWeight
		}
		b := ct.boundaryConfidence(industryID, metrics)
		boundaryScore = s*signal.SignalBoundaryMix + b*(1.0-signal.SignalBoundaryMix)
		if boundaryScore > signal.ConfidenceCeiling {
			boundaryScore = signal.ConfidenceCeiling
		}
		if boundaryScore < signal.ConfidenceFloor {
			boundaryScore = signal.ConfidenceFloor
		}
	} else {
		boundaryScore = signal.ConfidenceFloor
	}

	freshnessScore := freshnessFactor(metrics.DataFreshness)

	seasonalScore := 0.0
	if ct.seasonalEngine != nil {
		seasonalScore = ct.seasonalEngine.GetHistoricalAccuracy(time.Now())
	}

	linkageScore := 0.0
	if ct.linkageAnalyzer != nil && mix.WeightLinkage > 0 {
		linkageScore = ct.computeLinkageConfidence(industryID)
	}

	narrativeScore := 0.0
	if ct.narrativeHitRate != nil && mix.WeightNarrative > 0 {
		narrativeScore = ct.narrativeHitRate(industryID)
	}

	composite := boundaryScore*mix.WeightBoundary +
		freshnessScore*mix.WeightFreshness +
		seasonalScore*mix.WeightSeasonal +
		linkageScore*mix.WeightLinkage +
		narrativeScore*mix.WeightNarrative

	return &ConfidenceBreakdown{
		Composite:          composite,
		Boundary:           boundaryScore,
		Freshness:          freshnessScore,
		Seasonal:           seasonalScore,
		Linkage:            linkageScore,
		Narrative:          narrativeScore,
		DataFreshnessLevel: metrics.DataFreshness,
		DataFreshnessScore: freshnessScore,
		Weights: map[string]float64{
			"boundary":  mix.WeightBoundary,
			"freshness": mix.WeightFreshness,
			"seasonal":  mix.WeightSeasonal,
			"linkage":   mix.WeightLinkage,
			"narrative": mix.WeightNarrative,
		},
	}
}

func freshnessFactor(freshness DataFreshness) float64 {
	scores := &config.GetParametersConfig().Industry.FreshnessScores.Value
	switch freshness {
	case FreshLive:
		return scores.ScoreLive
	case FreshRecent:
		return scores.ScoreRecent
	case FreshStale:
		return scores.ScoreStale
	case FreshFallback:
		return scores.ScoreFallback
	default:
		return scores.ScoreDefault
	}
}

func (ct *CycleTracker) computeLinkageConfidence(industryID string) float64 {
	if ct.linkageAnalyzer == nil {
		return 0
	}
	score := ct.linkageAnalyzer.CalculateLinkageScore(industryID)
	if score == nil {
		return 0
	}
	minCorr := config.GetParametersConfig().Industry.LinkageParams.Value.MinCorrelationThreshold
	related := ct.linkageAnalyzer.GetCorrelationMatrix().GetCorrelatedIndustries(industryID, minCorr)
	if len(related) == 0 {
		return 0
	}
	consistent := 0.0
	total := 0.0
	for relatedID, corr := range related {
		if pos, ok := ct.GetPosition(relatedID); ok {
			total += math.Abs(corr)
			if pos.BusinessCycle == ct.positions[industryID].BusinessCycle {
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
		tr = config.GetParametersConfig().Industry.ConfidenceSignal.Value.ThresholdRangeFallback
	}
	denom := tr * params.ConfidenceSignal.Value.BoundaryDenomFactor

	revScore := math.Min(revMinDist/denom, 1.0)
	profitScore := math.Min(profitMinDist/denom, 1.0)

	return (revScore + profitScore) / 2.0
}

// getLeadingIndicators returns leading indicators for an industry.
func (ct *CycleTracker) getLeadingIndicators(industryID string, metrics IndustryMetrics) []Indicator {
	sig := config.GetParametersConfig().Industry.ConfidenceSignal.Value
	var indicators []Indicator

	if metrics.RevenueGrowthYoY != 0 {
		indicators = append(indicators, Indicator{
			Name:      "Revenue Growth YoY",
			Value:     metrics.RevenueGrowthYoY,
			Unit:      "%",
			Trend:     ct.getTrend(metrics.RevenueGrowthYoY, sig.RevenueTrendThreshold),
			IsLeading: true,
			Weight:    sig.RevenueIndicatorWeight,
		})
	}

	if metrics.InventoryTurnover != 0 {
		indicators = append(indicators, Indicator{
			Name:      "Inventory Turnover",
			Value:     metrics.InventoryTurnover,
			Unit:      "x",
			Trend:     ct.getTrend(metrics.InventoryTurnover, sig.InventoryTrendThreshold),
			IsLeading: true,
			Weight:    sig.InventoryIndicatorWeight,
		})
	}

	return indicators
}

func (ct *CycleTracker) getLaggingIndicators(industryID string, metrics IndustryMetrics) []Indicator {
	sig := config.GetParametersConfig().Industry.ConfidenceSignal.Value
	var indicators []Indicator

	if metrics.ProfitGrowthYoY != 0 {
		indicators = append(indicators, Indicator{
			Name:      "Profit Growth YoY",
			Value:     metrics.ProfitGrowthYoY,
			Unit:      "%",
			Trend:     ct.getTrend(metrics.ProfitGrowthYoY, sig.ProfitTrendThreshold),
			IsLeading: false,
			Weight:    sig.ProfitIndicatorWeight,
		})
	}

	if metrics.CapacityUtilization != 0 {
		indicators = append(indicators, Indicator{
			Name:      "Capacity Utilization",
			Value:     metrics.CapacityUtilization,
			Unit:      "%",
			Trend:     ct.getTrend(metrics.CapacityUtilization, sig.CapacityTrendThreshold),
			IsLeading: false,
			Weight:    sig.CapacityIndicatorWeight,
		})
	}

	return indicators
}

// getTrend determines the trend direction based on value and threshold.
func (ct *CycleTracker) getTrend(value, threshold float64) string {
	sig := config.GetParametersConfig().Industry.ConfidenceSignal.Value
	if value > threshold*sig.TrendUpMultiplier {
		return "up"
	} else if value < threshold*sig.TrendDownMultiplier {
		return "down"
	}
	return "stable"
}

func (cp *CyclePosition) IsFavorable() bool {
	minConfidence := config.GetParametersConfig().Industry.ConfidenceMix.Value.FavorableConfidenceMin
	return (cp.BusinessCycle == CycleRecovery || cp.BusinessCycle == CycleExpansion) &&
		cp.Confidence >= minConfidence
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
	scores := &config.GetParametersConfig().Industry.PhaseScores.Value
	switch cp.BusinessCycle {
	case CycleExpansion:
		return scores.ScoreExpansion
	case CycleRecovery:
		return scores.ScoreRecovery
	case CycleMature:
		return scores.ScoreMature
	case CycleRecession:
		return scores.ScoreRecession
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
	cfgTransitions := config.GetParametersConfig().Industry.CycleTransitions.Value
	result := make([]CyclePhaseTransition, 0, len(cfgTransitions))
	for _, ct := range cfgTransitions {
		result = append(result, CyclePhaseTransition{
			FromPhase:           CyclePhase(ct.FromPhase),
			ToPhase:             CyclePhase(ct.ToPhase),
			Triggers:            ct.Triggers,
			Probability:         ct.Probability,
			TypicalDurationDays: ct.TypicalDurationDays,
		})
	}
	return result
}
