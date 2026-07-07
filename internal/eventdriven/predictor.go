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
	calendar    *industry.EventCalendar
	capitalFlow CapitalFlowProvider
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
		GeneratedAt:  now,
		Window:       "5-day forward",
		Predictions:  predictions,
		ActiveEvents: items,
		Summary:      buildPredictionSummary(predictions, active, cfScore),
	}
}

// predictDay computes the predicted flow for a specific day.
func (p *Predictor) predictDay(day time.Time, timeline []industry.CalendarEvent, cfScore float64) (dir string, conf float64, drivers []string) {
	var bullishWeight, bearishWeight float64
	var driverNames []string

	for _, e := range timeline {
		if day.After(e.EndDate) || day.Before(e.StartDate) {
			continue
		}
		w := e.BaseWeight
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
