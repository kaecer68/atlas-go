package portfolio

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// mkOutcome builds one eligible outcome (real, passed, live-sourced period).
func mkOutcome(agent, period, source string, ret float64, hit bool, n int) domain.RecommendationOutcome {
	o := domain.RecommendationOutcome{
		AgentID:            agent,
		Symbol:             "2330",
		Side:               domain.SideBuy,
		Conviction:         80,
		Window:             "2026-04-01",
		RecordedAt:         time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		PassedGuards:       true,
		ForwardReturn:      ret,
		Hit:                hit,
		MarketPeriod:       period,
		MarketPeriodSource: source,
	}
	_ = n
	return o
}

// cellAt returns the matrix cell for (agent, period).
func cellAt(t *testing.T, m PeriodPerformanceMatrix, agent, period string) PeriodCell {
	t.Helper()
	for _, c := range m.Cells {
		if c.AgentID == agent && c.MarketPeriod == period {
			return c
		}
	}
	t.Fatalf("cell (%s, %s) not found", agent, period)
	return PeriodCell{}
}

func TestBuildPeriodPerformanceMatrix_StratifiesSevenPeriods(t *testing.T) {
	var outcomes []domain.RecommendationOutcome
	// Agent A: 30 wins in bull (real), 5 in plateau → insufficient.
	for i := 0; i < 30; i++ {
		outcomes = append(outcomes, mkOutcome("agent-a", "bull", "live", 0.02, true, i))
	}
	for i := 0; i < 5; i++ {
		outcomes = append(outcomes, mkOutcome("agent-a", "plateau", "live", 0.01, true, i))
	}
	// Agent B: 30 mixed in downturn (20 wins).
	for i := 0; i < 30; i++ {
		hit := i < 20
		ret := 0.01
		if !hit {
			ret = -0.01
		}
		outcomes = append(outcomes, mkOutcome("agent-b", "downturn", "live", ret, hit, i))
	}
	m := BuildPeriodPerformanceMatrix(outcomes, 30)

	if len(m.Periods) != 7 {
		t.Fatalf("expected 7 periods, got %d: %v", len(m.Periods), m.Periods)
	}
	if m.MinSamples != 30 {
		t.Fatalf("MinSamples = %d, want 30", m.MinSamples)
	}
	// Every agent×period cell present (2 agents × 7 periods = 14 cells).
	if len(m.Cells) != 14 {
		t.Fatalf("expected 14 cells, got %d", len(m.Cells))
	}

	aBull := cellAt(t, m, "agent-a", "bull")
	if aBull.Status != PeriodCellStatusOK || aBull.SampleCount != 30 {
		t.Fatalf("agent-a/bull = %+v, want ok/30", aBull)
	}
	if aBull.WinRate == nil || *aBull.WinRate != 1.0 {
		t.Errorf("agent-a/bull win_rate = %v, want 1.0", aBull.WinRate)
	}
	if aBull.Sharpe == nil {
		t.Error("agent-a/bull sharpe should be non-nil (constant series has zero stddev → 0)")
	}

	aPlateau := cellAt(t, m, "agent-a", "plateau")
	if aPlateau.Status != PeriodCellStatusInsufficientData || aPlateau.SampleCount != 5 {
		t.Fatalf("agent-a/plateau = %+v, want insufficient_data/5", aPlateau)
	}
	if aPlateau.WinRate != nil || aPlateau.Sharpe != nil || aPlateau.AvgReturn != nil {
		t.Error("insufficient cell must return nil numerics, not misleading values")
	}

	bDownturn := cellAt(t, m, "agent-b", "downturn")
	if bDownturn.Status != PeriodCellStatusOK || bDownturn.SampleCount != 30 {
		t.Fatalf("agent-b/downturn = %+v, want ok/30", bDownturn)
	}
	if bDownturn.WinRate == nil || *bDownturn.WinRate != 20.0/30.0 {
		t.Errorf("agent-b/downturn win_rate = %v, want 2/3", bDownturn.WinRate)
	}

	// Empty cells (agent-a/downturn with zero samples) → insufficient_data, n=0.
	aDownturn := cellAt(t, m, "agent-a", "downturn")
	if aDownturn.Status != PeriodCellStatusInsufficientData || aDownturn.SampleCount != 0 {
		t.Errorf("empty cell = %+v, want insufficient_data/0", aDownturn)
	}
}

func TestBuildPeriodPerformanceMatrix_Filters(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		mkOutcome("agent-a", "bull", "live", 0.05, true, 0), // eligible
		mkOutcome("agent-a", "bull", "live", 0.05, true, 0), // eligible
		// not passed guards → excluded
		func() domain.RecommendationOutcome {
			o := mkOutcome("agent-a", "bull", "live", 0.05, true, 0)
			o.PassedGuards = false
			return o
		}(),
		// synthetic outcome → excluded
		func() domain.RecommendationOutcome {
			o := mkOutcome("agent-a", "bull", "live", 0.05, true, 0)
			o.IsSynthetic = true
			return o
		}(),
		// backfilled period source → excluded
		mkOutcome("agent-a", "black_swan", "synthetic", 0.05, true, 0),
		// no period row (unknown) → excluded from canonical cells
		mkOutcome("agent-a", "", "", 0.05, true, 0),
	}
	m := BuildPeriodPerformanceMatrix(outcomes, 30)
	cell := cellAt(t, m, "agent-a", "bull")
	if cell.SampleCount != 2 {
		t.Errorf("eligible bull samples = %d, want 2 (guards/synthetic filtered)", cell.SampleCount)
	}
	bs := cellAt(t, m, "agent-a", "black_swan")
	if bs.SampleCount != 0 {
		t.Errorf("synthetic-source rows must be excluded; black_swan samples = %d", bs.SampleCount)
	}
}

func TestBuildPeriodPerformanceMatrix_DefaultMinSamples(t *testing.T) {
	m := BuildPeriodPerformanceMatrix(nil, 0)
	if m.MinSamples != PeriodMatrixMinSamplesDefault {
		t.Errorf("default MinSamples = %d, want %d", m.MinSamples, PeriodMatrixMinSamplesDefault)
	}
	if len(m.Cells) != 0 {
		t.Errorf("no outcomes → no cells, got %d", len(m.Cells))
	}
}
