package dailyreport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// testReport builds a report with a period section for workflow tests.
func testReport(date string) *Report {
	return &Report{
		Date:           date,
		Generated:      time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC),
		WorkflowStatus: WorkflowNeedsReview,
		Summary:        "盤勢中性（NEUTRAL）",
		Strategy: StrategySection{
			Active:    "all_weather",
			EntryCond: "等待回測支撐區間",
			Direction: "偏多",
		},
		Risk: RiskSection{
			StressIndex:   0.3,
			DrawdownAlert: false,
			RiskLevel:     "moderate",
		},
		Period: &PeriodSection{
			MarketPeriod:      "consolidation",
			PeriodNameZH:      "盤整",
			CashReserve:       35,
			AllowedStrategies: []string{"event_arbitrage", "all_weather"},
			Confidence:        0.6,
			ConditionsHit:     1,
			ConditionsTotal:   4,
		},
	}
}

func TestGenerate_SetsNeedsReview(t *testing.T) {
	dir := t.TempDir()
	gen := NewGenerator(dir)
	rep := gen.Generate()
	if rep.WorkflowStatus != WorkflowNeedsReview {
		t.Errorf("Generate() WorkflowStatus = %q, want %q", rep.WorkflowStatus, WorkflowNeedsReview)
	}
	if len(rep.RevisionHistory) != 0 {
		t.Errorf("fresh report should have no revision history, got %d entries", len(rep.RevisionHistory))
	}
}

func TestReport_CanTransitionTo(t *testing.T) {
	r := testReport("2026-08-10")
	r.WorkflowStatus = WorkflowNeedsReview

	if !r.CanTransitionTo(WorkflowCorrected) {
		t.Error("needs_review should allow -> corrected")
	}
	if !r.CanTransitionTo(WorkflowApproved) {
		t.Error("needs_review should allow -> approved")
	}
	if r.CanTransitionTo(WorkflowGenerated) {
		t.Error("needs_review should NOT allow -> generated (no rollback)")
	}
}

func TestLegacyReport_EmptyStatus_AllowsReviewTransitions(t *testing.T) {
	r := testReport("2026-08-10")
	r.WorkflowStatus = "" // pre-workflow persisted report

	if err := r.ApplyRevision(ReviseRequest{
		Note: "legacy correction",
		Fields: []ReviseField{
			{Path: "strategy.active_strategy", Value: "momentum"},
		},
	}); err != nil {
		t.Fatalf("legacy report revise should succeed: %v", err)
	}
	if r.WorkflowStatus != WorkflowCorrected {
		t.Errorf("status = %q, want corrected", r.WorkflowStatus)
	}
}

func TestApplyRevision_GeneratedToCorrected(t *testing.T) {
	r := testReport("2026-08-10")
	r.WorkflowStatus = WorkflowGenerated

	// Strict machine: generated must first pass through needs_review.
	if err := r.ApplyRevision(ReviseRequest{Note: "skip review", Fields: []ReviseField{{Path: "risk.risk_level", Value: "high"}}}); err == nil {
		t.Fatal("expected error revising directly from generated state")
	}
	if err := r.MarkNeedsReview(); err != nil {
		t.Fatalf("MarkNeedsReview: %v", err)
	}
	if r.WorkflowStatus != WorkflowNeedsReview {
		t.Fatalf("status = %q, want needs_review", r.WorkflowStatus)
	}

	err := r.ApplyRevision(ReviseRequest{
		Note: "人工訂正",
		By:   "ops-1",
		Fields: []ReviseField{
			{Path: "strategy.active_strategy", Value: "momentum"},
			{Path: "risk.risk_level", Value: "high"},
		},
	})
	if err != nil {
		t.Fatalf("ApplyRevision: %v", err)
	}
	if r.WorkflowStatus != WorkflowCorrected {
		t.Errorf("status = %q, want corrected", r.WorkflowStatus)
	}
	if r.Strategy.Active != "momentum" {
		t.Errorf("strategy.active_strategy = %q, want momentum", r.Strategy.Active)
	}
	if r.Risk.RiskLevel != "high" {
		t.Errorf("risk.risk_level = %q, want high", r.Risk.RiskLevel)
	}
	if r.RevisedBy != "ops-1" || r.RevisionNote != "人工訂正" {
		t.Errorf("revised_by/note = %q/%q", r.RevisedBy, r.RevisionNote)
	}
	if len(r.RevisionHistory) != 1 {
		t.Fatalf("revision history len = %d, want 1", len(r.RevisionHistory))
	}
	entry := r.RevisionHistory[0]
	if entry.By != "ops-1" || entry.Note != "人工訂正" {
		t.Errorf("entry by/note = %q/%q", entry.By, entry.Note)
	}
	if len(entry.FieldChanges) != 2 {
		t.Fatalf("field changes = %d, want 2", len(entry.FieldChanges))
	}
	if entry.FieldChanges[0].Path != "risk.risk_level" || entry.FieldChanges[0].OldValue != "moderate" || entry.FieldChanges[0].NewValue != "high" {
		t.Errorf("field change 0 = %+v", entry.FieldChanges[0])
	}
}

