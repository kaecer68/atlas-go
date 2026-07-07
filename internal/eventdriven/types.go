package eventdriven

import "time"

// FlowPrediction is a single day's predicted capital flow direction.
type FlowPrediction struct {
	Date            time.Time `json:"date"`
	Direction       string    `json:"direction"`        // "inflow", "outflow", "neutral"
	Confidence      float64   `json:"confidence"`       // 0-1
	DrivingEvents   []string  `json:"driving_events"`   // event names driving this prediction
	PredictedForces []string  `json:"predicted_forces"` // which forces likely to move
}

// EventCalendarItem is a simplified view of an upcoming event.
type EventCalendarItem struct {
	Name               string    `json:"name"`
	EventType          string    `json:"event_type"`
	Direction          string    `json:"direction"`
	StartDate          time.Time `json:"start_date"`
	EndDate            time.Time `json:"end_date"`
	AffectedIndustries []string  `json:"affected_industries,omitempty"`
	ExpectedFlowImpact string    `json:"expected_flow_impact"` // "bullish", "bearish", "neutral"
	Confidence         float64   `json:"confidence"`
}

// ETFEstimate represents the predicted capital flow from an ETF rebalance event.
type ETFEstimate struct {
	ETFName     string  `json:"etf_name"`
	StockSymbol string  `json:"stock_symbol"`
	StockName   string  `json:"stock_name"`
	Direction   string  `json:"direction"`  // "add" or "remove"
	EstWeight   float64 `json:"est_weight"` // 0-1
	ETFAUM      float64 `json:"etf_aum"`    // in NTD billions
	EstFlow     float64 `json:"est_flow"`   // = etf_aum × est_weight (NTD millions)
}

// RevenueSurprise is a revenue-surprise event analysis.
type RevenueSurprise struct {
	StockSymbol string  `json:"stock_symbol"`
	StockName   string  `json:"stock_name"`
	Expected    float64 `json:"expected"`     // expected revenue (NTD millions)
	Actual      float64 `json:"actual"`       // actual revenue (NTD millions)
	SurprisePct float64 `json:"surprise_pct"` // (actual - expected) / expected
	FlowImpact  string  `json:"flow_impact"`  // "bullish" if >10%, "bearish" if <-10%, else "neutral"
}

// PredictionReport is the complete 5-day event-driven prediction.
type PredictionReport struct {
	GeneratedAt      time.Time           `json:"generated_at"`
	Window           string              `json:"window"` // "5-day forward"
	Predictions      []FlowPrediction    `json:"predictions"`
	ActiveEvents     []EventCalendarItem `json:"active_events"`
	ETFEstimates     []ETFEstimate       `json:"etf_estimates,omitempty"`
	RevenueSurprises []RevenueSurprise   `json:"revenue_surprises,omitempty"`
	Summary          string              `json:"summary"`
}
