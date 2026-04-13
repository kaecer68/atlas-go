package domain

import "time"

type Regime string

const (
	RegimeRiskOn  Regime = "RISK_ON"
	RegimeRiskOff Regime = "RISK_OFF"
	RegimeNeutral Regime = "NEUTRAL"
)

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type Quote struct {
	Symbol     string
	Last       float64
	Open       float64
	High       float64
	Low        float64
	Volume     int64
	Market     string
	AsOf       time.Time
	IsTradable bool
	Source     string
}

type Recommendation struct {
	Agent           string
	Skill           string
	Symbol          string
	Side            Side
	Conviction      int
	Reason          string
	ReasoningChain  []string `json:"reasoning_chain,omitempty"`
	SupportingEvents []string `json:"supporting_events,omitempty"`
}

type Position struct {
	Symbol        string
	Quantity      int
	AverageCost   float64
	CurrentPrice  float64
	MarketValue   float64
	UnrealizedPnL float64
}

type Order struct {
	Symbol   string
	Side     Side
	Quantity int
	Price    float64
	Reason   string
}

type SimulationConstraints struct {
	StartingCash                float64
	MaxPositionWeight           float64
	MaxOpenPositions            int
	MinTradableVolume           int64
	MinRecommendationConviction int
	RequireCROPass              bool
	TransactionCostBPS          float64
	SlippageBPS                 float64
	ReserveCashFraction         float64
	StopLossPct                 float64 // sell when price drops below avgCost*(1+StopLossPct)
	TakeProfitPct               float64 // sell when price rises above avgCost*(1+TakeProfitPct)
}

type ExecutionPolicy struct {
	ConvictionFloor int
	RequireCROPass  bool
}

type SimulationResult struct {
	Regime         Regime
	Orders         []Order
	Positions      []Position
	EndingCash     float64
	PortfolioValue float64
	GuardOutcomes  []GuardOutcome
}
