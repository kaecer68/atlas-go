package industry

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"

)

// SiliconIndicatorSnapshot captures key silicon cycle indicators at a point in time,
// mirrored from SiliconIndicators in silicon_cycle.go.
type SiliconIndicatorSnapshot struct {
	TSMCMonthlyRevenueYoY          float64 `json:"tsmc_monthly_revenue_yoy"`
	GlobalSemiconductorBillingsYoY float64 `json:"global_semiconductor_billings_yoy"`
	DRAMSpotPriceTrend             float64 `json:"dram_spot_price_trend"`
	TaiwanSemiconductorIndexMA     float64 `json:"taiwan_semiconductor_index_ma"`
	TSMCCapexGuidance              float64 `json:"tsmc_capex_guidance"`
	PhiladelphiaSOXIndexYoY        float64 `json:"philadelphia_sox_index_yoy"`
}

// SeasonalPatternSnapshot is a lightweight view of an active seasonal pattern.
type SeasonalPatternSnapshot struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	AdjustmentFactor float64 `json:"adjustment_factor"`
}

// LayerAdjustment records one layer's contribution to the composite coefficient,
// providing a complete audit trail for the decision chain frontend.
type LayerAdjustment struct {
	Layer        string  `json:"layer"`
	RawValue     float64 `json:"raw_value"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
	Reason       string  `json:"reason"`
}

// CardConfig holds tunable parameters for the CycleStatusCard builder.
// These defaults are used until integration with ParametersConfig.Industry
// is completed (future: add a CompositeCard ParameterMetadata to IndustryParameters).
type CardConfig struct {
	LayerWeights        map[string]float64         `json:"layer_weights"`
	SentimentThresholds map[string]SentimentBounds `json:"sentiment_thresholds"`
	ClampMin            float64                    `json:"clamp_min"`
	ClampMax            float64                    `json:"clamp_max"`
}

// SentimentBounds defines the [min, max) range for a sentiment label.
type SentimentBounds struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// defaultCardConfig returns sensible defaults.
// Weights sum to 0.85, leaving 0.15 residual for future layers.
func defaultCardConfig() CardConfig {
	return CardConfig{
		LayerWeights: map[string]float64{
			"silicon":        0.25,
			"business_cycle": 0.20,
			"seasonal":       0.15,
			"events":         0.15,
			"supply_chain":   0.10,
		},
		SentimentThresholds: map[string]SentimentBounds{
			"強烈看多": {Min: 1.10, Max: math.Inf(1)},
			"偏多":   {Min: 1.05, Max: 1.10},
			"中性":   {Min: 0.95, Max: 1.05},
			"偏空":   {Min: 0.90, Max: 0.95},
			"強烈看空": {Min: 0.00, Max: 0.90},
		},
		ClampMin: 0.80,
		ClampMax: 1.20,
	}
}

// globalCycleCalibration holds the active calibration tracker, set by the
// application bootstrap. If nil, resolveCardConfig returns defaults.
var globalCycleCalibration *CycleCalibration

// SetGlobalCycleCalibration injects the calibration tracker into the
// cycle status card builder. Thread-safe: the calibration instance
// itself handles internal synchronization.
func SetGlobalCycleCalibration(cal *CycleCalibration) {
	globalCycleCalibration = cal
}

// GetGlobalCycleCalibration returns the current calibration tracker or nil.
func GetGlobalCycleCalibration() *CycleCalibration {
	return globalCycleCalibration
}

func resolveCardConfig() CardConfig {
	// Prefer ParametersConfig when available; fall back to hardcoded defaults.
	cfg := defaultCardConfig()
	if params := config.GetParametersConfig(); params != nil {
		cc := params.Industry.CompositeCard.Value
		thresholds := make(map[string]SentimentBounds, len(cc.SentimentThresholds))
		for k, v := range cc.SentimentThresholds {
			thresholds[k] = SentimentBounds{Min: v.Min, Max: v.Max}
		}
		cfg = CardConfig{
			LayerWeights:        cc.LayerWeights,
			SentimentThresholds: thresholds,
			ClampMin:            cc.ClampMin,
			ClampMax:            cc.ClampMax,
		}
	}

	// Overlay runtime calibration on top of the base config (from whichever source).
	if globalCycleCalibration != nil && globalCycleCalibration.GetOutcomeCount() > 0 {
		calibrated := globalCycleCalibration.CalibrateWeights(cfg.LayerWeights)
		cfg.LayerWeights = calibrated
	}

	return cfg
}

// CycleStatusCard is the daily composite sentiment card that combines all
// four cycle sub-systems into a single coefficient (0.8–1.2) with full
// audit trail for the decision chain frontend.
type CycleStatusCard struct {
	Date        time.Time `json:"date"`
	GeneratedAt time.Time `json:"generated_at"`

	SiliconPhase      int                       `json:"silicon_phase"`
	SiliconPhaseName  string                    `json:"silicon_phase_name"`
	SiliconScore      float64                   `json:"silicon_score"`
	SiliconIndicators *SiliconIndicatorSnapshot `json:"silicon_indicators"`

	BusinessCycle   string  `json:"business_cycle"`
	InventoryCycle  string  `json:"inventory_cycle"`
	CapexCycle      string  `json:"capex_cycle"`
	CycleConfidence float64 `json:"cycle_confidence"`
	IsFavorable     bool    `json:"is_favorable"`

	ActivePatterns     []SeasonalPatternSnapshot `json:"active_patterns"`
	SeasonalAdjustment float64                   `json:"seasonal_adjustment"`

	ActiveEvents   []CalendarEvent `json:"active_events"`
	EventSentiment float64         `json:"event_sentiment"`

	SupplyChainSignal float64 `json:"supply_chain_signal"`

	CompositeCoefficient float64           `json:"composite_coefficient"`
	SentimentLabel       string            `json:"sentiment_label"`
	Breakdown            []LayerAdjustment `json:"breakdown"`
}

// CycleStatusCardBuilder constructs a CycleStatusCard by combining the
// four sub-systems: silicon cycle tracker, business cycle tracker, seasonal
// patterns engine, and Taiwan calendar events, plus a supply chain signal.
type CycleStatusCardBuilder struct {
	siliconTracker  *SiliconCycleTracker
	cycleTracker    *CycleTracker
	seasonalEngine  *SeasonalEngine
	eventCalendar   *EventCalendar
	linkageAnalyzer *LinkageAnalyzer

	mu sync.RWMutex
}

// NewCycleStatusCardBuilder creates a builder wired to all four sub-systems.
// Any parameter may be nil; missing sub-systems contribute a neutral signal.
func NewCycleStatusCardBuilder(
	silicon *SiliconCycleTracker,
	cycle *CycleTracker,
	seasonal *SeasonalEngine,
	events *EventCalendar,
	linkage *LinkageAnalyzer,
) *CycleStatusCardBuilder {
	return &CycleStatusCardBuilder{
		siliconTracker:  silicon,
		cycleTracker:    cycle,
		seasonalEngine:  seasonal,
		eventCalendar:   events,
		linkageAnalyzer: linkage,
	}
}

// BuildCard produces a single-industry CycleStatusCard by evaluating all
// active sub-systems at the given time.
func (b *CycleStatusCardBuilder) BuildCard(now time.Time, industryID string) (*CycleStatusCard, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	card := &CycleStatusCard{
		Date:              now,
		GeneratedAt:       time.Now(),
		SiliconIndicators: &SiliconIndicatorSnapshot{},
		ActivePatterns:    []SeasonalPatternSnapshot{},
		ActiveEvents:      []CalendarEvent{},
		Breakdown:         []LayerAdjustment{},
	}

	cfg := resolveCardConfig()

	card.SiliconScore = b.resolveSiliconLayer(card)
	cycleConfidence := b.resolveCycleLayer(card, industryID)
	seasonalAdj := b.resolveSeasonalLayer(card, industryID, now)
	eventSentiment := b.resolveEventLayer(card, now)
	supplySignal := b.computeSupplyChainSignal(industryID)
	card.SupplyChainSignal = supplySignal

	card.CompositeCoefficient = computeCompositeCoefficient(
		card.SiliconScore, cycleConfidence, seasonalAdj, eventSentiment, supplySignal, cfg,
	)
	card.SentimentLabel = computeSentimentLabel(card.CompositeCoefficient, cfg)

	card.Breakdown = []LayerAdjustment{
		buildAdj("silicon", card.SiliconScore, cfg.LayerWeights["silicon"],
			fmt.Sprintf("silicon phase=%s score=%.3f", card.SiliconPhaseName, card.SiliconScore)),
		buildAdj("business_cycle", cycleConfidence, cfg.LayerWeights["business_cycle"],
			fmt.Sprintf("phase=%s confidence=%.3f", card.BusinessCycle, cycleConfidence)),
		buildAdj("seasonal", seasonalAdj, cfg.LayerWeights["seasonal"],
			fmt.Sprintf("%d active patterns", len(card.ActivePatterns))),
		buildAdj("events", eventSentiment, cfg.LayerWeights["events"],
			fmt.Sprintf("%d active events", len(card.ActiveEvents))),
		buildAdj("supply_chain", supplySignal, cfg.LayerWeights["supply_chain"],
			"upstream-downstream momentum"),
	}

	return card, nil
}

// BuildCompositeCard produces a market-wide CycleStatusCard aggregated
// across all known industries.
func (b *CycleStatusCardBuilder) BuildCompositeCard(now time.Time) (*CycleStatusCard, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	card := &CycleStatusCard{
		Date:              now,
		GeneratedAt:       time.Now(),
		SiliconIndicators: &SiliconIndicatorSnapshot{},
		ActivePatterns:    []SeasonalPatternSnapshot{},
		ActiveEvents:      []CalendarEvent{},
		Breakdown:         []LayerAdjustment{},
	}

	cfg := resolveCardConfig()

	card.SiliconScore = b.resolveSiliconLayer(card)

	if b.cycleTracker != nil {
		allPositions := b.cycleTracker.GetAllPositions()
		var totalConfidence float64
		var count int
		var totalSupplySignal float64
		for industryID, pos := range allPositions {
			if count == 0 {
				card.BusinessCycle = string(pos.BusinessCycle)
				card.InventoryCycle = string(pos.InventoryCycle)
				card.CapexCycle = string(pos.CapexCycle)
			}
			totalConfidence += pos.Confidence
			count++
			totalSupplySignal += b.computeSupplyChainSignal(industryID)
		}
		if count > 0 {
			card.CycleConfidence = totalConfidence / float64(count)
			card.SupplyChainSignal = totalSupplySignal / float64(count)
		}
	}

	seasonalAdj := b.resolveSeasonalLayer(card, "", now)
	eventSentiment := b.resolveEventLayer(card, now)

	card.CompositeCoefficient = computeCompositeCoefficient(
		card.SiliconScore, card.CycleConfidence, seasonalAdj, eventSentiment, card.SupplyChainSignal, cfg,
	)
	card.SentimentLabel = computeSentimentLabel(card.CompositeCoefficient, cfg)

	card.Breakdown = []LayerAdjustment{
		buildAdj("silicon", card.SiliconScore, cfg.LayerWeights["silicon"],
			fmt.Sprintf("silicon phase=%s", card.SiliconPhaseName)),
		buildAdj("business_cycle", card.CycleConfidence, cfg.LayerWeights["business_cycle"],
			"avg across industries"),
		buildAdj("seasonal", seasonalAdj, cfg.LayerWeights["seasonal"],
			fmt.Sprintf("%d active patterns", len(card.ActivePatterns))),
		buildAdj("events", eventSentiment, cfg.LayerWeights["events"],
			fmt.Sprintf("%d active events", len(card.ActiveEvents))),
		buildAdj("supply_chain", card.SupplyChainSignal, cfg.LayerWeights["supply_chain"],
			"avg upstream-downstream momentum"),
	}

	return card, nil
}

func (b *CycleStatusCardBuilder) resolveSiliconLayer(card *CycleStatusCard) float64 {
	if b.siliconTracker == nil {
		card.SiliconPhaseName = "n/a"
		card.SiliconScore = 0.5
		return 0.5
	}

	phase := b.siliconTracker.GetCurrentPhase()
	card.SiliconPhase = int(phase)
	card.SiliconPhaseName = GetPhaseName(phase)
	card.SiliconScore = GetPhaseScore(phase)

	// Populate indicators: prefer the most recent transition event for full
	// context, but fall back to the latest indicators (stored on every
	// DetectPhase call) so the frontend always shows live values.
	history := b.siliconTracker.GetHistory()
	if len(history) > 0 {
		latest := history[len(history)-1]
		card.SiliconIndicators = &SiliconIndicatorSnapshot{
			TSMCMonthlyRevenueYoY:          latest.Indicators.TSMCMonthlyRevenueYoY,
			GlobalSemiconductorBillingsYoY: latest.Indicators.GlobalSemiconductorBillingsYoY,
			DRAMSpotPriceTrend:             latest.Indicators.DRAMSpotPriceTrend,
			TaiwanSemiconductorIndexMA:     latest.Indicators.TaiwanSemiconductorIndexMA,
			TSMCCapexGuidance:              latest.Indicators.TSMCCapexGuidance,
			PhiladelphiaSOXIndexYoY:        latest.Indicators.PhiladelphiaSOXIndexYoY,
		}
	} else if latestInd, ok := b.siliconTracker.GetLatestIndicators(); ok {
		card.SiliconIndicators = &SiliconIndicatorSnapshot{
			TSMCMonthlyRevenueYoY:          latestInd.TSMCMonthlyRevenueYoY,
			GlobalSemiconductorBillingsYoY: latestInd.GlobalSemiconductorBillingsYoY,
			DRAMSpotPriceTrend:             latestInd.DRAMSpotPriceTrend,
			TaiwanSemiconductorIndexMA:     latestInd.TaiwanSemiconductorIndexMA,
			TSMCCapexGuidance:              latestInd.TSMCCapexGuidance,
			PhiladelphiaSOXIndexYoY:        latestInd.PhiladelphiaSOXIndexYoY,
		}
	}

	return card.SiliconScore
}

func (b *CycleStatusCardBuilder) resolveCycleLayer(card *CycleStatusCard, industryID string) float64 {
	if b.cycleTracker == nil {
		card.CycleConfidence = 0.5
		return 0.5
	}
	pos, ok := b.cycleTracker.GetPosition(industryID)
	if !ok {
		card.CycleConfidence = 0.5
		return 0.5
	}
	card.BusinessCycle = string(pos.BusinessCycle)
	card.InventoryCycle = string(pos.InventoryCycle)
	card.CapexCycle = string(pos.CapexCycle)
	card.CycleConfidence = pos.Confidence
	card.IsFavorable = pos.IsFavorable()
	return pos.Confidence
}

func (b *CycleStatusCardBuilder) resolveSeasonalLayer(card *CycleStatusCard, industryID string, now time.Time) float64 {
	if b.seasonalEngine == nil {
		card.SeasonalAdjustment = 1.0
		return 1.0
	}
	patterns := b.seasonalEngine.DetectCurrentPatterns(now)
	card.ActivePatterns = make([]SeasonalPatternSnapshot, 0, len(patterns))

	for _, p := range patterns {
		if industryID != "" && !p.IsRelevantForIndustry(industryID) {
			continue
		}
		card.ActivePatterns = append(card.ActivePatterns, SeasonalPatternSnapshot{
			ID:               p.ID,
			Name:             p.Name,
			AdjustmentFactor: p.AdjustmentFactor,
		})
	}

	adj := 1.0
	if industryID != "" {
		adj = b.seasonalEngine.GetPatternAdjustment(industryID, now)
	} else {
		var total float64
		for _, p := range patterns {
			total += p.AdjustmentFactor
		}
		if len(patterns) > 0 {
			adj = total / float64(len(patterns))
		}
	}
	card.SeasonalAdjustment = adj
	return adj
}

func (b *CycleStatusCardBuilder) resolveEventLayer(card *CycleStatusCard, now time.Time) float64 {
	if b.eventCalendar == nil {
		card.EventSentiment = 1.0
		return 1.0
	}
	events := b.eventCalendar.DetectActiveEvents(now)
	card.ActiveEvents = events
	if len(events) == 0 {
		card.EventSentiment = 1.0
		return 1.0
	}
	// ST-3 fix: actually assign the computed sentiment to the card field.
	// Previously this value was only returned for composite coefficient calculation
	// but card.EventSentiment remained at zero value (0.0).
	sentiment := b.eventCalendar.GetCompositeEventSentiment(now)
	card.EventSentiment = sentiment
	return sentiment
}

func computeCompositeCoefficient(
	siliconScore, cycleConfidence, seasonalAdj, eventSentiment, supplySignal float64,
	cfg CardConfig,
) float64 {
	w := cfg.LayerWeights
	base := 1.0

	base += (siliconScore - 0.5) * w["silicon"]
	base += (cycleConfidence - 0.5) * w["business_cycle"]
	base += (seasonalAdj - 1.0) * w["seasonal"]
	base += (eventSentiment - 1.0) * w["events"]
	base += supplySignal * w["supply_chain"]

	return clamp(base, cfg.ClampMin, cfg.ClampMax)
}

func computeSentimentLabel(coefficient float64, cfg CardConfig) string {
	for label, bounds := range cfg.SentimentThresholds {
		if coefficient >= bounds.Min && coefficient < bounds.Max {
			return label
		}
	}
	return "中性"
}

func (b *CycleStatusCardBuilder) computeSupplyChainSignal(industryID string) float64 {
	if b.linkageAnalyzer == nil {
		return 0.0
	}

	graph := b.linkageAnalyzer.GetSupplyChainGraph()
	cm := b.linkageAnalyzer.GetCorrelationMatrix()

	upstream := graph.GetUpstream(industryID)
	downstream := graph.GetDownstream(industryID)

	if len(upstream) == 0 && len(downstream) == 0 {
		return 0.0
	}

	var upstreamSignal, upstreamWeight float64
	for _, upID := range upstream {
		corr, ok := cm.GetCorrelation(industryID, upID)
		if !ok {
			continue
		}
		signalVal := math.Abs(corr) * b.cyclePositionScore(upID)
		upstreamSignal += signalVal * math.Abs(corr)
		upstreamWeight += math.Abs(corr)
	}

	var downstreamSignal, downstreamWeight float64
	for _, downID := range downstream {
		corr, ok := cm.GetCorrelation(industryID, downID)
		if !ok {
			continue
		}
		signalVal := math.Abs(corr) * b.cyclePositionScore(downID)
		downstreamSignal += signalVal * math.Abs(corr)
		downstreamWeight += math.Abs(corr)
	}

	upNorm := 0.0
	if upstreamWeight > 0 {
		upNorm = upstreamSignal / upstreamWeight
	}
	downNorm := 0.0
	if downstreamWeight > 0 {
		downNorm = downstreamSignal / downstreamWeight
	}

	return (downNorm - upNorm) * 0.5
}

func (b *CycleStatusCardBuilder) cyclePositionScore(industryID string) float64 {
	if b.cycleTracker == nil {
		return 0.0
	}
	return b.cycleTracker.GetContinuousPhaseScore(industryID)
}

func buildAdj(layer string, rawValue, weight float64, reason string) LayerAdjustment {
	var contribution float64
	switch layer {
	case "silicon", "business_cycle":
		contribution = (rawValue - 0.5) * weight
	case "seasonal", "events":
		contribution = (rawValue - 1.0) * weight
	default:
		contribution = rawValue * weight
	}
	return LayerAdjustment{
		Layer:        layer,
		RawValue:     rawValue,
		Weight:       weight,
		Contribution: math.Round(contribution*10000) / 10000,
		Reason:       reason,
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return math.Round(v*10000) / 10000
}
