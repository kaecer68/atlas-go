package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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

func TestReportServiceLoadRecommendationsForDateSupportsCanonicalOutcomeJSON(t *testing.T) {
	baseDir := t.TempDir()
	recordedAt := time.Date(2026, time.April, 22, 4, 2, 30, 0, time.UTC)
	sessionID := "session-20260422-daily"
	writeTestSessionArtifacts(t, baseDir, sessionID,
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

	svc := NewReportService(baseDir, baseDir)
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
	writeTestSessionArtifacts(t, baseDir, sessionID,
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

	svc := NewPipelineService(baseDir, baseDir)
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
