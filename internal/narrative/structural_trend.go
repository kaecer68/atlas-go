package narrative

import (
	"fmt"
	"time"
)

// StructuralTrend represents a long-term structural trend that can override macro risks
type StructuralTrend struct {
	Name       string    `json:"name"`
	Theme      string    `json:"theme"`
	Strength   float64   `json:"strength"`   // 0.0 to 1.0
	Confidence float64   `json:"confidence"` // 0.0 to 1.0
	HitRate    float64   `json:"hit_rate"`   // Historical accuracy
	Evidence   []string  `json:"evidence"`
	Timestamp  time.Time `json:"timestamp"`
}

// StructuralTrendAssessment evaluates structural trends vs macro risks
type StructuralTrendAssessment struct {
	Trends             []StructuralTrend `json:"trends"`
	DominantTrend      *StructuralTrend  `json:"dominant_trend,omitempty"`
	OverrideScore      float64           `json:"override_score"` // 0.0 to 1.0
	ShouldOverrideRisk bool              `json:"should_override_risk"`
	Rationale          string            `json:"rationale"`
	Timestamp          time.Time         `json:"timestamp"`
}

// StructuralTrendEngine evaluates structural trends that may override macro risks
type StructuralTrendEngine struct {
	minTrendStrength  float64
	minConfidence     float64
	minHitRate        float64
	overrideThreshold float64
}

// NewStructuralTrendEngine creates a new structural trend engine with default thresholds
func NewStructuralTrendEngine() *StructuralTrendEngine {
	return &StructuralTrendEngine{
		minTrendStrength:  0.7,  // Minimum trend strength to be considered significant
		minConfidence:     0.75, // Minimum confidence level for trend detection
		minHitRate:        0.70, // Minimum historical hit rate for trend validity
		overrideThreshold: 0.65, // Score threshold to override macro risk signals
	}
}

// Assess evaluates structural trends and determines if they override macro risks
func (e *StructuralTrendEngine) Assess(data MacroDataSnapshot, sectorData SectorDataSnapshot) *StructuralTrendAssessment {
	assessment := &StructuralTrendAssessment{
		Timestamp: time.Now(),
	}

	// Detect all active structural trends
	assessment.Trends = e.detectTrends(data, sectorData)

	// Find dominant trend (highest strength * confidence * hit_rate)
	assessment.DominantTrend = e.findDominantTrend(assessment.Trends)

	// Calculate override score
	assessment.OverrideScore = e.calculateOverrideScore(assessment.DominantTrend)

	// Determine if structural trends should override macro risks
	assessment.ShouldOverrideRisk = assessment.OverrideScore >= e.overrideThreshold

	// Build rationale
	assessment.Rationale = e.buildRationale(assessment)

	return assessment
}

// SectorDataSnapshot holds sector-specific data for trend detection
type SectorDataSnapshot struct {
	AIRevenueGrowth    float64 // TSMC and AI supply chain revenue YoY %
	CoWoSUtilization   float64 // CoWoS capacity utilization %
	CapexGrowth        float64 // AI-related capex growth %
	SemiconductorIndex float64 // Philadelphia Semiconductor Index level
}

func (e *StructuralTrendEngine) detectTrends(data MacroDataSnapshot, sector SectorDataSnapshot) []StructuralTrend {
	var trends []StructuralTrend

	// AI Capex Surge Detection (Theme: AI_capex_surge, HitRate: 0.81)
	if sector.AIRevenueGrowth > 50.0 && sector.CoWoSUtilization > 85.0 {
		strength := min(1.0, sector.AIRevenueGrowth/100.0)
		confidence := min(1.0, sector.CoWoSUtilization/100.0)

		trends = append(trends, StructuralTrend{
			Name:       "AI Capex Surge",
			Theme:      "AI_capex_surge",
			Strength:   strength,
			Confidence: confidence,
			HitRate:    0.81,
			Evidence: []string{
				fmt.Sprintf("AI revenue growth: %.1f%%", sector.AIRevenueGrowth),
				fmt.Sprintf("CoWoS utilization: %.1f%%", sector.CoWoSUtilization),
			},
			Timestamp: time.Now(),
		})
	}

	// Semiconductor Cycle Detection
	if sector.SemiconductorIndex > 0 && sector.AIRevenueGrowth > 30.0 {
		strength := min(1.0, sector.AIRevenueGrowth/80.0)

		trends = append(trends, StructuralTrend{
			Name:       "Semiconductor Upcycle",
			Theme:      "semiconductor_upcycle",
			Strength:   strength,
			Confidence: 0.80,
			HitRate:    0.72,
			Evidence: []string{
				fmt.Sprintf("SOX index elevated: %.0f", sector.SemiconductorIndex),
				fmt.Sprintf("Revenue growth: %.1f%%", sector.AIRevenueGrowth),
			},
			Timestamp: time.Now(),
		})
	}

	// Cloud Infrastructure Expansion
	if sector.CapexGrowth > 40.0 {
		strength := min(1.0, sector.CapexGrowth/100.0)

		trends = append(trends, StructuralTrend{
			Name:       "Cloud Infrastructure Expansion",
			Theme:      "cloud_expansion",
			Strength:   strength,
			Confidence: 0.75,
			HitRate:    0.68,
			Evidence: []string{
				fmt.Sprintf("Capex growth: %.1f%%", sector.CapexGrowth),
			},
			Timestamp: time.Now(),
		})
	}

	return trends
}

