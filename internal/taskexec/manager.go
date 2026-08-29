package taskexec

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("exec-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type Manager struct {
	store   Store
	runners map[string]Runner
	mu      sync.RWMutex
	active  map[string]*activeRun
	subMu   sync.RWMutex
	subs    map[string][]*subscription
	ctx     context.Context
}

type activeRun struct {
	execution  domain.TaskExecution
	cancelFunc context.CancelFunc
	sink       *localSink
}

type subscription struct {
	ch     chan domain.TaskExecutionEvent
	closed atomic.Bool
	mu     sync.Mutex
}

type localSink struct {
	manager     *Manager
	executionID string
	seq         atomic.Int64
}

func (s *localSink) Emit(event domain.TaskExecutionEvent) {
	seq := s.seq.Add(1)
	event.Sequence = seq
	event.ExecutionID = s.executionID
	event.CreatedAt = time.Now()

	ctx := s.manager.ctx
	if err := s.manager.store.AppendEvent(ctx, event); err != nil {
		logging.Error("taskexec", "append_event_failed", "err", err.Error())
	}

	s.manager.subMu.RLock()
	for _, sub := range s.manager.subs[s.executionID] {
		sub.mu.Lock()
		if !sub.closed.Load() {
			select {
			case sub.ch <- event:
			default:
			}
		}
		sub.mu.Unlock()
	}
	s.manager.subMu.RUnlock()
}

func (s *localSink) ExecutionID() string {
	return s.executionID
}

func (s *localSink) RecordLineage(lineage domain.ExperimentLineageRecord) error {
	ctx := s.manager.ctx
	if err := s.manager.store.UpsertLineage(ctx, lineage); err != nil {
		logging.Error("taskexec", "record_lineage_failed", "err", err.Error())
		return fmt.Errorf("record lineage: %w", err)
	}
	return nil
}

func (s *localSink) RecordBaselineHistory(record domain.BaselineHistoryRecord) error {
	ctx := s.manager.ctx
	if err := s.manager.store.InsertBaselineHistory(ctx, record); err != nil {
		logging.Error("taskexec", "record_baseline_history_failed", "err", err.Error())
		return fmt.Errorf("record baseline history: %w", err)
	}
	return nil
}

func (s *localSink) RecordMetrics(points []domain.MetricTrendPoint) error {
	ctx := s.manager.ctx
	if err := s.manager.store.InsertMetricPoints(ctx, points); err != nil {
		logging.Error("taskexec", "record_metrics_failed", "err", err.Error())
		return fmt.Errorf("record metrics: %w", err)
	}
	return nil
}

func NewManager(store Store) *Manager {
	return &Manager{
		store:   store,
		runners: make(map[string]Runner),
		active:  make(map[string]*activeRun),
		subs:    make(map[string][]*subscription),
		ctx:     context.Background(),
	}
}

func (m *Manager) SetContext(ctx context.Context) {
	if ctx != nil {
		m.ctx = ctx
	}
}

func (m *Manager) RegisterRunner(taskType string, runner Runner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runners[taskType] = runner
}

func (m *Manager) Submit(ctx context.Context, req SubmitRequest) (*domain.TaskExecution, error) {
	m.mu.RLock()
	runner, ok := m.runners[req.TaskType]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no runner registered for task type: %s", req.TaskType)
	}

	id := generateID()
	now := time.Now()
	exec := domain.TaskExecution{
		ID:             id,
		TaskType:       domain.TaskType(req.TaskType),
		CommandName:    runner.Name(),
		Status:         domain.TaskStatusQueued,
		Actor:          req.Actor,
		ActorSource:    req.ActorSource,
		IdempotencyKey: req.IdempotencyKey,
		SubmittedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if len(req.Payload) > 0 {
		b, err := json.Marshal(req.Payload)
		if err == nil {
			exec.RequestPayload = b
		}
	}

	if err := m.store.CreateExecution(ctx, exec); err != nil {
		return nil, fmt.Errorf("create execution: %w", err)
	}

	m.startRun(&exec, req, runner)
	return &exec, nil
}

