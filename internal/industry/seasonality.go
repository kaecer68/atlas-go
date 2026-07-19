package industry

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// SeasonalPattern represents a recurring seasonal pattern in Taiwan stock market.
type SeasonalPattern struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	NameEN             string   `json:"name_en"`
	StartMonth         int      `json:"start_month"`
	StartDay           int      `json:"start_day"`
	EndMonth           int      `json:"end_month"`
	EndDay             int      `json:"end_day"`
	FavoredIndustries  []string `json:"favored_industries"`
	AvoidedIndustries  []string `json:"avoided_industries"`
	AdjustmentFactor   float64  `json:"adjustment_factor"`   // e.g., 1.2 for favored
	HistoricalAccuracy float64  `json:"historical_accuracy"` // 0.0 to 1.0
	AvgMarketReturn    float64  `json:"avg_market_return"`   // 期間累積報酬（geometric compound return）：e.g. 0.032 = 該季節 3.2% 累積漲幅。校準器與預設值單位需對齊。
	Description        string   `json:"description"`
}

func (p SeasonalPattern) IsRelevantForIndustry(industryID string) bool {
	if slices.Contains(p.FavoredIndustries, industryID) {
		return true
	}
	return slices.Contains(p.AvoidedIndustries, industryID)
}

func (p SeasonalPattern) AffectedIndustries() []string {
	result := make([]string, 0, len(p.FavoredIndustries)+len(p.AvoidedIndustries))
	result = append(result, p.FavoredIndustries...)
	result = append(result, p.AvoidedIndustries...)
	return result
}

// SeasonalEngine detects and evaluates seasonal patterns.
type SeasonalEngine struct {
	patterns          []SeasonalPattern
	linkageGraph      *SupplyChainGraph
	narrativeProvider NarrativeSeasonalProvider
	dynamicEnv        *DynamicEnvModulator
}

// NarrativeSeasonalProvider supplies active macro-narrative themes that
// modulate seasonal adjustment factors based on real-world events.
type NarrativeSeasonalProvider interface {
	ActiveThemes() []string
	SeasonalMultiplier(theme string, industryID string, direction float64) float64
}

// NewSeasonalEngine creates a seasonal engine using the parameter-managed seasonal patterns.
func NewSeasonalEngine() *SeasonalEngine {
	cfg := config.GetParametersConfig()
	if cfg == nil {
		return NewSeasonalEngineFromConfig(nil)
	}
	return NewSeasonalEngineFromConfig(cfg)
}

