package risk

import "time"

// RiskPhase represents the checkpoint in the trade lifecycle where a risk decision is made.
type RiskPhase string

const (
	PhasePreTrade  RiskPhase = "pre_trade"
	PhaseInTrade   RiskPhase = "in_trade"
	PhasePostTrade RiskPhase = "post_trade"
)

// Verdict represents the risk gate ruling on an order or position.
type Verdict string

const (
	VerdictAllow     Verdict = "ALLOW"
	VerdictReduce    Verdict = "REDUCE"
	VerdictBlock     Verdict = "BLOCK"
	VerdictHalt      Verdict = "HALT"
	VerdictAlertOnly Verdict = "ALERT_ONLY"
)

// ActionType categorizes the concrete corrective action taken by the risk gate.
type ActionType string

const (
	ActionSell      ActionType = "SELL"
	ActionReduce    ActionType = "REDUCE"
	ActionFreeze    ActionType = "FREEZE"
	ActionLiquidate ActionType = "LIQUIDATE"
	ActionNotify    ActionType = "NOTIFY"
)

// RiskDecision is the unified risk gate verdict for a single checkpoint evaluation.
type RiskDecision struct {
	Phase    RiskPhase    `json:"phase"`
	Verdict  Verdict      `json:"verdict"`
	Reason   string       `json:"reason"`
	Action   RiskAction   `json:"action"`
	Details  []RuleResult `json:"details"`
	Mode     string       `json:"mode"`
	Symbol   string       `json:"symbol,omitempty"`
	Recorded time.Time    `json:"recorded_at"`
}

// RiskAction describes the concrete corrective measure prescribed by the risk decision.
type RiskAction struct {
	Type        ActionType `json:"type"`
	TargetPct   float64    `json:"target_pct"`
	Symbols     []string   `json:"symbols,omitempty"`
	Sectors     []string   `json:"sectors,omitempty"`
	Description string     `json:"description"`
}

// RuleResult captures the outcome of a single risk rule evaluation.
type RuleResult struct {
	RuleName     string  `json:"rule_name"`
	Passed       bool    `json:"passed"`
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	Severity     string  `json:"severity"`
	Message      string  `json:"message,omitempty"`
}

// OrderIntent represents a proposed order before execution, used as input to PreTradeCheck.
type OrderIntent struct {
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"`
	Quantity   int     `json:"quantity"`
	Price      float64 `json:"price"`
	Notional   float64 `json:"notional"`
	AgentID    string  `json:"agent_id"`
	Sector     string  `json:"sector"`
	Conviction int     `json:"conviction"`
}

// PortfolioState is a snapshot of the current portfolio used by risk checks.
type PortfolioState struct {
	TotalValue     float64            `json:"total_value"`
	Cash           float64            `json:"cash"`
	SectorExposure map[string]float64 `json:"sector_exposure"`
	Positions      map[string]float64 `json:"positions"`
	Var95          float64            `json:"var_95"`
	MaxDrawdown    float64            `json:"max_drawdown"`
}
