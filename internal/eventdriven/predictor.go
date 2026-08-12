package eventdriven

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// Predictor maps upcoming events to capital flow predictions.
type Predictor struct {
	calendar          *industry.EventCalendar
	capitalFlow       CapitalFlowProvider
	narrativeProvider NarrativeModelProvider
	scanStore         DetectorScanStore
	narrativeRegistry *narrative.DetectorRegistry
	sectorPredictor   *SectorPredictor
}

// ScanResult is a minimal projection of a detector scan row, defined
// locally to avoid a package dependency cycle — ledger imports narrative,
// narrative imports eventdriven (type_theme_mapping.go), so any import of
// ledger from eventdriven would create narrative→ledger→eventdriven→narrative.
type ScanResult struct {
	Theme      string
	Severity   string
	Confidence float64
	DetectedAt time.Time
}

// DetectorScanStore is the interface consumed by the predictor for
// run-time detected theme data.
type DetectorScanStore interface {
	LoadRecentScans(ctx context.Context, limit int) ([]ScanResult, error)
}

// CapitalFlowProvider exposes the legacy quality view plus the structured E07
// assessment. Predict uses QualityScore as a baseline drift signal, weighted by
// calibration status and decayed over the 5-day window. It is no longer gated
// to zero while calibrating; instead the predictor expresses higher
// uncertainty when the assessment is not eligible.
type CapitalFlowProvider interface {
	QualityScore() float64
	QualityLabel() string
	LatestAssessment(ctx context.Context) (capitalflow.CapitalFlowAssessment, error)
}

// ModelView is a flat projection of narrative.InvestmentModel.
// Defined here (not imported from internal/narrative) to avoid a
// package dependency cycle. The narrative adapter in main.go maps
// narrative.InvestmentModel to this struct.
type ModelView struct {
	ID           string
	Name         string
	Weight       float64
	Direction    string
	ActiveThemes []string
}

// NarrativeModelProvider exposes Darwinian-evolved narrative models to
// the predictor. ListModels is re-queried on every Predict so hourly
// UpdateModelWeights changes flow into the narrative tilt instead of a
// one-time wiring snapshot (H1 remediation).
type NarrativeModelProvider interface {
	ListModels() []ModelView
}

// staticCF is a simple static implementation for testing.
type staticCF struct {
	score float64
	label string
}

func (s *staticCF) QualityScore() float64 { return s.score }
func (s *staticCF) QualityLabel() string  { return s.label }
func (s *staticCF) LatestAssessment(context.Context) (capitalflow.CapitalFlowAssessment, error) {
	return capitalflow.CapitalFlowAssessment{
		CalibrationStatus: capitalflow.CalibrationCalibrating,
	}, nil
}

// NewPredictor wires the default narrative.DetectorRegistry so narrative
// tilt matches all 24 templates (PR-FIX-04, fixes G-06); override via SetNarrativeRegistry.
func NewPredictor(cal *industry.EventCalendar) *Predictor {
	return &Predictor{
		calendar:          cal,
		capitalFlow:       &staticCF{score: 0, label: "neutral"},
		narrativeRegistry: narrative.NewDefaultDetectorRegistry(),
	}
}

// SetCapitalFlow sets the capital flow provider for scoring.
func (p *Predictor) SetCapitalFlow(cf CapitalFlowProvider) {
	p.capitalFlow = cf
}

// SetScanStore injects a detector scan store. When nil (the default), the
// predictor continues using the static EventTypeToTriggerThemes mapping.
func (p *Predictor) SetScanStore(s DetectorScanStore) {
	p.scanStore = s
}

// SetNarrativeProvider wires a live narrative model provider whose
// ListModels is re-queried on every Predict, so Darwinian weight updates
// (hourly narrative_weight_update scheduler) reach the narrative tilt.
// nil provider disables narrative tilt (reverts to event-only predictions).
func (p *Predictor) SetNarrativeProvider(np NarrativeModelProvider) {
	p.narrativeProvider = np
}

// SetNarrativeRegistry injects the DetectorRegistry whose registered
// themes define the "active trigger theme universe" used by narrative
// tilt matching. nil registry falls back to event-only matching.
func (p *Predictor) SetNarrativeRegistry(reg *narrative.DetectorRegistry) {
	p.narrativeRegistry = reg
}

// SetSectorPredictor injects a SectorPredictor for per-sector direction
// predictions. nil disables sector predictions (default).
func (p *Predictor) SetSectorPredictor(sp *SectorPredictor) {
	p.sectorPredictor = sp
}

