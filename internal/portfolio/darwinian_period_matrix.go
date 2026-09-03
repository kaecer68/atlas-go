package portfolio

import (
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// capital-flow Phase 2 PR-2b: read-only per-period performance aggregation.
//
// Darwinian's in-memory rolling window (darwinian_weights.go DailyReturns)
// and the live weights are intentionally NOT touched here — this module reads
// the persisted outcome store and stratifies each agent's realized
// forward-return outcomes by the seven-period market classification that
// PR-2a joined at write time (outcome.market_period). Consumers that want a
// period-aware view of strategy performance (admin heatmap, future
// period-weighted Darwinian tiers) read through this module only.

const (
	// PeriodMatrixMinSamplesDefault is the per-cell sample floor for
	// win-rate / Sharpe / average-return estimates. Cells with fewer real
	// outcomes report status=insufficient_data and nil numeric values —
	// periods never borrow samples from each other (no sliding-window merge).
	PeriodMatrixMinSamplesDefault = 30

	// PeriodCellStatusOK marks a cell whose estimates are computed (real
	// outcome count >= min samples).
	PeriodCellStatusOK = "ok"
	// PeriodCellStatusInsufficientData marks a cell below the sample floor;
	// sample_count is still reported truthfully.
	PeriodCellStatusInsufficientData = "insufficient_data"
)

// periodMatrixOrder is the canonical display order of the seven periods
// (docs/ATLAS_METHODOLOGY.md §3). The matrix iterates in this order so API
// consumers and the admin heatmap get a stable column sequence.
var periodMatrixOrder = []domain.MarketPeriod{
	domain.PeriodDownturn,
	domain.PeriodTurnaroundUp,
	domain.PeriodBull,
	domain.PeriodPlateau,
	domain.PeriodConsolidation,
	domain.PeriodTurnaroundDown,
	domain.PeriodBlackSwan,
}

// PeriodCell is one (agent × market_period) cell of the performance matrix.
type PeriodCell struct {
	AgentID      string `json:"agent_id"`
	MarketPeriod string `json:"market_period"`
	SampleCount  int    `json:"sample_count"`
	// WinRate / Sharpe / AvgReturn are nil when the cell is below the
	// sample floor (status=insufficient_data) — never a misleading zero.
	WinRate   *float64 `json:"win_rate"`
	Sharpe    *float64 `json:"sharpe"`
	AvgReturn *float64 `json:"avg_return"`
	Status    string   `json:"status"`
}

// PeriodPerformanceMatrix is the full (agent × 7 periods) aggregation
// response. Cells are flattened: period-major (canonical order), then
// agent-id ascending within each period, so the payload stays stable and
// diff-friendly.
type PeriodPerformanceMatrix struct {
	GeneratedAt time.Time    `json:"generated_at"`
	MinSamples  int          `json:"min_samples"`
	Periods     []string     `json:"periods"`
	Cells       []PeriodCell `json:"cells"`
}

// matrixAccumulator collects returns/wins for one (agent, period) key.
type matrixAccumulator struct {
	returns []float64
	wins    int
}

// BuildPeriodPerformanceMatrix stratifies persisted outcomes by
// agent × market_period (read-only; never touches Darwinian weights).
//
// Eligible outcomes (all must hold):
//   - real rows only: outcome.IsSynthetic == false and the period row is
//     not backfilled (MarketPeriodSource != "synthetic") — synthetic
//     forward returns / backfilled-period rows would pollute win rates;
//   - passed control guards (PassedGuards), mirroring the scorecard
//     population; and
//   - a canonical market_period classification (rows whose trading day had
//     no period_history row carry an empty period and cannot be attributed
//     — they are omitted, matching the "unknown → 資料不足" contract).
//
// Win is the realized Hit flag (forward_return > 0 at write time — the same
// signal fed to the Darwinian manager). Sharpe uses the per-outcome
// convention (FrequencyPerOutcome, no annualization) with the given sample
// floor; periods are stratified independently.
func BuildPeriodPerformanceMatrix(outcomes []domain.RecommendationOutcome, minSamples int) PeriodPerformanceMatrix {
	if minSamples < 1 {
		minSamples = PeriodMatrixMinSamplesDefault
	}

	periodRank := make(map[string]int, len(periodMatrixOrder))
	for i, p := range periodMatrixOrder {
		periodRank[string(p)] = i
	}

	type key struct {
		agent  string
		period string
	}
	acc := make(map[key]*matrixAccumulator)
	agents := map[string]bool{}
	for _, o := range outcomes {
		if o.AgentID == "" || !o.PassedGuards || o.IsSynthetic {
			continue
		}
		if o.MarketPeriodSource == "synthetic" {
			continue
		}
		if _, ok := periodRank[o.MarketPeriod]; !ok {
			continue
		}
		k := key{agent: o.AgentID, period: o.MarketPeriod}
		a, ok := acc[k]
		if !ok {
			a = &matrixAccumulator{}
			acc[k] = a
		}
		a.returns = append(a.returns, o.ForwardReturn)
		if o.Hit {
			a.wins++
		}
		agents[o.AgentID] = true
	}

	agentList := make([]string, 0, len(agents))
	for a := range agents {
		agentList = append(agentList, a)
	}
	sort.Strings(agentList)

	periods := make([]string, len(periodMatrixOrder))
	for i, p := range periodMatrixOrder {
		periods[i] = string(p)
	}

	matrix := PeriodPerformanceMatrix{
		GeneratedAt: time.Now().UTC(),
		MinSamples:  minSamples,
		Periods:     periods,
		Cells:       make([]PeriodCell, 0, len(agents)*len(periodMatrixOrder)),
	}
	for _, p := range periodMatrixOrder {
		for _, agent := range agentList {
			a := acc[key{agent: agent, period: string(p)}]
			cell := PeriodCell{
				AgentID:      agent,
				MarketPeriod: string(p),
				Status:       PeriodCellStatusInsufficientData,
			}
			if a != nil {
				cell.SampleCount = len(a.returns)
				if cell.SampleCount >= minSamples {
					winRate := float64(a.wins) / float64(cell.SampleCount)
					avgReturn := meanFloat(a.returns)
					cell.WinRate = &winRate
					cell.AvgReturn = &avgReturn
					sharpe := ComputeSharpe(a.returns, SharpeConfig{
						Frequency:  FrequencyPerOutcome,
						MinSamples: minSamples,
					})
					cell.Sharpe = &sharpe
					cell.Status = PeriodCellStatusOK
				}
			}
			matrix.Cells = append(matrix.Cells, cell)
		}
	}
	return matrix
}

func meanFloat(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}
