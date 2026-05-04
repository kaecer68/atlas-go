package taskexec

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Store interface {
	CreateExecution(ctx context.Context, exec domain.TaskExecution) error
	UpdateExecution(ctx context.Context, exec domain.TaskExecution) error
	GetExecution(ctx context.Context, id string) (*domain.TaskExecution, error)
	ListExecutions(ctx context.Context, filter domain.ExecutionFilter) ([]domain.TaskExecution, error)

	AppendEvent(ctx context.Context, event domain.TaskExecutionEvent) error
	ListEventsAfter(ctx context.Context, executionID string, afterSeq int64) ([]domain.TaskExecutionEvent, error)

	UpsertLineage(ctx context.Context, lineage domain.ExperimentLineageRecord) error
	GetLineage(ctx context.Context, experimentID string) (*domain.ExperimentLineageRecord, error)
	GetLineageChildren(ctx context.Context, parentExperimentID string) ([]domain.ExperimentLineageRecord, error)

	InsertBaselineHistory(ctx context.Context, record domain.BaselineHistoryRecord) error
	ListBaselineHistory(ctx context.Context, limit int) ([]domain.BaselineHistoryRecord, error)

	InsertMetricPoints(ctx context.Context, points []domain.MetricTrendPoint) error
	QueryMetricTrends(ctx context.Context, filter domain.MetricTrendFilter) ([]domain.MetricTrendPoint, error)
}
