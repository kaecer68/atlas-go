package eventlogic

import "time"

// Direction constants for event-driven price movement expectations.
const (
	DirUp       = "up"
	DirDown     = "down"
	DirVolatile = "volatile"
)

// Status constants for rule lifecycle state.
const (
	StatusActive   = "active"
	StatusDegraded = "degraded"
	StatusExpired  = "expired"
)

// ConfidenceSource constants track the provenance of a rule's hit rate.
const (
	SourceBacktest       = "backtest"
	SourceManual         = "manual"
	SourceAutoDiscovered = "auto_discovered"
)

// Condition represents a single programmable condition within an event rule.
// Conditions are ANDed together when evaluating whether a rule fires.
type Condition struct {
	Field    string  `json:"field"`
	Operator string  `json:"operator"` // "gt", "lt", "gte", "lte", "eq"
	Value    float64 `json:"value"`
}

// EventRule represents a self-improving event-cause-effect prediction rule.
// Rules track their own hit rate and degrade/expire based on performance.
type EventRule struct {
	ID               string      `json:"id"`
	Pattern          string      `json:"pattern"`
	Conditions       []Condition `json:"conditions"`
	AffectedSectors  []string    `json:"affected_sectors"`
	AffectedStocks   []string    `json:"affected_stocks"`
	Direction        string      `json:"direction"`
	HitRate          float64     `json:"hit_rate"`
	TotalTests       int         `json:"total_tests"`
	TotalHits        int         `json:"total_hits"`
	ConfidenceSource string      `json:"confidence_source"`
	Status           string      `json:"status"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// NewEventRule creates a new EventRule with sensible defaults.
// The rule starts in active status with manual confidence and a neutral 0.5 hit rate.
func NewEventRule(id, pattern string, conditions []Condition, sectors []string, direction string) *EventRule {
	now := time.Now()
	return &EventRule{
		ID:               id,
		Pattern:          pattern,
		Conditions:       conditions,
		AffectedSectors:  sectors,
		AffectedStocks:   nil,
		Direction:        direction,
		HitRate:          0.5,
		TotalTests:       0,
		TotalHits:        0,
		ConfidenceSource: SourceManual,
		Status:           StatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}
