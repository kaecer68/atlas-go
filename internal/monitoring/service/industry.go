package service

import (
	"fmt"
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
						"base_weight": gc.Weight,
						"description": gc.Description,
					})
				}
				childList = append(childList, map[string]any{
					"id":          child.ID,
					"name":        child.Name,
					"weight":      child.Weight,
					"base_weight": child.Weight,
					"description": child.Description,
					"children":    grandchildList,
				})
			}
			result = append(result, map[string]any{
				"id":          seg.ID,
				"name":        seg.Name,
				"weight":      seg.Weight,
				"base_weight": seg.Weight,
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
					"typical_return":      p.TypicalReturn(),
					"adjustment_factor":   p.AdjustmentFactor,
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
	Industry            string                        `json:"industry"`
	Name                string                        `json:"name"`
	BusinessCycle       string                        `json:"business_cycle"`
	InventoryCycle      string                        `json:"inventory_cycle"`
	CapexCycle          string                        `json:"capex_cycle"`
	Confidence          float64                       `json:"confidence"`
	ConfidenceBreakdown *industry.ConfidenceBreakdown `json:"confidence_breakdown,omitempty"`
	UpdatedAt           time.Time                     `json:"updated_at"`
	IsFavorable         bool                          `json:"is_favorable"`
	PhaseScore          float64                       `json:"phase_score"`
	Trend               string                        `json:"trend"`
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
					Industry:            seg.ID,
					Name:                seg.Name,
					BusinessCycle:       string(pos.BusinessCycle),
					InventoryCycle:      string(pos.InventoryCycle),
					CapexCycle:          string(pos.CapexCycle),
					Confidence:          pos.Confidence,
					ConfidenceBreakdown: pos.ConfidenceBreakdown,
					UpdatedAt:           pos.UpdatedAt,
					IsFavorable:         pos.IsFavorable(),
					PhaseScore:          pos.GetPhaseScore(),
					Trend:               pos.GetTrend(),
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
		Industry:            industryID,
		BusinessCycle:       string(position.BusinessCycle),
		InventoryCycle:      string(position.InventoryCycle),
		CapexCycle:          string(position.CapexCycle),
		Confidence:          position.Confidence,
		ConfidenceBreakdown: position.ConfidenceBreakdown,
		UpdatedAt:           position.UpdatedAt,
		IsFavorable:         position.IsFavorable(),
		PhaseScore:          position.GetPhaseScore(),
		Trend:               position.GetTrend(),
	}}, true
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
	ID                  string                         `json:"id"`
	Name                string                         `json:"name"`
	CyclePhase          string                         `json:"cycle_phase"`
	InventoryCycle      string                         `json:"inventory_cycle"`
	CapexCycle          string                         `json:"capex_cycle"`
	CycleConfidence     float64                        `json:"cycle_confidence"`
	ConfidenceBreakdown *industry.ConfidenceBreakdown  `json:"confidence_breakdown,omitempty"`
	IsFavorable         bool                           `json:"is_favorable"`
	SeasonalPatterns    []string                       `json:"seasonal_patterns"`
	LinkageScore        *industry.IndustryLinkageScore `json:"linkage_score"`
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
			ID:                  seg.ID,
			Name:                seg.Name,
			CyclePhase:          string(cyclePos.BusinessCycle),
			InventoryCycle:      string(cyclePos.InventoryCycle),
			CapexCycle:          string(cyclePos.CapexCycle),
			CycleConfidence:     cyclePos.Confidence,
			ConfidenceBreakdown: cyclePos.ConfidenceBreakdown,
			IsFavorable:         cyclePos.IsFavorable(),
			SeasonalPatterns:    activePatternNames,
			LinkageScore:        linkageScore,
		})
	}
	return industries
}

// WeightDerivation explains how an industry's weight is determined
type WeightDerivation struct {
	BaseWeight        float64        `json:"base_weight"`
	DerivationFactors []WeightFactor `json:"derivation_factors"`
	Interpretation    string         `json:"interpretation"`
	RiskFactors       []string       `json:"risk_factors"`
	Opportunities     []string       `json:"opportunities"`
}

