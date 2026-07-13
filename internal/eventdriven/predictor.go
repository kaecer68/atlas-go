package eventdriven

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// Predictor maps upcoming events to capital flow predictions.
type Predictor struct {
	calendar        *industry.EventCalendar
	capitalFlow     CapitalFlowProvider
	narrativeModels []ModelView
}

// CapitalFlowProvider provides the current capital quality score.
type CapitalFlowProvider interface {
	QualityScore() float64
	QualityLabel() string
}

// staticCF is a simple static implementation for testing.
type staticCF struct {
	score float64
	label string
}

func (s *staticCF) QualityScore() float64 { return s.score }
func (s *staticCF) QualityLabel() string  { return s.label }

// ModelView is a flat projection of narrative.InvestmentModel.
// Defined here (not imported from internal/narrative) to avoid a
// package dependency cycle. main.go's narrative adapter maps the
// narrative type to ModelView.
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

// NewPredictor creates an event-driven flow predictor.
func NewPredictor(cal *industry.EventCalendar) *Predictor {
	return &Predictor{
		calendar:    cal,
		capitalFlow: &staticCF{score: 0, label: "neutral"},
	}
}

// SetCapitalFlow sets the capital flow provider for scoring.
func (p *Predictor) SetCapitalFlow(cf CapitalFlowProvider) {
	p.capitalFlow = cf
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
			Confidence:         e.BaseWeight,
			Backfilled:         e.Backfilled,
			CrossSourceStatus:  e.CrossSourceStatus,
		})
	}

	// Generate day-by-day predictions over 5 days
	cfScore := p.capitalFlow.QualityScore()
	predictions := make([]FlowPrediction, 5)
	for i := 0; i < 5; i++ {
		day := now.AddDate(0, 0, i+1)
		dir, conf, drivers := p.predictDay(day, timeline, cfScore)
		predictions[i] = FlowPrediction{
			Date:            day,
			Direction:       dir,
			Confidence:      conf,
			DrivingEvents:   drivers,
			PredictedForces: forcesForDirection(drivers),
		}
	}

	return PredictionReport{
		GeneratedAt:      now,
		Window:           "5-day forward",
		Predictions:      predictions,
		ActiveEvents:     items,
		ETFEstimates:     p.buildETFEstimates(timeline),
		RevenueSurprises: p.buildRevenueSurprises(timeline),
		Summary:          buildPredictionSummary(predictions, active, cfScore),
	}
}

// predictDay computes the predicted flow for a specific day.
func (p *Predictor) predictDay(day time.Time, timeline []industry.CalendarEvent, cfScore float64) (dir string, conf float64, drivers []string) {
	var bullishWeight, bearishWeight float64
	var driverNames []string
	var activeThemes []string

	for _, e := range timeline {
		if day.After(e.EndDate) || day.Before(e.StartDate) {
			continue
		}
		w := e.BaseWeight
		// Stage 2.2c: backfilled events carry 0.7x weight.
		if e.Backfilled {
			w *= 0.7
		}
		switch e.Direction {
		case "bullish":
			bullishWeight += w
			driverNames = append(driverNames, e.Name)
		case "bearish":
			bearishWeight += w
			driverNames = append(driverNames, e.Name)
		case "mixed":
			bullishWeight += w * 0.3
			bearishWeight += w * 0.3
		}
		activeThemes = append(activeThemes, eventTypeToThemes(e.EventType)...)
	}

	for _, m := range p.narrativeModels {
		if !themeMatchesAny(m.ActiveThemes, activeThemes) {
			continue
		}
		switch m.Direction {
		case "bullish":
			bullishWeight += m.Weight
		case "bearish":
			bearishWeight += m.Weight
		}
	}

	// Tilt direction by capital quality score
	bullishWeight += cfScore * 0.3
	bearishWeight -= cfScore * 0.3

	net := bullishWeight - bearishWeight
	switch {
	case net > 0.3:
		dir = "inflow"
	case net < -0.3:
		dir = "outflow"
	default:
		dir = "neutral"
	}

	// Confidence: sigmoid of net weight scaled by number of drivers
	conf = sigmoid(math.Abs(net) * float64(len(driverNames)+1))
	conf = math.Round(conf*100) / 100

	return dir, conf, driverNames
}

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
			strings.Contains(dl, "營收"):
			forceSet["foreign"] = true
		case strings.Contains(dl, "etf"),
			strings.Contains(dl, "換股"),
			strings.Contains(dl, "成分股"),
			strings.Contains(dl, "taiwan50"):
			forceSet["institutional"] = true
		case strings.Contains(dl, "投信"),
			strings.Contains(dl, "基金"),
			strings.Contains(dl, "法說"):
			forceSet["institutional"] = true
			forceSet["foreign"] = true
		case strings.Contains(dl, "做帳"),
			strings.Contains(dl, "季底"),
			strings.Contains(dl, "window_dressing"):
			forceSet["institutional"] = true
			forceSet["dealer"] = true
		case strings.Contains(dl, "融資"),
			strings.Contains(dl, "散戶"):
			forceSet["retail"] = true
		case strings.Contains(dl, "結算"),
			strings.Contains(dl, "settlement"):
			forceSet["dealer"] = true
		case strings.Contains(dl, "配息"),
			strings.Contains(dl, "除權息"),
			strings.Contains(dl, "股利"),
			strings.Contains(dl, "dividend"):
			forceSet["retail"] = true
			forceSet["institutional"] = true
		case strings.Contains(dl, "股東會"),
			strings.Contains(dl, "shareholder"):
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

func guessETFName(eventName string) string {
	l := strings.ToLower(eventName)
	switch {
	case strings.Contains(l, "0050") || strings.Contains(l, "臺灣50") || strings.Contains(l, "台灣50"):
		return "0050 臺灣50"
	case strings.Contains(l, "0056") || strings.Contains(l, "高股息"):
		return "0056 高股息"
	case strings.Contains(l, "00878") || strings.Contains(l, "永續"):
		return "00878 永續高股息"
	default:
		return ""
	}
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
