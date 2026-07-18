package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

type IndustryService struct {
	Classifier        *industry.ClassificationTree
	SeasonalEngine    *industry.SeasonalEngine
	CycleTracker      *industry.CycleTracker
	LinkageAnalyzer   *industry.LinkageAnalyzer
	RiskMonitor       *industry.RiskMonitor
	SiliconTracker    *industry.SiliconCycleTracker
	EventCalendar     *industry.EventCalendar
	CardBuilder       *industry.CycleStatusCardBuilder
	CycleCalibration  *industry.CycleCalibration
	siliconAggregator *industry.SiliconDataAggregator
	ODMChannel        *industry.ODMChannel
	DataAggregator    *industry.DataAggregator
	ParamsPath        string
	// WeightEngine is the authoritative sector allocation engine for detail
	// weight derivation. Production wiring may override it after NewIndustryService.
	WeightEngine sectorallocation.WeightEngine

	// snapshotReader provides persisted simulation-closing snapshot (SA08).
	// nil means snapshot-based sector allocation is unavailable.
	snapshotReader sectorallocation.SnapshotReader
}

func NewIndustryService(
	classifier *industry.ClassificationTree,
	seasonalEngine *industry.SeasonalEngine,
	cycleTracker *industry.CycleTracker,
	linkageAnalyzer *industry.LinkageAnalyzer,
	riskMonitor *industry.RiskMonitor,
	siliconTracker *industry.SiliconCycleTracker,
	eventCalendar *industry.EventCalendar,
	odmChannel *industry.ODMChannel,
	dataAggregator *industry.DataAggregator,
	paramsPath string,
) *IndustryService {
	if seasonalEngine != nil && linkageAnalyzer != nil {
		seasonalEngine.SetLinkageGraph(linkageAnalyzer.GetSupplyChainGraph())
	}
	cardBuilder := industry.NewCycleStatusCardBuilder(
		siliconTracker, cycleTracker, seasonalEngine, eventCalendar, linkageAnalyzer,
	)
	// SA06: WeightEngine is injected by the caller (composition root or
	// dashboard) after construction. IndustryService no longer creates a
	// nil-provider partial engine.
	return &IndustryService{
		Classifier:      classifier,
		SeasonalEngine:  seasonalEngine,
		CycleTracker:    cycleTracker,
		LinkageAnalyzer: linkageAnalyzer,
		RiskMonitor:     riskMonitor,
		SiliconTracker:  siliconTracker,
		EventCalendar:   eventCalendar,
		CardBuilder:     cardBuilder,
		ODMChannel:      odmChannel,
		DataAggregator:  dataAggregator,
		ParamsPath:      paramsPath,
	}
}

