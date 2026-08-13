//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestDualWriteAlerts_SaveAndLoadAll(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	alert := domain.AlertRecord{
		ID:        "test-alert-int-1",
		Timestamp: time.Now(),
		Rule:      "test-rule",
		Severity:  "warning",
		Message:   "integration test alert",
		Value:     0.95,
		Threshold: 0.80,
	}

	if err := repo.SaveAlert(ctx, alert); err != nil {
		t.Fatalf("SaveAlert failed: %v", err)
	}

	alerts, err := repo.LoadAllAlerts(ctx, 10)
	if err != nil {
		t.Fatalf("LoadAllAlerts failed: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.ID == "test-alert-int-1" {
			found = true
			if a.Severity != "warning" {
				t.Errorf("Expected severity 'warning', got %q", a.Severity)
			}
			if a.Rule != "test-rule" {
				t.Errorf("Expected rule 'test-rule', got %q", a.Rule)
			}
		}
	}
	if !found {
		t.Fatal("Saved alert not found in LoadAllAlerts results")
	}
}

func TestDualWriteAlerts_UnacknowledgedAndAcknowledge(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	alert := domain.AlertRecord{
		ID:           "test-alert-ack-1",
		Timestamp:    time.Now(),
		Rule:         "ack-test",
		Severity:     "critical",
		Message:      "needs ack",
		Acknowledged: false,
	}

	if err := repo.SaveAlert(ctx, alert); err != nil {
		t.Fatalf("SaveAlert failed: %v", err)
	}

	// Should appear in unacknowledged list
	unacked, err := repo.LoadUnacknowledgedAlerts(ctx)
	if err != nil {
		t.Fatalf("LoadUnacknowledgedAlerts failed: %v", err)
	}
	found := false
	for _, a := range unacked {
		if a.ID == "test-alert-ack-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Unacknowledged alert not found in LoadUnacknowledgedAlerts")
	}

	// Acknowledge it
	if err := repo.AcknowledgeAlert(ctx, "test-alert-ack-1", "tester"); err != nil {
		t.Fatalf("AcknowledgeAlert failed: %v", err)
	}

	// Should no longer appear in unacknowledged list
	unacked2, err := repo.LoadUnacknowledgedAlerts(ctx)
	if err != nil {
		t.Fatalf("LoadUnacknowledgedAlerts (after ack) failed: %v", err)
	}
	for _, a := range unacked2 {
		if a.ID == "test-alert-ack-1" {
			t.Fatal("Alert still appears as unacknowledged after AcknowledgeAlert")
		}
	}
}

func TestDualWriteAlerts_LoadBySeverity(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	alerts := []domain.AlertRecord{
		{ID: "alert-sev-1", Timestamp: time.Now(), Rule: "r1", Severity: "critical", Message: "critical alert"},
		{ID: "alert-sev-2", Timestamp: time.Now(), Rule: "r2", Severity: "warning", Message: "warning alert"},
		{ID: "alert-sev-3", Timestamp: time.Now(), Rule: "r3", Severity: "info", Message: "info alert"},
	}
	for _, a := range alerts {
		if err := repo.SaveAlert(ctx, a); err != nil {
			t.Fatalf("SaveAlert(%s) failed: %v", a.ID, err)
		}
	}

	critical, err := repo.LoadAlertsBySeverity(ctx, "critical", 10)
	if err != nil {
		t.Fatalf("LoadAlertsBySeverity failed: %v", err)
	}
	if len(critical) == 0 {
		t.Fatal("Expected at least 1 critical alert")
	}
	if critical[0].ID != "alert-sev-1" {
		t.Errorf("Expected alert-sev-1, got %q", critical[0].ID)
	}
}

func TestDualWriteAlerts_LoadByTimeRange(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	alert := domain.AlertRecord{
		ID: "alert-time-1", Timestamp: time.Now(), Rule: "r1", Severity: "info", Message: "time range test",
	}
	if err := repo.SaveAlert(ctx, alert); err != nil {
		t.Fatalf("SaveAlert failed: %v", err)
	}

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	results, err := repo.LoadAlertsByTimeRange(ctx, start, end)
	if err != nil {
		t.Fatalf("LoadAlertsByTimeRange failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Expected at least 1 alert in time range")
	}
}

func TestDualWriteAlerts_QuerySessions(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	// QuerySessions needs outcomes to be present. A01: RecordOutcomes is the
	// GLOBAL write path — rows land with session_id='' (Window is preserved in
	// metadata only), so the session query groups them under the empty
	// (global) session. Session-scoped rows are written via
	// PostgresLedgerStore.RecordSessionOutcomes (session.ID), which is not
	// reachable through the dual-write repository.
	outcomes := []domain.RecommendationOutcome{
		{Window: "session-001", Symbol: "2330.TW", AgentID: "agent1"},
	}
	if err := repo.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	sessions, err := repo.QuerySessions(ctx)
	if err != nil {
		t.Fatalf("QuerySessions failed: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("Expected at least 1 session from QuerySessions")
	}
	if sessions[0].SessionID != "" {
		t.Errorf("Expected global (empty) session_id for RecordOutcomes rows, got %q", sessions[0].SessionID)
	}
	if sessions[0].OutcomeCount != 1 {
		t.Errorf("Expected outcome_count 1, got %d", sessions[0].OutcomeCount)
	}
}
