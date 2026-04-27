package service

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

type IndustryService struct {
	Classifier      *industry.ClassificationTree
	SeasonalEngine  *industry.SeasonalEngine
	CycleTracker    *industry.CycleTracker
	LinkageAnalyzer *industry.LinkageAnalyzer
	RiskMonitor     *industry.RiskMonitor
}

func NewIndustryService(
	classifier *industry.ClassificationTree,
	seasonalEngine *industry.SeasonalEngine,
	cycleTracker *industry.CycleTracker,
	linkageAnalyzer *industry.LinkageAnalyzer,
	riskMonitor *industry.RiskMonitor,
) *IndustryService {
	return &IndustryService{
		Classifier:      classifier,
		SeasonalEngine:  seasonalEngine,
		CycleTracker:    cycleTracker,
		LinkageAnalyzer: linkageAnalyzer,
		RiskMonitor:     riskMonitor,
	}
}

func (s *IndustryService) GetClassificationTree() []map[string]interface{} {
	segments := s.Classifier.GetAllSegments()
	var result []map[string]interface{}
	for _, seg := range segments {
		if seg.ParentID == "" {
			children := s.Classifier.GetChildren(seg.ID)
			var childList []map[string]interface{}
			for _, child := range children {
				grandchildren := s.Classifier.GetChildren(child.ID)
				var grandchildList []map[string]interface{}
				for _, gc := range grandchildren {
					grandchildList = append(grandchildList, map[string]interface{}{
						"id":          gc.ID,
						"name":        gc.Name,
						"weight":      gc.Weight,
						"description": gc.Description,
					})
				}
				childList = append(childList, map[string]interface{}{
					"id":          child.ID,
					"name":        child.Name,
					"weight":      child.Weight,
					"description": child.Description,
					"children":    grandchildList,
				})
			}
			result = append(result, map[string]interface{}{
				"id":          seg.ID,
				"name":        seg.Name,
				"weight":      seg.Weight,
				"description": seg.Description,
				"children":    childList,
			})
		}
	}
	return result
}

type SeasonalPattern struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	NameEN             string   `json:"name_en,omitempty"`
	Description        string   `json:"description"`
	StartMonth         int      `json:"start_month"`
	StartDay           int      `json:"start_day"`
	EndMonth           int      `json:"end_month"`
	EndDay             int      `json:"end_day"`
	HistoricalAccuracy float64  `json:"historical_accuracy"`
	TypicalReturn      float64  `json:"typical_return"`
	AdjustmentFactor   float64  `json:"adjustment_factor"`
	FavoredIndustries  []string `json:"favored_industries,omitempty"`
	AvoidedIndustries  []string `json:"avoided_industries,omitempty"`
	AffectedIndustries []string `json:"affected_industries,omitempty"`
	Impact             string   `json:"impact,omitempty"`
}

func (s *IndustryService) GetSeasonalPatterns(industryID string, now time.Time) (active []SeasonalPattern, historical []SeasonalPattern, adjustment float64) {
	patterns := s.SeasonalEngine.DetectCurrentPatterns(now)
	for _, p := range patterns {
		if industryID == "" || p.IsRelevantForIndustry(industryID) {
			active = append(active, SeasonalPattern{
				ID:                 p.ID,
				Name:               p.Name,
				Description:        p.Description,
				StartMonth:         p.StartMonth,
				StartDay:           p.StartDay,
				EndMonth:           p.EndMonth,
				EndDay:             p.EndDay,
				HistoricalAccuracy: p.HistoricalAccuracy,
				TypicalReturn:      p.TypicalReturn(),
				AffectedIndustries: p.AffectedIndustries(),
			})
		}
	}

	if industryID != "" {
		adjustment = s.SeasonalEngine.GetPatternAdjustment(industryID, now)
	}

	allPatterns := s.SeasonalEngine.GetAllPatterns()
	for _, p := range allPatterns {
		if industryID == "" || p.IsRelevantForIndustry(industryID) {
			impact := ""
			if industryID != "" {
				impact, _ = s.SeasonalEngine.GetIndustryImpact(p.ID, industryID)
			}
			historical = append(historical, SeasonalPattern{
				ID:                 p.ID,
				Name:               p.Name,
				NameEN:             p.NameEN,
				Description:        p.Description,
				StartMonth:         p.StartMonth,
				StartDay:           p.StartDay,
				EndMonth:           p.EndMonth,
				EndDay:             p.EndDay,
				HistoricalAccuracy: p.HistoricalAccuracy,
				TypicalReturn:      p.TypicalReturn(),
				AdjustmentFactor:   p.AdjustmentFactor,
				FavoredIndustries:  p.FavoredIndustries,
				AvoidedIndustries:  p.AvoidedIndustries,
				Impact:             impact,
			})
		}
	}
	return active, historical, adjustment
}

