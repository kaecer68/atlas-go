package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/liveness"
)

// ---------- HandleTaskLiveness ----------

// fakeLivenessProvider returns fixed rows.
type fakeLivenessProvider struct {
	rows []liveness.Row
	err  error
}

func (f fakeLivenessProvider) List(context.Context) ([]liveness.Row, error) { return f.rows, f.err }

// fakeSchedulerProvider returns fixed runtime status.
type fakeSchedulerProvider struct {
	status []apigateway.TaskStatus
}

func (f fakeSchedulerProvider) Status() []apigateway.TaskStatus { return f.status }

// doLivenessRequest runs a handler and returns the response decoded from its
// JSON wire representation (what clients actually receive).
func doLivenessRequest(t *testing.T, status int, body any, expectedStatus int) map[string]any {
	t.Helper()
	if status != expectedStatus {
		t.Fatalf("expected status %d, got %d (body=%v)", expectedStatus, status, body)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal response: %v (raw=%s)", err, raw)
	}
	return resp
}

func TestHandleTaskLiveness_NotConfigured_503(t *testing.T) {
	h := newTestHandlers(t)
	status, body := h.HandleTaskLiveness(httptest.NewRequest(http.MethodGet, "/api/dashboard/task-liveness", nil))
	resp := doLivenessRequest(t, status, body, http.StatusServiceUnavailable)
	if resp["error"] == "" {
		t.Error("expected error message when provider is missing")
	}
}

func TestHandleTaskLiveness_StoreError_Degraded(t *testing.T) {
	// A store failure must gracefully degrade (200 + empty snapshot), not 500:
	// liveness is an observability endpoint, and in smoke/dev environments the
	// PG store may be unavailable without making the dashboard error out.
	h := newTestHandlers(t)
	h.TaskLivenessProvider = fakeLivenessProvider{err: context.DeadlineExceeded}
	status, body := h.HandleTaskLiveness(httptest.NewRequest(http.MethodGet, "/api/dashboard/task-liveness", nil))
	resp := doLivenessRequest(t, status, body, http.StatusOK)
	if resp["status"] != "degraded" {
		t.Errorf("expected status=degraded, got %v", resp["status"])
	}
	tasks, ok := resp["tasks"].([]any)
	if !ok || len(tasks) != 0 {
		t.Errorf("expected empty tasks list on degraded, got %v", resp["tasks"])
	}
}

