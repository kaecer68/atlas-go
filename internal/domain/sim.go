package domain

import "time"

// SimulationState tracks cross-day portfolio state for multi-day backtests.
type SimulationState struct {
	Cash            float64            `json:"cash"`
	Positions       []Position         `json:"positions"`
	RealizedPnL     float64            `json:"realized_pnl"`
	StartingCash    float64            `json:"starting_cash"`
	EquityCurve     []float64          `json:"equity_curve"`
	DailyReturns    []float64          `json:"daily_returns"`
	PreviousValues  map[string]float64 `json:"previous_values"`
	MaxEquity       float64            `json:"max_equity"`
	CurrentDrawdown float64            `json:"current_drawdown"`
}

// NewSimulationState initializes a simulation state with starting cash.
func NewSimulationState(startingCash float64) SimulationState {
	return SimulationState{
		Cash:           startingCash,
		StartingCash:   startingCash,
		Positions:      make([]Position, 0),
		EquityCurve:    make([]float64, 0),
		DailyReturns:   make([]float64, 0),
		PreviousValues: make(map[string]float64),
		MaxEquity:      startingCash,
	}
}

// PortfolioValue computes total value (cash + market value of positions).
func (s SimulationState) PortfolioValue() float64 {
	value := s.Cash
	for _, p := range s.Positions {
		value += p.MarketValue
	}
	return value
}

// SellLogicEnabled is always true in the upgraded simulator;
// individual sell triggers are gated by their own pct thresholds.
func (c SimulationConstraints) SellLogicEnabled() bool {
	return true
}

// DayResult captures the outcome of a single simulated trading day.
type DayResult struct {
	Date           time.Time     `json:"date"`
	Regime         Regime        `json:"regime"`
	Orders         []Order       `json:"orders"`
	Trades         []TradeRecord `json:"trades"`
	Positions      []Position    `json:"positions"`
	Cash           float64       `json:"cash"`
	PortfolioValue float64       `json:"portfolio_value"`
	DailyPnL       float64       `json:"daily_pnl"`
	FallbackEvents []string      `json:"fallback_events,omitempty"`
}

type SimulationReport struct {
	TotalReturn    float64            `json:"total_return"`
	SharpeRatio    float64            `json:"sharpe_ratio"`
	MaxDrawdown    float64            `json:"max_drawdown"`
	EquityCurve    []float64          `json:"equity_curve"`
	AgentHitRates  map[string]float64 `json:"agent_hit_rates"`
	TradeCount     int                `json:"trade_count"`
	StartDate      time.Time          `json:"start_date"`
	EndDate        time.Time          `json:"end_date"`
	FallbackEvents []string           `json:"fallback_events,omitempty"`
}