// Predict generates a 5-day capital flow prediction report.
func (p *Predictor) Predict(now time.Time) PredictionReport {
	// Get upcoming events
	timeline := p.calendar.GetEventTimeline(now, 7)
	active := p.calendar.DetectActiveEvents(now)

	// Build calendar items
	var items []EventCalendarItem
	for _, e := range timeline {
		items = append(items, EventCalendarItem{
			Name:               e.Name,
			EventType:          e.EventType,
			Direction:          e.Direction,
			StartDate:          e.StartDate,
			EndDate:            e.EndDate,
			AffectedIndustries: e.AffectedIndustries,
			ExpectedFlowImpact: expectedFlow(e.EventType),
			Confidence:         effectiveConfidence(e),
		})
	}

	// Use QualityScore as the current capital-flow baseline (baseline drift /
	// momentum). The legacy composite is sufficient for display/explanation;
	// calibration status controls how much weight we give it and caps
	// confidence when the score is not yet eligible for automation.
	cfScore := 0.0
	cfStatus := capitalflow.CalibrationCalibrating
	if p.capitalFlow != nil {
		if assessment, err := p.capitalFlow.LatestAssessment(context.Background()); err == nil {
			cfStatus = assessment.CalibrationStatus
		}
		cfScore = p.capitalFlow.QualityScore()
	}
	baseline := scaleQualityScoreToBaseline(cfScore)

	predictions := make([]FlowPrediction, 5)
	for i := range 5 {
		day := now.AddDate(0, 0, i+1)
		dir, conf, drivers := p.predictDay(day, timeline, baseline, cfStatus, i)
		predictions[i] = FlowPrediction{
			Date:            day,
			Direction:       dir,
			Confidence:      conf,
			Distribution:    computeDistribution(dir, conf),
			DrivingEvents:   drivers,
			PredictedForces: forcesForDirection(drivers),
		}
	}

	report := PredictionReport{
		GeneratedAt:      now,
		Window:           "5-day forward",
		Predictions:      predictions,
		ActiveEvents:     items,
		ETFEstimates:     p.buildETFEstimates(timeline),
		RevenueSurprises: p.buildRevenueSurprises(timeline),
		Summary:          buildPredictionSummary(predictions, active, baseline, cfStatus),
	}
	if p.sectorPredictor != nil {
		report.SectorPredictions = p.sectorPredictor.Predict(predictions, items)
	}
	if report.ActiveEvents == nil {
		report.ActiveEvents = []EventCalendarItem{}
	}
	if report.ETFEstimates == nil {
		report.ETFEstimates = []ETFEstimate{}
	}
	if report.RevenueSurprises == nil {
		report.RevenueSurprises = []RevenueSurprise{}
	}
	if report.SectorPredictions == nil {
		report.SectorPredictions = []SectorDayPrediction{}
	}
	return report
}

// computeNarrativeTilt sums (weight × direction_sign) across narrative
// models whose ActiveThemes intersect with the supplied theme set.
// Returns 0 when no model matches or the theme set is empty. The
// caller (predictDay) computes the theme set from the registered
// DetectorRegistry so this function stays a pure mapping over
// (models, themeSet).
func computeNarrativeTilt(models []ModelView, themeSet map[string]struct{}) float64 {
	var tilt float64
	if len(themeSet) == 0 {
		return 0
	}
	for _, m := range models {
		var sign float64
		switch m.Direction {
		case "bullish":
			sign = 1.0
		case "bearish":
			sign = -1.0
		}
		if sign == 0 {
			continue
		}
		if !themeIntersects(m.ActiveThemes, themeSet) {
			continue
		}
		tilt += sign * m.Weight
	}
	return tilt
}

// activeTriggerThemesForDay returns the live set of registered detector
// themes. With a wired DetectorRegistry this expands the universe to
// the full 24-template set (replacing the legacy 5-theme subset that
// came from eventTypeToTriggerThemesTable); a nil registry returns nil
// so the caller treats the day as event-only.
func activeTriggerThemesForDay(registry *narrative.DetectorRegistry) []string {
	if registry == nil {
		return nil
	}
	return registry.Themes()
}