func (s *IndustryService) GetClassificationTree() []map[string]any {
	segments := s.Classifier.GetAllSegments()
	var result []map[string]any
	for _, seg := range segments {
		if seg.ParentID == "" {
			children := s.Classifier.GetChildren(seg.ID)
			var childList []map[string]any
			for _, child := range children {
				grandchildren := s.Classifier.GetChildren(child.ID)
				var grandchildList []map[string]any
				for _, gc := range grandchildren {
					grandchildList = append(grandchildList, map[string]any{
						"id":          gc.ID,
						"name":        gc.Name,
						"weight":      gc.Weight,
						"description": gc.Description,
					})
				}
				childList = append(childList, map[string]any{
					"id":          child.ID,
					"name":        child.Name,
					"weight":      child.Weight,
					"description": child.Description,
					"children":    grandchildList,
				})
			}
			result = append(result, map[string]any{
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
	AvgMarketReturn    float64  `json:"avg_market_return"`
	AdjustmentFactor   float64  `json:"adjustment_factor"`
	FavoredIndustries  []string `json:"favored_industries,omitempty"`
	AvoidedIndustries  []string `json:"avoided_industries,omitempty"`
	AffectedIndustries []string `json:"affected_industries,omitempty"`
	Impact             string   `json:"impact,omitempty"`
}

// GetAdjustmentBreakdown returns the per-layer decomposition of the seasonal adjustment.
func (s *IndustryService) GetAdjustmentBreakdown(industryID string, now time.Time) *industry.AdjustmentBreakdown {
	if s.SeasonalEngine == nil {
		return nil
	}
	return s.SeasonalEngine.GetAdjustmentBreakdown(industryID, now)
}

// GetActiveNarrativeThemes returns the narrative themes currently active for an industry.
func (s *IndustryService) GetActiveNarrativeThemes(industryID string) []string {
	if s.SeasonalEngine == nil {
		return []string{}
	}

	patterns := s.SeasonalEngine.GetActivePatternNames(time.Now())
	if patterns == nil {
		return []string{}
	}
	return patterns
}

// UpdateDynamicEnv pushes a fresh macro snapshot into the seasonal engine's environment modulator.
func (s *IndustryService) UpdateDynamicEnv(snap marketdata.MacroDataSnapshot) {
	if s.SeasonalEngine != nil {
		s.SeasonalEngine.UpdateDynamicEnv(snap)
	}
}

// GetCalibrationEvidence returns calibration metadata if seasonal patterns
// have been calibrated against real market data.
func (s *IndustryService) GetCalibrationEvidence() map[string]any {
	return industry.LoadCalibrationEvidence(constants.ParametersFile)
}

// RebuildCorrelations recomputes all pairwise industry correlations from return data.
func (s *IndustryService) RebuildCorrelations(industryReturns map[string][]float64) {
	if s.LinkageAnalyzer != nil {
		s.LinkageAnalyzer.GetCorrelationMatrix().RecalculateFromReturns(industryReturns)
	}
}

// GetSeasonalPatterns returns active and historical seasonal patterns for an industry.
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
				AvgMarketReturn:    p.AvgMarketReturn,
				AdjustmentFactor:   p.AdjustmentFactor,
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
				AvgMarketReturn:    p.AvgMarketReturn,
				AdjustmentFactor:   p.AdjustmentFactor,
				FavoredIndustries:  p.FavoredIndustries,
				AvoidedIndustries:  p.AvoidedIndustries,
				Impact:             impact,
			})
		}
	}
	return active, historical, adjustment
}