// WeightFactor represents a single factor contributing to industry weight
type WeightFactor struct {
	Factor   string  `json:"factor"`
	Weight   float64 `json:"contribution"`
	Source   string  `json:"source"`
	Evidence string  `json:"evidence"`
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
	ID                   string                  `json:"id"`
	Name                 string                  `json:"name"`
	NameEN               string                  `json:"name_en"`
	Description          string                  `json:"description"`
	Level                int                     `json:"level"`
	Weight               float64                 `json:"weight"`
	WeightDerivation     WeightDerivation        `json:"weight_derivation"`
	RepresentativeStocks []string                `json:"representative_stocks"`
	CyclePosition        *CyclePosition          `json:"cycle_position"`
	LinkageInfo          *LinkageInfo            `json:"linkage_info"`
	RiskInfo             *RiskInfo               `json:"risk_info"`
	SeasonalPatterns     []SeasonalPattern       `json:"seasonal_patterns"`
	Recommendation       *IndustryRecommendation `json:"recommendation"`
	RegimeContext        string                  `json:"regime_context"`
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

	// Build weight derivation based on industry characteristics
	weightDerivation := s.calculateWeightDerivation(segment)

	// Generate recommendation
	recommendation := s.generateRecommendation(segment, cyclePos, weightDerivation)

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
		Weight:               segment.Weight,
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

func (s *IndustryService) calculateWeightDerivation(seg *industry.IndustrySegment) WeightDerivation {
	wd := WeightDerivation{
		BaseWeight:        seg.Weight,
		DerivationFactors: []WeightFactor{},
		RiskFactors:       []string{},
		Opportunities:     []string{},
	}

	wd.DerivationFactors = []WeightFactor{
		{Factor: "基準權重", Weight: seg.Weight, Source: "parameters.json (industry.sector_weights)", Evidence: fmt.Sprintf("%.1f%% 來自統一參數配置", seg.Weight*100)},
	}
	wd.Interpretation = fmt.Sprintf("權重 %.1f%% 來自 configs/parameters.json 的產業配置；因子分布分析待 Phase 2 真實市場數據源整合後提供", seg.Weight*100)

	// 定性風險與機會分析（不宣稱外部數據源，純屬內部分析觀點）
	wd.RiskFactors = s.getDefaultRiskFactors(seg.ID)
	wd.Opportunities = s.getDefaultOpportunities(seg.ID)

	return wd
}

func (s *IndustryService) getDefaultRiskFactors(id string) []string {
	switch id {
	case "semiconductor":
		return []string{"美中科技戰出口管制", "先進製程竞争加剧", "成熟製程中國大陸產能過剩"}
	case "ai_supply_chain":
		return []string{"GB200延期出貨風險", "供應商過度集中", "中國供應鏈競爭"}
	case "robotics":
		return []string{"中國廠商低價競爭", "日本、歐洲傳統強權技術領先", "景氣循環影響資本支出"}
	case "financials":
		return []string{"信用風險攀升", "房市修正壓力", "數位金融顛覆"}
	case "shipping":
		return []string{"紅海危機常態化", "新造船交付過剩", "環保法規成本增加"}
	case "energy":
		return []string{"國際燃料價格波動", "核能政策不確定性", "電網韌性不足"}
	case "electronics":
		return []string{"中國大陸低價競爭", "景氣放緩影響消費電子", "規格標準化壓縮毛利"}
	case "consumer":
		return []string{"人均所得停滯", "人口減少趨勢", "電商侵蝕毛利率"}
	case "industrial":
		return []string{"中國基建投資放緩", "原物料價格上漲", "環保法規趨嚴"}
	default:
		return nil
	}
}

func (s *IndustryService) getDefaultOpportunities(id string) []string {
	switch id {
	case "semiconductor":
		return []string{"AI晶片需求爆發", "CoWoS先進封裝供需吃緊", "HPC高效能運算長期趨勢"}
	case "ai_supply_chain":
		return []string{"CSP資本支出持續擴張", "邊緣AI運算需求興起", "液冷散熱滲透率提升"}
	case "robotics":
		return []string{"半導體先進封裝設備需求", "電動車組裝自動化", "醫療手術機器人滲透"}
	case "financials":
		return []string{"升息循環持續利差收益", "理財商品手續費收入", "不動產逆向房貸商機"}
	case "shipping":
		return []string{"全球供應鏈重組", "低碳航運轉型落後者", "碼頭擁堵再現"}
	case "energy":
		return []string{"離岸風電國產化", "太陽能模組需求", "儲能系統商轉"}
	case "electronics":
		return []string{"車用電子滲透率提升", "AI終端裝置", "高速傳輸介面升級"}
	case "consumer":
		return []string{"健康意識抬頭", "高端餐飲需求", "寵物經濟"}
	case "industrial":
		return []string{"半導體廠建設需求", "綠能基礎設施", "前瞻軌道建設"}
	default:
		return nil
	}
}

func (s *IndustryService) generateRecommendation(seg *industry.IndustrySegment, pos *industry.CyclePosition, wd WeightDerivation) *IndustryRecommendation {
	rec := &IndustryRecommendation{
		CurrentWeight: seg.Weight,
		TargetWeight:  seg.Weight,
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
		rec.TargetWeight = seg.Weight * 1.2
		rec.Rationale = fmt.Sprintf("%s處於擴張期，%s庫存週期有利，建議超配", seg.Name, pos.InventoryCycle)
		rec.TimeHorizon = "3-6個月"

	case pos.IsFavorable() && pos.BusinessCycle == industry.CycleRecovery:
		rec.Action = "溫和增持"
		rec.Conviction = "中"
		rec.TargetWeight = seg.Weight * 1.1
		rec.Rationale = fmt.Sprintf("%s處於復甦初期，资本支出開始擴張，建議適度超配", seg.Name)
		rec.TimeHorizon = "6-12個月"

	case !pos.IsFavorable() && pos.BusinessCycle == industry.CycleRecession:
		rec.Action = "減持"
		rec.Conviction = "高"
		rec.TargetWeight = seg.Weight * 0.7
		rec.Rationale = fmt.Sprintf("%s處於衰退期，庫存去化中，建議低配", seg.Name)
		rec.TimeHorizon = "3-6個月"

	case !pos.IsFavorable() && pos.BusinessCycle == industry.CycleMature:
		rec.Action = "中性"
		rec.Conviction = "中"
		rec.TargetWeight = seg.Weight
		rec.Rationale = fmt.Sprintf("%s處於成熟期，循環方向不明確，建議標配", seg.Name)
		rec.TimeHorizon = "1-3個月"

	default:
		rec.Action = "中性"
		rec.Conviction = "低"
		rec.TargetWeight = seg.Weight
		rec.Rationale = fmt.Sprintf("%s目前無明確方向，建議維持基準權重", seg.Name)
		rec.TimeHorizon = "觀望"
	}

	rec.Delta = rec.TargetWeight - rec.CurrentWeight

	// Risk adjustment based on capex cycle
	if pos.CapexCycle == industry.CapexExpansion {
		rec.RiskAdjusted = true
		rec.Rationale += "。資本支出擴張中，景氣有撐。"
	} else if pos.CapexCycle == industry.CapexContraction {
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