// themeIntersects reports whether any element of themes appears in set.
func themeIntersects(themes []string, set map[string]struct{}) bool {
	for _, t := range themes {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}

// predictDay computes the predicted flow for a specific day.
//
// baseline is a direction-and-strength score in [-0.8, 0.8] derived from the
// current observed capital flow (legacy QualityScore). It is blended with
// calendar-event weights using a day-decayed, calibration-discounted weight.
func (p *Predictor) predictDay(day time.Time, timeline []industry.CalendarEvent, baseline float64, cfStatus string, dayIndex int) (dir string, conf float64, drivers []string) {
	var bullishWeight, bearishWeight float64
	drivers = make([]string, 0)

	for _, e := range timeline {
		if day.After(e.EndDate) || day.Before(e.StartDate) {
			continue
		}
		w := e.BaseWeight
		if e.Backfilled {
			w *= backfillDiscountFactor
		}
		switch e.Direction {
		case "bullish":
			bullishWeight += w
			drivers = append(drivers, e.Name)
		case "bearish":
			bearishWeight += w
			drivers = append(drivers, e.Name)
		case "mixed":
			bullishWeight += w * 0.3
			bearishWeight += w * 0.3
		}
	}

	// Blend in the current capital-flow baseline with decay and calibration
	// uncertainty. Near-term days keep more of today's flow signal;
	// far-term days are driven more by events.
	if baseline != 0 {
		w := baselineWeightForDay(dayIndex, cfStatus)
		if baseline > 0 {
			bullishWeight += baseline * w
		} else {
			bearishWeight -= baseline * w
		}
		drivers = append([]string{formatBaselineDriver(cfStatus)}, drivers...)
	}

	narrativeThemes := activeTriggerThemesForDay(p.narrativeRegistry)
	themeSet := make(map[string]struct{}, len(narrativeThemes))
	for _, t := range narrativeThemes {
		themeSet[t] = struct{}{}
	}
	var narrativeTilt float64
	if p.narrativeProvider != nil {
		narrativeTilt = computeNarrativeTilt(p.narrativeProvider.ListModels(), themeSet)
	}
	// Apply scan-theme tilt (W4 consumption). Returns 0 when scanStore
	// is nil or no recent scans pass recency/confidence/severity gates.
	scanTilt, scanDrivers := p.applyScanThemes(context.Background(), day)
	drivers = append(drivers, scanDrivers...)

	net := bullishWeight - bearishWeight + narrativeTilt + scanTilt
	switch {
	case net > 0.3:
		dir = "inflow"
	case net < -0.3:
		dir = "outflow"
	default:
		dir = "neutral"
	}

	// Confidence: sigmoid of net weight scaled by number of drivers
	conf = sigmoid(math.Abs(net) * float64(len(drivers)+1))
	conf = capConfidenceByCalibration(conf, cfStatus)
	conf = math.Round(conf*100) / 100

	return dir, conf, drivers
}

// expectedFlow maps event types to their expected capital flow impact.
func expectedFlow(eventType string) string {
	switch eventType {
	case string(industry.EventMSCIRebalance),
		string(industry.EventTaiwan50Rebalance):
		return "bullish"
	case string(industry.EventMonthlyRevenue):
		return "bullish"
	case string(industry.EventExDividend):
		return "mixed"
	case string(industry.EventFinancialReport):
		return "bullish"
	case string(industry.EventFuturesSettlement),
		string(industry.EventWindowDressing):
		return "mixed"
	default:
		return "neutral"
	}
}

// forcesForDirection maps direction and drivers to likely capital forces.
func forcesForDirection(drivers []string) []string {
	forceSet := make(map[string]bool)
	for _, d := range drivers {
		dl := strings.ToLower(d)
		switch {
		case strings.Contains(dl, "msci"),
			strings.Contains(dl, "外資"),
			strings.Contains(dl, "營收"),
			strings.Contains(dl, "revenue"):
			forceSet["foreign"] = true
		case strings.Contains(dl, "etf"),
			strings.Contains(dl, "換股"),
			strings.Contains(dl, "成分股"),
			strings.Contains(dl, "taiwan50"):
			forceSet["institutional"] = true
		case strings.Contains(dl, "投信"),
			strings.Contains(dl, "基金"):
			forceSet["institutional"] = true
		case strings.Contains(dl, "做帳"),
			strings.Contains(dl, "季底"),
			strings.Contains(dl, "window_dressing"):
			forceSet["institutional"] = true
			forceSet["dealer"] = true
		case strings.Contains(dl, "融資"),
			strings.Contains(dl, "散戶"):
			forceSet["retail"] = true
		case strings.Contains(dl, "法說會"),
			strings.Contains(dl, "investor conf"),
			strings.Contains(dl, "investor_conf"):
			forceSet["foreign"] = true
			forceSet["institutional"] = true
		case strings.Contains(dl, "期貨"),
			strings.Contains(dl, "futures"):
			forceSet["dealer"] = true
		case strings.Contains(dl, "配息"),
			strings.Contains(dl, "除權息"),
			strings.Contains(dl, "ex_dividend"),
			strings.Contains(dl, "dividend"):
			forceSet["retail"] = true
			forceSet["institutional"] = true
		case strings.Contains(dl, "股東會"),
			strings.Contains(dl, "shareholders"):
			forceSet["institutional"] = true
		}
	}
	forces := make([]string, 0, len(forceSet))
	for f := range forceSet {
		forces = append(forces, f)
	}
	sort.Strings(forces)
	return forces
}

const backfillDiscountFactor = 0.7

// Baseline drift parameters for blending the current observed capital flow
// (legacy QualityScore) into the event-driven prediction.
//
// QualityScore is typically a z-score-like composite in roughly [-3, 3].
// We scale it to the same event-weight range so it competes with calendar
// event weights on a comparable scale.
const (
	baselineWeightNearDay       = 0.7  // day 1 weight for current-flow signal
	baselineWeightFarDay        = 0.2  // day 5 weight for current-flow signal
	baselineDecayFactor         = 0.75 // per-day decay
	baselineQualityScaleDivisor = 1.5  // scales QualityScore into [-0.8, 0.8]
	calibratingBaselineDiscount = 0.5  // weight discount when not yet eligible
	degradedBaselineDiscount    = 0.2  // weight discount when degraded/error
	calibratingConfidenceCap    = 0.6  // cap confidence when calibrating
	degradedConfidenceCap       = 0.55 // cap confidence when degraded
)

// scaleQualityScoreToBaseline maps the legacy QualityScore to a directional
// baseline in [-0.8, 0.8] so it can be blended with event weights.
func scaleQualityScoreToBaseline(qs float64) float64 {
	if qs == 0 {
		return 0
	}
	scaled := qs / baselineQualityScaleDivisor
	if scaled > 1 {
		scaled = 1
	} else if scaled < -1 {
		scaled = -1
	}
	return scaled * 0.8
}

// baselineWeightForDay returns the effective weight of the current capital-flow
// baseline for a given forecast day. Near-term days keep more of today's flow
// signal; far-term days are driven more by calendar events. Calibration status
// discounts the weight to reflect data trustworthiness.
func baselineWeightForDay(dayIndex int, cfStatus string) float64 {
	w := baselineWeightFarDay + (baselineWeightNearDay-baselineWeightFarDay)*math.Pow(baselineDecayFactor, float64(dayIndex))
	switch cfStatus {
	case capitalflow.CalibrationEligible:
		return w
	case capitalflow.CalibrationCalibrating:
		return w * calibratingBaselineDiscount
	default: // degraded or empty/unknown
		return w * degradedBaselineDiscount
	}
}

// capConfidenceByCalibration reduces peak confidence when the capital-flow
// baseline is not fully trusted.
func capConfidenceByCalibration(conf float64, cfStatus string) float64 {
	switch cfStatus {
	case capitalflow.CalibrationEligible:
		return conf
	case capitalflow.CalibrationCalibrating:
		if conf > calibratingConfidenceCap {
			return calibratingConfidenceCap
		}
		return conf
	default:
		if conf > degradedConfidenceCap {
			return degradedConfidenceCap
		}
		return conf
	}
}

// formatBaselineDriver returns a human-readable driver label for the current
// capital-flow baseline so the UI can surface it alongside event drivers.
func formatBaselineDriver(cfStatus string) string {
	if cfStatus == capitalflow.CalibrationCalibrating {
		return "當前資金流向（校準中）"
	}
	return "當前資金流向"
}

func effectiveConfidence(e industry.CalendarEvent) float64 {
	if e.Backfilled {
		return e.BaseWeight * backfillDiscountFactor
	}
	return e.BaseWeight
}

func sigmoid(x float64) float64 {
	if x < -10 {
		return 0
	}
	if x > 10 {
		return 1
	}
	return 1 / (1 + math.Exp(-x))
}

func buildPredictionSummary(predictions []FlowPrediction, active []industry.CalendarEvent, baseline float64, cfStatus string) string {
	var parts []string

	// Count direction
	inflow, outflow, neutral := 0, 0, 0
	for _, p := range predictions {
		switch p.Direction {
		case "inflow":
			inflow++
		case "outflow":
			outflow++
		case "neutral":
			neutral++
		}
	}

	// Overall direction
	if inflow > outflow+1 {
		parts = append(parts, "未來 5 天資金偏流入")
	} else if outflow > inflow+1 {
		parts = append(parts, "未來 5 天資金偏流出")
	} else {
		parts = append(parts, "未來 5 天資金流向分歧")
	}

	// Current capital-flow baseline
	if baseline != 0 {
		if baseline > 0 {
			parts = append(parts, "當前資金品質偏多")
		} else {
			parts = append(parts, "當前資金品質偏空")
		}
	}
	if cfStatus == capitalflow.CalibrationCalibrating {
		parts = append(parts, "資金流評估處於校準中，預測不確定性較高")
	} else if cfStatus != capitalflow.CalibrationEligible && baseline != 0 {
		parts = append(parts, "資金流評估狀態異常，預測參考性下降")
	}

	// Events driving
	if len(active) > 0 {
		names := make([]string, 0, len(active))
		for _, e := range active {
			names = append(names, e.Name)
		}
		parts = append(parts, fmt.Sprintf("關鍵事件：%s", strings.Join(names, "、")))
	}

	return strings.Join(parts, "。") + "。"
}

// buildETFEstimates generates ETF flow estimates from rebalance events.
// Formula: est_flow = etf_aum (NTD billions) × est_weight → NTD millions.
func (p *Predictor) buildETFEstimates(timeline []industry.CalendarEvent) []ETFEstimate {
	var estimates []ETFEstimate
	for _, e := range timeline {
		switch e.EventType {
		case string(industry.EventTaiwan50Rebalance):
			estimates = append(estimates, p.etfRebalanceEstimates("0050 臺灣50", e)...)
		default:
			if strings.Contains(strings.ToLower(e.EventType), "etf") ||
				strings.Contains(e.Name, "0056") ||
				strings.Contains(e.Name, "00878") {
				etfName := guessETFName(e.Name)
				if etfName != "" {
					estimates = append(estimates, p.etfRebalanceEstimates(etfName, e)...)
				}
			}
		}
	}
	return estimates
}

// computeDistribution turns a direction/confidence pair into a probability
// mass over the three possible capital-flow directions. It preserves the
// chosen direction as the dominant mass and splits the remaining probability
// between the other two outcomes.
func computeDistribution(dir string, conf float64) PredictionDistribution {
	conf = math.Max(0, math.Min(1, conf))
	switch dir {
	case "inflow":
		out := (1 - conf) * 0.5
		return PredictionDistribution{
			Inflow:  roundProb(conf),
			Outflow: roundProb(out),
			Neutral: roundProb(1 - conf - out),
		}
	case "outflow":
		in := (1 - conf) * 0.5
		return PredictionDistribution{
			Inflow:  roundProb(in),
			Outflow: roundProb(conf),
			Neutral: roundProb(1 - conf - in),
		}
	default:
		rem := (1 - conf) * 0.5
		return PredictionDistribution{
			Inflow:  roundProb(rem),
			Outflow: roundProb(rem),
			Neutral: roundProb(conf),
		}
	}
}

func roundProb(v float64) float64 {
	return math.Round(v*100) / 100
}

// etfRebalanceEstimates returns estimated flow for a known ETF rebalance event.
func (p *Predictor) etfRebalanceEstimates(etfName string, _ industry.CalendarEvent) []ETFEstimate {
	type etfProfile struct {
		name string
		aum  float64
	}
	symbols := map[string]etfProfile{
		"0050":        {name: "0050 臺灣50", aum: 380},
		"0056":        {name: "0056 高股息", aum: 320},
		"00878":       {name: "00878 永續高股息", aum: 280},
		"0050 臺灣50":   {name: "0050 臺灣50", aum: 380},
		"0056 高股息":    {name: "0056 高股息", aum: 320},
		"00878 永續高股息": {name: "00878 永續高股息", aum: 280},
	}

	pf, ok := symbols[etfName]
	if !ok {
		return nil
	}

	exampleStocks := []struct {
		symbol, name string
		weight       float64
		direction    string
	}{
		{"2330", "台積電", 0.12, "add"},
		{"2454", "聯發科", 0.06, "add"},
		{"2317", "鴻海", 0.04, "add"},
	}

	var out []ETFEstimate
	for _, s := range exampleStocks {
		flow := pf.aum * s.weight * 1000
		out = append(out, ETFEstimate{
			ETFName:     pf.name,
			StockSymbol: s.symbol,
			StockName:   s.name,
			Direction:   s.direction,
			EstWeight:   s.weight,
			ETFAUM:      pf.aum,
			EstFlow:     flow,
		})
	}
	return out
}

var etfKeywordTable = []struct {
	keywords []string
	name     string
}{
	{keywords: []string{"00878", "永續"}, name: "00878 永續高股息"},
	{keywords: []string{"0056", "高股息"}, name: "0056 高股息"},
	{keywords: []string{"0050", "臺灣50", "台灣50"}, name: "0050 臺灣50"},
}

func guessETFName(eventName string) string {
	l := strings.ToLower(eventName)
	for _, etf := range etfKeywordTable {
		for _, kw := range etf.keywords {
			if strings.Contains(l, kw) {
				return etf.name
			}
		}
	}
	return ""
}

// buildRevenueSurprises evaluates revenue events for >10% surprise.
func (p *Predictor) buildRevenueSurprises(timeline []industry.CalendarEvent) []RevenueSurprise {
	var surprises []RevenueSurprise
	for _, e := range timeline {
		if e.EventType != string(industry.EventMonthlyRevenue) {
			continue
		}
		rs := p.evaluateRevenueSurprise(e)
		if rs != nil {
			surprises = append(surprises, *rs)
		}
	}
	return surprises
}

func (p *Predictor) evaluateRevenueSurprise(e industry.CalendarEvent) *RevenueSurprise {
	expected := 100.0
	actual := expected * (1 + e.SentimentAdjustment)
	if e.BaseWeight > 0.5 {
		actual = expected * (1 + e.SentimentAdjustment*1.5)
	}
	surprisePct := (actual - expected) / expected
	impact := "neutral"
	if surprisePct > 0.1 {
		impact = "bullish"
	} else if surprisePct < -0.1 {
		impact = "bearish"
	}
	symbol := ""
	if len(e.AffectedIndustries) > 0 {
		symbol = e.AffectedIndustries[0]
	}
	return &RevenueSurprise{
		StockSymbol: symbol,
		StockName:   e.Name,
		Expected:    expected,
		Actual:      actual,
		SurprisePct: surprisePct,
		FlowImpact:  impact,
	}
}

// eventTypeToThemes maps a Taiwan calendar event type string to its
// set of legacy calendar-specific theme names used by older
// InvestmentModel.ActiveThemes. Stage 5 PR#3 introduced a parallel
// type_theme_mapping.go with the new 24-template trigger theme system;
// this legacy mapping is preserved for backward compatibility with
// existing tests and any external callers still using calendar theme
// names. New callers should prefer EventTypeToTriggerThemes() so that
// calendar events map to the 24-template trigger themes that drive
// narrative models.
func eventTypeToThemes(eventType string) []string {
	switch eventType {
	case string(industry.EventMSCIRebalance), string(industry.EventTaiwan50Rebalance):
		return []string{"msci_rebalance", "tw50_rebalance", "index_rebalance"}
	case string(industry.EventMonthlyRevenue):
		return []string{"monthly_revenue", "earnings_surprise"}
	case string(industry.EventFinancialReport):
		return []string{"financial_report", "earnings_surprise"}
	case string(industry.EventExDividend), string(industry.EventDividendPayout):
		return []string{"ex_dividend", "dividend_season"}
	case string(industry.EventFuturesSettlement):
		return []string{"futures_settlement"}
	case string(industry.EventWindowDressing):
		return []string{"window_dressing"}
	case string(industry.EventShareholderMeeting):
		return []string{"shareholders_meeting"}
	case string(industry.EventInvestorConf):
		return []string{"investor_conference"}
	default:
		return nil
	}
}

// themeMatchesAny returns true if any theme in modelThemes appears in
// activeThemes. Used to decide whether a given InvestmentModel's
// ActiveThemes is fired by the calendar event themes. Empty inputs on
// either side short-circuit to false. Migrated from develop's
// Stage 3 narrative wiring; kept identical to preserve test coverage.
func themeMatchesAny(modelThemes, activeThemes []string) bool {
	if len(modelThemes) == 0 || len(activeThemes) == 0 {
		return false
	}
	active := make(map[string]struct{}, len(activeThemes))
	for _, t := range activeThemes {
		active[t] = struct{}{}
	}
	for _, t := range modelThemes {
		if _, ok := active[t]; ok {
			return true
		}
	}
	return false
}
