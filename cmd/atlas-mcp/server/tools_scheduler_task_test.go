package server

import (
	"context"
	"testing"
)

func TestHandleSchedulerGetStatus_DecodesArray(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`[{"name":"task1","enabled":true,"channel_id":"ch1"},{"name":"task2","enabled":false}]`)
	_, out, err := s.handleSchedulerGetStatus(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/scheduler/status" {
		t.Fatalf("path=%s", rec.path)
	}
	if len(out.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(out.Tasks))
	}
	if out.Tasks[0]["name"] != "task1" {
		t.Fatalf("expected first task name=task1, got %v", out.Tasks[0]["name"])
	}
	if out.Tasks[1]["enabled"] != false {
		t.Fatalf("expected second task enabled=false, got %v", out.Tasks[1]["enabled"])
	}
}

func TestHandleSchedulerGetStatus_EmptyArray(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`[]`)
	_, out, err := s.handleSchedulerGetStatus(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Tasks) != 0 {
		t.Fatalf("expected empty tasks, got %d", len(out.Tasks))
	}
}

func TestHandleTaskList_DecodesArray(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`[{"id":"task-1","status":"running","task_type":"backtest"},{"id":"task-2","status":"done"}]`)
	_, out, err := s.handleTaskList(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/tasks" {
		t.Fatalf("path=%s", rec.path)
	}
	if len(out.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(out.Tasks))
	}
	if out.Tasks[0]["id"] != "task-1" {
		t.Fatalf("expected first task id=task-1, got %v", out.Tasks[0]["id"])
	}
}

func TestHandleTaskGet_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleTaskGet(context.Background(), nil, taskIDInput{TaskID: "task-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/tasks/task-42" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleTaskGetEvents_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"events":[{"seq":1,"type":"started"}]}`)
	_, out, err := s.handleTaskGetEvents(context.Background(), nil, taskIDInput{TaskID: "task-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/tasks/task-42/events/snapshot" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

// TestHandleSchedulerGetStatus_OmitzeroShape is the Batch 2 smoke for the
// omitzero change: the gateway now omits zero-value time keys
// (last_data_as_of / last_persisted_at) from TaskStatus JSON. The MCP
// summary must keep working — including the defensive "" branch when a
// run timestamp key is absent — and never choke on the missing keys.
func TestHandleSchedulerGetStatus_OmitzeroShape(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	// Payload mirrors TaskStatus after omitzero: zero time fields absent,
	// zero next_run/last_run still serialized as the zero time string,
	// and one task missing run timestamps entirely (defensive "" branch).
	rec.responseBody = []byte(`[
		{"name":"fresh","enabled":true,"channel_id":"ch1","interval":3600000000000,
		 "last_run":"0001-01-01T00:00:00Z","next_run":"0001-01-01T00:00:00Z",
		 "consecutive_failures":0},
		{"name":"no-keys","enabled":true,"channel_id":"ch2",
		 "consecutive_failures":0},
		{"name":"ran","enabled":true,"channel_id":"ch3",
		 "last_run":"2026-08-29T08:00:00Z","next_run":"2026-08-29T09:00:00Z",
		 "last_data_as_of":"2026-08-29T08:00:00Z","last_persisted_at":"2026-08-29T08:01:00Z",
		 "consecutive_failures":2},
		{"name":"disabled","enabled":false,"channel_id":"ch4",
		 "consecutive_failures":3}
	]`)
	_, out, err := s.handleSchedulerGetStatus(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/scheduler/status" {
		t.Fatalf("path=%s", rec.path)
	}
	if len(out.Tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(out.Tasks))
	}
	// The two tasks with missing/zero run timestamps must be Pending; the
	// task with real timestamps must not; disabled counts separately.
	if got := out.Summary.Total; got != 4 {
		t.Errorf("Summary.Total=%d, want 4", got)
	}
	if got := out.Summary.Enabled; got != 3 {
		t.Errorf("Summary.Enabled=%d, want 3", got)
	}
	if got := out.Summary.Disabled; got != 1 {
		t.Errorf("Summary.Disabled=%d, want 1", got)
	}
	if got := out.Summary.Pending; got != 2 {
		t.Errorf(`Summary.Pending=%d, want 2 (zero-time and missing-key "" branch)`, got)
	}
	// Errored only counts enabled tasks: the disabled task's failures are
	// skipped by the handler's continue.
	if got := out.Summary.Errored; got != 1 {
		t.Errorf("Summary.Errored=%d, want 1 (enabled tasks with consecutive_failures>0)", got)
	}
}
