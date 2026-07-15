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
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleTaskGetEvents(context.Background(), nil, taskIDInput{TaskID: "task-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/tasks/task-42/events" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}
