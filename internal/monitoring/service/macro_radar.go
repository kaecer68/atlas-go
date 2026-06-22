package service

import (
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// PR5-2 — macro radar source extracted from pipeline.go (Issue #611 sub-issue-5):
//   - LoadMacroRadar: aggregate session-level macro radar summary.
//   - MacroRadarData: response shape for /api/dashboard/macro-radar.
//   - computeAgentRegimeBreakdown + computeRegimeStability +
//     bothNonZeroAndDivergent: agent-vs-regime OOS scoring helpers.

func (s *PipelineService) LoadMacroRadar(sessionID string) (*MacroRadarData, error) {
	var summary *domain.SessionSummary
	var err error
	if sessionID == "" {
		summary, err = FindLatestSessionSummary(s.store, s.LedgerDir)
	} else {
		summary, err = LoadSessionSummary(s.LedgerDir, sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("load macro radar data: %w", err)
	}
	if summary == nil {
		return nil, nil
	}

	return &MacroRadarData{
		SessionID:     summary.SessionID,
		Regime:        summary.Regime,
		GuardOutcomes: append([]domain.GuardOutcome(nil), summary.GuardOutcomes...),
		BrokerRuntime: summary.BrokerRuntime,
		RecordedAt:    summary.RecordedAt,
	}, nil
}

type MacroRadarData struct {
	SessionID     string
	Regime        domain.Regime
	GuardOutcomes []domain.GuardOutcome
	BrokerRuntime domain.BrokerRuntimeAudit
	RecordedAt    time.Time
}

func computeAgentRegimeBreakdown(outcomes []domain.RecommendationOutcome, agentID, defaultRegime string) *domain.RegimeBreakdown {
	agentOutcomes := make([]domain.RecommendationOutcome, 0)
	for _, o := range outcomes {
		if o.AgentID == agentID {
			agentOutcomes = append(agentOutcomes, o)
		}
	}
	if len(agentOutcomes) == 0 {
		return nil
	}
	byRegime := make(map[string][]domain.RecommendationOutcome)
	for _, o := range agentOutcomes {
		regime := o.Regime
		if regime == "" {
			regime = defaultRegime
		}
		byRegime[regime] = append(byRegime[regime], o)
	}
	regs := make(map[string]domain.RegimePerformance, len(byRegime))
	for regime, rs := range byRegime {
		var total, sumReturn float64
		hits := 0
		for _, o := range rs {
			total += o.ForwardReturn
			sumReturn += o.ForwardReturn
			if o.Hit {
				hits++
			}
		}
		n := len(rs)
		avg := sumReturn / float64(n)
		winRate := float64(hits) / float64(n)
		regs[regime] = domain.RegimePerformance{
			Regime:       regime,
			SessionCount: n,
			TotalReturn:  total,
			WinRate:      winRate,
			AvgReturn:    avg,
		}
	}
	return &domain.RegimeBreakdown{Regimes: regs}
}

func computeRegimeStability(rb *domain.RegimeBreakdown) *float64 {
	if rb == nil || len(rb.Regimes) < 2 {
		return nil
	}
	avgs := make([]float64, 0, len(rb.Regimes))
	for _, p := range rb.Regimes {
		avgs = append(avgs, p.AvgReturn)
	}
	mean := 0.0
	for _, v := range avgs {
		mean += v
	}
	mean /= float64(len(avgs))
	variance := 0.0
	for _, v := range avgs {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(avgs))
	std := math.Sqrt(variance)
	return &std
}

func bothNonZeroAndDivergent(a, b float64) bool {
	const epsilon = 0.05
	if math.Abs(a) < epsilon || math.Abs(b) < epsilon {
		return false
	}
	relDiff := math.Abs(a-b) / math.Max(math.Abs(a), math.Abs(b))
	return relDiff > 0.10
}