func (s *IndustryService) GetSeasonalCalendar(industryID string, year int) []map[string]any {
	calendar := s.SeasonalEngine.GenerateCalendar(year)
	var months []map[string]any
	for m := 1; m <= 12; m++ {
		monthPatterns := calendar.ByMonth[m]
		var relevantPatterns []map[string]any
		for _, p := range monthPatterns {
			if industryID == "" || p.IsRelevantForIndustry(industryID) {
				relevantPatterns = append(relevantPatterns, map[string]any{
					"id":                  p.ID,
					"name":                p.Name,
					"historical_accuracy": p.HistoricalAccuracy,
					"avg_market_return":   p.AvgMarketReturn,
					// Deprecated: remove after 2026-Q3 migration window.
					"typical_return":    p.AvgMarketReturn,
					"adjustment_factor": p.AdjustmentFactor,
				})
			}
		}
		months = append(months, map[string]any{
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
	// Confidence decomposition (computed at service layer)
	ConfidenceBreakdown map[string]float64 `json:"confidence_breakdown,omitempty"`
	NarrativeTheme      string             `json:"narrative_theme,omitempty"`
	// Threshold evidence quality from config
	ThresholdEvidence map[string]string `json:"threshold_evidence,omitempty"`
	// Evidence tracks whether this cycle position is based on empirical FinMind data or fallback defaults
	Evidence string `json:"evidence"`
}

func (s *IndustryService) GetCyclePositions(industryID string) ([]CyclePosition, bool) {
	ev := map[string]string{
		"source_type":       "heuristic",
		"evidence_quality":  "low",
		"update_policy":     "auto",
		"validation_method": "empirical_calibration",
	}

	buildCyclePosition := func(pos *industry.CyclePosition, name string) CyclePosition {
		evidence := s.CycleTracker.EvidenceTier(pos.IndustryID)
		narrativeTheme := s.CycleTracker.NarrativeTheme(pos.IndustryID)
		return CyclePosition{
			Industry:            pos.IndustryID,
			Name:                name,
			BusinessCycle:       string(pos.BusinessCycle),
			InventoryCycle:      string(pos.InventoryCycle),
			CapexCycle:          string(pos.CapexCycle),
			Confidence:          pos.Confidence,
			UpdatedAt:           pos.UpdatedAt,
			IsFavorable:         pos.IsFavorable(),
			PhaseScore:          pos.GetPhaseScore(),
			Trend:               pos.GetTrend(),
			ConfidenceBreakdown: s.CycleTracker.BuildConfidenceBreakdown(pos.IndustryID),
			NarrativeTheme:      narrativeTheme,
			ThresholdEvidence:   ev,
			Evidence:            evidence,
		}
	}

	if industryID == "" {
		var allPositions []CyclePosition
		for _, seg := range s.Classifier.GetAllSegments() {
			if seg.ParentID != "" {
				continue
			}
			if pos, ok := s.CycleTracker.GetPosition(seg.ID); ok {
				allPositions = append(allPositions, buildCyclePosition(pos, seg.Name))
			}
		}
		return allPositions, true
	}

	position, ok := s.CycleTracker.GetPosition(industryID)
	if !ok {
		return nil, false
	}
	seg, ok := s.Classifier.GetSegment(industryID)
	name := industryID
	if ok {
		name = seg.Name
	}
	return []CyclePosition{buildCyclePosition(position, name)}, true
}

type LinkageInfo struct {
	Industry     string                         `json:"industry"`
	Upstream     []string                       `json:"upstream"`
	Downstream   []string                       `json:"downstream"`
	Correlations []map[string]any               `json:"correlations"`
	LinkageScore *industry.IndustryLinkageScore `json:"linkage_score"`
}

func (s *IndustryService) GetLinkageInfo(industryID string) (*LinkageInfo, error) {
	graph := s.LinkageAnalyzer.GetSupplyChainGraph()
	upstream := graph.GetUpstream(industryID)
	downstream := graph.GetDownstream(industryID)

	correlations := s.LinkageAnalyzer.GetCorrelationMatrix().GetCorrelatedIndustries(industryID, 0.0)
	var correlationList []map[string]any
	for otherIndustry, correlation := range correlations {
		strength := "low"
		if abs(correlation) > 0.7 {
			strength = "high"
		} else if abs(correlation) > 0.4 {
			strength = "medium"
		}
		correlationList = append(correlationList, map[string]any{
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
	Symbol      string           `json:"symbol"`
	Industry    string           `json:"industry"`
	RiskCount   int              `json:"risk_count"`
	Risks       []map[string]any `json:"risks"`
	HighestRisk map[string]any   `json:"highest_risk"`
}

func (s *IndustryService) GetRiskInfo(symbol, industryID string) *RiskInfo {
	var risks []industry.RiskEvent
	if symbol == "" && industryID != "" {
		risks = s.RiskMonitor.GetAllRisks("ALL", industryID, 0, 0)
	} else {
		risks = s.RiskMonitor.GetAllRisks(symbol, industryID, 0, 0)
	}

	var riskList []map[string]any
	for _, risk := range risks {
		riskList = append(riskList, map[string]any{
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
	var highestRisk map[string]any
	if highest != nil {
		highestRisk = map[string]any{
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
	ID                 string                         `json:"id"`
	Name               string                         `json:"name"`
	BaseWeight         float64                        `json:"base_weight"`
	AdjustedWeight     float64                        `json:"adjusted_weight"`
	CyclePhase         string                         `json:"cycle_phase"`
	InventoryCycle     string                         `json:"inventory_cycle"`
	CapexCycle         string                         `json:"capex_cycle"`
	CycleConfidence    float64                        `json:"cycle_confidence"`
	IsFavorable        bool                           `json:"is_favorable"`
	SeasonalPatterns   []string                       `json:"seasonal_patterns"`
	LinkageScore       *industry.IndustryLinkageScore `json:"linkage_score"`
	CycleMultiplier    float64                        `json:"cycle_multiplier"`
	SeasonalMultiplier float64                        `json:"seasonal_multiplier"`
	LinkageMultiplier  float64                        `json:"linkage_multiplier"`
	AdjustmentLog      []string                       `json:"adjustment_log"`
}

func (s *IndustryService) getSectorWeight(segID string, fallback float64) float64 {
	if w, ok := config.GetParametersConfig().SectorAllocation.BaseWeights[segID]; ok {
		return w
	}
	return fallback
}

func (s *IndustryService) GetIndustryOverview(now time.Time) []IndustryOverview {
	segments := s.Classifier.GetAllSegments()
	sectorWeights := config.GetParametersConfig().SectorAllocation.BaseWeights
	weightFloor := config.GetParametersConfig().Industry.WeightFloor.Value
	linkageImpact := config.GetParametersConfig().Industry.LinkageWeightImpact.Value

	type rawWeight struct {
		overview IndustryOverview
		raw      float64
	}

	var rawWeights []rawWeight

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

		baseWeight, ok := sectorWeights[seg.ID]
		if !ok {
			baseWeight = seg.Weight
		}

		cycleMultiplier := s.CycleTracker.GetWeightModulator(seg.ID)
		seasonalMultiplier := s.SeasonalEngine.GetPatternAdjustment(seg.ID, now)

		linkageMultiplier := 1.0
		if linkageScore != nil {
			deviation := linkageScore.SystemicImportance - 0.5
			linkageMultiplier = 1.0 + deviation*linkageImpact
		}

		rawAdjusted := baseWeight * cycleMultiplier * seasonalMultiplier * linkageMultiplier

		var adjustmentLog []string
		adjustmentLog = append(adjustmentLog, fmt.Sprintf("base_weight=%.4f", baseWeight))
		adjustmentLog = append(adjustmentLog, fmt.Sprintf("cycle_multiplier=%.4f (phase=%s, confidence=%.2f)", cycleMultiplier, cyclePos.BusinessCycle, cyclePos.Confidence))
		adjustmentLog = append(adjustmentLog, fmt.Sprintf("seasonal_multiplier=%.4f", seasonalMultiplier))
		if linkageScore != nil {
			adjustmentLog = append(adjustmentLog, fmt.Sprintf("linkage_multiplier=%.4f (systemic_importance=%.4f)", linkageMultiplier, linkageScore.SystemicImportance))
		}
		adjustmentLog = append(adjustmentLog, fmt.Sprintf("raw_adjusted=%.4f", rawAdjusted))

		overview := IndustryOverview{
			ID:                 seg.ID,
			Name:               seg.Name,
			BaseWeight:         baseWeight,
			CyclePhase:         string(cyclePos.BusinessCycle),
			InventoryCycle:     string(cyclePos.InventoryCycle),
			CapexCycle:         string(cyclePos.CapexCycle),
			CycleConfidence:    cyclePos.Confidence,
			IsFavorable:        cyclePos.IsFavorable(),
			SeasonalPatterns:   activePatternNames,
			LinkageScore:       linkageScore,
			CycleMultiplier:    cycleMultiplier,
			SeasonalMultiplier: seasonalMultiplier,
			LinkageMultiplier:  linkageMultiplier,
			AdjustmentLog:      adjustmentLog,
		}

		rawWeights = append(rawWeights, rawWeight{overview: overview, raw: rawAdjusted})
	}

	totalWeight := 0.0
	for i := range rawWeights {
		if rawWeights[i].raw < weightFloor {
			rawWeights[i].raw = weightFloor
		}
		totalWeight += rawWeights[i].raw
	}

	var industries []IndustryOverview
	if totalWeight > 0 {
		scale := 1.0 / totalWeight
		for _, rw := range rawWeights {
			rw.overview.AdjustedWeight = rw.raw * scale
			industries = append(industries, rw.overview)
		}
	}

	return industries
}

// GetLatestSectorAllocation returns the latest persisted simulation-closing
// snapshot. Returns nil, error when the snapshot reader is unavailable or no
// snapshot has been persisted yet (SA08 contract).
func (s *IndustryService) GetLatestSectorAllocation(ctx context.Context) (*sectorallocation.SectorAllocationSnapshot, error) {
	if s.snapshotReader == nil {
		return nil, fmt.Errorf("snapshot reader not configured")
	}
	return s.snapshotReader.LatestSnapshot(), nil
}

// WithSnapshotReader injects a snapshot reader for sector allocation queries.
func (s *IndustryService) WithSnapshotReader(r sectorallocation.SnapshotReader) {
	s.snapshotReader = r
}

// IndustryRecommendation provides actionable recommendation for an industry
type IndustryRecommendation struct {
	Action        string  `json:"action"`
	Conviction    string  `json:"conviction"`
	TargetWeight  float64 `json:"target_weight"`
	CurrentWeight float64 `json:"current_weight"`
	Delta         float64 `json:"delta"`
	Rationale     string  `json:"rationale"`
	TimeHorizon   string  `json:"time_horizon"`
	RiskAdjusted  bool    `json:"risk_adjusted"`
}

// IndustryDetail provides comprehensive industry analysis for the detail modal
type IndustryDetail struct {
	ID                   string                            `json:"id"`
	Name                 string                            `json:"name"`
	NameEN               string                            `json:"name_en"`
	Description          string                            `json:"description"`
	Level                int                               `json:"level"`
	Weight               float64                           `json:"weight"`
	WeightDerivation     sectorallocation.WeightDerivation `json:"weight_derivation"`
	RepresentativeStocks []string                          `json:"representative_stocks"`
	CyclePosition        *CyclePosition                    `json:"cycle_position"`
	LinkageInfo          *LinkageInfo                      `json:"linkage_info"`
	RiskInfo             *RiskInfo                         `json:"risk_info"`
	SeasonalPatterns     []SeasonalPattern                 `json:"seasonal_patterns"`
	Recommendation       *IndustryRecommendation           `json:"recommendation"`
	RegimeContext        string                            `json:"regime_context"`
}

// GetIndustryDetail returns comprehensive industry analysis including weight explanation
func (s *IndustryService) GetIndustryDetail(industryID string, now time.Time) (*IndustryDetail, error) {
	segment, ok := s.Classifier.GetSegment(industryID)
	if !ok {
		return nil, fmt.Errorf("industry not found: %s", industryID)
	}

	// Get cycle position
	cyclePos, _ := s.CycleTracker.GetPosition(industryID)

	// Get linkage info
	linkageInfo, _ := s.GetLinkageInfo(industryID)

	// Get risk info
	riskInfo := s.GetRiskInfo("", industryID)

	// Get seasonal patterns
	_, historicalPatterns, _ := s.GetSeasonalPatterns(industryID, now)

	weightDerivation := s.computeWeightDerivation(industryID, now)

	// Generate recommendation
	recommendation := s.generateRecommendation(segment, cyclePos)

	// Determine regime context
	regimeContext := s.getRegimeContext(segment, cyclePos)

	var cyclePosPtr *CyclePosition
	if cyclePos != nil {
		cyclePosPtr = &CyclePosition{
			Industry:       cyclePos.IndustryID,
			Name:           segment.Name,
			BusinessCycle:  string(cyclePos.BusinessCycle),
			InventoryCycle: string(cyclePos.InventoryCycle),
			CapexCycle:     string(cyclePos.CapexCycle),
			Confidence:     cyclePos.Confidence,
			UpdatedAt:      cyclePos.UpdatedAt,
			IsFavorable:    cyclePos.IsFavorable(),
			PhaseScore:     cyclePos.GetPhaseScore(),
			Trend:          cyclePos.GetTrend(),
		}
	}

	return &IndustryDetail{
		ID:                   segment.ID,
		Name:                 segment.Name,
		NameEN:               segment.NameEN,
		Description:          segment.Description,
		Level:                int(segment.Level),
		Weight:               s.getSectorWeight(segment.ID, segment.Weight),
		WeightDerivation:     weightDerivation,
		RepresentativeStocks: segment.RepresentativeStocks,
		CyclePosition:        cyclePosPtr,
		LinkageInfo:          linkageInfo,
		RiskInfo:             riskInfo,
		SeasonalPatterns:     historicalPatterns,
		Recommendation:       recommendation,
		RegimeContext:        regimeContext,
	}, nil
}

func (s *IndustryService) computeWeightDerivation(industryID string, now time.Time) sectorallocation.WeightDerivation {
	if s.WeightEngine == nil {
		return sectorallocation.WeightDerivation{}
	}

	sw, _ := s.WeightEngine.ComputeWeight(context.Background(), industryID, now)
	if sw == nil {
		return sectorallocation.WeightDerivation{}
	}

	return sectorallocation.WeightDerivation{
		BaseWeight:        sw.AdjustedWeight,
		DerivationFactors: sw.DerivationFactors,
	}
}

func (s *IndustryService) generateRecommendation(seg *industry.IndustrySegment, pos *industry.CyclePosition) *IndustryRecommendation {
	baseWeight := s.getSectorWeight(seg.ID, seg.Weight)
	rec := &IndustryRecommendation{
		CurrentWeight: baseWeight,
		TargetWeight:  baseWeight,
		RiskAdjusted:  false,
	}

	if pos == nil {
		rec.Action = "觀望"
		rec.Conviction = "低"
		rec.Rationale = "缺乏足夠的週期數據來形成判斷"
		rec.TimeHorizon = "等待數據"
		return rec
	}

	// Determine action based on cycle position and favorability
	switch {
	case pos.IsFavorable() && pos.BusinessCycle == industry.CycleExpansion:
		rec.Action = "增持"
		rec.Conviction = "高"
		rec.TargetWeight = baseWeight * 1.2
		rec.Rationale = fmt.Sprintf("%s處於擴張期，%s庫存週期有利，建議超配", seg.Name, pos.InventoryCycle)

	case pos.IsFavorable() && pos.BusinessCycle == industry.CycleRecovery:
		rec.Action = "溫和增持"
		rec.Conviction = "中"
		rec.TargetWeight = baseWeight * 1.1
		rec.Rationale = fmt.Sprintf("%s處於復甦初期，资本支出開始擴張，建議適度超配", seg.Name)
		rec.TimeHorizon = "6-12個月"

	case !pos.IsFavorable() && pos.BusinessCycle == industry.CycleRecession:
		rec.Action = "減持"
		rec.Conviction = "高"
		rec.TargetWeight = baseWeight * 0.7
		rec.Rationale = fmt.Sprintf("%s處於衰退期，庫存去化中，建議低配", seg.Name)
		rec.TimeHorizon = "3-6個月"

	case !pos.IsFavorable() && pos.BusinessCycle == industry.CycleMature:
		rec.Action = "中性"
		rec.Conviction = "中"
		rec.TargetWeight = baseWeight
		rec.Rationale = fmt.Sprintf("%s處於成熟期，循環方向不明確，建議標配", seg.Name)
		rec.TimeHorizon = "1-3個月"

	default:
		rec.Action = "中性"
		rec.Conviction = "低"
		rec.TargetWeight = baseWeight
		rec.Rationale = fmt.Sprintf("%s目前無明確方向，建議維持基準權重", seg.Name)
		rec.TimeHorizon = "觀望"
	}

	rec.Delta = rec.TargetWeight - rec.CurrentWeight

	// Risk adjustment based on capex cycle
	switch pos.CapexCycle {
	case industry.CapexExpansion:
		rec.RiskAdjusted = true
		rec.Rationale += "。資本支出擴張中，景氣有撐。"
	case industry.CapexContraction:
		rec.RiskAdjusted = true
		rec.Rationale += "。資本支出收縮中，需留意下行風險。"
	}

	return rec
}

func (s *IndustryService) getRegimeContext(seg *industry.IndustrySegment, pos *industry.CyclePosition) string {
	if pos == nil {
		return "目前無市場體制數據"
	}

	// Check for AI/半導體超級循環信號
	if seg.ID == "semiconductor" || seg.ID == "ai_supply_chain" {
		if pos.BusinessCycle == industry.CycleExpansion && pos.InventoryCycle == industry.InvRestockingActive {
			return "AI超級循環信號：半導體+AI供應鏈處於主升段"
		}
	}

	// check for 防禦模式
	if seg.ID == "financials" || seg.ID == "consumer" {
		if pos.BusinessCycle == industry.CycleMature || pos.BusinessCycle == industry.CycleRecession {
			return "防禦模式：內需型產業相對抗跌"
		}
	}

	// 景氣循環判斷
	switch pos.BusinessCycle {
	case industry.CycleExpansion:
		return "擴張期：風險偏好高，進取型產業表現較佳"
	case industry.CycleRecovery:
		return "復甦期：景氣回溫，循環型產業開始表現"
	case industry.CycleMature:
		return "成熟期：景氣高原，配置宜均衡"
	case industry.CycleRecession:
		return "衰退期：避險情緒升溫，防禦型產業相對穩健"
	}

	return "目前市場體制分析中"
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

// BuildCycleStatusCard produces a market-wide composite cycle status card.
func (s *IndustryService) BuildCycleStatusCard(now time.Time) (*industry.CycleStatusCard, error) {
	if s.CardBuilder == nil {
		return nil, fmt.Errorf("card builder not initialized")
	}
	return s.CardBuilder.BuildCompositeCard(now)
}

// BuildIndustryCycleStatusCard produces a single-industry cycle status card.
func (s *IndustryService) BuildIndustryCycleStatusCard(now time.Time, industryID string) (*industry.CycleStatusCard, error) {
	if s.CardBuilder == nil {
		return nil, fmt.Errorf("card builder not initialized")
	}
	return s.CardBuilder.BuildCard(now, industryID)
}

// SetMacroProvider wires a MacroDataProvider into the silicon cycle aggregator
// so that scheduled silicon indicator updates can pull real TSMC/SOX data.
// Safe to call multiple times; each call rebuilds the aggregator with the
// latest provider.
func (s *IndustryService) SetMacroProvider(mp marketdata.MacroDataProvider) {
	if s.SiliconTracker != nil {
		s.siliconAggregator = industry.NewSiliconDataAggregator(s.SiliconTracker, mp)
	}
}

// UpdateSiliconIndicators triggers a refresh of silicon cycle indicators
// from the configured macro provider. When no aggregator is initialized
// (SetMacroProvider not yet called), it returns nil (no-op).
func (s *IndustryService) UpdateSiliconIndicators(ctx context.Context) error {
	if s.siliconAggregator == nil {
		return nil // no-op: aggregator not wired
	}
	return s.siliconAggregator.AggregateSiliconIndicators(ctx)
}

// SetCycleCalibration injects the calibration tracker and wires it into
// the global card builder state so resolveCardConfig picks up calibrated weights.
func (s *IndustryService) SetCycleCalibration(cal *industry.CycleCalibration) {
	s.CycleCalibration = cal
	industry.SetGlobalCycleCalibration(cal)
}

// GetCalibrationMetrics returns per-layer accuracy metrics from the
// cycle calibration tracker, or nil if no calibration is active.
func (s *IndustryService) GetCalibrationMetrics() map[string]industry.LayerMetrics {
	if s.CycleCalibration == nil {
		return nil
	}
	return s.CycleCalibration.GetMetrics()
}

// RecordCycleCalibrationOutcome stores one calibration data point.
// layerSignals should map the layer name to its raw signal value from the
// CycleStatusCard (e.g., silicon score, business_cycle confidence, etc.).
func (s *IndustryService) RecordCycleCalibrationOutcome(
	sessionID string, date time.Time, layerSignals map[string]float64, actualReturn float64,
) {
	if s.CycleCalibration != nil {
		s.CycleCalibration.RecordOutcome(sessionID, date, layerSignals, actualReturn)
	}
}

// ODMChannelSnapshot is the dashboard-facing view of ODMChannel state.
type ODMChannelSnapshot struct {
	RegisteredSymbols []string
	Revenues          map[string]float64
	FetchedAt         time.Time
	FetchErrors       []string
	CowosCurrent      float64
	CowosBaseline     float64
	CowosDelta        float64
	CowosTrend        string
	CowosLastUpdate   time.Time
	Transmission      *industry.ODMTransmissionModel
	TransmissionAt    time.Time
}

// GetODMChannelSnapshot returns the current state of the ODM channel.
// Returns a zero-value snapshot when s.ODMChannel is nil.
func (s *IndustryService) GetODMChannelSnapshot(ctx context.Context) ODMChannelSnapshot {
	snap := ODMChannelSnapshot{
		Revenues: make(map[string]float64),
	}
	if s.ODMChannel == nil {
		return snap
	}

	symSnapshot := s.ODMChannel.CowosTracker().Snapshot()
	snap.CowosCurrent = symSnapshot.CurrentUtilization
	snap.CowosBaseline = 0.75
	snap.CowosDelta = symSnapshot.CurrentUtilization - snap.CowosBaseline
	snap.CowosTrend = symSnapshot.TrendDirection
	snap.CowosLastUpdate = symSnapshot.LastUpdated

	revs, err := s.ODMChannel.GetAllRevenues(ctx)
	snap.Revenues = revs
	snap.FetchedAt = time.Now()
	if err != nil {
		snap.FetchErrors = append(snap.FetchErrors, err.Error())
	}

	for sym := range revs {
		snap.RegisteredSymbols = append(snap.RegisteredSymbols, sym)
	}
	sort.Strings(snap.RegisteredSymbols)

	model, terr := s.ODMChannel.CalculateTransmission(ctx)
	if terr == nil {
		snap.Transmission = model
		snap.TransmissionAt = time.Now()
	}
	return snap
}

// DataAggregatorSummary is the dashboard-facing summary of a data
// aggregator run over all Level-1 industries.
type DataAggregatorSummary struct {
	Industries []DataAggregatorIndustry `json:"industries"`
	Count      int                      `json:"count"`
	FetchedAt  time.Time                `json:"fetched_at"`
}

// DataAggregatorIndustry is a per-industry snapshot of the latest
// aggregated growth metrics plus evidence quality.
type DataAggregatorIndustry struct {
	IndustryID       string    `json:"industry_id"`
	RevenueGrowthYoY float64   `json:"revenue_growth_yoy"`
	ProfitGrowthYoY  float64   `json:"profit_growth_yoy"`
	UpdatedAt        time.Time `json:"updated_at"`
	HasData          bool      `json:"has_data"`
}

// GetDataAggregatorSummary reads CycleTracker state for every Level-1
// industry and returns the aggregate snapshot. Returns an empty summary
// when s.DataAggregator or s.Classifier is nil.
func (s *IndustryService) GetDataAggregatorSummary() DataAggregatorSummary {
	summary := DataAggregatorSummary{
		Industries: []DataAggregatorIndustry{},
		FetchedAt:  time.Now(),
	}
	if s.DataAggregator == nil || s.Classifier == nil || s.CycleTracker == nil {
		return summary
	}
	for _, seg := range s.Classifier.GetAllSegments() {
		if seg.ParentID != "" {
			continue
		}
		item := DataAggregatorIndustry{IndustryID: seg.ID}
		if pos, ok := s.CycleTracker.GetPosition(seg.ID); ok {
			item.RevenueGrowthYoY = pos.RevenueGrowthYoY
			item.ProfitGrowthYoY = pos.ProfitGrowthYoY
			item.UpdatedAt = pos.UpdatedAt
			item.HasData = pos.RevenueGrowthYoY != 0 || pos.ProfitGrowthYoY != 0
		}
		summary.Industries = append(summary.Industries, item)
	}
	summary.Count = len(summary.Industries)
	return summary
}

// GetSeasonalHealth returns the calibration health summary for the
// configured parameters.json path. Returns nil when s.ParamsPath is empty.
func (s *IndustryService) GetSeasonalHealth() (*industry.CalibrationHealthSummary, error) {
	if s.ParamsPath == "" {
		return nil, fmt.Errorf("seasonal health: params path not configured")
	}
	return industry.SummarizeCalibrationHealth(s.ParamsPath)
}

// CorrelationLoaderMetadata is the dashboard-facing metadata for the
// correlation loader (sample size, sector coverage, last rebuild time).
type CorrelationLoaderMetadata struct {
	ReplayPath        string    `json:"replay_path"`
	SectorSymbolsPath string    `json:"sector_symbols_path"`
	Sectors           []string  `json:"sectors"`
	SectorCount       int       `json:"sector_count"`
	MinObservations   int       `json:"min_observations"`
	LastUpdated       time.Time `json:"last_updated"`
}

// GetCorrelationLoaderMetadata returns metadata about the configured
// replay/sector-symbols paths and the linkage analyzer's correlation
// matrix. Returns empty metadata when s.LinkageAnalyzer is nil.
func (s *IndustryService) GetCorrelationLoaderMetadata(replayPath, sectorSymbolsPath string) CorrelationLoaderMetadata {
	meta := CorrelationLoaderMetadata{
		ReplayPath:        replayPath,
		SectorSymbolsPath: sectorSymbolsPath,
		LastUpdated:       time.Now(),
		MinObservations:   15,
	}
	if s.LinkageAnalyzer == nil {
		return meta
	}
	cm := s.LinkageAnalyzer.GetCorrelationMatrix()
	for industryID := range cm.GetAllCorrelations() {
		meta.Sectors = append(meta.Sectors, industryID)
	}
	sort.Strings(meta.Sectors)
	meta.SectorCount = len(meta.Sectors)
	return meta
}