// Deprecated: use cfg.Industry.SeasonalPatterns from parameters.json instead.
func DefaultSeasonalPatterns() []SeasonalPattern {
	return []SeasonalPattern{
		{
			ID:                 "spring_festival",
			Name:               "春節行情",
			NameEN:             "Spring Festival Rally",
			StartMonth:         1,
			StartDay:           15,
			EndMonth:           2,
			EndDay:             15,
			FavoredIndustries:  []string{"financials", "mining"},
			AvoidedIndustries:  []string{"semiconductor", "ai_supply_chain"},
			AdjustmentFactor:   1.15,
			HistoricalAccuracy: 0.70,
			AvgMarketReturn:    0.032,
			Description:        "年前資金回籠，高股息與金融股受追捧；電子股進入淡季",
		},
		{
			ID:                 "earnings_window",
			Name:               "季報空窗期",
			NameEN:             "Earnings Report Window",
			StartMonth:         3,
			StartDay:           1,
			EndMonth:           4,
			EndDay:             15,
			FavoredIndustries:  []string{"ai_supply_chain"},
			AdjustmentFactor:   1.10,
			HistoricalAccuracy: 0.55,
			AvgMarketReturn:    0.015,
			Description:        "季報空窗期，成長股與AI供應鏈有表現空間",
		},
		{
			ID:                 "dividend_season",
			Name:               "除權除息季",
			NameEN:             "Dividend Season",
			StartMonth:         5,
			StartDay:           1,
			EndMonth:           6,
			EndDay:             30,
			FavoredIndustries:  []string{"financials", "consumer"},
			AvoidedIndustries:  []string{"semiconductor", "ai_supply_chain"},
			AdjustmentFactor:   1.20,
			HistoricalAccuracy: 0.65,
			AvgMarketReturn:    0.025,
			Description:        "除權除息旺季，高股息股與金融股表現較佳；科技股相對弱勢",
		},
		{
			ID:                 "tech_peak_season",
			Name:               "科技旺季",
			NameEN:             "Technology Peak Season",
			StartMonth:         7,
			StartDay:           1,
			EndMonth:           9,
			EndDay:             15,
			FavoredIndustries:  []string{"semiconductor", "ai_supply_chain", "electronics"},
			AvoidedIndustries:  []string{"consumer"},
			AdjustmentFactor:   1.25,
			HistoricalAccuracy: 0.75,
			AvgMarketReturn:    0.085,
			Description:        "蘋果新機拉貨、AI晶片需求高峰，科技股表現最強",
		},
		{
			ID:                 "earnings_verification",
			Name:               "季報驗證期",
			NameEN:             "Earnings Verification",
			StartMonth:         9,
			StartDay:           15,
			EndMonth:           10,
			EndDay:             31,
			FavoredIndustries:  []string{},
			AvoidedIndustries:  []string{"earnings_missers"},
			AdjustmentFactor:   1.10,
			HistoricalAccuracy: 0.60,
			AvgMarketReturn:    0.020,
			Description:        "季報公布，獲利優於預期股受追捧，低於預期股遭拋售",
		},
		{
			ID:                 "year_end_rally",
			Name:               "年底作帳",
			NameEN:             "Year-End Window Dressing",
			StartMonth:         11,
			StartDay:           1,
			EndMonth:           12,
			EndDay:             31,
			FavoredIndustries:  []string{"financials"},
			AvoidedIndustries:  []string{},
			AdjustmentFactor:   1.12,
			HistoricalAccuracy: 0.58,
			AvgMarketReturn:    0.018,
			Description:        "[已棄用] 由年底法人作帳取代",
		},
		{
			ID:                 "summer_electricity",
			Name:               "夏季用電高峰",
			NameEN:             "Summer Electricity Peak",
			StartMonth:         6,
			StartDay:           1,
			EndMonth:           8,
			EndDay:             31,
			FavoredIndustries:  []string{"energy"},
			AvoidedIndustries:  []string{},
			AdjustmentFactor:   1.08,
			HistoricalAccuracy: 0.62,
			AvgMarketReturn:    0.012,
			Description:        "夏季用電高峰，能源與公用事業相對強勢；高耗電製造業成本上升",
		},
		{
			ID:                 "election_local",
			Name:               "地方選舉行情",
			NameEN:             "Local Election Rally",
			StartMonth:         11,
			StartDay:           1,
			EndMonth:           12,
			EndDay:             5,
			FavoredIndustries:  []string{"financials", "construction", "consumer"},
			AvoidedIndustries:  []string{"semiconductor", "electronics"},
			AdjustmentFactor:   1.10,
			HistoricalAccuracy: 0.60,
			AvgMarketReturn:    0.020,
			Description:        "地方選舉選前政策護盤，金融營建內需股受惠；科技股相對觀望。證據強度：地方選舉選前 40-20 日上漲機率 86%（富邦研究）",
		},
		{
			ID:                 "election_presidential",
			Name:               "總統大選行情",
			NameEN:             "Presidential Election Rally",
			StartMonth:         1,
			StartDay:           1,
			EndMonth:           2,
			EndDay:             15,
			FavoredIndustries:  []string{"financials", "consumer", "defensive"},
			AvoidedIndustries:  []string{"semiconductor", "electronics"},
			AdjustmentFactor:   1.08,
			HistoricalAccuracy: 0.55,
			AvgMarketReturn:    0.015,
			Description:        "總統大選前後政策不確定性，內需防禦型受追捧；科技出口股觀望。證據強度：總統大選前後報酬不一致（MacroMicro），國際情勢影響大於選舉本身",
		},
	}
}

// DetectCurrentPatterns returns all seasonal patterns active at the given time.
func (se *SeasonalEngine) DetectCurrentPatterns(t time.Time) []SeasonalPattern {
	var active []SeasonalPattern
	month := int(t.Month())
	day := t.Day()

	for _, p := range se.patterns {
		if se.isDateInRange(month, day, p.StartMonth, p.StartDay, p.EndMonth, p.EndDay) {
			active = append(active, p)
		}
	}
	return active
}

