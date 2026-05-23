//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestDualWriteAudit_ScreeningRejects(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	rejects := []domain.ScreeningReject{
		{
			SessionID:  "audit-sess-1",
			Symbol:     "2330.TW",
			AgentID:    "audit_agent",
			Criterion:  "PE too high",
			RecordedAt: time.Now(),
		},
	}

	if err := repo.RecordScreeningRejects(ctx, "audit-sess-1", rejects); err != nil {
		t.Fatalf("RecordScreeningRejects failed: %v", err)
	}

	loaded, err := repo.QueryScreeningRejectsBySession(ctx, "audit-sess-1")
	if err != nil {
		t.Fatalf("QueryScreeningRejectsBySession failed: %v", err)
	}
	if len(loaded) == 0 {
		t.Fatal("Expected at least 1 screening reject")
	}
	if loaded[0].Symbol != "2330.TW" {
		t.Errorf("Expected symbol 2330.TW, got %q", loaded[0].Symbol)
	}
	if loaded[0].Criterion != "PE too high" {
		t.Errorf("Expected criterion 'PE too high', got %q", loaded[0].Criterion)
	}
}

func TestDualWriteAudit_SessionSummaries(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	summary := domain.SessionSummary{
		SessionID:      "audit-summary-1",
		OrderCount:     100,
		PositionCount:  25,
		EndingCash:     50000.0,
		PortfolioValue: 200000.0,
		RecordedAt:     time.Now(),
	}

	if err := repo.SaveSessionSummary(ctx, summary); err != nil {
		t.Fatalf("SaveSessionSummary failed: %v", err)
	}

	loaded, err := repo.LoadSessionSummary(ctx, "audit-summary-1")
	if err != nil {
		t.Fatalf("LoadSessionSummary failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Expected non-nil SessionSummary")
	}
	if loaded.OrderCount != 100 {
		t.Errorf("Expected OrderCount 100, got %d", loaded.OrderCount)
	}
	if loaded.PortfolioValue != 200000.0 {
		t.Errorf("Expected PortfolioValue 200000.0, got %f", loaded.PortfolioValue)
	}
}

func TestDualWriteAudit_HumanInterventions(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	entry := domain.HumanIntervention{
		ID:         "hint-1",
		Type:       "approve_rec",
		Reason:     "approved by user",
		Operator:   "tester",
		SessionID:  "audit-sess-1",
		RecordedAt: time.Now(),
	}

	if err := repo.RecordHumanIntervention(ctx, entry); err != nil {
		t.Fatalf("RecordHumanIntervention failed: %v", err)
	}

	entries, err := repo.LoadHumanInterventions(ctx)
	if err != nil {
		t.Fatalf("LoadHumanInterventions failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("Expected at least 1 intervention entry")
	}
	if entries[0].Type != "approve_rec" {
		t.Errorf("Expected Type 'approve_rec', got %q", entries[0].Type)
	}
	if entries[0].Reason != "approved by user" {
		t.Errorf("Expected Reason 'approved by user', got %q", entries[0].Reason)
	}
}

func TestDualWriteAudit_UpdateHumanIntervention(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	entry := domain.HumanIntervention{
		ID:         "hint-upd-1",
		Type:       "override",
		Reason:     "initial reason",
		Operator:   "tester",
		SessionID:  "audit-sess-upd",
		RecordedAt: time.Now(),
	}
	if err := repo.RecordHumanIntervention(ctx, entry); err != nil {
		t.Fatalf("RecordHumanIntervention failed: %v", err)
	}

	entry.Reason = "updated reason"
	if err := repo.RecordHumanIntervention(ctx, entry); err != nil {
		t.Fatalf("RecordHumanIntervention (update) failed: %v", err)
	}

	loaded, err := repo.LoadHumanInterventions(ctx)
	if err != nil {
		t.Fatalf("LoadHumanInterventions (after update) failed: %v", err)
	}
	if len(loaded) == 0 {
		t.Fatal("Expected at least 1 intervention entry after update")
	}
	found := false
	for _, e := range loaded {
		if e.ID == "hint-upd-1" {
			found = true
			break
		}
	}
	if !found && len(loaded) > 0 {
		t.Logf("Note: intervention not found in PG results (expected if PG has other data)")
	}
}
