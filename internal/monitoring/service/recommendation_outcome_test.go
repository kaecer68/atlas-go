package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func writeTestSessionArtifacts(t *testing.T, baseDir, sessionID string, summary domain.SessionSummary, outcome domain.RecommendationOutcome) {
	t.Helper()
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	summaryBytes, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryBytes, 0o644); err != nil {
		t.Fatalf("write summary.json: %v", err)
	}

	outcomeBytes, err := json.Marshal(outcome)
	if err != nil {
		t.Fatalf("marshal outcome: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), append(outcomeBytes, '\n'), 0o644); err != nil {
		t.Fatalf("write recommendation_outcomes.jsonl: %v", err)
	}
}

func writeTestSessionSummaryOnly(t *testing.T, baseDir, sessionID string, summary domain.SessionSummary) {
	t.Helper()
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	summaryBytes, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryBytes, 0o644); err != nil {
		t.Fatalf("write summary.json: %v", err)
	}
}

// Test (a): FindLatestSessionSummary should prefer the session with non-zero OutcomeCount
// when auto-selecting from two sessions where the newest has zero outcomes.
func TestFindLatestSessionSummaryPrefersNonEmptyOutcomeCount(t *testing.T) {
	baseDir := t.TempDir()

	olderDate := time.Date(2026, time.April, 20, 4, 0, 0, 0, time.UTC)
	newerDate := time.Date(2026, time.April, 21, 4, 0, 0, 0, time.UTC)

	olderID := "session-20260420-daily"
	newerID := "session-20260421-daily"

	// Older session: date is earlier but has real OutcomeCount.
	writeTestSessionSummaryOnly(t, baseDir, olderID, domain.SessionSummary{
		SessionID:    olderID,
		Regime:       domain.RegimeRiskOn,
		RecordedAt:   olderDate,
		OutcomeCount: 5,
	})

	// Newer session: date is later but OutcomeCount is zero (no recommendations produced).
	writeTestSessionSummaryOnly(t, baseDir, newerID, domain.SessionSummary{
		SessionID:    newerID,
		Regime:       domain.RegimeRiskOn,
		RecordedAt:   newerDate,
		OutcomeCount: 0,
	})

	// FindLatestSessionSummary with nil store falls back to disk-based auto-selection.
	// Expected: prefer the session with non-zero OutcomeCount when the newest one has zero outcomes.
	summary, err := FindLatestSessionSummary(nil, baseDir)
	if err != nil {
		t.Fatalf("FindLatestSessionSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("expected a summary, got nil")
	}
	if summary.SessionID != olderID {
		t.Fatalf("auto-selection: expected %q (non-empty OutcomeCount), got %q", olderID, summary.SessionID)
	}
}

func TestLoadSessionSummaryRequiresNonEmptySessionID(t *testing.T) {
	baseDir := t.TempDir()

	_, err := LoadSessionSummary(baseDir, "")
	if err == nil {
		t.Fatal("expected error for empty sessionID, got nil")
	}
}

func TestLoadSessionSummarySpecificSession(t *testing.T) {
	baseDir := t.TempDir()
	recordedAt := time.Date(2026, time.April, 22, 4, 2, 30, 0, time.UTC)
	sessionID := "session-20260422-daily"

	writeTestSessionSummaryOnly(t, baseDir, sessionID, domain.SessionSummary{
		SessionID:    sessionID,
		Regime:       domain.RegimeRiskOn,
		RecordedAt:   recordedAt,
		OutcomeCount: 3,
	})

	summary, err := LoadSessionSummary(baseDir, sessionID)
	if err != nil {
		t.Fatalf("LoadSessionSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("expected a summary, got nil")
	}
	if summary.SessionID != sessionID {
		t.Fatalf("expected %q, got %q", sessionID, summary.SessionID)
	}
}

// Test (b): LoadAgentObservatory should read from the resolved session's
// recommendation_outcomes.jsonl, not the global root file.
func TestLoadAgentObservatoryReadsFromSessionScope(t *testing.T) {
	baseDir := t.TempDir()
	recordedAt := time.Date(2026, time.April, 22, 4, 2, 30, 0, time.UTC)
	sessionID := "session-20260422-daily"

	// Write only the session-scoped outcome file.
	writeTestSessionArtifacts(
		t, baseDir, sessionID,
		domain.SessionSummary{SessionID: sessionID, Regime: domain.RegimeRiskOn, RecordedAt: recordedAt, OutcomeCount: 1},
		domain.RecommendationOutcome{
			AgentID:             "agent-session",
			Skill:               "growth_momentum",
			Layer:               domain.LayerStyle,
			Symbol:              "2330.TW",
			Side:                domain.SideBuy,
			Conviction:          88,
			TargetPrice:         1050,
			StopLossPrice:       980,
			Reason:              "breakout",
			FactorScores:        domain.FactorScores{Total: 0.91},
			ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 70, Floor: 50, Final: 88},
		},
	)

	svc := NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	data, err := svc.LoadAgentObservatory(sessionID, 10)
	if err != nil {
		t.Fatalf("LoadAgentObservatory: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	// Verify the outcome was loaded from the session-scoped file.
	found := false
	for _, sc := range data.Scorecards {
		if sc.AgentID == "agent-session" {
			found = true
			break
		}
	}
	// The regression: current code calls store.LoadOutcomes() which reads the global
	// root file (which is empty / non-existent in this test), so the scorecard list is empty.
	if !found {
		t.Fatal("LoadAgentObservatory did not read from session-scoped recommendation_outcomes.jsonl; got outcome from global root file instead")
	}
}

// Test (c): LoadForecastVsReality / loadSymbolPredictions should read symbol predictions
// from the selected session's recommendation_outcomes.jsonl, not the global root file.
func TestLoadForecastVsRealityReadsPredictionsFromSelectedSession(t *testing.T) {
	baseDir := t.TempDir()
	recordedAt := time.Date(2026, time.April, 22, 4, 2, 30, 0, time.UTC)
	sessionID := "session-20260422-daily"

	// Write a session-scoped outcome with a ForwardReturn so Hit can be determined.
	writeTestSessionArtifacts(
		t, baseDir, sessionID,
		domain.SessionSummary{SessionID: sessionID, Regime: domain.RegimeRiskOn, RecordedAt: recordedAt, OutcomeCount: 1},
		domain.RecommendationOutcome{
			AgentID:       "agent-fvr",
			Skill:         "value_yield",
			Layer:         domain.LayerStyle,
			Symbol:        "2317.TW",
			Side:          domain.SideBuy,
			Conviction:    72,
			TargetPrice:   155.5,
			StopLossPrice: 146.0,
			ForwardReturn: 0.021,
			Reason:        "cheap",
			Price:         150.0,
			PassedGuards:  true,
			RecordedAt:    recordedAt,
		},
	)

	svc := NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	data, err := svc.LoadForecastVsReality("", 50)
	if err != nil {
		t.Fatalf("LoadForecastVsReality: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	// The regression: loadSymbolPredictions hard-codes the root path
	//   filepath.Join(s.LedgerDir, "recommendation_outcomes.jsonl")
	// instead of the selected session's file. So this assertion fails.
	found := false
	for _, p := range data.SymbolPredictions {
		if p.AgentID == "agent-fvr" && p.Symbol == "2317.TW" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("loadSymbolPredictions did not read from selected session's recommendation_outcomes.jsonl; got predictions from global root file instead")
	}
}

func TestReportServiceLoadRecommendationsForDateSupportsCanonicalOutcomeJSON(t *testing.T) {
	baseDir := t.TempDir()
	recordedAt := time.Date(2026, time.April, 22, 4, 2, 30, 0, time.UTC)
	sessionID := "session-20260422-daily"
	writeTestSessionArtifacts(
		t, baseDir, sessionID,
		domain.SessionSummary{SessionID: sessionID, Regime: domain.RegimeRiskOn, RecordedAt: recordedAt},
		domain.RecommendationOutcome{
			AgentID:             "agent-1",
			Skill:               "growth_momentum",
			Layer:               domain.LayerStyle,
			Symbol:              "2330.TW",
			Side:                domain.SideBuy,
			Conviction:          88,
			TargetPrice:         1050,
			StopLossPrice:       980,
			Reason:              "breakout",
			FactorScores:        domain.FactorScores{Total: 0.91},
			ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 70, Floor: 50, Final: 88},
		},
	)

	svc := NewReportService(baseDir, baseDir, nil)
	recs := svc.loadRecommendationsForDate("2026-04-22")
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if recs[0].Agent != "agent-1" {
		t.Fatalf("agent: got %q", recs[0].Agent)
	}
	if recs[0].Symbol != "2330.TW" {
		t.Fatalf("symbol: got %q", recs[0].Symbol)
	}
	if recs[0].FactorScores.Total != 0.91 {
		t.Fatalf("factor_scores.total: got %v", recs[0].FactorScores.Total)
	}
}

func TestPipelineServiceLoadRecommendationPipelineSupportsCanonicalOutcomeJSON(t *testing.T) {
	baseDir := t.TempDir()
	recordedAt := time.Date(2026, time.April, 22, 4, 2, 30, 0, time.UTC)
	sessionID := "session-20260422-daily"
	writeTestSessionArtifacts(
		t, baseDir, sessionID,
		domain.SessionSummary{SessionID: sessionID, Regime: domain.RegimeRiskOn, RecordedAt: recordedAt},
		domain.RecommendationOutcome{
			AgentID:             "agent-2",
			Skill:               "value_yield",
			Layer:               domain.LayerStyle,
			Symbol:              "2317.TW",
			Side:                domain.SideBuy,
			Conviction:          72,
			TargetPrice:         155.5,
			StopLossPrice:       146.0,
			ForwardReturn:       0.015,
			Reason:              "cheap",
			Price:               150.0,
			PassedGuards:        true,
			GuardReason:         "ok",
			RecordedAt:          recordedAt,
			FactorScores:        domain.FactorScores{Total: 0.67},
			ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 60, Floor: 50, Final: 72},
		},
	)

	svc := NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	data, err := svc.LoadRecommendationPipeline(sessionID, true)
	if err != nil {
		t.Fatalf("load recommendation pipeline: %v", err)
	}
	if data == nil {
		t.Fatalf("expected pipeline data")
	}
	if len(data.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(data.Items))
	}
	if data.Items[0].AgentID != "agent-2" {
		t.Fatalf("agent_id: got %q", data.Items[0].AgentID)
	}
	if data.Items[0].Symbol != "2317.TW" {
		t.Fatalf("symbol: got %q", data.Items[0].Symbol)
	}
	if data.Items[0].FactorScores.Total != 0.67 {
		t.Fatalf("factor_scores.total: got %v", data.Items[0].FactorScores.Total)
	}
}
