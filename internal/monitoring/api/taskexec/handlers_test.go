package taskexec_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/taskexec"
	taskexecpkg "github.com/kaecer68/atlas-go/internal/taskexec"
)

func setTestAPIKey(t *testing.T) {
	t.Setenv("ATLAS_API_KEY", "test-key")
}

func setAPIKeyHeader(req *http.Request) {
	req.Header.Set("X-API-Key", "test-key")
}

// stubRunner is a minimal Runner for handler tests.
// Set fn to customize Run behavior; set blocks to make Run wait until ctx cancellation.
type stubRunner struct {
	name   string
	fn     func(ctx context.Context, req taskexecpkg.SubmitRequest, sink taskexecpkg.EventSink) error
	blocks bool
}

func (r *stubRunner) Name() string { return r.name }
func (r *stubRunner) Run(ctx context.Context, req taskexecpkg.SubmitRequest, sink taskexecpkg.EventSink) error {
	if r.fn != nil {
		return r.fn(ctx, req, sink)
	}
	if r.blocks {
		<-ctx.Done()
		return ctx.Err()
	}
	// Emit a done event so SSE subscribers see output.
	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventDone,
		Stream:    "system",
		Message:   "succeeded",
		Payload:   []byte(`{"status":"succeeded"}`),
	})
	return nil
}

// setup creates a Handlers backed by an in-memory store and returns the helper objects.
func setup(t *testing.T) (*taskexec.Handlers, *taskexecpkg.Manager, *taskexecpkg.InMemoryStore, *http.ServeMux) {
	t.Helper()
	setTestAPIKey(t)
	store := taskexecpkg.NewInMemoryStore()
	manager := taskexecpkg.NewManager(store)

	// Register runners for common task types used in tests.
	manager.RegisterRunner(string(domain.TaskTypeRunExperiment), &stubRunner{
		name: "run_experiment",
	})
	manager.RegisterRunner(string(domain.TaskTypeJudgeExperiment), &stubRunner{
		name: "judge_experiment",
	})
	manager.RegisterRunner("blocking", &stubRunner{
		name:   "blocking",
		blocks: true,
	})

	handlers := taskexec.NewHandlers(manager)
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	return handlers, manager, store, mux
}

// seedExecution inserts a TaskExecution directly into the store.
func seedExecution(t *testing.T, store *taskexecpkg.InMemoryStore, exec domain.TaskExecution) {
	t.Helper()
	if err := store.CreateExecution(context.Background(), exec); err != nil {
		t.Fatalf("seedExecution: %v", err)
	}
}

