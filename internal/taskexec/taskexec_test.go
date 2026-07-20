package taskexec

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// testRunner is a mock Runner for testing the Manager lifecycle.
type testRunner struct {
	name  string
	runFn func(ctx context.Context, req SubmitRequest, sink EventSink) error
	calls int
	mu    sync.Mutex
}

func (r *testRunner) Name() string { return r.name }
func (r *testRunner) Run(ctx context.Context, req SubmitRequest, sink EventSink) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if r.runFn != nil {
		return r.runFn(ctx, req, sink)
	}
	return nil
}

// --- InMemoryStore ---

func TestInMemoryStore_CreateAndGet(t *testing.T) {
	store := NewInMemoryStore()
	exec := domain.TaskExecution{ID: "exec-1", TaskType: "test", Status: domain.TaskStatusQueued}
	if err := store.CreateExecution(context.Background(), exec); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.GetExecution(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "exec-1" || got.TaskType != "test" {
		t.Errorf("got %+v", got)
	}
}

func TestInMemoryStore_GetNotFound(t *testing.T) {
	store := NewInMemoryStore()
	_, err := store.GetExecution(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInMemoryStore_Update(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateExecution(context.Background(), domain.TaskExecution{ID: "exec-1", Status: domain.TaskStatusQueued})
	store.UpdateExecution(context.Background(), domain.TaskExecution{ID: "exec-1", Status: domain.TaskStatusRunning})
	got, _ := store.GetExecution(context.Background(), "exec-1")
	if got.Status != domain.TaskStatusRunning {
		t.Errorf("status = %s", got.Status)
	}
}

func TestInMemoryStore_List(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateExecution(context.Background(), domain.TaskExecution{ID: "e1", Status: domain.TaskStatusQueued})
	store.CreateExecution(context.Background(), domain.TaskExecution{ID: "e2", Status: domain.TaskStatusSucceeded})
	list, _ := store.ListExecutions(context.Background(), domain.ExecutionFilter{})
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestInMemoryStore_ListFilter(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateExecution(context.Background(), domain.TaskExecution{ID: "e1", Status: domain.TaskStatusSucceeded})
	store.CreateExecution(context.Background(), domain.TaskExecution{ID: "e2", Status: domain.TaskStatusFailed})
	list, _ := store.ListExecutions(context.Background(), domain.ExecutionFilter{Status: string(domain.TaskStatusSucceeded)})
	if len(list) != 1 || list[0].ID != "e1" {
		t.Errorf("expected [e1], got %d", len(list))
	}
}

func TestInMemoryStore_Events(t *testing.T) {
	store := NewInMemoryStore()
	store.AppendEvent(context.Background(), domain.TaskExecutionEvent{ExecutionID: "exec-1", EventType: domain.TaskEventStatus, Message: "a", Sequence: 1})
	store.AppendEvent(context.Background(), domain.TaskExecutionEvent{ExecutionID: "exec-1", EventType: domain.TaskEventProgress, Message: "b", Sequence: 2})
	events, _ := store.ListEventsAfter(context.Background(), "exec-1", 0)
	if len(events) != 2 {
		t.Errorf("expected 2, got %d", len(events))
	}
}

func TestInMemoryStore_Concurrent(t *testing.T) {
	store := NewInMemoryStore()
	var wg sync.WaitGroup
	n := 50
	for i := range n {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			store.CreateExecution(context.Background(), domain.TaskExecution{
				ID: fmt.Sprintf("exec-%d", id), Status: domain.TaskStatusQueued,
			})
		}(i)
	}
	wg.Wait()
	list, _ := store.ListExecutions(context.Background(), domain.ExecutionFilter{})
	if len(list) != n {
		t.Errorf("expected %d, got %d", n, len(list))
	}
}

// --- Manager ---

func TestManager_SubmitUnknown(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	_, err := mgr.Submit(context.Background(), SubmitRequest{TaskType: "nonexistent"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestManager_SubmitAndGet(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("test", &testRunner{name: "test"})
	exec, err := mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if exec.ID == "" || exec.Status != domain.TaskStatusQueued {
		t.Errorf("exec: %+v", exec)
	}
	time.Sleep(50 * time.Millisecond)
	got, _ := mgr.Get(context.Background(), exec.ID)
	if got.ID != exec.ID {
		t.Error("ID mismatch")
	}
}

func TestManager_List(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("test", &testRunner{name: "test"})
	mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	time.Sleep(50 * time.Millisecond)
	list, _ := mgr.List(context.Background(), domain.ExecutionFilter{})
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestManager_Cancel(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("slow", &testRunner{name: "slow", runFn: func(ctx context.Context, _ SubmitRequest, _ EventSink) error {
		<-ctx.Done()
		return ctx.Err()
	}})
	exec, _ := mgr.Submit(context.Background(), SubmitRequest{TaskType: "slow"})
	time.Sleep(20 * time.Millisecond)
	if err := mgr.Cancel(context.Background(), exec.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
}

func TestManager_CancelNotFound(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	if err := mgr.Cancel(context.Background(), "nonexistent"); err == nil {
		t.Fatal("expected error")
	}
}

func TestManager_Subscribe(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("ping", &testRunner{name: "ping", runFn: func(_ context.Context, _ SubmitRequest, sink EventSink) error {
		sink.Emit(domain.TaskExecutionEvent{EventType: domain.TaskEventStatus, Message: "ping"})
		return nil
	}})
	exec, _ := mgr.Submit(context.Background(), SubmitRequest{TaskType: "ping"})
	ch, unsub := mgr.Subscribe(exec.ID)
	defer unsub()
	select {
	case ev := <-ch:
		// Manager emits "running" status first, then our custom event.
		// Drain "running" if present, then expect "ping".
		if ev.Message == "running" {
			select {
			case ev = <-ch:
			case <-time.After(3 * time.Second):
				t.Fatal("timeout waiting for custom event")
			}
		}
		if ev.Message != "ping" {
			t.Errorf("got %s, want ping", ev.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestManager_RegisterDuplicate(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("dup", &testRunner{name: "dup1"})
	mgr.RegisterRunner("dup", &testRunner{name: "dup2"})
	exec, err := mgr.Submit(context.Background(), SubmitRequest{TaskType: "dup"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if exec == nil {
		t.Fatal("expected execution")
	}
}

func TestManager_Retry(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("test", &testRunner{name: "test"})
	exec, _ := mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	time.Sleep(50 * time.Millisecond)
	retryExec, err := mgr.Retry(context.Background(), exec.ID, "web_ui")
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retryExec.ID == "" || retryExec.ID == exec.ID {
		t.Errorf("expected new ID")
	}
}

// --- InMemoryStore lineage ---

func TestInMemoryStore_Lineage_UpsertAndGet(t *testing.T) {
	store := NewInMemoryStore()
	rec := domain.ExperimentLineageRecord{
		ExperimentID:       "exp-001",
		ExecutionID:        "exec-001",
		RootExperimentID:   "exp-root",
		LineageDepth:       1,
		TargetAgentID:      "agent-1",
		TargetSkill:        "momentum",
		Status:             "accepted",
		RecordedAt:         time.Now(),
		ParentExperimentID: "exp-root",
	}
	if err := store.UpsertLineage(context.Background(), rec); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := store.GetLineage(context.Background(), "exp-001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ExperimentID != "exp-001" || got.TargetAgentID != "agent-1" {
		t.Errorf("got %+v", got)
	}
}

func TestInMemoryStore_Lineage_GetNotFound(t *testing.T) {
	store := NewInMemoryStore()
	_, err := store.GetLineage(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInMemoryStore_Lineage_Children(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now()
	store.UpsertLineage(context.Background(), domain.ExperimentLineageRecord{
		ExperimentID: "child-1", RootExperimentID: "root", ParentExperimentID: "root",
		Status: "accepted", RecordedAt: now,
	})
	store.UpsertLineage(context.Background(), domain.ExperimentLineageRecord{
		ExperimentID: "child-2", RootExperimentID: "root", ParentExperimentID: "root",
		Status: "rejected", RecordedAt: now,
	})
	store.UpsertLineage(context.Background(), domain.ExperimentLineageRecord{
		ExperimentID: "orphan", RootExperimentID: "orphan", ParentExperimentID: "other",
		Status: "accepted", RecordedAt: now,
	})
	children, err := store.GetLineageChildren(context.Background(), "root")
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestInMemoryStore_Lineage_ChildrenNone(t *testing.T) {
	store := NewInMemoryStore()
	children, err := store.GetLineageChildren(context.Background(), "no-children")
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("expected 0, got %d", len(children))
	}
}

// --- InMemoryStore baseline history ---

func TestInMemoryStore_BaselineHistory_InsertAndList(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now()
	r1 := domain.BaselineHistoryRecord{ID: "bh-1", VersionBefore: 1, VersionAfter: 2, PromotedBy: "test", PromotedAt: now}
	r2 := domain.BaselineHistoryRecord{ID: "bh-2", VersionBefore: 2, VersionAfter: 3, PromotedBy: "test", PromotedAt: now.Add(time.Hour)}
	if err := store.InsertBaselineHistory(context.Background(), r1); err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	if err := store.InsertBaselineHistory(context.Background(), r2); err != nil {
		t.Fatalf("insert 2: %v", err)
	}
	all, err := store.ListBaselineHistory(context.Background(), 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}
}

func TestInMemoryStore_BaselineHistory_ListWithLimit(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now()
	for i := range 5 {
		store.InsertBaselineHistory(context.Background(), domain.BaselineHistoryRecord{
			ID: "bh-" + string(rune('0'+i)), VersionBefore: i, VersionAfter: i + 1,
			PromotedBy: "test", PromotedAt: now.Add(time.Duration(i) * time.Hour),
		})
	}
	limited, err := store.ListBaselineHistory(context.Background(), 2)
	if err != nil {
		t.Fatalf("list limit: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2, got %d", len(limited))
	}
}

// --- InMemoryStore metrics ---

func TestInMemoryStore_Metrics_InsertAndQuery(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now()
	points := []domain.MetricTrendPoint{
		{ID: "m1", ExecutionID: "exec-1", ExperimentID: "exp-a", SeriesKey: "sharpe", MetricName: "sharpe_ratio", MetricValue: 1.5, SampledAt: now},
		{ID: "m2", ExecutionID: "exec-1", ExperimentID: "exp-a", SeriesKey: "sharpe", MetricName: "sharpe_ratio", MetricValue: 1.7, SampledAt: now.Add(time.Hour)},
		{ID: "m3", ExecutionID: "exec-2", ExperimentID: "exp-b", SeriesKey: "drawdown", MetricName: "max_drawdown", MetricValue: -0.05, SampledAt: now},
	}
	if err := store.InsertMetricPoints(context.Background(), points); err != nil {
		t.Fatalf("insert: %v", err)
	}

	all, err := store.QueryMetricTrends(context.Background(), domain.MetricTrendFilter{})
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}
}

func TestInMemoryStore_Metrics_QueryFilterByExperiment(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now()
	store.InsertMetricPoints(context.Background(), []domain.MetricTrendPoint{
		{ID: "m1", ExecutionID: "e1", ExperimentID: "exp-a", SeriesKey: "s1", MetricName: "n1", MetricValue: 1.0, SampledAt: now},
		{ID: "m2", ExecutionID: "e2", ExperimentID: "exp-b", SeriesKey: "s1", MetricName: "n1", MetricValue: 2.0, SampledAt: now},
	})
	result, err := store.QueryMetricTrends(context.Background(), domain.MetricTrendFilter{ExperimentID: "exp-a"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result) != 1 || result[0].ID != "m1" {
		t.Errorf("expected [m1], got %d results", len(result))
	}
}

func TestInMemoryStore_Metrics_QueryFilterByTime(t *testing.T) {
	store := NewInMemoryStore()
	base := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	store.InsertMetricPoints(context.Background(), []domain.MetricTrendPoint{
		{ID: "early", ExecutionID: "e1", SeriesKey: "s1", MetricName: "n1", MetricValue: 1.0, SampledAt: base},
		{ID: "mid", ExecutionID: "e1", SeriesKey: "s1", MetricName: "n1", MetricValue: 2.0, SampledAt: base.Add(48 * time.Hour)},
		{ID: "late", ExecutionID: "e1", SeriesKey: "s1", MetricName: "n1", MetricValue: 3.0, SampledAt: base.Add(96 * time.Hour)},
	})
	result, err := store.QueryMetricTrends(context.Background(), domain.MetricTrendFilter{
		Start: base.Add(24 * time.Hour),
		End:   base.Add(72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result) != 1 || result[0].ID != "mid" {
		t.Errorf("expected [mid], got %d results", len(result))
	}
}

func TestInMemoryStore_Metrics_QueryFilterBySeriesKey(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now()
	store.InsertMetricPoints(context.Background(), []domain.MetricTrendPoint{
		{ID: "m1", ExecutionID: "e1", SeriesKey: "sharpe", MetricName: "sharpe", MetricValue: 1.0, SampledAt: now},
		{ID: "m2", ExecutionID: "e1", SeriesKey: "drawdown", MetricName: "drawdown", MetricValue: -0.1, SampledAt: now},
	})
	result, err := store.QueryMetricTrends(context.Background(), domain.MetricTrendFilter{SeriesKey: "drawdown"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result) != 1 || result[0].ID != "m2" {
		t.Errorf("expected [m2], got %d results", len(result))
	}
}

// --- localSink (EventSink) ---

func TestLocalSink_ExecutionID(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("test", &testRunner{name: "test", runFn: func(_ context.Context, _ SubmitRequest, sink EventSink) error {
		if id := sink.ExecutionID(); id == "" {
			t.Error("ExecutionID should not be empty")
		}
		return nil
	}})
	exec, err := mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	_ = exec
	time.Sleep(50 * time.Millisecond)
}

func TestLocalSink_RecordLineage(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("test", &testRunner{name: "test", runFn: func(_ context.Context, _ SubmitRequest, sink EventSink) error {
		return sink.RecordLineage(domain.ExperimentLineageRecord{
			ExperimentID: "exp-recordlineage",
			ExecutionID:  sink.ExecutionID(),
			Status:       "accepted",
			RecordedAt:   time.Now(),
		})
	}})
	exec, err := mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	lin, err := mgr.store.GetLineage(context.Background(), "exp-recordlineage")
	if err != nil {
		t.Fatalf("get lineage: %v", err)
	}
	if lin.ExperimentID != "exp-recordlineage" || lin.ExecutionID != exec.ID {
		t.Errorf("lineage: %+v", lin)
	}
}

func TestLocalSink_RecordBaselineHistory(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("test", &testRunner{name: "test", runFn: func(_ context.Context, _ SubmitRequest, sink EventSink) error {
		return sink.RecordBaselineHistory(domain.BaselineHistoryRecord{
			ID: "bh-sink-test", VersionBefore: 1, VersionAfter: 2, PromotedBy: "test", PromotedAt: time.Now(),
		})
	}})
	_, err := mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	records, err := mgr.store.ListBaselineHistory(context.Background(), 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, r := range records {
		if r.ID == "bh-sink-test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("RecordBaselineHistory did not persist the record")
	}
}

func TestLocalSink_RecordMetrics(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("test", &testRunner{name: "test", runFn: func(_ context.Context, _ SubmitRequest, sink EventSink) error {
		return sink.RecordMetrics([]domain.MetricTrendPoint{
			{ID: "metric-sink-test", ExecutionID: sink.ExecutionID(), SeriesKey: "test", MetricName: "test_metric", MetricValue: 1.0, SampledAt: time.Now()},
		})
	}})
	_, err := mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	points, err := mgr.store.QueryMetricTrends(context.Background(), domain.MetricTrendFilter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	found := false
	for _, p := range points {
		if p.ID == "metric-sink-test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("RecordMetrics did not persist the metric point")
	}
}

func TestManager_ListEvents(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	mgr.RegisterRunner("test", &testRunner{name: "test", runFn: func(_ context.Context, _ SubmitRequest, sink EventSink) error {
		sink.Emit(domain.TaskExecutionEvent{EventType: domain.TaskEventStatus, Message: "event1"})
		return nil
	}})
	exec, err := mgr.Submit(context.Background(), SubmitRequest{TaskType: "test"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	events, err := mgr.ListEvents(context.Background(), exec.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected at least 1 event, got 0")
	}
}

// --- Manager: SetContext + localSink ---

func TestManager_SetContext(t *testing.T) {
	mgr := NewManager(NewInMemoryStore())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.SetContext(ctx)
	if mgr.ctx != ctx {
		t.Error("expected context to be set")
	}
}

// --- generateID ---

func TestGenerateID_Format(t *testing.T) {
	if len(generateID()) != 36 {
		t.Errorf("expected 36 chars")
	}
}

func TestGenerateID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for range 100 {
		id := generateID()
		if ids[id] {
			t.Errorf("duplicate: %s", id)
		}
		ids[id] = true
	}
}
