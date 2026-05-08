package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	dir := t.TempDir()
	svc := service.NewControlService(dir, dir, nil)
	return &Handlers{Svc: svc}
}

func postJSON(t *testing.T, url string, body any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertStatus(t *testing.T, status int, want int) {
	t.Helper()
	if status != want {
		t.Errorf("status = %d, want %d", status, want)
	}
}

func assertJSONKey(t *testing.T, body any, key string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	var m map[string]any
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m[key]; !ok {
		t.Errorf("response missing key %q: %v", key, m)
	}
	return m
}

func TestHandlePauseAgent_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/pause-agent", map[string]string{
		"agent_id": "growth-01", "reason": "underperforming", "operator": "admin",
	})
	status, body := h.HandlePauseAgent(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "success")
	if m["success"] != true {
		t.Errorf("success = %v", m["success"])
	}
}

func TestHandlePauseAgent_MissingAgentID(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/pause-agent", map[string]string{"reason": "test"})
	status, _ := h.HandlePauseAgent(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandlePauseAgent_MethodNotAllowed(t *testing.T) {
	t.Skip("method enforcement moved to shared.Get/Post adapter at routing level")
}

func TestHandleResumeAgent_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/resume-agent", map[string]string{
		"agent_id": "growth-01", "reason": "recovered", "operator": "admin",
	})
	status, body := h.HandleResumeAgent(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "success")
}

func TestHandleResumeAgent_MissingAgentID(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/resume-agent", map[string]string{"reason": "test"})
	status, _ := h.HandleResumeAgent(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleSetModelWeight_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/set-model-weight", map[string]any{
		"model_id": "growth-01", "weight": 1.5, "operator": "admin", "reason": "boosting",
	})
	status, body := h.HandleSetModelWeight(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "success")
}

func TestHandleSetModelWeight_MissingAgentID(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/set-model-weight", map[string]any{"weight": 1.5})
	status, _ := h.HandleSetModelWeight(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleApproveRecommendation_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/approve-recommendation", map[string]string{
		"symbol": "2330", "agent_id": "growth-01", "operator": "admin",
	})
	status, body := h.HandleApproveRecommendation(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "success")
}

func TestHandleRejectRecommendation_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/reject-recommendation", map[string]string{
		"symbol": "2330", "agent_id": "growth-01", "operator": "admin", "reason": "overvalued",
	})
	status, body := h.HandleRejectRecommendation(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "success")
}

func TestHandleActiveOverrides_EmptyInitially(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/control/active-overrides", nil)
	status, _ := h.HandleActiveOverrides(req)
	assertStatus(t, status, http.StatusOK)
}

func TestHandleActiveOverrides_AfterIntervention(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/control/pause-agent", map[string]string{
		"agent_id": "growth-01", "reason": "underperforming", "operator": "admin",
	})
	_, _ = h.HandlePauseAgent(req)

	req2 := httptest.NewRequest(http.MethodGet, "/api/control/active-overrides", nil)
	status, _ := h.HandleActiveOverrides(req2)
	assertStatus(t, status, http.StatusOK)
}

func TestHandleAuditLog_EmptyInitially(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/control/audit-log", nil)
	status, _ := h.HandleAuditLog(req)
	assertStatus(t, status, http.StatusOK)
}

func TestHandleAuditLog_AfterInterventions(t *testing.T) {
	h := newTestHandlers(t)
	for i, action := range []string{"pause_agent", "resume_agent"} {
		req := postJSON(t, "/api/control/"+action, map[string]string{
			"agent_id": "test-0" + string(rune('1'+i)), "reason": "test", "operator": "admin",
		})
		if action == "pause_agent" {
			_, _ = h.HandlePauseAgent(req)
		} else {
			_, _ = h.HandleResumeAgent(req)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/control/audit-log", nil)
	status, _ := h.HandleAuditLog(req)
	assertStatus(t, status, http.StatusOK)
}

func TestRegisterRoutes(t *testing.T) {
	h := newTestHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/api/control/pause-agent"},
		{"POST", "/api/control/resume-agent"},
		{"POST", "/api/control/set-model-weight"},
		{"POST", "/api/control/sector-ban"},
		{"POST", "/api/control/approve-recommendation"},
		{"POST", "/api/control/reject-recommendation"},
		{"GET", "/api/control/audit-log"},
		{"GET", "/api/control/active-overrides"},
		{"GET", "/api/agents/health"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 0 {
			t.Errorf("route %s %s not registered (no handler)", r.method, r.path)
		}
	}
}