func TestApplyRevision_SecondRevisionAppendsHistory(t *testing.T) {
	r := testReport("2026-08-10")
	_ = r.ApplyRevision(ReviseRequest{Note: "first", Fields: []ReviseField{{Path: "strategy.direction", Value: "偏空"}}})
	_ = r.ApplyRevision(ReviseRequest{Note: "second", Fields: []ReviseField{{Path: "risk.warning", Value: "警戒：波動放大"}}})

	if len(r.RevisionHistory) != 2 {
		t.Fatalf("history len = %d, want 2", len(r.RevisionHistory))
	}
	if r.RevisionHistory[1].Note != "second" {
		t.Errorf("latest entry note = %q, want second", r.RevisionHistory[1].Note)
	}
	if r.Risk.Warning != "警戒：波動放大" {
		t.Errorf("risk.warning = %q", r.Risk.Warning)
	}
	if r.WorkflowStatus != WorkflowCorrected {
		t.Errorf("status = %q, want corrected", r.WorkflowStatus)
	}
}

func TestApplyRevision_ApprovedToCorrected(t *testing.T) {
	r := testReport("2026-08-10")
	_ = r.Approve()
	if r.WorkflowStatus != WorkflowApproved {
		t.Fatalf("status = %q, want approved", r.WorkflowStatus)
	}
	// Post-approval correction is allowed and becomes the downstream version.
	if err := r.ApplyRevision(ReviseRequest{Note: "post-approval fix", Fields: []ReviseField{{Path: "risk.risk_level", Value: "extreme"}}}); err != nil {
		t.Fatalf("approved -> corrected revise should succeed: %v", err)
	}
	if r.WorkflowStatus != WorkflowCorrected {
		t.Errorf("status = %q, want corrected", r.WorkflowStatus)
	}
}

func TestApplyRevision_WhitelistRejection(t *testing.T) {
	r := testReport("2026-08-10")

	// Non-whitelisted field (machine-owned provenance) must be rejected.
	err := r.ApplyRevision(ReviseRequest{
		Note:   "hack",
		Fields: []ReviseField{{Path: "global.summary", Value: "篡改"}},
	})
	if err == nil {
		t.Fatal("expected error for non-whitelisted field")
	}
	if !strings.Contains(err.Error(), "not whitelisted") {
		t.Errorf("error = %v, want whitelist mention", err)
	}
	// The failed revision must not touch the report.
	if r.WorkflowStatus != WorkflowNeedsReview || len(r.RevisionHistory) != 0 {
		t.Errorf("failed revision mutated report: status=%q history=%d", r.WorkflowStatus, len(r.RevisionHistory))
	}
}