// GetPatternAdjustment returns the combined adjustment factor for an industry.
// When a SupplyChainGraph is set via SetLinkageGraph, upstream/downstream
// industries of favored/avoided sectors receive partial adjustments with decay.
func (se *SeasonalEngine) GetPatternAdjustment(industryID string, t time.Time) float64 {
	patterns := se.DetectCurrentPatterns(t)
	if len(patterns) == 0 {
		return 1.0
	}

	adjustment := 1.0
	for _, p := range patterns {
		// Direct match: industry is explicitly favored or avoided
		if slices.Contains(p.FavoredIndustries, industryID) {
			adjustment *= p.AdjustmentFactor
		}
		if slices.Contains(p.AvoidedIndustries, industryID) {
			adjustment *= (1.0 / p.AdjustmentFactor)
		}

		// Supply chain propagation: if our industry is upstream/downstream
		// of a favored or avoided sector, apply a partial adjustment with decay.
		if se.linkageGraph != nil {
			decay := config.GetParametersConfig().Industry.LinkageParams.Value.SeasonalDecayFactor
			// Check if our industry is upstream of a favored industry
			for _, favoredID := range p.FavoredIndustries {
				if industryID == favoredID {
					continue // already handled above
				}
				upstream := se.linkageGraph.GetUpstreamChain(favoredID, 3)
				for _, id := range upstream {
					if id == industryID {
						boost := 1.0 + (p.AdjustmentFactor-1.0)*decay
						adjustment *= boost
						break
					}
				}
				downstream := se.linkageGraph.GetDownstreamChain(favoredID, 3)
				for _, id := range downstream {
					if id == industryID {
						boost := 1.0 + (p.AdjustmentFactor-1.0)*decay
						adjustment *= boost
						break
					}
				}
			}
			// Check if our industry is upstream of an avoided industry
			// (supplying to a struggling industry = negative spillover)
			for _, avoidedID := range p.AvoidedIndustries {
				if industryID == avoidedID {
					continue // already handled above
				}
				upstream := se.linkageGraph.GetUpstreamChain(avoidedID, 3)
				for _, id := range upstream {
					if id == industryID {
						dampen := 1.0 - (1.0-1.0/p.AdjustmentFactor)*decay
						adjustment *= dampen
						break
					}
				}
			}
		}
	}

	// Narrative event overlay: active macro themes modulate seasonal adjustment.
	if se.narrativeProvider != nil {
		for _, theme := range se.narrativeProvider.ActiveThemes() {
			direction := se.detectThemeDirection(theme)
			multiplier := se.narrativeProvider.SeasonalMultiplier(theme, industryID, direction)
			adjustment *= multiplier
		}
	}

	if se.dynamicEnv != nil {
		adjustment *= se.dynamicEnv.SeasonalModulation(industryID)
	}

	if adjustment <= 0 {
		adjustment = config.GetParametersConfig().Industry.AdjustmentFloor.Value
	}
	return adjustment
}

// GetActivePatternNames returns the names of currently active patterns.
func (se *SeasonalEngine) GetActivePatternNames(t time.Time) []string {
	patterns := se.DetectCurrentPatterns(t)
	names := make([]string, len(patterns))
	for i, p := range patterns {
		names[i] = p.Name
	}
	return names
}

// daysInMonth for non-leap year (used for day-of-year conversion).
var daysInMonth = []int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// dayOfYear converts month/day to day-of-year (1..365) using a non-leap-year reference.
func dayOfYear(month, day int) int {
	doy := 0
	for m := 1; m < month; m++ {
		doy += daysInMonth[m]
	}
	return doy + day
}

// isDateInRange checks if a date falls within a seasonal range (handles year wrap).
func (se *SeasonalEngine) isDateInRange(month, day, startMonth, startDay, endMonth, endDay int) bool {
	dateDOY := dayOfYear(month, day)
	startDOY := dayOfYear(startMonth, startDay)
	endDOY := dayOfYear(endMonth, endDay)

	if startDOY <= endDOY {
		// Normal range (e.g., day 182 to day 258)
		return dateDOY >= startDOY && dateDOY <= endDOY
	}
	// Wrapped range (e.g., day 354 to day 15)
	return dateDOY >= startDOY || dateDOY <= endDOY
}

