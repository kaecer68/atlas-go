package circuitbreaker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func testAPIKey(t *testing.T) {
	t.Setenv("ATLAS_API_KEY", "test-key")
}

func setAPIKeyHeader(req *http.Request) {
	req.Header.Set("X-API-Key", "test-key")
}

func TestHandleGetState(t *testing.T) {
	tmpDir := t.TempDir()
	cbSvc := service.NewCircuitBreakerService(tmpDir)
	h := &Handlers{Svc: cbSvc}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/circuit-breaker", nil)
	rec := httptest.NewRecorder()

	handler := shared.Get(h.HandleGetState)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp service.CircuitBreakerStateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.State != "uninitialized" {
		t.Errorf("expected state 'uninitialized', got %q", resp.State)
	}
	if resp.Events == nil {
		t.Error("expected events to be empty slice, got nil")
	}
}

func TestHandleReset(t *testing.T) {
	testAPIKey(t)
	tmpDir := t.TempDir()

	statePath := filepath.Join(tmpDir, "data/state/circuit_breaker_state.json")
	stateDir := filepath.Dir(statePath)
	os.MkdirAll(stateDir, 0o755)
	initialState := `{"state":"paused","state_changed_at":"2026-01-01T00:00:00Z","consecutive_sl":3,"cooldown_until":"2026-01-01T00:15:00Z","intraday_peak":100000,"day_start_value":100000}`
	os.WriteFile(statePath, []byte(initialState), 0o644)

	logPath := filepath.Join(tmpDir, "data/state/circuit_breaker_log.jsonl")
	logDir := filepath.Dir(logPath)
	os.MkdirAll(logDir, 0o755)
	os.WriteFile(logPath, []byte(`{"timestamp":"2026-01-01T00:00:00Z","from_state":"normal","to_state":"paused","reason":"consecutive stop losses: 3","day_pnl_pct":-2.5,"drawdown_pct":3.1}`+"\n"), 0o644)

	cbSvc := service.NewCircuitBreakerService(tmpDir)
	h := &Handlers{Svc: cbSvc}

	body := `{"reason":"manual override"}`
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/circuit-breaker/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setAPIKeyHeader(req)
	rec := httptest.NewRecorder()

	handler := shared.Post(h.HandleReset)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resetResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resetResp); err != nil {
		t.Fatalf("failed to decode reset response: %v", err)
	}
	if resetResp["status"] != "reset" {
		t.Errorf("expected status 'reset', got %q", resetResp["status"])
	}

	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	var newState struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(stateData, &newState); err != nil {
		t.Fatalf("failed to unmarshal new state: %v", err)
	}
	if newState.State != "normal" {
		t.Errorf("expected state 'normal' after reset, got %q", newState.State)
	}
}

func TestHandleReset_EmptyBody(t *testing.T) {
	testAPIKey(t)
	tmpDir := t.TempDir()
	cbSvc := service.NewCircuitBreakerService(tmpDir)
	h := &Handlers{Svc: cbSvc}

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/circuit-breaker/reset", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	setAPIKeyHeader(req)
	rec := httptest.NewRecorder()

	handler := shared.Post(h.HandleReset)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestHandleReset_InvalidBody(t *testing.T) {
	testAPIKey(t)
	tmpDir := t.TempDir()
	cbSvc := service.NewCircuitBreakerService(tmpDir)
	h := &Handlers{Svc: cbSvc}

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/circuit-breaker/reset", strings.NewReader(`{invalid}`))
	req.Header.Set("Content-Type", "application/json")
	setAPIKeyHeader(req)
	rec := httptest.NewRecorder()

	handler := shared.Post(h.HandleReset)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestHandleGetState_MethodNotAllowed(t *testing.T) {
	testAPIKey(t)
	tmpDir := t.TempDir()
	cbSvc := service.NewCircuitBreakerService(tmpDir)
	h := &Handlers{Svc: cbSvc}

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/circuit-breaker", nil)
	setAPIKeyHeader(req)
	rec := httptest.NewRecorder()

	handler := shared.Get(h.HandleGetState)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandleReset_MethodNotAllowed(t *testing.T) {
	testAPIKey(t)
	tmpDir := t.TempDir()
	cbSvc := service.NewCircuitBreakerService(tmpDir)
	h := &Handlers{Svc: cbSvc}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/circuit-breaker/reset", nil)
	setAPIKeyHeader(req)
	rec := httptest.NewRecorder()

	handler := shared.Post(h.HandleReset)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}
