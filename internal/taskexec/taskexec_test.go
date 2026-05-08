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
