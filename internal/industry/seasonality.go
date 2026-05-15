package industry

import (
	"fmt"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
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
	AvgMarketReturn    float64  `json:"avg_market_return"`   // Historical average TAIEX return
	Description        string   `json:"description"`
}

func (p SeasonalPattern) IsRelevantForIndustry(industryID string) bool {
	if slices.Contains(p.FavoredIndustries, industryID) {
		return true
	}
	return slices.Contains(p.AvoidedIndustries, industryID)
}

func (p SeasonalPattern) TypicalReturn() float64 {
	return p.AvgMarketReturn
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
}

// NarrativeSeasonalProvider supplies active macro-narrative themes that
// modulate seasonal adjustment factors based on real-world events.
type NarrativeSeasonalProvider interface {
	ActiveThemes() []string
	SeasonalMultiplier(theme string, industryID string) float64
}

// NewSeasonalEngine creates a seasonal engine using the parameter-managed seasonal patterns.
func NewSeasonalEngine() *SeasonalEngine {
	return NewSeasonalEngineFromConfig(config.GetParametersConfig())
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
			FavoredIndustries:  []string{"financials", "high_dividend", "small_cap"},
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
			FavoredIndustries:  []string{"ai_supply_chain", "growth_momentum"},
			AvoidedIndustries:  []string{"traditional", "commodity"},
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
			FavoredIndustries:  []string{"financials", "high_dividend", "consumer"},
			AvoidedIndustries:  []string{"semiconductor", "ai_supply_chain", "technology"},
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
			FavoredIndustries:  []string{"semiconductor", "ai_supply_chain", "pcb", "electronics"},
			AvoidedIndustries:  []string{"consumer", "tourism", "traditional"},
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
			FavoredIndustries:  []string{"earnings_beaters"},
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
			FavoredIndustries:  []string{"large_cap", "financials", "index_heavyweights"},
			AvoidedIndustries:  []string{"small_cap", "speculative"},
			AdjustmentFactor:   1.12,
			HistoricalAccuracy: 0.58,
			AvgMarketReturn:    0.018,
			Description:        "法人年底作帳，大型權值股與金融股相對強勢",
		},
		{
			ID:                 "summer_electricity",
			Name:               "夏季用電高峰",
			NameEN:             "Summer Electricity Peak",
			StartMonth:         6,
			StartDay:           1,
			EndMonth:           8,
			EndDay:             31,
			FavoredIndustries:  []string{"energy", "utilities", "power_equipment"},
			AvoidedIndustries:  []string{"high_power_consumption", "steel", "petrochemicals"},
			AdjustmentFactor:   1.08,
			HistoricalAccuracy: 0.62,
			AvgMarketReturn:    0.012,
			Description:        "夏季用電高峰，能源與公用事業相對強勢；高耗電製造業成本上升",
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
			multiplier := se.narrativeProvider.SeasonalMultiplier(theme, industryID)
			adjustment *= multiplier
		}
	}

	if adjustment <= 0 {
		adjustment = 0.01
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

// isDateInRange checks if a date falls within a seasonal range (handles year wrap).
func (se *SeasonalEngine) isDateInRange(month, day, startMonth, startDay, endMonth, endDay int) bool {
	dateValue := month*100 + day
	startValue := startMonth*100 + startDay
	endValue := endMonth*100 + endDay

	if startValue <= endValue {
		// Normal range (e.g., 7/1 to 9/15)
		return dateValue >= startValue && dateValue <= endValue
	}
	// Wrapped range (e.g., 12/20 to 1/15)
	return dateValue >= startValue || dateValue <= endValue
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
	return fmt.Sprintf("%s (%s): %02d/%02d-%02d/%02d, Accuracy: %.0f%%, Avg Return: %.1f%%",
		p.Name, p.NameEN,
		p.StartMonth, p.StartDay,
		p.EndMonth, p.EndDay,
		p.HistoricalAccuracy*100,
		p.AvgMarketReturn*100,
	)
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
