package reporting

import (
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRenderASCIIChart(t *testing.T) {
	values := []float64{1000, 1100, 1050, 1200, 1150}
	chart := RenderASCIIChart(values, 20, 5)
	if chart == "" || chart == "(no data)" {
		t.Fatal("expected non-empty chart")
	}
	if !strings.Contains(chart, "*") {
		t.Error("expected chart to contain data points")
	}
}

func TestRenderAgentPerformanceTable(t *testing.T) {
	rows := []AgentPerformanceRow{
		{AgentID: "agent_a", Layer: "sector", WindowCount: 5, HitRate: 0.6, SharpeLike: 1.2, MaxDrawdown: 0.05, Weight: 1.5},
		{AgentID: "agent_b", Layer: "style", WindowCount: 3, HitRate: 0.4, SharpeLike: 0.3, MaxDrawdown: 0.10, Weight: 0.8},
	}
	md := RenderAgentPerformanceTable(rows)
	if !strings.Contains(md, "agent_a") {
		t.Error("expected table to contain agent_a")
	}
	if !strings.Contains(md, "Sharpe") {
		t.Error("expected table header with Sharpe")
	}
}

func TestRenderMutationSummary(t *testing.T) {
	stats := MutationStats{Total: 10, Kept: 6, Reverted: 3, Pending: 1, SurvivalRate: 0.6}
	md := RenderMutationSummary(stats)
	if !strings.Contains(md, "10") {
		t.Error("expected total count in summary")
	}
	if !strings.Contains(md, "60.0%") {
		t.Error("expected survival rate in summary")
	}
}

func TestRenderMarkdownReport(t *testing.T) {
	data := BacktestReportData{
		WindowID:     "window-20260101-20260131",
		StartDate:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		SessionCount: 20,
		OutcomeCount: 100,
		EquityCurve:  []float64{1_000_000, 1_020_000, 1_010_000, 1_050_000},
		AgentRows: []AgentPerformanceRow{
			{AgentID: "a", Layer: "sector", WindowCount: 5, HitRate: 0.5, SharpeLike: 0.8, MaxDrawdown: 0.03, Weight: 1.0},
		},
		MutationStats:   MutationStats{Total: 5, Kept: 3, Reverted: 1, Pending: 1, SurvivalRate: 0.75},
		WorstAgentID:    "bad_agent",
		WorstAgentSkill: "value",
		WorstSharpeLike: -0.2,
		RegimeCounts:    map[string]int{"RISK_ON": 15, "RISK_OFF": 5},
	}
	md := RenderMarkdown(data)
	if !strings.Contains(md, "window-20260101-20260131") {
		t.Error("expected window id in report")
	}
	if !strings.Contains(md, "Equity Curve") {
		t.Error("expected equity curve section")
	}
	if !strings.Contains(md, "Agent Performance") {
		t.Error("expected agent performance section")
	}
	if !strings.Contains(md, "Mutation Summary") {
		t.Error("expected mutation summary section")
	}
	if !strings.Contains(md, "bad_agent") {
		t.Error("expected worst agent in report")
	}
}

func TestBuildAgentRows(t *testing.T) {
	scorecards := []domain.Scorecard{
		{AgentID: "x", Skill: "tech", WindowCount: 2, HitRate: 0.5, SharpeLike: 0.4, MaxDrawdown: 0.02},
	}
	weights := map[string]float64{"x": 1.5}
	rows := BuildAgentRows(scorecards, weights)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Weight != 1.5 {
		t.Errorf("expected weight 1.5, got %f", rows[0].Weight)
	}
}

func TestRenderMarkdown_CoversAllSummaryFields(t *testing.T) {
	summary := domain.BacktestWindowSummary{
		WindowID:              "window-20260101-20260131",
		StartDate:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:               time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		SessionCount:          20,
		OutcomeCount:          100,
		WorstAgentID:          "bad_agent",
		WorstAgentSkill:       "value",
		WorstAgentLayer:       "style",
		WorstAgentWindowCount: 5,
		WorstAgentSharpeLike:  -0.25,
		GeneratedAt:           time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC),
	}

	data := BacktestReportData{
		WindowID:        summary.WindowID,
		StartDate:       summary.StartDate,
		EndDate:         summary.EndDate,
		SessionCount:    summary.SessionCount,
		OutcomeCount:    summary.OutcomeCount,
		EquityCurve:     []float64{1_000_000, 1_020_000},
		AgentRows:       []AgentPerformanceRow{{AgentID: "a", Layer: "sector", WindowCount: 5, HitRate: 0.5, SharpeLike: 0.8, MaxDrawdown: 0.03, Weight: 1.0}},
		MutationStats:   MutationStats{Total: 5, Kept: 3, Reverted: 1, Pending: 1, SurvivalRate: 0.75},
		WorstAgentID:    summary.WorstAgentID,
		WorstAgentSkill: summary.WorstAgentSkill,
		WorstSharpeLike: summary.WorstAgentSharpeLike,
		RegimeCounts:    map[string]int{"RISK_ON": 15, "RISK_OFF": 5},
	}

	md := RenderMarkdown(data)

	fields := map[string]string{
		summary.WindowID:       "WindowID",
		"2026-01-01":           "StartDate",
		"2026-01-31":           "EndDate",
		"20":                   "SessionCount",
		"100":                  "OutcomeCount",
		"bad_agent":            "WorstAgentID",
		"value":                "WorstAgentSkill",
		"-0.2500":              "WorstSharpeLike",
		"Equity Curve":         "Section",
		"Agent Performance":    "Section",
		"Mutation Summary":     "Section",
		"Regime Distribution":  "Section",
		"Experiment Candidate": "Section",
	}

	for expected, fieldName := range fields {
		if !strings.Contains(md, expected) {
			t.Errorf("field %s (value: %q) not found in rendered report", fieldName, expected)
		}
	}
}
