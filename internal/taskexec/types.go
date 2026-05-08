package taskexec

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type SubmitRequest struct {
	TaskType       string         `json:"task_type"`
	Actor          string         `json:"actor"`
	ActorSource    string         `json:"actor_source"`
	Payload        map[string]any `json:"payload"`
	Confirmed      bool           `json:"confirmed"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
}

type EventSink interface {
	Emit(event domain.TaskExecutionEvent)
	ExecutionID() string
	RecordLineage(lineage domain.ExperimentLineageRecord) error
	RecordBaselineHistory(record domain.BaselineHistoryRecord) error
	RecordMetrics(points []domain.MetricTrendPoint) error
}

type Runner interface {
	Name() string
	Run(ctx context.Context, req SubmitRequest, sink EventSink) error
}