func (s *IndustryService) GetSeasonalCalendar(industryID string, year int) []map[string]interface{} {
	calendar := s.SeasonalEngine.GenerateCalendar(year)
	var months []map[string]interface{}
	for m := 1; m <= 12; m++ {
		monthPatterns := calendar.ByMonth[m]
		var relevantPatterns []map[string]interface{}
		for _, p := range monthPatterns {
			if industryID == "" || p.IsRelevantForIndustry(industryID) {
				relevantPatterns = append(relevantPatterns, map[string]interface{}{
					"id":                  p.ID,
					"name":                p.Name,
					"historical_accuracy": p.HistoricalAccuracy,
					"typical_return":      p.TypicalReturn(),
					"adjustment_factor":   p.AdjustmentFactor,
				})
			}
		}
		months = append(months, map[string]interface{}{
			"month":    m,
			"patterns": relevantPatterns,
			"count":    len(relevantPatterns),
		})
	}
	return months
}

type CyclePosition struct {
	Industry       string    `json:"industry"`
	Name           string    `json:"name"`
	BusinessCycle  string    `json:"business_cycle"`
	InventoryCycle string    `json:"inventory_cycle"`
	CapexCycle     string    `json:"capex_cycle"`
	Confidence     float64   `json:"confidence"`
	UpdatedAt      time.Time `json:"updated_at"`
	IsFavorable    bool      `json:"is_favorable"`
	PhaseScore     float64   `json:"phase_score"`
	Trend          string    `json:"trend"`
}

func (s *IndustryService) GetCyclePositions(industryID string) ([]CyclePosition, bool) {
	if industryID == "" {
		var allPositions []CyclePosition
		for _, seg := range s.Classifier.GetAllSegments() {
			if seg.ParentID != "" {
				continue
			}
			if pos, ok := s.CycleTracker.GetPosition(seg.ID); ok {
				allPositions = append(allPositions, CyclePosition{
					Industry:       seg.ID,
					Name:           seg.Name,
					BusinessCycle:  string(pos.BusinessCycle),
					InventoryCycle: string(pos.InventoryCycle),
					CapexCycle:     string(pos.CapexCycle),
					Confidence:     pos.Confidence,
					UpdatedAt:      pos.UpdatedAt,
					IsFavorable:    pos.IsFavorable(),
					PhaseScore:     pos.GetPhaseScore(),
					Trend:          pos.GetTrend(),
				})
			}
		}
		return allPositions, true
	}

	position, ok := s.CycleTracker.GetPosition(industryID)
	if !ok {
		return nil, false
	}
	return []CyclePosition{{
		Industry:       industryID,
		BusinessCycle:  string(position.BusinessCycle),
		InventoryCycle: string(position.InventoryCycle),
		CapexCycle:     string(position.CapexCycle),
		Confidence:     position.Confidence,
		UpdatedAt:      position.UpdatedAt,
		IsFavorable:    position.IsFavorable(),
		PhaseScore:     position.GetPhaseScore(),
		Trend:          position.GetTrend(),
	}}, true
}

type LinkageInfo struct {
	Industry     string                         `json:"industry"`
	Upstream     []string                       `json:"upstream"`
	Downstream   []string                       `json:"downstream"`
	Correlations []map[string]interface{}       `json:"correlations"`
	LinkageScore *industry.IndustryLinkageScore `json:"linkage_score"`
}