// GetPatternByID returns a specific seasonal pattern by ID.
func (se *SeasonalEngine) GetPatternByID(id string) (SeasonalPattern, bool) {
	for _, p := range se.patterns {
		if p.ID == id {
			return p, true
		}
	}
	return SeasonalPattern{}, false
}

// GetAllPatterns returns all registered seasonal patterns.
func (se *SeasonalEngine) GetAllPatterns() []SeasonalPattern {
	result := make([]SeasonalPattern, len(se.patterns))
	copy(result, se.patterns)
	return result
}

func (se *SeasonalEngine) GetPatternsForIndustry(industryID string) []SeasonalPattern {
	var result []SeasonalPattern
	for _, p := range se.patterns {
		if p.IsRelevantForIndustry(industryID) {
			result = append(result, p)
		}
	}
	return result
}

func (se *SeasonalEngine) GetIndustryImpact(patternID, industryID string) (impact string, adjustment float64) {
	pattern, ok := se.GetPatternByID(patternID)
	if !ok {
		return "neutral", 1.0
	}

	if slices.Contains(pattern.FavoredIndustries, industryID) {
		return "favored", pattern.AdjustmentFactor
	}
	if slices.Contains(pattern.AvoidedIndustries, industryID) {
		return "avoided", 1.0 / pattern.AdjustmentFactor
	}
	return "neutral", 1.0
}

// GetHistoricalAccuracy returns the average historical accuracy of active patterns.
func (se *SeasonalEngine) GetHistoricalAccuracy(t time.Time) float64 {
	patterns := se.DetectCurrentPatterns(t)
	if len(patterns) == 0 {
		return 0.0
	}

	total := 0.0
	for _, p := range patterns {
		total += p.HistoricalAccuracy
	}
	return total / float64(len(patterns))
}

// SeasonalCalendar represents a full-year calendar view of seasonal patterns.
type SeasonalCalendar struct {
	Year     int                       `json:"year"`
	Patterns []SeasonalPattern         `json:"patterns"`
	ByMonth  map[int][]SeasonalPattern `json:"by_month"`
}

// GenerateCalendar creates a calendar view for the given year.
func (se *SeasonalEngine) GenerateCalendar(year int) *SeasonalCalendar {
	calendar := &SeasonalCalendar{
		Year:    year,
		ByMonth: make(map[int][]SeasonalPattern),
	}

	for _, p := range se.patterns {
		calendar.Patterns = append(calendar.Patterns, p)

		// Add pattern to all months it spans
		for m := 1; m <= 12; m++ {
			if se.patternSpansMonth(p, m) {
				calendar.ByMonth[m] = append(calendar.ByMonth[m], p)
			}
		}
	}

	return calendar
}

// patternSpansMonth checks if a seasonal pattern spans any part of a given month.
func (se *SeasonalEngine) patternSpansMonth(p SeasonalPattern, month int) bool {
	if p.StartMonth <= p.EndMonth {
		// Normal range
		return month >= p.StartMonth && month <= p.EndMonth
	}
	// Wrapped range (e.g., Dec to Jan)
	return month >= p.StartMonth || month <= p.EndMonth
}

// String returns a human-readable summary of the seasonal pattern.
func (p SeasonalPattern) String() string {
	return fmt.Sprintf(
		"%s (%s): %02d/%02d-%02d/%02d, Accuracy: %.0f%%, Avg Return: %.1f%%",
		p.Name, p.NameEN,
		p.StartMonth, p.StartDay,
		p.EndMonth, p.EndDay,
		p.HistoricalAccuracy*100,
		p.AvgMarketReturn*100,
	)
}

// AdjustmentBreakdown decomposes the composite seasonal adjustment into
// per-layer contributions for visualization in the seasonal patterns UI.
type AdjustmentBreakdown struct {
	DirectMatch float64 `json:"direct_match"` // contribution from favored/avoided lists
	SupplyChain float64 `json:"supply_chain"` // extra from supply chain propagation
	Narrative   float64 `json:"narrative"`    // extra from narrative event overlay
	DynamicEnv  float64 `json:"dynamic_env"`  // extra from oil/DXY/BDI modulation
	Composite   float64 `json:"composite"`    // final adjustment (product of all)
}