func (e *StructuralTrendEngine) findDominantTrend(trends []StructuralTrend) *StructuralTrend {
	if len(trends) == 0 {
		return nil
	}

	var dominant *StructuralTrend
	maxScore := 0.0

	for i := range trends {
		score := trends[i].Strength * trends[i].Confidence * trends[i].HitRate
		if score > maxScore {
			maxScore = score
			dominant = &trends[i]
		}
	}

	return dominant
}

func (e *StructuralTrendEngine) calculateOverrideScore(dominant *StructuralTrend) float64 {
	if dominant == nil {
		return 0.0
	}

	// Override score combines strength, confidence, and historical accuracy
	score := dominant.Strength * dominant.Confidence * dominant.HitRate

	// Boost for high-confidence, high-hit-rate trends
	if dominant.Confidence > 0.8 && dominant.HitRate > 0.75 {
		score *= 1.2
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

func (e *StructuralTrendEngine) buildRationale(assessment *StructuralTrendAssessment) string {
	if assessment.DominantTrend == nil {
		return "No dominant structural trend detected. Macro risk assessment takes precedence."
	}

	trend := assessment.DominantTrend
	rationale := fmt.Sprintf(
		"Dominant structural trend: %s (theme: %s). "+
			"Strength: %.2f, Confidence: %.2f, Hit Rate: %.2f. "+
			"Override Score: %.2f (threshold: %.2f). ",
		trend.Name, trend.Theme, trend.Strength, trend.Confidence, trend.HitRate,
		assessment.OverrideScore, e.overrideThreshold,
	)

	if assessment.ShouldOverrideRisk {
		rationale += "Structural trend OVERRIDES macro risk signals."
	} else {
		rationale += "Macro risk signals take precedence."
	}

	return rationale
}

// CanWithstandMacroRisk determines if the portfolio can withstand current macro risk
// given the strength of structural trends
func (e *StructuralTrendEngine) CanWithstandMacroRisk(
	macroRisk MacroRiskLevel,
	structuralAssessment *StructuralTrendAssessment,
) bool {
	// Green/Yellow risk: always withstand
	if macroRisk <= MacroRiskYellow {
		return true
	}

	// No dominant trend: follow macro risk
	if structuralAssessment.DominantTrend == nil {
		return false
	}

	// Orange risk: withstand if strong structural trend
	if macroRisk == MacroRiskOrange {
		return structuralAssessment.OverrideScore >= 0.55
	}

	// Red risk: only withstand if very strong structural trend
	if macroRisk == MacroRiskRed {
		return structuralAssessment.OverrideScore >= 0.75
	}

	return false
}

// GetRecommendedAction returns the recommended action given macro risk and structural trends
func (e *StructuralTrendEngine) GetRecommendedAction(
	macroAssessment *MacroRiskAssessment,
	structuralAssessment *StructuralTrendAssessment,
) (action string, rationale string) {
	// Check if structural trends can override macro risks
	canWithstand := e.CanWithstandMacroRisk(macroAssessment.Level, structuralAssessment)

	switch macroAssessment.Level {
	case MacroRiskGreen:
		return "normal", "Normal market conditions. Proceed with standard allocation."

	case MacroRiskYellow:
		return "cautious", "Elevated risk detected. Maintain positions with tighter risk controls."

	case MacroRiskOrange:
		if canWithstand && structuralAssessment.DominantTrend != nil {
			return "hold_with_hedge",
				fmt.Sprintf("High macro risk (%s) but strong structural trend (%s) supports maintaining exposure with hedges.",
					macroAssessment.Level.String(), structuralAssessment.DominantTrend.Name)
		}
		return "reduce", "High macro risk. Reduce exposure and increase defensive positioning."

	case MacroRiskRed:
		if canWithstand && structuralAssessment.DominantTrend != nil {
			return "selective_hold",
				fmt.Sprintf("Critical macro risk (%s) but exceptional structural trend (%s) allows selective exposure in trend-aligned sectors.",
					macroAssessment.Level.String(), structuralAssessment.DominantTrend.Name)
		}
		return "exit", "Critical macro risk. Exit positions and move to cash/defensive assets."

	default:
		return "unknown", "Unable to determine action."
	}
}
