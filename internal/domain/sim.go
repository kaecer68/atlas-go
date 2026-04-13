package domain

import "time"

// SimulationState tracks cross-day portfolio state for multi-day backtests.
type SimulationState struct {
	Cash            float64
	Positions       []Position
	CumulativePnL   float64
	EquityCurve     []float64 // portfolio value at end of each day
	DailyReturns    []float64
	PreviousValues  map[string]float64 // symbol -> previous day close (for drawdown calc)
	MaxEquity       float64
	CurrentDrawdown float64
}

// NewSimulationState initializes a simulation state with starting cash.
func NewSimulationState(startingCash float64) SimulationState {
	return SimulationState{
		Cash:           startingCash,
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
	Date           time.Time
	Regime         Regime
	Orders         []Order
	Positions      []Position
	Cash           float64
	PortfolioValue float64
	DailyPnL       float64
}

// SimulationReport aggregates metrics from a multi-day run.
type SimulationReport struct {
	TotalReturn   float64
	SharpeRatio   float64
	MaxDrawdown   float64
	EquityCurve   []float64
	AgentHitRates map[string]float64
	TradeCount    int
	StartDate     time.Time
	EndDate       time.Time
}
