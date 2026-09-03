package service

import (
	"fmt"
	"maps"
	"math"
	"sync/atomic"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// PR5-3 — agent observatory source extracted from pipeline.go (Issue #611 sub-issue-5):
//   - LoadAgentObservatory: aggregate agent scorecards with OOS metrics
//     (is_sharpe, oos_sharpe, is_oos_ratio, overfit_warning,
//     rolling_sharpe_trend, oos_sample_warning).
//   - AgentObservatoryData: response shape for /api/dashboard/agent-observatory.

// LoadAgentObservatory serves the agent-observatory aggregate with a 60s
// TTL cache and in-flight dedup: the uncached full-history load pulls ALL
// recommendation outcomes (~1.9GB, #1780), and concurrent dashboard polls
// stacked until the container was OOM-killed (2026-09-03 outage).
func (s *PipelineService) LoadAgentObservatory(sessionID string, limit int) (*AgentObservatoryData, error) {
	cacheKey := fmt.Sprintf("%s|%d", sessionID, limit)

	s.obsMu.Lock()
	// In-flight dedup: another caller is computing the same view right now —
	// wait for it and reuse its result.
	if s.obsInflight && s.obsCacheKey == cacheKey {
		for s.obsInflight && s.obsCacheKey == cacheKey && time.Since(s.obsCacheAt) < 90*time.Second {
			s.obsMu.Unlock()
			time.Sleep(200 * time.Millisecond)
			s.obsMu.Lock()
		}
		if s.obsCache != nil && s.obsCacheKey == cacheKey {
			data := s.obsCache
			s.obsMu.Unlock()
			return data, nil
		}
		s.obsMu.Unlock()
		return nil, fmt.Errorf("load agent observatory: concurrent load in progress")
	}
	if s.obsCache != nil && s.obsCacheKey == cacheKey && time.Since(s.obsCacheAt) < 60*time.Second {
		data := s.obsCache
		s.obsMu.Unlock()
		return data, nil
	}
	s.obsInflight = true
	s.obsCacheKey = cacheKey
	s.obsMu.Unlock()

	data, err := s.loadAgentObservatoryUncached(sessionID, limit)

	s.obsMu.Lock()
	s.obsInflight = false
	if err == nil && data != nil {
		s.obsCacheAt = time.Now()
		s.obsCache = data
	}
	s.obsMu.Unlock()

	return data, err
}

func (s *PipelineService) loadAgentObservatoryUncached(sessionID string, limit int) (*AgentObservatoryData, error) {
	var summary *domain.SessionSummary
	var err error
	if sessionID == "" {
		summary, err = FindLatestSessionSummary(s.store, s.LedgerDir)
	} else {
		summary, err = LoadSessionSummary(s.LedgerDir, sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("load agent observatory summary: %w", err)
	}

	store := s.store
	var outcomes []domain.RecommendationOutcome
	if sessionID == "" {
		// Full historical view: load outcomes from ALL sessions for proper
		// OOS validation (IS/OOS split needs ≥10 train / ≥5 test per agent).
		// A single session yields only 1-10 outcomes per agent. #1780 Phase 1:
		// prefer the 8-field slim projection (LoadScorecardOutcomes) so the
		// full-metadata ~1.9GB unmarshal never happens on this hot path;
		// stores without the optional loader fall back to the full read with
		// a warn + counter (B1).
		outcomes, err = loadScorecardOutcomes(store)
		if err != nil {
			return nil, fmt.Errorf("load outcomes from sessions: %w", err)
		}
	} else {
		if summary != nil {
			if o, err := store.LoadSessionOutcomes(summary.SessionID); err != nil {
				logging.Warn("pipeline_service", "load_session_outcomes_failed", logging.Err(err))
			} else {
				outcomes = o
			}
		}
		if outcomes == nil {
			outcomes, err = store.LoadOutcomes()
			if err != nil {
				return nil, fmt.Errorf("load recommendation outcomes: %w", err)
			}
		}
	}
	scorecards := ledger.BuildScorecards(outcomes)
	if len(scorecards) > limit {
		scorecards = scorecards[:limit]
	}

	darwinData, darwinErr := s.LoadDarwinianStatus()
	darwinByAgent := map[string]DarwinianAgentInfo{}
	if darwinErr != nil {
		logging.Warn("pipeline_service", "load_darwinian_status_failed", logging.Err(darwinErr))
	} else if darwinData != nil {
		maps.Copy(darwinByAgent, darwinData.Agents)
	}

	defaultRegime := "unknown"
	if summary != nil && string(summary.Regime) != "" {
		defaultRegime = string(summary.Regime)
	}

	for i := range scorecards {
		sc := &scorecards[i]
		if da, ok := darwinByAgent[sc.AgentID]; ok {
			sc.DarwinianWeight = da.Weight
			if !math.IsNaN(da.RollingSharpe) && !math.IsInf(da.RollingSharpe, 0) {
				sharpe := da.RollingSharpe
				sc.DarwinianSharpe = &sharpe
			}
			// Phase 2: BuildScorecards and DarwinianWeightManager now share
			// internal/portfolio.ComputeSharpe, so their Sharpe values are
			// guaranteed to be identical. The DataConsistencyWarning field
			// is preserved for backward compatibility but no longer triggered.
			_ = bothNonZeroAndDivergent
		}
		sc.RegimeBreakdown = computeAgentRegimeBreakdown(outcomes, sc.AgentID, defaultRegime)
		if rb := sc.RegimeBreakdown; rb != nil && len(rb.Regimes) >= 2 {
			if stab := computeRegimeStability(rb); stab != nil {
				sc.RegimeStability = stab
			}
		}
	}

	data := &AgentObservatoryData{
		Scorecards: scorecards,
	}
	if summary != nil {
		data.SessionID = summary.SessionID
		data.NextExperimentAgentID = summary.NextExperimentAgentID
		data.BrokerRuntime = summary.BrokerRuntime
		data.RecordedAt = summary.RecordedAt
	}
	return data, nil
}

// scorecardSlimServiceFallbackTotal counts observatory slim-projection
// fallbacks at the PipelineService layer (#1780 Phase 1, B1): when the
// configured outcome store lacks the optional LoadScorecardOutcomes method,
// the sessionID=="" observatory view silently falling back to
// LoadOutcomesFromSessions would re-enable the full-metadata ~1.9GB load the
// slim projection eliminates, with no signal. A non-zero delta since deploy
// means the slim path is NOT active and the OOM mitigation is not engaged.
var scorecardSlimServiceFallbackTotal atomic.Int64

// ScorecardSlimServiceFallbackTotal returns the total service-layer
// slim-projection fallbacks. Exposed for monitoring/alerting consumption.
func ScorecardSlimServiceFallbackTotal() int64 {
	return scorecardSlimServiceFallbackTotal.Load()
}

// loadScorecardOutcomes loads the observatory slim projection when the store
// implements the optional 8-field loader; otherwise it warns, increments the
// fallback counter, and uses the pre-#1780 full metadata read.
func loadScorecardOutcomes(store ledger.OutcomeStore) ([]domain.RecommendationOutcome, error) {
	if sl, ok := store.(interface {
		LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error)
	}); ok {
		return sl.LoadScorecardOutcomes()
	}
	scorecardSlimServiceFallbackTotal.Add(1)
	logging.Warn("pipeline_service", "scorecard_slim_fallback",
		"layer", "service",
		"store_type", fmt.Sprintf("%T", store),
		"reason", "store does not implement LoadScorecardOutcomes; full metadata read")
	return store.LoadOutcomesFromSessions()
}

type AgentObservatoryData struct {
	SessionID             string
	NextExperimentAgentID string
	Scorecards            []domain.Scorecard
	BrokerRuntime         domain.BrokerRuntimeAudit
	RecordedAt            time.Time
}
