package risktest

import "github.com/kaecer68/atlas-go/internal/domain"

// Config holds external, user-supplied scenario parameters loaded from
// a JSON config file. All fields are optional; if not provided, defaults are used.
type Config struct {
	Scenarios map[string]ScenarioConfig `json:"scenarios"`
}

// ScenarioConfig holds per-scenario overrides.
type ScenarioConfig struct {
	PriceDeclinePct      float64            `json:"price_decline_pct"`
	VolatilityBoost      float64            `json:"volatility_boost"`
	MacroRiskLevel       string             `json:"macro_risk_level"`
	PrimaryFlow          string             `json:"primary_flow"`
	CyclicalSymbols      []string           `json:"cyclical_symbols"`
	DefensiveSymbols     []string           `json:"defensive_symbols"`
	PortfolioWeights     map[string]float64 `json:"portfolio_weights"`
	VaRConfidence        float64            `json:"var_confidence"`
	ForeignOutflowProb   float64            `json:"foreign_outflow_prob"`
	StructuralOverride   bool               `json:"structural_override"`
	RecessionIndustryPct float64            `json:"recession_industry_pct"`
	PortfolioRiskScore   float64            `json:"portfolio_risk_score"`
}

// Result is the JSON-serializable output of a single stress test scenario.
type Result struct {
	Scenario  string  `json:"scenario"`
	Timestamp string  `json:"timestamp"`
	Results   Results `json:"results"`
	Passed    bool    `json:"passed"`
	Error     string  `json:"error,omitempty"`
}

// Results holds all sub-results for a given scenario.
type Results struct {
	DrawdownAction    string               `json:"drawdown_action"`
	DrawdownPct       float64              `json:"drawdown_percentage"`
	MaxExposure       float64              `json:"max_exposure"`
	Breakdown         []Step               `json:"breakdown"`
	VaRBreach         bool                 `json:"var_breach"`
	RiskGateHalt      bool                 `json:"risk_gate_halt"`
	WeightAdjustments []WeightAdjustment   `json:"weight_adjustments"`
	RiskSnapshot      *domain.RiskSnapshot `json:"risk_snapshot,omitempty"`
	Rationale         string               `json:"rationale"`
}

// Step represents one step in the drawdown breakdown chain.
type Step struct {
	Source     string  `json:"source"`
	Rule       string  `json:"rule"`
	Action     string  `json:"action"`
	Confidence float64 `json:"confidence"`
}

// WeightAdjustment records a single portfolio weight adjustment.
type WeightAdjustment struct {
	Symbol string  `json:"symbol"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Reason string  `json:"reason"`
}

// AllResults is the aggregate output used for "all" scenarios.
type AllResults struct {
	Timestamp string   `json:"timestamp"`
	Scenarios []Result `json:"scenarios"`
	Summary   Summary  `json:"summary"`
}

// Summary aggregates results across all scenarios.
type Summary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}