// newExec creates a minimal valid TaskExecution with sensible defaults.
func newExec(id string, taskType domain.TaskType, status domain.TaskStatus) domain.TaskExecution {
	now := time.Now()
	return domain.TaskExecution{
		ID:          id,
		TaskType:    taskType,
		Status:      status,
		SubmittedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// ──── Create Task ────

func TestCreateTask_Success(t *testing.T) {
	_, _, _, mux := setup(t)

	body := `{"task_type":"run_experiment","payload":{"experiment_id":"exp-001"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(body))
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "test-user")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == "" {
		t.Error("expected non-empty id")
	}
	if resp.Status != string(domain.TaskStatusQueued) {
		t.Errorf("status = %q, want %q", resp.Status, domain.TaskStatusQueued)
	}
}

func TestCreateTask_InvalidJSON(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader("not-json"))
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body["error"], "invalid request") {
		t.Errorf("error = %q, want contains 'invalid request'", body["error"])
	}
}

func TestCreateTask_NoRunner(t *testing.T) {
	_, _, _, mux := setup(t)

	body := `{"task_type":"nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(body))
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var bodyMap map[string]string
	json.NewDecoder(w.Body).Decode(&bodyMap)
	if !strings.Contains(bodyMap["error"], "submit failed") {
		t.Errorf("error = %q, want contains 'submit failed'", bodyMap["error"])
	}
}

func TestCreateTask_WrongMethod(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodPut, "/api/tasks", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ──── List Tasks ────

func TestListTasks_Success(t *testing.T) {
	_, _, store, mux := setup(t)

	seedExecution(t, store, newExec("exec-1", domain.TaskTypeRunExperiment, domain.TaskStatusSucceeded))
	seedExecution(t, store, newExec("exec-2", domain.TaskTypeJudgeExperiment, domain.TaskStatusRunning))

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var executions []domain.TaskExecution
	if err := json.NewDecoder(w.Body).Decode(&executions); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(executions) != 2 {
		t.Errorf("got %d executions, want 2", len(executions))
	}
}

func TestListTasks_WithFilter(t *testing.T) {
	_, _, store, mux := setup(t)

	seedExecution(t, store, newExec("exec-a", domain.TaskTypeRunExperiment, domain.TaskStatusSucceeded))
	seedExecution(t, store, newExec("exec-b", domain.TaskTypeJudgeExperiment, domain.TaskStatusFailed))

	req := httptest.NewRequest(http.MethodGet, "/api/tasks?task_type=run_experiment&status=succeeded&limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var executions []domain.TaskExecution
	if err := json.NewDecoder(w.Body).Decode(&executions); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("got %d executions, want 1", len(executions))
	}
	if executions[0].ID != "exec-a" {
		t.Errorf("got ID %q, want %q", executions[0].ID, "exec-a")
	}
}

func TestListTasks_Empty(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var executions []domain.TaskExecution
	if err := json.NewDecoder(w.Body).Decode(&executions); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(executions) != 0 {
		t.Errorf("got %d executions, want 0", len(executions))
	}
}

func TestListTasks_WrongMethod(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodPut, "/api/tasks", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ──── Get Task ────

func TestGetTask_Success(t *testing.T) {
	_, _, store, mux := setup(t)

	seedExecution(t, store, newExec("exec-get-1", domain.TaskTypeRunExperiment, domain.TaskStatusSucceeded))

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/exec-get-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var exec domain.TaskExecution
	if err := json.NewDecoder(w.Body).Decode(&exec); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if exec.ID != "exec-get-1" {
		t.Errorf("got ID %q, want %q", exec.ID, "exec-get-1")
	}
}

func TestGetTask_NotFound(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body["error"], "not found") {
		t.Errorf("error = %q, want contains 'not found'", body["error"])
	}
}

func TestGetTask_WrongMethod(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/any-id", nil)
	setAPIKeyHeader(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ──── Cancel Task ────

func TestCancelTask_Success(t *testing.T) {
	_, manager, store, mux := setup(t)

	// Register a blocking runner so the submitted task stays "running".
	manager.RegisterRunner("cancel-test", &stubRunner{
		name:   "cancel-test",
		blocks: true,
	})

	// Submit a blocking task.
	body := `{"task_type":"cancel-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(body))
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create for cancel: status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var createResp struct {
		ID string `json:"id"`
	}
	json.NewDecoder(w.Body).Decode(&createResp)

	// Wait deterministically for the goroutine to start running.
	deadline := time.Now().Add(2 * time.Second)
	for {
		current, err := store.GetExecution(context.Background(), createResp.ID)
		if err == nil && current.Status == domain.TaskStatusRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not reach running within 2s; last status = %v, err = %v", current, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Cancel the running task.
	req2 := httptest.NewRequest(http.MethodPost, "/api/tasks/"+createResp.ID+"/cancel", nil)
	setAPIKeyHeader(req2)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNoContent {
		t.Fatalf("cancel: status = %d, want %d; body = %s", w2.Code, http.StatusNoContent, w2.Body.String())
	}

	// Verify the execution status was updated.
	exec, err := store.GetExecution(context.Background(), createResp.ID)
	if err != nil {
		t.Fatalf("get after cancel: %v", err)
	}
	if exec.Status != domain.TaskStatusCancelRequested {
		t.Errorf("status after cancel = %q, want %q", exec.Status, domain.TaskStatusCancelRequested)
	}
}

// TestCancelTask_RunnerCompletionPreservesCancelStatus is a regression test
// for the race where the runner's deferred completion overwrites the
// "cancel_requested" status set by Manager.Cancel() with "failed".
//
// Race window: Manager.Cancel() writes "cancel_requested" to the store, then
// calls cancelFunc() which cancels the runner's context. The runner returns
// with err=context.Canceled; the deferred completion in startRun then writes
// "failed" to the store, overwriting the cancel status.
//
// This test polls for the final terminal state with a 2s timeout so the
// goroutine has time to fully complete. Without the fix, the final state
// will be "failed" instead of "cancel_requested".
func TestCancelTask_RunnerCompletionPreservesCancelStatus(t *testing.T) {
	_, manager, store, mux := setup(t)

	manager.RegisterRunner("cancel-preserve-test", &stubRunner{
		name:   "cancel-preserve-test",
		blocks: true,
	})

	body := `{"task_type":"cancel-preserve-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(body))
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create: status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var createResp struct {
		ID string `json:"id"`
	}
	json.NewDecoder(w.Body).Decode(&createResp)

	// Wait for the goroutine to start running.
	deadline := time.Now().Add(2 * time.Second)
	for {
		current, err := store.GetExecution(context.Background(), createResp.ID)
		if err == nil && current.Status == domain.TaskStatusRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not reach running within 2s; last status = %v, err = %v", current, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Cancel the running task.
	req2 := httptest.NewRequest(http.MethodPost, "/api/tasks/"+createResp.ID+"/cancel", nil)
	setAPIKeyHeader(req2)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNoContent {
		t.Fatalf("cancel: status = %d, want %d; body = %s", w2.Code, http.StatusNoContent, w2.Body.String())
	}

	// Wait long enough for the runner's goroutine to fully complete.
	// Manager.Cancel() returns 204 after writing "cancel_requested" to the
	// store, but cancelFunc() then cancels the runner's context, and the
	// runner's deferred completion runs afterwards. Without the fix, the
	// deferred completion overwrites "cancel_requested" with "failed".
	// We sleep so the goroutine has time to settle, then check the final
	// status.
	time.Sleep(500 * time.Millisecond)

	exec, err := store.GetExecution(context.Background(), createResp.ID)
	if err != nil {
		t.Fatalf("get after settle: %v", err)
	}
	if exec.Status == domain.TaskStatusFailed {
		t.Fatalf("status overwritten by runner completion: got %q, want %q "+
			"(runner.Run returned ctx.Err but the deferred completion in "+
			"startRun wrote 'failed' over the 'cancel_requested' set by Manager.Cancel)",
			exec.Status, domain.TaskStatusCancelRequested)
	}
	if exec.Status != domain.TaskStatusCancelRequested {
		t.Errorf("final status = %q, want %q", exec.Status, domain.TaskStatusCancelRequested)
	}
}

func TestCancelTask_NotRunning(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/nonexistent/cancel", nil)
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body["error"], "cancel failed") {
		t.Errorf("error = %q, want contains 'cancel failed'", body["error"])
	}
}

// ──── Retry Task ────

func TestRetryTask_Success(t *testing.T) {
	_, _, store, mux := setup(t)

	exec := newExec("exec-retry-src", domain.TaskTypeRunExperiment, domain.TaskStatusFailed)
	exec.CommandName = "run_experiment"
	seedExecution(t, store, exec)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/exec-retry-src/retry", nil)
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "retry-user")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == "" {
		t.Error("expected non-empty id")
	}
	if resp.Status != string(domain.TaskStatusQueued) {
		t.Errorf("status = %q, want %q", resp.Status, domain.TaskStatusQueued)
	}
}

func TestRetryTask_NotFound(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/nonexistent/retry", nil)
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body["error"], "retry failed") {
		t.Errorf("error = %q, want contains 'retry failed'", body["error"])
	}
}

// ──── Confirm Task ────

func TestConfirmTask_Success(t *testing.T) {
	_, _, store, mux := setup(t)

	exec := newExec("exec-confirm-q", domain.TaskTypeRunExperiment, domain.TaskStatusQueued)
	exec.CommandName = "run_experiment"
	exec.RequiresConfirmation = true
	seedExecution(t, store, exec)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/exec-confirm-q/confirm", nil)
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "confirm-user")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == "" {
		t.Error("expected non-empty id")
	}
	if resp.Status != string(domain.TaskStatusQueued) {
		t.Errorf("status = %q, want %q", resp.Status, domain.TaskStatusQueued)
	}
}

func TestConfirmTask_NotFound(t *testing.T) {
	_, _, _, mux := setup(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/nonexistent/confirm", nil)
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body["error"], "not found") {
		t.Errorf("error = %q, want contains 'not found'", body["error"])
	}
}

func TestConfirmTask_NotQueued(t *testing.T) {
	_, _, store, mux := setup(t)

	exec := newExec("exec-confirm-nq", domain.TaskTypeRunExperiment, domain.TaskStatusSucceeded)
	exec.CommandName = "run_experiment"
	seedExecution(t, store, exec)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/exec-confirm-nq/confirm", nil)
	setAPIKeyHeader(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "task does not require confirmation" {
		t.Errorf("error = %q, want 'task does not require confirmation'", body["error"])
	}
}

// ──── Task Events (SSE) ────

func TestTaskEvents_MissingID(t *testing.T) {
	handlers, _, _, _ := setup(t)

	// Directly call HandleTaskEvents with a request that has no path value set.
	req := httptest.NewRequest(http.MethodGet, "/api/tasks//events", nil)
	w := httptest.NewRecorder()
	status, data := handlers.HandleTaskEvents(w, req)

	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
	errMap, ok := data.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %T", data)
	}
	if errMap["error"] != "task id required" {
		t.Errorf("error = %q, want 'task id required'", errMap["error"])
	}
}

func TestTaskEvents_SSEHeadersAndData(t *testing.T) {
	_, manager, store, _ := setup(t)

	execID := "sse-test-1"
	seedExecution(t, store, newExec(execID, domain.TaskTypeRunExperiment, domain.TaskStatusRunning))

	// Store an event so Subscribe replays it.
	if err := store.AppendEvent(context.Background(), domain.TaskExecutionEvent{
		ExecutionID: execID, Sequence: 1,
		EventType: domain.TaskEventStatus, Stream: "system",
		Message: "running", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	// Create a new mux with routes registered against the same manager.
	mux := http.NewServeMux()
	taskexec.NewHandlers(manager).RegisterRoutes(mux)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+execID+"/events", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Check SSE headers.
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want contains 'text/event-stream'", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want 'no-cache'", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q, want 'keep-alive'", conn)
	}

	// Check that at least one SSE data line was written.
	body := w.Body.String()
	if !strings.Contains(body, "data: ") {
		t.Errorf("body = %q, want contains 'data: '", body)
	}
	// Should contain the "running" event.
	if !strings.Contains(body, `"running"`) {
		t.Errorf("body = %q, want contains '\"running\"'", body)
	}
}

// ──── Task Events Snapshot (JSON variant of the SSE stream) ────

func TestHandleTaskEventsSnapshot(t *testing.T) {
	_, _, store, mux := setup(t)
	seedExecution(t, store, newExec("task-ev-1", domain.TaskTypeRunExperiment, domain.TaskStatusRunning))
	if err := store.AppendEvent(context.Background(), domain.TaskExecutionEvent{
		ExecutionID: "task-ev-1",
		Sequence:    1,
		EventType:   domain.TaskEventStatus,
		Stream:      "system",
		Message:     "started",
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/task-ev-1/events/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp.Events))
	}
	if resp.Events[0]["message"] != "started" {
		t.Errorf("unexpected event payload: %v", resp.Events[0])
	}
}

func TestHandleTaskEventsSnapshotEmpty(t *testing.T) {
	_, _, store, mux := setup(t)
	seedExecution(t, store, newExec("task-ev-2", domain.TaskTypeRunExperiment, domain.TaskStatusQueued))

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/task-ev-2/events/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"events":[]`) {
		t.Errorf("expected empty events array, got %s", w.Body.String())
	}
}