func (s *IndustryService) GetLinkageInfo(industryID string) (*LinkageInfo, error) {
	graph := s.LinkageAnalyzer.GetSupplyChainGraph()
	upstream := graph.GetUpstream(industryID)
	downstream := graph.GetDownstream(industryID)

	correlations := s.LinkageAnalyzer.GetCorrelationMatrix().GetCorrelatedIndustries(industryID, 0.0)
	var correlationList []map[string]interface{}
	for otherIndustry, correlation := range correlations {
		strength := "low"
		if abs(correlation) > 0.7 {
			strength = "high"
		} else if abs(correlation) > 0.4 {
			strength = "medium"
		}
		correlationList = append(correlationList, map[string]interface{}{
			"industry":    otherIndustry,
			"correlation": correlation,
			"strength":    strength,
		})
	}

	score := s.LinkageAnalyzer.CalculateLinkageScore(industryID)

	return &LinkageInfo{
		Industry:     industryID,
		Upstream:     upstream,
		Downstream:   downstream,
		Correlations: correlationList,
		LinkageScore: score,
	}, nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

type RiskInfo struct {
	Symbol      string                   `json:"symbol"`
	Industry    string                   `json:"industry"`
	RiskCount   int                      `json:"risk_count"`
	Risks       []map[string]interface{} `json:"risks"`
	HighestRisk map[string]interface{}   `json:"highest_risk"`
}

func (s *IndustryService) GetRiskInfo(symbol, industryID string) *RiskInfo {
	var risks []industry.RiskEvent
	if symbol == "" && industryID != "" {
		risks = s.RiskMonitor.GetAllRisks("ALL", industryID, 0, 0)
	} else {
		risks = s.RiskMonitor.GetAllRisks(symbol, industryID, 0, 0)
	}

	var riskList []map[string]interface{}
	for _, risk := range risks {
		riskList = append(riskList, map[string]interface{}{
			"id":                 risk.ID,
			"type":               risk.Type,
			"severity":           risk.Severity,
			"description":        risk.Description,
			"impact_estimate":    risk.ImpactEstimate,
			"confidence":         risk.Confidence,
			"detected_at":        risk.DetectedAt,
			"recommended_action": risk.RecommendedAction,
		})
	}

	highest := s.RiskMonitor.GetHighestRisk(risks)
	var highestRisk map[string]interface{}
	if highest != nil {
		highestRisk = map[string]interface{}{
			"id":          highest.ID,
			"type":        highest.Type,
			"severity":    highest.Severity,
			"description": highest.Description,
		}
	}

	return &RiskInfo{
		Symbol:      symbol,
		Industry:    industryID,
		RiskCount:   len(riskList),
		Risks:       riskList,
		HighestRisk: highestRisk,
	}
}

type IndustryOverview struct {
	ID               string                         `json:"id"`
	Name             string                         `json:"name"`
	CyclePhase       string                         `json:"cycle_phase"`
	InventoryCycle   string                         `json:"inventory_cycle"`
	CapexCycle       string                         `json:"capex_cycle"`
	CycleConfidence  float64                        `json:"cycle_confidence"`
	IsFavorable      bool                           `json:"is_favorable"`
	SeasonalPatterns []string                       `json:"seasonal_patterns"`
	LinkageScore     *industry.IndustryLinkageScore `json:"linkage_score"`
}

func (s *IndustryService) GetIndustryOverview(now time.Time) []IndustryOverview {
	segments := s.Classifier.GetAllSegments()
	var industries []IndustryOverview
	for _, seg := range segments {
		if seg.ParentID != "" {
			continue
		}

		cyclePos, ok := s.CycleTracker.GetPosition(seg.ID)
		if !ok {
			continue
		}

		patterns := s.SeasonalEngine.DetectCurrentPatterns(now)
		var activePatternNames []string
		for _, p := range patterns {
			if p.IsRelevantForIndustry(seg.ID) {
				activePatternNames = append(activePatternNames, p.Name)
			}
		}

		linkageScore := s.LinkageAnalyzer.CalculateLinkageScore(seg.ID)

		industries = append(industries, IndustryOverview{
			ID:               seg.ID,
			Name:             seg.Name,
			CyclePhase:       string(cyclePos.BusinessCycle),
			InventoryCycle:   string(cyclePos.InventoryCycle),
			CapexCycle:       string(cyclePos.CapexCycle),
			CycleConfidence:  cyclePos.Confidence,
			IsFavorable:      cyclePos.IsFavorable(),
			SeasonalPatterns: activePatternNames,
			LinkageScore:     linkageScore,
		})
	}
	return industries
}

type ShockImpact struct {
	Industry string  `json:"industry"`
	Impact   float64 `json:"impact"`
}

func (s *IndustryService) PropagateShock(sourceIndustry string, shockMagnitude float64, maxDepth int) []ShockImpact {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	impacts := s.LinkageAnalyzer.PropagateShock(sourceIndustry, shockMagnitude, maxDepth)
	var result []ShockImpact
	for industry, impact := range impacts {
		result = append(result, ShockImpact{
			Industry: industry,
			Impact:   impact,
		})
	}
	return result
}

type GraphNode struct {
	ID                 string  `json:"id"`
	SystemicImportance float64 `json:"systemic_importance"`
	UpstreamCount      int     `json:"upstream_count"`
	DownstreamCount    int     `json:"downstream_count"`
}

type GraphEdge struct {
	Source      string  `json:"source"`
	Target      string  `json:"target"`
	Correlation float64 `json:"correlation"`
	Strength    string  `json:"strength"`
}

func (s *IndustryService) GetIndustryGraph() ([]GraphNode, []GraphEdge) {
	cm := s.LinkageAnalyzer.GetCorrelationMatrix()

	var nodes []GraphNode
	var edges []GraphEdge
	nodeSet := make(map[string]bool)

	allCorrelations := cm.GetAllCorrelations()
	for industryA, correlations := range allCorrelations {
		if !nodeSet[industryA] {
			nodeSet[industryA] = true
			score := s.LinkageAnalyzer.CalculateLinkageScore(industryA)
			nodes = append(nodes, GraphNode{
				ID:                 industryA,
				SystemicImportance: score.SystemicImportance,
				UpstreamCount:      score.UpstreamCount,
				DownstreamCount:    score.DownstreamCount,
			})
		}

		for industryB, correlation := range correlations {
			if industryA >= industryB {
				continue
			}
			if !nodeSet[industryB] {
				nodeSet[industryB] = true
				score := s.LinkageAnalyzer.CalculateLinkageScore(industryB)
				nodes = append(nodes, GraphNode{
					ID:                 industryB,
					SystemicImportance: score.SystemicImportance,
					UpstreamCount:      score.UpstreamCount,
					DownstreamCount:    score.DownstreamCount,
				})
			}

			strength := "low"
			if abs(correlation) > 0.7 {
				strength = "high"
			} else if abs(correlation) > 0.4 {
				strength = "medium"
			}

			edges = append(edges, GraphEdge{
				Source:      industryA,
				Target:      industryB,
				Correlation: correlation,
				Strength:    strength,
			})
		}
	}
	return nodes, edges
}
