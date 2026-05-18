package taskexec

import (
	"context"
	"testing"
	"time"

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

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	sink := &mockEventSink{}
	err := r.Run(ctx, SubmitRequest{}, sink)

	if err == nil {
		t.Fatal("expected error for empty workDir, got nil")
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
