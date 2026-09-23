package domain

import "time"

type LockedCashEntry struct {
	UnlockDay time.Time `json:"unlock_day"`
	Amount    float64   `json:"amount"`
}

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
	LockedCash      []LockedCashEntry  `json:"locked_cash"`

	// LastSessionDate records the trading session (YYYY-MM-DD) the last
	// DailyReturns/EquityCurve entry belongs to. Without it the series had no
	// date semantics at all: every RunDailySimulation appended, so several
	// writers (auto_daily_simulation, stress_test_daily,
	// POST /admin/trigger-simulation) could push many same-day entries and the
	// zero returns of those re-runs diluted VaR/CVaR at the tail (#1900).
	//
	// Empty means "unknown" — a state file written before the field existed, or
	// a run whose quotes carry no session date. Unknown is handled by keeping
	// the legacy append behavior and adopting the semantics on the next dated
	// run; history is never dropped.
	LastSessionDate string `json:"last_session_date,omitempty"`
	// SessionBaseValue is the portfolio value that closed the session *before*
	// LastSessionDate, i.e. the denominator the dated return of
	// LastSessionDate was computed from. Re-running that session must recompute
	// against this same base, not against the value left behind by the earlier
	// run of the same session.
	SessionBaseValue float64 `json:"session_base_value,omitempty"`
}

// SessionDateLayout is the layout of SimulationState.LastSessionDate.
const SessionDateLayout = "2006-01-02"

// SessionDateKey maps a trading day to the key stored in LastSessionDate.
// The zero time (quotes without a session date) maps to "", i.e. "unknown".
func SessionDateKey(day time.Time) string {
	if day.IsZero() {
		return ""
	}
	return day.Format(SessionDateLayout)
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
		LockedCash:     make([]LockedCashEntry, 0),
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

func (s SimulationState) AvailableCash(day time.Time) float64 {
	available := s.Cash
	for _, lc := range s.LockedCash {
		if day.Before(lc.UnlockDay) {
			available -= lc.Amount
		}
	}
	return available
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

	// SessionRerun/SessionReturn/SessionReturnRecorded describe how this run
	// updated the daily-return series (see SimulationState.LastSessionDate).
	// They are mirrored into domain.SimulationResult fields of the same names
	// so callers with their own return accumulator can apply the same date
	// semantics (#1900).
	SessionRerun          bool    `json:"session_rerun,omitempty"`
	SessionReturn         float64 `json:"session_return,omitempty"`
	SessionReturnRecorded bool    `json:"session_return_recorded,omitempty"`
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
