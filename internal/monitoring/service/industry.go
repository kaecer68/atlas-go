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

	switch seg.ID {
	case "semiconductor":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "出口比重", Weight: 0.35, Source: "海關統計", Evidence: "佔台灣總出口超過35%"},
			{Factor: "龍頭市值", Weight: 0.25, Source: "TWSE", Evidence: "台積電(2330)為最大權值股"},
			{Factor: "戰略價值", Weight: 0.25, Source: "地緣政治", Evidence: "全球先進製程核心供應商"},
			{Factor: "就業創造", Weight: 0.15, Source: "主計總處", Evidence: "直接就業人數超過10萬人"},
		}
		wd.Interpretation = "半導體為台灣經濟命脈，权重反映其在出口、市值、就業的核心地位"
		wd.RiskFactors = []string{"美中科技戰出口管制", "先進製程竞争加剧", "成熟製程中國大陸產能過剩"}
		wd.Opportunities = []string{"AI晶片需求爆發", "CoWoS先進封裝供需吃緊", "HPC高效能運算長期趨勢"}

	case "ai_supply_chain":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "需求增速", Weight: 0.30, Source: "機構預估", Evidence: "2024-2026 AI伺服器CAGR>40%"},
			{Factor: "台灣供應鏈完整性", Weight: 0.25, Source: "內部分析", Evidence: "全球80% AI伺服器組裝在台灣"},
			{Factor: "毛利率支撐", Weight: 0.25, Source: "廠商財報", Evidence: "散熱、電源供應商毛利率>25%"},
			{Factor: "政策支持", Weight: 0.20, Source: "國發基金", Evidence: "AI產業發展獲得政府資源挹注"},
		}
		wd.Interpretation = "AI供應鏈為台灣下一個核心成長引擎，权重反映其爆發性成長潛力"
		wd.RiskFactors = []string{"GB200延期出貨風險", "供應商過度集中", "中國供應鏈競爭"}
		wd.Opportunities = []string{"CSP資本支出持續擴張", "邊緣AI運算需求興起", "液冷散熱滲透率提升"}

	case "robotics":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "技術含量", Weight: 0.35, Source: "專利分析", Evidence: "全球減速機專利密度前三"},
			{Factor: "製造業升級需求", Weight: 0.30, Source: "工業局", Evidence: "台灣工具機產值全球第四"},
			{Factor: "人機協作趨勢", Weight: 0.20, Source: "IFR報告", Evidence: "2025協作型機器人安裝量預估成長30%"},
			{Factor: "出口競爭力", Weight: 0.15, Source: "海關", Evidence: "精密機械出口年成長8%"},
		}
		wd.Interpretation = "機器人產業权重低但技術壁壘高，為長期核心戰略產業"
		wd.RiskFactors = []string{"中國廠商低價競爭", "日本、歐洲傳統強權技術領先", "景氣循環影響資本支出"}
		wd.Opportunities = []string{"半導體先進封裝設備需求", "電動車組裝自動化", "醫療手術機器人滲透"}

	case "financials":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "放款基礎", Weight: 0.35, Source: "金管會", Evidence: "本國銀行放款規模超過40兆"},
			{Factor: "內需關聯", Weight: 0.30, Source: "央行", Evidence: "民間消費與金融業高度相關"},
			{Factor: "政策調控", Weight: 0.20, Source: "金管會", Evidence: "金融業受政策影響顯著"},
			{Factor: "升息環境", Weight: 0.15, Source: "Fed觀察", Evidence: "利差擴張有利銀行獲利"},
		}
		wd.Interpretation = "金融業权重反映其在內需與政策中的核心地位，防御性質明顯"
		wd.RiskFactors = []string{"信用風險攀升", "房市修正壓力", "數位金融顛覆"}
		wd.Opportunities = []string{"升息循環持續利差收益", "理財商品手續費收入", "不動產逆向房貸商機"}

	case "shipping":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "全球貿易量", Weight: 0.35, Source: "Clarksons", Evidence: "BDI指數與全球GDP增速高度相關"},
			{Factor: "產業集中度", Weight: 0.30, Source: "Alphaliner", Evidence: "長榮、陽明、萬海市佔率全球前10"},
			{Factor: "景氣循環", Weight: 0.25, Source: "歷史統計", Evidence: "航運景氣與全球貿易波動高度一致"},
			{Factor: "塞港紅利", Weight: 0.10, Source: "Clarksons", Evidence: "供應鏈瓶頸期超額利潤"},
		}
		wd.Interpretation = "航運業景氣循環特性鮮明，权重反映其高波動性但不可預測的本質"
		wd.RiskFactors = []string{"紅海危機常態化", "新造船交付過剩", "環保法規成本增加"}
		wd.Opportunities = []string{"全球供應鏈重組", "低碳航運轉型落後者", "碼頭擁堵再現"}

	case "energy":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "政策目標", Weight: 0.35, Source: "經濟部", Evidence: "2025綠能發電佔比20%目標"},
			{Factor: "進口依賴", Weight: 0.30, Source: "能源局", Evidence: "化石燃料進口依賴度>95%"},
			{Factor: "電價調整", Weight: 0.20, Source: "台電", Evidence: "2022-2024電價累計調漲45%"},
			{Factor: "地緣風險", Weight: 0.15, Source: "外交部", Evidence: "能源進口集中度高於軍事風險"},
		}
		wd.Interpretation = "能源業權重反映台灣對外部能源的高度依賴與能源轉型的結構性需求"
		wd.RiskFactors = []string{"國際燃料價格波動", "核能政策不確定性", "電網韌性不足"}
		wd.Opportunities = []string{"離岸風電國產化", "太陽能模組需求", "儲能系統商轉"}

	case "electronics":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "終端需求", Weight: 0.35, Source: "IDC", Evidence: "全球電子終端市場規模>2兆美元"},
			{Factor: "被動元件景氣", Weight: 0.25, Source: "TrendForce", Evidence: "MLCC市場供需循環"},
			{Factor: "中國供應鏈", Weight: 0.25, Source: "海關", Evidence: "台灣電子零組件對中出口比重高"},
			{Factor: "規格升級", Weight: 0.15, Source: "廠商財報", Evidence: "車用、工業用毛利率較佳"},
		}
		wd.Interpretation = "電子零組件為半導體下游，权重反映其作為供應鏈關鍵零組件的地位"
		wd.RiskFactors = []string{"中國大陸低價競爭", "景氣放緩影響消費電子", "規格標準化壓縮毛利"}
		wd.Opportunities = []string{"車用電子滲透率提升", "AI終端裝置", "高速傳輸介面升級"}

	case "consumer":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "內需消費", Weight: 0.40, Source: "主計總處", Evidence: "民間消費佔GDP約55%"},
			{Factor: "通膨轉嫁", Weight: 0.25, Source: "央行", Evidence: "食品飲料價格剛性上漲"},
			{Factor: "人口結構", Weight: 0.20, Source: "內政部", Evidence: "老化指數持續攀升"},
			{Factor: "出口導向", Weight: 0.15, Source: "海關", Evidence: "紡織、鞋類出口依賴國際景氣"},
		}
		wd.Interpretation = "傳產消費業权重反映其防御性質，與日常民生高度相關但成長性有限"
		wd.RiskFactors = []string{"人均所得停滯", "人口減少趨勢", "電商侵蝕毛利率"}
		wd.Opportunities = []string{"健康意識抬頭", "高端餐飲需求", "寵物經濟"}

	case "industrial":
		wd.DerivationFactors = []WeightFactor{
			{Factor: "基礎建設", Weight: 0.35, Source: "工程會", Evidence: "公共工程預算持續成長"},
			{Factor: "製造業PMI", Weight: 0.25, Source: "S&P Global", Evidence: "台灣製造業PMI榮枯線參考"},
			{Factor: "原物料價格", Weight: 0.25, Source: "商品指數", Evidence: "鋼鐵、塑化報價波動影響獲利"},
			{Factor: "出口競爭力", Weight: 0.15, Source: "海關", Evidence: "工具機出口中國比重高"},
		}
		wd.Interpretation = "工業製造業权重反映其與景氣循環的高度相關性"
		wd.RiskFactors = []string{"中國基建投資放緩", "原物料價格上漲", "環保法規趨嚴"}
		wd.Opportunities = []string{"半導體廠建設需求", "綠能基礎設施", "前瞻軌道建設"}

	default:
		wd.Interpretation = fmt.Sprintf("權重 %.1f%% 基於該產業在台灣經濟中的綜合重要性評估", seg.Weight*100)
		wd.DerivationFactors = []WeightFactor{
			{Factor: "綜合評估", Weight: 1.0, Source: "內部分析", Evidence: "結合出口、市值、就業等維度"},
		}
	}

	return wd
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