func TestHandleTaskLiveness_ResponseStructure(t *testing.T) {
	now := time.Now().UTC()
	h := newTestHandlers(t)
	h.TaskLivenessProvider = fakeLivenessProvider{rows: []liveness.Row{
		{
			TaskName:            "channel_health_sync",
			LastRunAt:           now.Add(-2 * time.Minute),
			LastSuccessAt:       now.Add(-2 * time.Minute),
			ConsecutiveFailures: 0,
			LastDurationMs:      1234,
		},
		{
			TaskName:            "government_flow_aggregate",
			LastRunAt:           now.Add(-50 * time.Minute),
			LastSuccessAt:       now.Add(-2 * time.Hour),
			LastError:           "upstream 503",
			ConsecutiveFailures: 2,
			LastDurationMs:      8000,
		},
		{
			TaskName:      "cron_geo_ingest",
			LastRunAt:     now.Add(-30 * time.Hour),
			LastSuccessAt: now.Add(-30 * time.Hour),
		},
	}}
	h.SchedulerStatus = fakeSchedulerProvider{status: []apigateway.TaskStatus{
		{Name: "channel_health_sync", Enabled: true, Interval: 5 * time.Minute, NextRun: now.Add(3 * time.Minute)},
		{Name: "government_flow_aggregate", Enabled: true, Interval: 10 * time.Minute, NextRun: now.Add(10 * time.Minute)},
	}}

	status, body := h.HandleTaskLiveness(httptest.NewRequest(http.MethodGet, "/api/dashboard/task-liveness", nil))
	resp := doLivenessRequest(t, status, body, http.StatusOK)

	if resp["total"] != float64(3) {
		t.Errorf("total = %v, want 3", resp["total"])
	}
	if resp["stale_count"] != float64(1) {
		t.Errorf("stale_count = %v, want 1", resp["stale_count"])
	}
	tasks, ok := resp["tasks"].([]any)
	if !ok || len(tasks) != 3 {
		t.Fatalf("tasks = %T len=%v, want []any len 3", resp["tasks"], len(tasks))
	}

	byName := map[string]map[string]any{}
	for _, item := range tasks {
		m := item.(map[string]any)
		byName[m["name"].(string)] = m
	}

	// Fresh BTM task: not stale, runtime fields merged.
	sync := byName["channel_health_sync"]
	if sync["stale"] != false {
		t.Errorf("channel_health_sync stale = %v, want false", sync["stale"])
	}
	if sync["source"] != "btm" {
		t.Errorf("source = %v, want btm", sync["source"])
	}
	if sync["interval"] != "5m0s" {
		t.Errorf("interval = %v, want 5m0s", sync["interval"])
	}
	if sync["interval_seconds"] != float64(300) {
		t.Errorf("interval_seconds = %v, want 300", sync["interval_seconds"])
	}
	if sync["enabled"] != true {
		t.Errorf("enabled = %v, want true", sync["enabled"])
	}
	if sync["last_duration_ms"] != float64(1234) {
		t.Errorf("last_duration_ms = %v, want 1234", sync["last_duration_ms"])
	}
	if _, ok := sync["next_run_at"]; !ok {
		t.Error("next_run_at must be present for BTM tasks")
	}

	// Failing task that last ran 50m ago with a 10m interval: stale (50m > 30m).
	gov := byName["government_flow_aggregate"]
	if gov["stale"] != true {
		t.Errorf("government_flow_aggregate stale = %v, want true", gov["stale"])
	}
	if gov["consecutive_failures"] != float64(2) {
		t.Errorf("consecutive_failures = %v, want 2", gov["consecutive_failures"])
	}
	if gov["last_error"] != "upstream 503" {
		t.Errorf("last_error = %v, want upstream 503", gov["last_error"])
	}
	if gov["stale_reason"] == "" {
		t.Error("stale_reason must be present for stale tasks")
	}

	// Cron-only row: no runtime status -> source cron, not stale (no interval).
	cron := byName["cron_geo_ingest"]
	if cron["source"] != "cron" {
		t.Errorf("source = %v, want cron", cron["source"])
	}
	if cron["stale"] != false {
		t.Errorf("cron stale = %v, want false (no BTM interval)", cron["stale"])
	}
	if _, ok := cron["enabled"]; ok {
		t.Error("enabled must be omitted for cron-only rows")
	}
	if cron["last_success_at"] == nil {
		t.Error("last_success_at must be present")
	}
	if le, ok := cron["last_error"]; ok && le != "" {
		t.Errorf("last_error = %v, want empty/omitted", le)
	}
}

func TestHandleTaskLiveness_NeverSucceeded_Omitted(t *testing.T) {
	now := time.Now().UTC()
	h := newTestHandlers(t)
	h.TaskLivenessProvider = fakeLivenessProvider{rows: []liveness.Row{
		{TaskName: "never_ok", LastRunAt: now.Add(-time.Minute), ConsecutiveFailures: 3, LastError: "x"},
	}}
	h.SchedulerStatus = fakeSchedulerProvider{status: []apigateway.TaskStatus{
		{Name: "never_ok", Enabled: true, Interval: time.Minute},
	}}

	status, body := h.HandleTaskLiveness(httptest.NewRequest(http.MethodGet, "/api/dashboard/task-liveness", nil))
	resp := doLivenessRequest(t, status, body, http.StatusOK)
	tasks := resp["tasks"].([]any)
	task := tasks[0].(map[string]any)
	if _, ok := task["last_success_at"]; ok {
		t.Error("last_success_at must be omitted when task never succeeded")
	}
}