func TestApplyRevision_TypeCoercion(t *testing.T) {
	r := testReport("2026-08-10")
	err := r.ApplyRevision(ReviseRequest{
		Note: "coercion",
		Fields: []ReviseField{
			{Path: "period.cash_reserve", Value: 50},    // Go int → float64
			{Path: "period.conditions_hit", Value: 3.0}, // JSON float64 integral → int
			{Path: "period.allowed_strategies", Value: []any{"momentum", "event_arbitrage"}},
			{Path: "risk.drawdown_alert", Value: true},
			{Path: "risk.stress_index", Value: 0.95},
		},
	})
	if err != nil {
		t.Fatalf("ApplyRevision: %v", err)
	}
	if r.Period.CashReserve != 50 {
		t.Errorf("cash_reserve = %v, want 50", r.Period.CashReserve)
	}
	if r.Period.ConditionsHit != 3 {
		t.Errorf("conditions_hit = %d, want 3", r.Period.ConditionsHit)
	}
	if len(r.Period.AllowedStrategies) != 2 || r.Period.AllowedStrategies[0] != "momentum" {
		t.Errorf("allowed_strategies = %v", r.Period.AllowedStrategies)
	}
	if !r.Risk.DrawdownAlert || r.Risk.StressIndex != 0.95 {
		t.Errorf("risk fields not applied: %+v", r.Risk)
	}
}

func TestApplyRevision_BadTypesRejected(t *testing.T) {
	r := testReport("2026-08-10")
	err := r.ApplyRevision(ReviseRequest{
		Note:   "bad type",
		Fields: []ReviseField{{Path: "risk.stress_index", Value: "high"}},
	})
	if err == nil {
		t.Fatal("expected type error for string into numeric field")
	}
	err = r.ApplyRevision(ReviseRequest{
		Note:   "bad int",
		Fields: []ReviseField{{Path: "period.conditions_hit", Value: 3.5}},
	})
	if err == nil {
		t.Fatal("expected error for non-integral float into int field")
	}
	err = r.ApplyRevision(ReviseRequest{
		Note:   "bad bool",
		Fields: []ReviseField{{Path: "risk.drawdown_alert", Value: "yes"}},
	})
	if err == nil {
		t.Fatal("expected error for string into bool field")
	}
}

func TestApplyRevision_DuplicateFieldRejected(t *testing.T) {
	r := testReport("2026-08-10")
	err := r.ApplyRevision(ReviseRequest{
		Note: "dup",
		Fields: []ReviseField{
			{Path: "risk.risk_level", Value: "high"},
			{Path: "risk.risk_level", Value: "low"},
		},
	})
	if err == nil {
		t.Fatal("expected error for duplicate field")
	}
}

func TestApplyRevision_NoFieldsRejected(t *testing.T) {
	r := testReport("2026-08-10")
	if err := r.ApplyRevision(ReviseRequest{Note: "empty"}); err == nil {
		t.Fatal("expected error for empty fields")
	}
}

func TestApplyRevision_PeriodFieldOnNilPeriodRejected(t *testing.T) {
	r := testReport("2026-08-10")
	r.Period = nil
	err := r.ApplyRevision(ReviseRequest{
		Note:   "period on nil",
		Fields: []ReviseField{{Path: "period.cash_reserve", Value: 50}},
	})
	if err == nil {
		t.Fatal("expected error for period field on report without period section")
	}
}

func TestApprove_NeedsReviewToApproved(t *testing.T) {
	r := testReport("2026-08-10")
	if err := r.Approve(); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if r.WorkflowStatus != WorkflowApproved {
		t.Errorf("status = %q, want approved", r.WorkflowStatus)
	}
	// Approving twice is idempotent.
	if err := r.Approve(); err != nil {
		t.Fatalf("second Approve should be idempotent: %v", err)
	}
}

func TestReportJSON_BackwardCompat(t *testing.T) {
	// A legacy report payload (no workflow fields) must decode with empty
	// workflow state and remain marshallable without new required fields.
	legacy := `{"date":"2026-08-10","generated_at":"2026-08-10T06:00:00Z","summary":"s","global":{},"capital":{},"events":{},"strategy":{"active_strategy":"all_weather"},"risk":{"risk_level":"moderate"}}`
	var r Report
	if err := json.Unmarshal([]byte(legacy), &r); err != nil {
		t.Fatalf("decode legacy: %v", err)
	}
	if r.WorkflowStatus != "" {
		t.Errorf("legacy WorkflowStatus = %q, want empty", r.WorkflowStatus)
	}
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	s := string(out)
	for _, forbidden := range []string{"revision_history", "workflow_status"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("legacy round-trip unexpectedly contains %q: %s", forbidden, s)
		}
	}
}
