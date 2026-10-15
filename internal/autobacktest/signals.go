package autobacktest

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
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

// NewSignalEngine builds the signal engine on the backend-aware ledger store,
// so production (ATLAS_STORE_BACKEND=postgres) evaluates the returns and
// session summaries that live in PostgreSQL instead of an unpopulated JSONL
// directory.
//
// Do not reintroduce a raw ledger.NewStore(dir) here: the JSONL store answers
// a missing file with (nil, nil), so Evaluate() returns zero signals and a nil
// error, and the CIRCUIT_BREAKER consumer in SignalApply never fires
// (production incident 2026-10; see scripts/ci/check_ledger_seam.py).
func NewSignalEngine(cfg config.Config) (*SignalEngine, error) {
	store, err := ledger.NewFullStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("autobacktest signal engine: %w", err)
	}
	return NewSignalEngineWithStore(store), nil
}

// NewSignalEngineWithStore injects an already-constructed store (DI path for
// production wiring and tests). store must be non-nil.
func NewSignalEngineWithStore(store ledger.FullStore) *SignalEngine {
	return &SignalEngine{store: store}
}

// NewSignalEngineFromDir keeps the pre-2026-10 directory-based call sites
// (tests, CLI helpers) working: it selects the JSONL backend explicitly rather
// than by omission, so an unpinned caller can never silently read the wrong
// backend.
func NewSignalEngineFromDir(ledgerDir string) (*SignalEngine, error) {
	store, err := jsonlFullStore(ledgerDir)
	if err != nil {
		return nil, err
	}
	return NewSignalEngineWithStore(store), nil
}

// jsonlFullStore is the explicit-JSONL factory shared by the directory-based
// compatibility constructors.
func jsonlFullStore(ledgerDir string) (ledger.FullStore, error) {
	store, err := ledger.NewFullStore(config.Config{StoreBackend: "jsonl", LedgerDir: ledgerDir})
	if err != nil {
		return nil, fmt.Errorf("autobacktest jsonl store: %w", err)
	}
	return store, nil
}

func (se *SignalEngine) Evaluate() (Signals, error) {
	outcomes, err := outcomesFor(se.store)
	if err != nil {
		return Signals{}, err
	}
	scorecards := ledger.BuildScorecards(outcomes)

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

// outcomesFor prefers the slim projection when the store provides it (signal
// evaluation only needs returns/scorecards), so the metadata JSONB is never
// transferred; stores without it keep the previous flat read.
func outcomesFor(store ledger.OutcomeStore) ([]domain.RecommendationOutcome, error) {
	if slim, ok := store.(ledger.ScorecardOutcomeStore); ok {
		if outcomes, err := slim.LoadScorecardOutcomes(); err == nil && len(outcomes) > 0 {
			return outcomes, nil
		}
	}
	return store.LoadOutcomes()
}
