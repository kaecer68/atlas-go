package autobacktest

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/ledger"
	riskpkg "github.com/kaecer68/atlas-go/internal/risk"
)

type Signal string

const (
	SignalNone              Signal = "NONE"
	SignalVaRWarning        Signal = "VaR_WARNING"
	SignalSharpeDegradation Signal = "SHARPE_DEGRADATION"
	SignalCircuitBreaker    Signal = "CIRCUIT_BREAKER"
)

func (s Signal) String() string {
	return string(s)
}

type Signals struct {
	Active      []Signal
	VaR95       float64 // negative = loss (e.g. -0.05 = 5% loss at 95th percentile)
	VaR99       float64 // negative = loss
	SharpeShort float64
	SharpeLong  float64
	DrawdownPct float64 // positive magnitude (e.g. 0.15 = 15% drawdown)
}

type SignalEngine struct {
	store ledger.FullStore
}

func NewSignalEngine(ledgerDir string) (*SignalEngine, error) {
	store, ok := ledger.NewStore(ledgerDir).(ledger.FullStore)
	if !ok {
		return nil, fmt.Errorf("ledger store does not implement FullStore: backtest signals require scorecard and summary access")
	}
	return &SignalEngine{store: store}, nil
}

func (se *SignalEngine) Evaluate() (Signals, error) {
	outcomes, err := se.store.LoadOutcomes()
	if err != nil {
		return Signals{}, err
	}

	var returns []float64
	for _, o := range outcomes {
		returns = append(returns, o.ForwardReturn)
	}

	// sort.Float64s removed — canonical risk.CalculateVaR sorts internally (#1265)

	n := len(returns)
	if n == 0 {
		return Signals{}, nil
	}

	var95 := 0.0
	var99 := 0.0
	// Use the canonical VaR calculator (#1265 canonical metric source).
	if n >= 20 {
		var95 = riskpkg.CalculateVaRPercentile(returns, 0.95)
		var99 = riskpkg.CalculateVaRPercentile(returns, 0.99)
	}

	scorecards, _, err := se.store.LoadAllSessionScorecards()
	if err != nil {
		return Signals{}, err
	}

	var recentShort, recentLong float64
	const shortN = 5
	const longN = 20
	if len(scorecards) >= shortN {
		for i := len(scorecards) - shortN; i < len(scorecards); i++ {
			recentShort += scorecards[i].SharpeLike
		}
		recentShort /= shortN
	}
	if len(scorecards) >= longN {
		for i := len(scorecards) - longN; i < len(scorecards); i++ {
			recentLong += scorecards[i].SharpeLike
		}
		recentLong /= longN
	}

	summaries, err := se.store.LoadSessionSummaries()
	if err != nil {
		return Signals{}, err
	}

	pvValues := make([]float64, len(summaries))
	for i, s := range summaries {
		pvValues[i] = s.PortfolioValue
	}
	drawdown := riskpkg.CalculateMaxDrawdown(pvValues)

	var active []Signal
	if var95 < -0.05 {
		active = append(active, SignalVaRWarning)
	}
	if recentLong > 0 && recentShort < recentLong*0.7 {
		active = append(active, SignalSharpeDegradation)
	}
	if drawdown > 0.15 {
		active = append(active, SignalCircuitBreaker)
	}

	return Signals{
		Active:      active,
		VaR95:       var95,
		VaR99:       var99,
		SharpeShort: recentShort,
		SharpeLong:  recentLong,
		DrawdownPct: drawdown,
	}, nil
}