func (m *Manager) startRun(exec *domain.TaskExecution, req SubmitRequest, runner Runner) {
	ctx, cancel := context.WithCancel(m.ctx)

	sink := &localSink{
		manager:     m,
		executionID: exec.ID,
	}

	ar := &activeRun{
		execution:  *exec,
		cancelFunc: cancel,
		sink:       sink,
	}

	m.mu.Lock()
	m.active[exec.ID] = ar
	m.mu.Unlock()

	go func() {
		// Work on a copy to avoid racing with the caller.
		execCopy := *exec
		defer func() {
			m.mu.Lock()
			delete(m.active, exec.ID)
			m.mu.Unlock()
			cancel()
		}()

		now := time.Now()
		execCopy.Status = domain.TaskStatusRunning
		execCopy.StartedAt = &now
		if err := m.store.UpdateExecution(ctx, execCopy); err != nil {
			logging.Error("taskexec", "update_execution_to_running_failed", "err", err.Error())
		}

		sink.Emit(domain.TaskExecutionEvent{
			EventType: domain.TaskEventStatus,
			Stream:    "system",
			Message:   "running",
			Payload:   mustJSON(map[string]string{"status": "running"}),
		})

		err := runner.Run(ctx, req, sink)

		finishTime := time.Now()
		execCopy.FinishedAt = &finishTime

		// Preserve cancel_requested when the user requested cancel via
		// Manager.Cancel(). The store is the synchronization point:
		// Manager.Cancel writes "cancel_requested" before invoking
		// cancelFunc(), and the deferred completion here must not
		// overwrite it with "failed" / "succeeded".
		// Use context.Background because ctx is already cancelled.
		currentExec, getErr := m.store.GetExecution(context.Background(), exec.ID)
		userCanceled := getErr == nil && currentExec.Status == domain.TaskStatusCancelRequested

		switch {
		case userCanceled:
			execCopy.Status = domain.TaskStatusCancelRequested
			sink.Emit(domain.TaskExecutionEvent{
				EventType: domain.TaskEventDone,
				Stream:    "system",
				Message:   "canceled by user",
				Payload:   mustJSON(map[string]string{"status": "cancel_requested"}),
			})
		case err != nil:
			execCopy.Status = domain.TaskStatusFailed
			execCopy.ErrorMessage = err.Error()
			sink.Emit(domain.TaskExecutionEvent{
				EventType: domain.TaskEventDone,
				Stream:    "system",
				Level:     "error",
				Message:   err.Error(),
				Payload:   mustJSON(map[string]string{"status": "failed"}),
			})
		default:
			execCopy.Status = domain.TaskStatusSucceeded
			sink.Emit(domain.TaskExecutionEvent{
				EventType: domain.TaskEventDone,
				Stream:    "system",
				Message:   "succeeded",
				Payload:   mustJSON(map[string]string{"status": "succeeded"}),
			})
		}

		if updateErr := m.store.UpdateExecution(context.Background(), execCopy); updateErr != nil {
			logging.Error("taskexec", "update_execution_to_final_status_failed", "err", updateErr.Error())
		}
	}()
}

func (m *Manager) Get(ctx context.Context, id string) (*domain.TaskExecution, error) {
	return m.store.GetExecution(ctx, id)
}

func (m *Manager) List(ctx context.Context, filter domain.ExecutionFilter) ([]domain.TaskExecution, error) {
	return m.store.ListExecutions(ctx, filter)
}

// ListEvents returns the persisted lifecycle events of one execution in
// sequence order. It is the request/response counterpart to Subscribe's
// live stream and backs the JSON snapshot endpoint.
func (m *Manager) ListEvents(ctx context.Context, executionID string) ([]domain.TaskExecutionEvent, error) {
	return m.store.ListEventsAfter(ctx, executionID, 0)
}

func (m *Manager) Cancel(ctx context.Context, id string) error {
	m.mu.RLock()
	ar, ok := m.active[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("execution not found or not running: %s", id)
	}

	now := time.Now()
	ar.execution.CancelRequestedAt = &now
	ar.execution.Status = domain.TaskStatusCancelRequested
	if err := m.store.UpdateExecution(ctx, ar.execution); err != nil {
		return fmt.Errorf("update execution for cancel: %w", err)
	}

	ar.cancelFunc()
	return nil
}

func (m *Manager) Retry(ctx context.Context, id string, actor string) (*domain.TaskExecution, error) {
	original, err := m.store.GetExecution(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get original execution: %w", err)
	}

	newID := generateID()
	now := time.Now()
	exec := domain.TaskExecution{
		ID:                   newID,
		TaskType:             original.TaskType,
		CommandName:          original.CommandName,
		CommandArgs:          original.CommandArgs,
		RequestPayload:       original.RequestPayload,
		Status:               domain.TaskStatusQueued,
		Actor:                actor,
		ActorSource:          "web_ui",
		RetryOf:              id,
		RequiresConfirmation: original.RequiresConfirmation,
		SubmittedAt:          now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := m.store.CreateExecution(ctx, exec); err != nil {
		return nil, fmt.Errorf("create retry execution: %w", err)
	}

	m.mu.RLock()
	runner, ok := m.runners[string(original.TaskType)]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no runner registered for task type: %s", original.TaskType)
	}

	req := SubmitRequest{
		TaskType:    string(original.TaskType),
		Actor:       actor,
		ActorSource: "web_ui",
		Payload:     make(map[string]any),
		Confirmed:   true,
	}
	m.startRun(&exec, req, runner)
	return &exec, nil
}

func (m *Manager) Subscribe(executionID string) (<-chan domain.TaskExecutionEvent, func()) {
	ch := make(chan domain.TaskExecutionEvent, 100)
	sub := &subscription{ch: ch}

	// Replay existing events before adding to subscriber list.
	// Events emitted before Subscribe was called are replayed here;
	// events emitted afterward are delivered via Emit's normal path.
	if events, err := m.store.ListEventsAfter(m.ctx, executionID, 0); err == nil {
		for _, ev := range events {
			ch <- ev
		}
	}

	m.subMu.Lock()
	m.subs[executionID] = append(m.subs[executionID], sub)
	m.subMu.Unlock()

	unsubscribe := func() {
		sub.mu.Lock()
		sub.closed.Store(true)
		close(ch)
		sub.mu.Unlock()

		m.subMu.Lock()
		subs := m.subs[executionID]
		for i, s := range subs {
			if s == sub {
				m.subs[executionID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		m.subMu.Unlock()
	}

	return ch, unsubscribe
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
