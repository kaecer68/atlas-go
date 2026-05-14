package scheduler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

func TestHandleStatus(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	h := NewHandlers(NewSchedulerService(mgr))
	status, body := h.HandleStatus(httptest.NewRequest(http.MethodGet, "/api/scheduler/status", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if len(body.([]apigateway.TaskStatus)) != 0 {
		t.Fatal("expected empty status")
	}
}

func TestHandleToggle(t *testing.T) {
	task := &apigateway.ScheduledTask{Name: "alpha"}
	mgr := apigateway.NewBackgroundTaskManager(nil)
	mgr.Register(task)

	h := NewHandlers(NewSchedulerService(mgr))
	reqBody, _ := json.Marshal(map[string]any{"name": "alpha", "enabled": false})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/toggle", bytes.NewReader(reqBody))
	status, body := h.HandleToggle(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	resp := body.(map[string]any)
	if resp["status"] != "ok" || resp["enabled"] != false {
		t.Fatalf("bad response: %#v", resp)
	}
	if task.IsEnabled() {
		t.Fatal("task should be disabled")
	}

	reqBody, _ = json.Marshal(map[string]any{"name": "alpha", "enabled": true})
	req = httptest.NewRequest(http.MethodPost, "/api/scheduler/toggle", bytes.NewReader(reqBody))
	status, _ = h.HandleToggle(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if !task.IsEnabled() {
		t.Fatal("task should be enabled")
	}
}

func TestHandleToggleMissingTask(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	h := NewHandlers(NewSchedulerService(mgr))
	reqBody, _ := json.Marshal(map[string]any{"name": "missing", "enabled": true})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/toggle", bytes.NewReader(reqBody))
	status, body := h.HandleToggle(req)
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d", status)
	}
	if _, ok := body.(map[string]string); !ok {
		t.Fatal("expected error body")
	}
}

func TestHandleToggleBadJSON(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	h := NewHandlers(NewSchedulerService(mgr))
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/toggle", bytes.NewBufferString("{"))
	status, _ := h.HandleToggle(req)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d", status)
	}
}
