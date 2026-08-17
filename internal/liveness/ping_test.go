package liveness

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newPingEnv(t *testing.T) *fakeExecer {
	t.Helper()
	t.Setenv(PingEnvToken, "test-secret")
	return &fakeExecer{}
}

func doPing(t *testing.T, store *Store, token string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/task-liveness", bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("X-Liveness-Token", token)
	}
	rec := httptest.NewRecorder()
	HandlePing(store, rec, req)
	return rec
}

func TestHandlePing_NoTokenConfigured_FailsClosed(t *testing.T) {
	t.Setenv(PingEnvToken, "")
	ex := &fakeExecer{}
	rec := doPing(t, newStoreWithExec(ex), "whatever", `{"task_name":"cron_x","exit_code":0}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503 (fail-closed when token unset)", rec.Code)
	}
	if ex.calls != 0 {
		t.Fatalf("store must not be written when endpoint is unconfigured, got %d calls", ex.calls)
	}
}

func TestHandlePing_WrongToken_Rejected(t *testing.T) {
	ex := newPingEnv(t)
	rec := doPing(t, newStoreWithExec(ex), "wrong-secret", `{"task_name":"cron_x","exit_code":0}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
	if ex.calls != 0 {
		t.Fatalf("store must not be written with wrong token, got %d calls", ex.calls)
	}
}

func TestHandlePing_NoTokenHeader_Rejected(t *testing.T) {
	ex := newPingEnv(t)
	rec := doPing(t, newStoreWithExec(ex), "", `{"task_name":"cron_x","exit_code":0}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestHandlePing_SuccessExitCode_RecordsSuccess(t *testing.T) {
	ex := newPingEnv(t)
	rec := doPing(t, newStoreWithExec(ex), "test-secret", `{"task_name":"cron_x","exit_code":0,"duration_ms":1234}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ex.calls != 1 {
		t.Fatalf("expected 1 store write, got %d", ex.calls)
	}
	args := ex.args[0]
	if args[0] != "cron_x" {
		t.Errorf("task_name = %v, want cron_x", args[0])
	}
	if args[2] == nil {
		t.Error("last_success_at must be set for exit_code 0")
	}
	if args[3] != "" {
		t.Errorf("last_error = %q, want empty", args[3])
	}
	if args[5] != int64(1234) {
		t.Errorf("duration_ms = %v, want 1234", args[5])
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("response status = %v, want ok", resp["status"])
	}
}

func TestHandlePing_NonZeroExitCode_RecordsFailure(t *testing.T) {
	ex := newPingEnv(t)
	rec := doPing(t, newStoreWithExec(ex), "test-secret", `{"task_name":"cron_x","exit_code":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	args := ex.args[0]
	if args[2] != nil {
		t.Error("last_success_at must be nil for non-zero exit code")
	}
	if args[3] != "exit code 2" {
		t.Errorf("last_error = %q, want %q", args[3], "exit code 2")
	}
	if args[4] != 1 {
		t.Errorf("seed failures = %v, want 1", args[4])
	}
}

func TestHandlePing_NonZeroExitWithMessage(t *testing.T) {
	ex := newPingEnv(t)
	rec := doPing(t, newStoreWithExec(ex), "test-secret", `{"task_name":"cron_x","exit_code":3,"error":"disk full"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ex.args[0][3] != "disk full" {
		t.Errorf("last_error = %q, want %q", ex.args[0][3], "disk full")
	}
}

func TestHandlePing_MissingTaskName_400(t *testing.T) {
	ex := newPingEnv(t)
	rec := doPing(t, newStoreWithExec(ex), "test-secret", `{"exit_code":0}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	if ex.calls != 0 {
		t.Fatalf("no write expected, got %d", ex.calls)
	}
}

func TestHandlePing_MalformedBody_400(t *testing.T) {
	ex := newPingEnv(t)
	rec := doPing(t, newStoreWithExec(ex), "test-secret", `{not-json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestHandlePing_NilStore_503(t *testing.T) {
	t.Setenv(PingEnvToken, "test-secret")
	rec := doPing(t, nil, "test-secret", `{"task_name":"cron_x","exit_code":0}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not configured") {
		t.Errorf("body = %s, want 'not configured'", rec.Body.String())
	}
}
