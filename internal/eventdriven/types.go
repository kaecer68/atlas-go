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

// PredictionReport is the complete 5-day event-driven prediction.
type PredictionReport struct {
	GeneratedAt  time.Time           `json:"generated_at"`
	Window       string              `json:"window"` // "5-day forward"
	Predictions  []FlowPrediction    `json:"predictions"`
	ActiveEvents []EventCalendarItem `json:"active_events"`
	Summary      string              `json:"summary"`
}
