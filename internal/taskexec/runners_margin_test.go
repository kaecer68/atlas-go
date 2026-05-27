package taskexec

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestMarginBackfillRunner_Name(t *testing.T) {
	r := NewMarginBackfillRunner("/tmp/test")
	if r.Name() != "margin-backfill" {
		t.Fatalf("expected name 'margin-backfill', got %s", r.Name())
	}
}

func TestMarginBackfillRunner_Run_ContextCancellation(t *testing.T) {
	r := NewMarginBackfillRunner("/tmp/test")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sink := &mockEventSink{}
	err := r.Run(ctx, SubmitRequest{}, sink)

	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestMarginBackfillRunner_Run_EmptyWorkDir(t *testing.T) {
	r := NewMarginBackfillRunner("")

	// Runner validates workDir before any async operations,
	// so no timing dependency — error is returned immediately.
	sink := &mockEventSink{}
	err := r.Run(context.Background(), SubmitRequest{}, sink)

	if err == nil {
		t.Fatal("expected error for empty workDir, got nil")
	}
	if err.Error() != "margin backfill: workDir is required" {
		t.Fatalf("expected 'margin backfill: workDir is required', got %q", err.Error())
	}
}

type mockEventSink struct {
	events []domain.TaskExecutionEvent
}

func (m *mockEventSink) Emit(e domain.TaskExecutionEvent) {
	m.events = append(m.events, e)
}

func (m *mockEventSink) ExecutionID() string { return "test-exec-id" }

func (m *mockEventSink) RecordLineage(lineage domain.ExperimentLineageRecord) error { return nil }

func (m *mockEventSink) RecordBaselineHistory(record domain.BaselineHistoryRecord) error { return nil }

func (m *mockEventSink) RecordMetrics(points []domain.MetricTrendPoint) error { return nil }
