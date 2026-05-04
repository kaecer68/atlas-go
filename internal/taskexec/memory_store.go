package taskexec

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type InMemoryStore struct {
	mu              sync.RWMutex
	executions      map[string]domain.TaskExecution
	events          map[string][]domain.TaskExecutionEvent
	lineage         map[string]domain.ExperimentLineageRecord
	baselineHistory []domain.BaselineHistoryRecord
	metrics         []domain.MetricTrendPoint
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		executions:      make(map[string]domain.TaskExecution),
		events:          make(map[string][]domain.TaskExecutionEvent),
		lineage:         make(map[string]domain.ExperimentLineageRecord),
		baselineHistory: make([]domain.BaselineHistoryRecord, 0),
		metrics:         make([]domain.MetricTrendPoint, 0),
	}
}

func (s *InMemoryStore) CreateExecution(_ context.Context, exec domain.TaskExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[exec.ID] = exec
	return nil
}

func (s *InMemoryStore) UpdateExecution(_ context.Context, exec domain.TaskExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[exec.ID] = exec
	return nil
}

func (s *InMemoryStore) GetExecution(_ context.Context, id string) (*domain.TaskExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exec, ok := s.executions[id]
	if !ok {
		return nil, fmt.Errorf("execution not found: %s", id)
	}
	return &exec, nil
}

func (s *InMemoryStore) ListExecutions(_ context.Context, filter domain.ExecutionFilter) ([]domain.TaskExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []domain.TaskExecution
	for _, exec := range s.executions {
		if filter.TaskType != "" && string(exec.TaskType) != filter.TaskType {
			continue
		}
		if filter.Status != "" && string(exec.Status) != filter.Status {
			continue
		}
		if filter.Actor != "" && exec.Actor != filter.Actor {
			continue
		}
		result = append(result, exec)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SubmittedAt.After(result[j].SubmittedAt)
	})

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result, nil
}

func (s *InMemoryStore) AppendEvent(_ context.Context, event domain.TaskExecutionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[event.ExecutionID] = append(s.events[event.ExecutionID], event)
	return nil
}

func (s *InMemoryStore) ListEventsAfter(_ context.Context, executionID string, afterSeq int64) ([]domain.TaskExecutionEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []domain.TaskExecutionEvent
	for _, ev := range s.events[executionID] {
		if ev.Sequence > afterSeq {
			result = append(result, ev)
		}
	}
	return result, nil
}

func (s *InMemoryStore) UpsertLineage(_ context.Context, lineage domain.ExperimentLineageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lineage[lineage.ExperimentID] = lineage
	return nil
}

func (s *InMemoryStore) GetLineage(_ context.Context, experimentID string) (*domain.ExperimentLineageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.lineage[experimentID]
	if !ok {
		return nil, fmt.Errorf("lineage not found: %s", experimentID)
	}
	return &rec, nil
}

func (s *InMemoryStore) GetLineageChildren(_ context.Context, parentExperimentID string) ([]domain.ExperimentLineageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []domain.ExperimentLineageRecord
	for _, rec := range s.lineage {
		if rec.ParentExperimentID == parentExperimentID {
			result = append(result, rec)
		}
	}
	return result, nil
}

func (s *InMemoryStore) InsertBaselineHistory(_ context.Context, record domain.BaselineHistoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baselineHistory = append(s.baselineHistory, record)
	return nil
}

func (s *InMemoryStore) ListBaselineHistory(_ context.Context, limit int) ([]domain.BaselineHistoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.baselineHistory) {
		return append([]domain.BaselineHistoryRecord(nil), s.baselineHistory...), nil
	}

	start := len(s.baselineHistory) - limit
	return append([]domain.BaselineHistoryRecord(nil), s.baselineHistory[start:]...), nil
}

func (s *InMemoryStore) InsertMetricPoints(_ context.Context, points []domain.MetricTrendPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = append(s.metrics, points...)
	return nil
}

func (s *InMemoryStore) QueryMetricTrends(_ context.Context, filter domain.MetricTrendFilter) ([]domain.MetricTrendPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []domain.MetricTrendPoint
	for _, p := range s.metrics {
		if filter.ExperimentID != "" && p.ExperimentID != filter.ExperimentID {
			continue
		}
		if filter.SeriesKey != "" && p.SeriesKey != filter.SeriesKey {
			continue
		}
		if filter.MetricName != "" && p.MetricName != filter.MetricName {
			continue
		}
		if !filter.Start.IsZero() && p.SampledAt.Before(filter.Start) {
			continue
		}
		if !filter.End.IsZero() && p.SampledAt.After(filter.End) {
			continue
		}
		result = append(result, p)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SampledAt.After(result[j].SampledAt)
	})

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result, nil
}