// GetAdjustmentBreakdown returns the per-layer contribution breakdown.
func (se *SeasonalEngine) GetAdjustmentBreakdown(industryID string, t time.Time) *AdjustmentBreakdown {
	ab := &AdjustmentBreakdown{
		DirectMatch: 1.0,
		SupplyChain: 1.0,
		Narrative:   1.0,
		DynamicEnv:  1.0,
		Composite:   1.0,
	}

	patterns := se.DetectCurrentPatterns(t)
	if len(patterns) == 0 {
		return ab
	}

	// Layer 1: Direct match
	direct := 1.0
	for _, p := range patterns {
		if slices.Contains(p.FavoredIndustries, industryID) {
			direct *= p.AdjustmentFactor
		}
		if slices.Contains(p.AvoidedIndustries, industryID) {
			direct *= (1.0 / p.AdjustmentFactor)
		}
	}
	ab.DirectMatch = direct

	// Layer 2: Supply chain
	sc := 1.0
	if se.linkageGraph != nil {
		decay := config.GetParametersConfig().Industry.LinkageParams.Value.SeasonalDecayFactor
		for _, p := range patterns {
			for _, favoredID := range p.FavoredIndustries {
				if industryID == favoredID {
					continue
				}
				for _, id := range se.linkageGraph.GetUpstreamChain(favoredID, 3) {
					if id == industryID {
						sc *= 1.0 + (p.AdjustmentFactor-1.0)*decay
					}
				}
				for _, id := range se.linkageGraph.GetDownstreamChain(favoredID, 3) {
					if id == industryID {
						sc *= 1.0 + (p.AdjustmentFactor-1.0)*decay
					}
				}
			}
			for _, avoidedID := range p.AvoidedIndustries {
				if industryID == avoidedID {
					continue
				}
				for _, id := range se.linkageGraph.GetUpstreamChain(avoidedID, 3) {
					if id == industryID {
						sc *= 1.0 - (1.0-1.0/p.AdjustmentFactor)*decay
					}
				}
			}
		}
	}
	ab.SupplyChain = sc

	// Layer 3: Narrative overlay
	narr := 1.0
	if se.narrativeProvider != nil {
		for _, theme := range se.narrativeProvider.ActiveThemes() {
			direction := se.detectThemeDirection(theme)
			narr *= se.narrativeProvider.SeasonalMultiplier(theme, industryID, direction)
		}
	}
	ab.Narrative = narr

	// Layer 4: Dynamic environment
	env := 1.0
	if se.dynamicEnv != nil {
		env = se.dynamicEnv.SeasonalModulation(industryID)
	}
	ab.DynamicEnv = env

	ab.Composite = math.Max(direct*sc*narr*env, config.GetParametersConfig().Industry.AdjustmentFloor.Value)
	return ab
}

func seasonalPatternsFromConfig(cfgs []config.SeasonalPatternConfig) []SeasonalPattern {
	patterns := make([]SeasonalPattern, 0, len(cfgs))
	for _, c := range cfgs {
		patterns = append(patterns, SeasonalPattern{
			ID:                 c.ID,
			Name:               c.Name,
			NameEN:             c.NameEN,
			StartMonth:         c.StartMonth,
			StartDay:           c.StartDay,
			EndMonth:           c.EndMonth,
			EndDay:             c.EndDay,
			FavoredIndustries:  c.FavoredIndustries,
			AvoidedIndustries:  c.AvoidedIndustries,
			AdjustmentFactor:   c.AdjustmentFactor,
			HistoricalAccuracy: c.HistoricalAccuracy,
			AvgMarketReturn:    c.AvgMarketReturn,
			Description:        c.Description,
		})
	}
	return patterns
}

func NewSeasonalEngineFromConfig(cfg *config.ParametersConfig) *SeasonalEngine {
	if cfg == nil {
		return &SeasonalEngine{patterns: DefaultSeasonalPatterns()}
	}
	patterns := seasonalPatternsFromConfig(cfg.Industry.SeasonalPatterns.Value)
	return &SeasonalEngine{patterns: patterns}
}

// SetLinkageGraph enables supply-chain-aware seasonal adjustment.
// When set, GetPatternAdjustment propagates partial adjustments to
// upstream/downstream industries of favored/avoided sectors with decay.
// Passing nil disables supply-chain propagation (safe default).
func (se *SeasonalEngine) SetLinkageGraph(graph *SupplyChainGraph) {
	se.linkageGraph = graph
}

