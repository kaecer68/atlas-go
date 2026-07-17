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
	narrativeModels   []ModelView
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
// assessment gate. Predict must not consume QualityScore while the assessment
// is calibrating or degraded.
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
// the predictor. ListModels is called once at wiring time; the
// predictor caches the snapshot and filters by per-day active themes.
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

// SetNarrativeProvider caches a snapshot of Darwinian-evolved models.
// nil provider clears the cache (reverts to event-only predictions).
func (p *Predictor) SetNarrativeProvider(np NarrativeModelProvider) {
	if np == nil {
		p.narrativeModels = nil
		return
	}
	p.narrativeModels = np.ListModels()
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

	// E07 deliberately has no uncalibrated overall score (spec §9.5).
	// Keep the legacy QualityScore as a compatibility fallback, but only
	// behind the canonical assessment gate; calibrating/degraded/error paths
	// contribute zero tilt and never invoke QualityScore.
	cfScore := 0.0
	if assessment, err := p.capitalFlow.LatestAssessment(context.Background()); err == nil && assessment.EligibleForAutomation() {
		cfScore = p.capitalFlow.QualityScore()
	}
	predictions := make([]FlowPrediction, 5)
	for i := 0; i < 5; i++ {
		day := now.AddDate(0, 0, i+1)
		dir, conf, drivers := p.predictDay(day, timeline, cfScore)
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
		Summary:          buildPredictionSummary(predictions, active, cfScore),
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
func (p *Predictor) predictDay(day time.Time, timeline []industry.CalendarEvent, cfScore float64) (dir string, conf float64, drivers []string) {
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

	// Tilt direction by capital quality score
	bullishWeight += cfScore * 0.3
	bearishWeight -= cfScore * 0.3

	narrativeThemes := activeTriggerThemesForDay(p.narrativeRegistry)
	themeSet := make(map[string]struct{}, len(narrativeThemes))
	for _, t := range narrativeThemes {
		themeSet[t] = struct{}{}
	}
	narrativeTilt := computeNarrativeTilt(p.narrativeModels, themeSet)
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

func buildPredictionSummary(predictions []FlowPrediction, active []industry.CalendarEvent, cfScore float64) string {
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

	// Events driving
	if len(active) > 0 {
		names := make([]string, 0, len(active))
		for _, e := range active {
			names = append(names, e.Name)
		}
		parts = append(parts, fmt.Sprintf("關鍵事件：%s", strings.Join(names, "、")))
	}

	// Quality score
	if cfScore > 0.5 {
		parts = append(parts, "當前資金品質偏多")
	} else if cfScore < -0.5 {
		parts = append(parts, "當前資金品質偏空")
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
