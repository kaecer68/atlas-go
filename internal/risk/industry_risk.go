package risk

import (
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// IndustryRiskAssessment evaluates the risk contribution from industry cycle positions.
type IndustryRiskAssessment struct {
	TotalIndustryCount     int                `json:"total_industry_count"`
	RecessionIndustryCount int                `json:"recession_industry_count"`
	ExpansionIndustryCount int                `json:"expansion_industry_count"`
	WeightedCycleScore     float64            `json:"weighted_cycle_score"` // -1.0 to 1.0, confidence-weighted average
	TopRiskIndustries      []IndustryRiskItem `json:"top_risk_industries"`
	Timestamp              time.Time          `json:"timestamp"`
}

// IndustryRiskItem represents a single industry's risk contribution.
type IndustryRiskItem struct {
	IndustryID    string              `json:"industry_id"`
	BusinessCycle industry.CyclePhase `json:"business_cycle"`
	Confidence    float64             `json:"confidence"`
	PhaseScore    float64             `json:"phase_score"`
	Weight        float64             `json:"weight"`
}

// IndustryRiskProvider defines the interface for assessing industry cycle risk.
type IndustryRiskProvider interface {
	Assess() (*IndustryRiskAssessment, error)
}

// CycleTrackerRiskProvider uses an industry.CycleTracker to assess industry cycle risk.
type CycleTrackerRiskProvider struct {
	tracker       *industry.CycleTracker
	sectorWeights map[string]float64
}

// NewCycleTrackerRiskProvider creates a new risk provider backed by a CycleTracker.
// sectorWeights maps industry IDs to their portfolio weights (or importance weights).
// If nil or empty, all tracked industries are weighted equally.
func NewCycleTrackerRiskProvider(tracker *industry.CycleTracker, sectorWeights map[string]float64) *CycleTrackerRiskProvider {
	return &CycleTrackerRiskProvider{
		tracker:       tracker,
		sectorWeights: sectorWeights,
	}
}

// Assess evaluates cycle risk across all tracked industries and returns an assessment.
func (p *CycleTrackerRiskProvider) Assess() (*IndustryRiskAssessment, error) {
	if p.tracker == nil {
		return nil, fmt.Errorf("industry_risk: tracker is nil")
	}

	assessment := &IndustryRiskAssessment{
		Timestamp: time.Now(),
	}

	positions := p.tracker.GetAllPositions()
	assessment.TotalIndustryCount = len(positions)
	if len(positions) == 0 {
		return assessment, nil
	}

	items := make([]IndustryRiskItem, 0, len(positions))
	var weightedScoreSum float64
	var totalWeight float64

	for id, pos := range positions {
		weight := p.getWeight(id)
		if weight <= 0 {
			continue
		}
		continuousScore := p.tracker.GetContinuousPhaseScore(id)

		item := IndustryRiskItem{
			IndustryID:    id,
			BusinessCycle: pos.BusinessCycle,
			Confidence:    pos.Confidence,
			PhaseScore:    continuousScore,
			Weight:        weight,
		}
		items = append(items, item)

		weightedScoreSum += continuousScore * weight
		totalWeight += weight

		switch pos.BusinessCycle {
		case industry.CycleRecession:
			assessment.RecessionIndustryCount++
		case industry.CycleExpansion:
			assessment.ExpansionIndustryCount++
		}
	}

	if totalWeight > 0 {
		assessment.WeightedCycleScore = weightedScoreSum / totalWeight
	}

	// Sort by phase score, riskiest first
	sort.Slice(items, func(i, j int) bool {
		return items[i].PhaseScore < items[j].PhaseScore
	})

	topN := min(5, len(items))
	assessment.TopRiskIndustries = items[:topN]

	return assessment, nil
}

func (p *CycleTrackerRiskProvider) getWeight(industryID string) float64 {
	if p.sectorWeights == nil {
		return 1.0
	}
	if w, ok := p.sectorWeights[industryID]; ok {
		return w
	}
	return 1.0
}
