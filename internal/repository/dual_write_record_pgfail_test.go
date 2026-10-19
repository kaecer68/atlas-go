package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// newFailingPGDualWrite builds a DualWriteRepository whose PostgreSQL backend is
// USABLE (the pgUsable() SELECT 1 probe succeeds) but whose metric write fails.
// This is the fault-injection seam that FU-20261019-01 asks for: the pool is
// already an interface (pgPool), so no production change is required.
func newFailingPGDualWrite() *DualWriteRepository {
	pool := &fakePGPool{
		// The pgUsable() probe must SUCCEED (fakeRow{values:[1]} scans into the
		// probe int); only the metric INSERT is made to fail.
		queryRowFunc: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{values: []any{1}}
		},
		execFunc: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("injected metric write failure")
		},
	}
	return &DualWriteRepository{
		pg:    &PostgresRepository{pool: pool},
		jsonl: &JSONLRepository{metricsStore: &mockMetricsStore{}},
	}
}

// TestDualWriteMetrics_RecordPGFailureIsObservable pins the contract that a
// failed PostgreSQL metric write is NOT silent: FU-20261016-06 changed the old
// `return nil` swallow into a counter + ERROR log. Mutation: deleting the
// metricsPGErrorTotal.Add(1) call makes this test fail.
func TestDualWriteMetrics_RecordPGFailureIsObservable(t *testing.T) {
	repo := newFailingPGDualWrite()
	ctx := context.Background()

	before := MetricsPGErrorTotal()

	// Record stays best-effort: it must not fail the caller.
	if err := repo.Record(ctx, "test_metric", 42.0, map[string]string{"agent_id": "a1"}); err != nil {
		t.Fatalf("Record must not propagate PG errors (best effort): %v", err)
	}

	if got := MetricsPGErrorTotal(); got <= before {
		t.Fatalf("PG write failure was swallowed: MetricsPGErrorTotal()=%d, want > %d", got, before)
	}
}

// Positive control: a HEALTHY pool must not increment the failure counter.
// Without this the test above could pass on any counter bump.
func TestDualWriteMetrics_RecordPGSuccessNoErrorCounter(t *testing.T) {
	repo := &DualWriteRepository{
		pg: &PostgresRepository{pool: &fakePGPool{
			queryRowFunc: func(context.Context, string, ...any) pgx.Row { return fakeRow{values: []any{1}} },
		}},
		jsonl: &JSONLRepository{metricsStore: &mockMetricsStore{}},
	}
	ctx := context.Background()

	before := MetricsPGErrorTotal()
	if err := repo.Record(ctx, "test_metric", 42.0, map[string]string{"agent_id": "a1"}); err != nil {
		t.Fatalf("Record with healthy PG: %v", err)
	}
	if got := MetricsPGErrorTotal(); got != before {
		t.Fatalf("healthy PG write must not bump the failure counter: got %d, want %d", got, before)
	}
}
