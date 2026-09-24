package industry

import (
	"fmt"
	"maps"
	"math"
	"slices"
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
//
// ParametersConfig.Industry.CompositeCard is the config authority for these
// values (issue #1944 Batch 2, new inert item): the field ships populated in
// configs/parameters.json, but this function used to return hardcoded copies,
// so editing composite_card in config had no effect at all. Each field is
// overridden only when the config value is present/non-zero (merge convention),
// so a partial or legacy config keeps the defaults below. The shipped config is
// value-identical to these defaults, so consuming it is behaviour-neutral today.
func defaultCardConfig() CardConfig {
	cfg := CardConfig{
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
	return applyCompositeCardConfig(cfg)
}

// applyCompositeCardConfig overlays ParametersConfig.Industry.CompositeCard onto
// cfg. Fields that are unset in config (empty map / zero value) keep the
// hardcoded default, so an all-zero or legacy config cannot collapse the
// throttle window to [0, 0] or drop every layer weight.
func applyCompositeCardConfig(cfg CardConfig) CardConfig {
	p := config.GetParametersConfig()
	if p == nil {
		return cfg
	}
	cc := p.Industry.CompositeCard.Value
	if len(cc.LayerWeights) > 0 {
		cfg.LayerWeights = maps.Clone(cc.LayerWeights)
	}
	if len(cc.SentimentThresholds) > 0 {
		thresholds := make(map[string]SentimentBounds, len(cc.SentimentThresholds))
		for label, bounds := range cc.SentimentThresholds {
			thresholds[label] = SentimentBounds{Min: bounds.Min, Max: bounds.Max}
		}
		cfg.SentimentThresholds = thresholds
	}
	if cc.ClampMin != 0 {
		cfg.ClampMin = cc.ClampMin
	}
	if cc.ClampMax != 0 {
		cfg.ClampMax = cc.ClampMax
	}
	return cfg
}

// EffectiveCardConfig returns the CardConfig the card builder will actually use
// (config overlay + calibration redistribution). Exported so diagnostics — e.g.
// the cycle_calibrate background task — can report the applied weights instead
// of a discarded local copy (issue #1944 Batch 2, Q6 I2).
func EffectiveCardConfig() CardConfig {
	return resolveCardConfig()
}

// CalibrationApplied reports whether calibration evidence actually
// redistributed the layer weights, i.e. whether the effective config differs
// from the config/default baseline. Callers that expose an outward "calibrated"
// flag must derive it from this instead of from "the metrics map was non-nil"
// (issue #1944 Batch 2, E-item): a tracker with samples but an unusable clamp
// window, or with samples for layers that are not funded, changes nothing.
func CalibrationApplied() bool {
	base := defaultCardConfig()
	effective := resolveCardConfig()
	if len(base.LayerWeights) != len(effective.LayerWeights) {
		return true
	}
	for layer, weight := range base.LayerWeights {
		if math.Abs(effective.LayerWeights[layer]-weight) > 1e-12 {
			return true
		}
	}
	return false
}

// resolveCardConfig returns the CardConfig used to build a card.
//
// When a calibration tracker has been injected (see
// SetGlobalCycleCalibration; production wires it from
// monitoring.IndustryService.SetCycleCalibration) its *observed* per-layer
// accuracy redistributes the layer weights. Until #1944 Batch 1 the injected
// tracker was ignored and defaults were returned unconditionally, while
// IndustryService.SetCycleCalibration documented the opposite — a silent
// failure: the API exposed calibration metrics that never reached the card.
//
// Two rules keep the consumption honest and non-destabilizing:
//  1. Evidence gate — with no layer metrics (no recorded outcomes) the
//     defaults are returned unchanged. CalibrateWeights normalises to sum=1,
//     so consuming an empty tracker would silently rescale the composite
//     coefficient by 1/0.85 even though nothing was learned.
//  2. Residual preservation — the funded weight budget stays at the default
//     sum (0.85; the 0.15 residual is reserved for future layers), so
//     calibration redistributes *within* the funded layers and cannot change
//     the composite coefficient scale.
func resolveCardConfig() CardConfig {
	cfg := defaultCardConfig()
	cal := GetGlobalCycleCalibration()
	if cal == nil {
		return cfg
	}
	return applyCycleCalibration(cfg, cal)
}

// applyCycleCalibration returns cfg with layer weights redistributed by the
// tracker's observed per-layer accuracy, preserving the funded weight sum.
func applyCycleCalibration(cfg CardConfig, cal *CycleCalibration) CardConfig {
	if len(cal.GetMetrics()) == 0 {
		// No observed layer accuracy yet: consume nothing (rule 1).
		return cfg
	}
	baseSum := weightSum(cfg.LayerWeights)
	if baseSum <= 0 {
		return cfg
	}
	calibrated := cal.CalibrateWeights(cfg.LayerWeights)
	calibratedSum := weightSum(calibrated)
	if calibratedSum <= 0 {
		return cfg
	}
	scale := baseSum / calibratedSum
	adjusted := make(map[string]float64, len(cfg.LayerWeights))
	largestLayer, largestWeight := "", math.Inf(-1)
	for layer, w := range cfg.LayerWeights {
		// Layers the tracker has no opinion about keep their default share.
		calibratedWeight, known := calibrated[layer]
		if !known {
			calibratedWeight = w
		}
		adjusted[layer] = calibratedWeight * scale
		if adjusted[layer] > largestWeight {
			largestLayer, largestWeight = layer, adjusted[layer]
		}
	}
	// Preserve the funded sum exactly: CalibrateWeights normalises through
	// normalizeWeights, which rounds each layer to 4 dp, so the rescaled sum
	// can drift by up to N*5e-5. Absorb that residue into the largest funded
	// layer (a relative nudge < 1e-3) instead of leaving it to accumulate
	// across layers.
	if largestLayer != "" {
		adjusted[largestLayer] += baseSum - weightSum(adjusted)
	}
	cfg.LayerWeights = adjusted
	return cfg
}

// weightSum sums weights in a deterministic (sorted-key) order. Floating-point
// addition is order-dependent and Go map iteration is randomized, so an
// unordered sum of the same values can differ in the last bits — which would
// make the balanced rescale in applyCycleCalibration non-deterministic.
func weightSum(weights map[string]float64) float64 {
	var sum float64
	for _, layer := range slices.Sorted(maps.Keys(weights)) {
		sum += weights[layer]
	}
	return sum
}

// globalCycleCalibration is the singleton calibration tracker injected at
// application bootstrap. If nil, resolveCardConfig returns defaults.
// Guarded by globalCycleCalibrationMu: injection happens on the bootstrap
// goroutine while cards are built from request/tick goroutines.
var (
	globalCycleCalibrationMu sync.RWMutex
	globalCycleCalibration   *CycleCalibration
)

// SetGlobalCycleCalibration injects the calibration tracker into the
// cycle status card builder. The tracker's own metrics are internally
// synchronized; this mutex guards the injected pointer.
func SetGlobalCycleCalibration(cal *CycleCalibration) {
	globalCycleCalibrationMu.Lock()
	defer globalCycleCalibrationMu.Unlock()
	globalCycleCalibration = cal
}

// GetGlobalCycleCalibration returns the current calibration tracker or nil.
func GetGlobalCycleCalibration() *CycleCalibration {
	globalCycleCalibrationMu.RLock()
	defer globalCycleCalibrationMu.RUnlock()
	return globalCycleCalibration
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
		SiliconIndicators: nil, // nil → frontend renders "尚無矽循環指標明細"
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
		SiliconIndicators: nil, // nil → frontend renders "尚無矽循環指標明細"
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
	// Prefer latest indicators (updated every macro ingestion) over history.
	if latest, ok := b.siliconTracker.GetLatestIndicators(); ok {
		card.SiliconIndicators = &SiliconIndicatorSnapshot{
			TSMCMonthlyRevenueYoY:          latest.TSMCMonthlyRevenueYoY,
			GlobalSemiconductorBillingsYoY: latest.GlobalSemiconductorBillingsYoY,
			DRAMSpotPriceTrend:             latest.DRAMSpotPriceTrend,
			TaiwanSemiconductorIndexMA:     latest.TaiwanSemiconductorIndexMA,
			TSMCCapexGuidance:              latest.TSMCCapexGuidance,
			PhiladelphiaSOXIndexYoY:        latest.PhiladelphiaSOXIndexYoY,
		}
	} else if len(history) > 0 {
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
		return 1.0
	}
	return b.eventCalendar.GetCompositeEventSentiment(now)
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