// SetNarrativeProvider enables narrative-event-aware seasonal adjustment.
// When set, GetPatternAdjustment applies additional multipliers based on
// active macro-narrative themes (e.g., oil_price_shock amplifies energy sector).
// Passing nil disables narrative overlay (safe default).
func (se *SeasonalEngine) SetNarrativeProvider(provider NarrativeSeasonalProvider) {
	se.narrativeProvider = provider
}

// SetDynamicEnv enables real-world macro-aware seasonal adjustment.
// When set, GetPatternAdjustment modulates adjustment factors based on
// current oil prices, USD strength, and other macro indicators.
// Passing nil disables dynamic environment overlay (safe default).
func (se *SeasonalEngine) SetDynamicEnv(modulator *DynamicEnvModulator) {
	se.dynamicEnv = modulator
}

// UpdateDynamicEnv pushes a fresh macro snapshot into the environment modulator
// and updates the rolling baseline. No-op if no modulator is set.
func (se *SeasonalEngine) UpdateDynamicEnv(snap marketdata.MacroDataSnapshot) {
	if se.dynamicEnv != nil {
		se.dynamicEnv.UpdateCurrent(snap)
		se.dynamicEnv.RecordSnapshot(snap)
		se.dynamicEnv.UpdateRollingBaseline()
	}
}

// detectThemeDirection returns the direction multiplier for a given narrative theme.
// +1 means the event is materializing in its "up" direction (e.g., oil prices rising),
// -1 means the "down" direction (e.g., oil prices falling).
// Default is +1 when direction cannot be determined.
// Uses actual macro data from dynamicEnv when available.
func (se *SeasonalEngine) detectThemeDirection(theme string) float64 {
	if se.dynamicEnv == nil {
		// Fallback to heuristic when no macro data available
		switch theme {
		case "JPY_carry_unwind":
			return -1.0
		case "geopolitical_risk_spike":
			return 1.0
		default:
			return 1.0
		}
	}

	oilThreshold := 0.05
	usRatesThreshold := 0.03
	jpyCarryThreshold := 0.03
	if cfg := config.GetParametersConfig(); cfg != nil {
		de := cfg.Industry.DynamicEnv.Value
		if t := de.OilPriceShockThreshold; t > 0 {
			oilThreshold = t
		}
		if t := de.UsRatesDxyThreshold; t > 0 {
			usRatesThreshold = t
		}
		if t := de.JpyCarryDxyThreshold; t > 0 {
			jpyCarryThreshold = t
		}
	}

	switch theme {
	case "oil_price_shock":
		// Use actual oil price deviation
		dev := se.dynamicEnv.OilDeviation()
		if dev > oilThreshold {
			return 1.0 // oil rising
		} else if dev < -oilThreshold {
			return -1.0 // oil falling
		}
		// Near neutral, use small positive bias (shocks are typically supply disruptions = price up)
		return 1.0

	case "US_rates_up":
		// Use US 10Y yield deviation as proxy for rate direction
		// DXY deviation is positively correlated with rates
		dxyDev := se.dynamicEnv.DXYDeviation()
		if dxyDev > usRatesThreshold {
			return 1.0 // dollar strong = rates likely rising
		} else if dxyDev < -usRatesThreshold {
			return -1.0 // dollar weak = rates likely falling
		}
		return 1.0

	case "AI_capex_surge":
		// Proxy: strong dollar + high BDI = global trade/Capex expansion
		dxyDev := se.dynamicEnv.DXYDeviation()
		bdiDev := se.dynamicEnv.BDIDeviation()
		if dxyDev > 0 && bdiDev > 0 {
			return 1.0 // strong dollar + high shipping = expansion
		} else if dxyDev < 0 && bdiDev < 0 {
			return -1.0 // weak dollar + low shipping = contraction
		}
		return 1.0

	case "JPY_carry_unwind":
		// JPY carry unwind is inherently a negative event for exporters
		// Direction is determined by JPY strength (not directly tracked, use DXY inverse)
		dxyDev := se.dynamicEnv.DXYDeviation()
		if dxyDev < -jpyCarryThreshold {
			return -1.0 // dollar weak = JPY likely strong = unwind pressure
		}
		return -1.0 // Default negative

	case "geopolitical_risk_spike":
		// Use VIX or gold deviation as proxy (if available)
		// For now, risk spikes are always +1 (escalation)
		return 1.0

	default:
		return 1.0
	}
}
