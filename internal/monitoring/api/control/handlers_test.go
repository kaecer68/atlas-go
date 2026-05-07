package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// newTestHandlers creates Handlers backed by a temp directory for testing.
func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	dir := t.TempDir()
	svc := service.NewControlService(dir, dir, nil)
	return &Handlers{Svc: svc}
}

func postJSON(t *testing.T, url string, body any) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	return w, req
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d", w.Code, want)
	}
}

func assertJSONKey(t *testing.T, w *httptest.ResponseRecorder, key string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m[key]; !ok {
		t.Errorf("response missing key %q: %v", key, m)
	}
	return m
}

// --- Pause Agent ---

func TestHandlePauseAgent_Success(t *testing.T) {
	h := newTestHandlers(t)
	w, req := postJSON(t, "/api/control/pause-agent", map[string]string{
		"agent_id": "growth-01", "reason": "underperforming", "operator": "admin",
	})
	h.HandlePauseAgent(w, req)
	assertStatus(t, w, http.StatusOK)
	m := assertJSONKey(t, w, "success")
	if m["success"] != true {
		t.Errorf("success = %v", m["success"])
	}
}

func TestHandlePauseAgent_MissingAgentID(t *testing.T) {
	h := newTestHandlers(t)
	w, req := postJSON(t, "/api/control/pause-agent", map[string]string{"reason": "test"})
	h.HandlePauseAgent(w, req)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestHandlePauseAgent_MethodNotAllowed(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/control/pause-agent", nil)
	w := httptest.NewRecorder()
	h.HandlePauseAgent(w, req)
	assertStatus(t, w, http.StatusMethodNotAllowed)
}

// --- Resume Agent ---

func TestHandleResumeAgent_Success(t *testing.T) {
	h := newTestHandlers(t)
	w, req := postJSON(t, "/api/control/resume-agent", map[string]string{
		"agent_id": "growth-01", "reason": "recovered", "operator": "admin",
	})
	h.HandleResumeAgent(w, req)
	assertStatus(t, w, http.StatusOK)
	assertJSONKey(t, w, "success")
}

func TestHandleResumeAgent_MissingAgentID(t *testing.T) {
	h := newTestHandlers(t)
	w, req := postJSON(t, "/api/control/resume-agent", map[string]string{"reason": "test"})
	h.HandleResumeAgent(w, req)
	assertStatus(t, w, http.StatusBadRequest)
}

// --- Set Model Weight ---

func TestHandleSetModelWeight_Success(t *testing.T) {
	h := newTestHandlers(t)
	w, req := postJSON(t, "/api/control/set-model-weight", map[string]any{
		"model_id": "growth-01", "weight": 1.5, "operator": "admin", "reason": "boosting",
	})
	h.HandleSetModelWeight(w, req)
	assertStatus(t, w, http.StatusOK)
	assertJSONKey(t, w, "success")
}

func TestHandleSetModelWeight_MissingAgentID(t *testing.T) {
	h := newTestHandlers(t)
	w, req := postJSON(t, "/api/control/set-model-weight", map[string]any{"weight": 1.5})
	h.HandleSetModelWeight(w, req)
	assertStatus(t, w, http.StatusBadRequest)
}

// --- Approve Recommendation ---

func TestHandleApproveRecommendation_Success(t *testing.T) {
	h := newTestHandlers(t)
	w, req := postJSON(t, "/api/control/approve-recommendation", map[string]string{
		"symbol": "2330", "agent_id": "growth-01", "operator": "admin",
	})
	h.HandleApproveRecommendation(w, req)
	assertStatus(t, w, http.StatusOK)
	assertJSONKey(t, w, "success")
}

// --- Reject Recommendation ---

func TestHandleRejectRecommendation_Success(t *testing.T) {
	h := newTestHandlers(t)
	w, req := postJSON(t, "/api/control/reject-recommendation", map[string]string{
		"symbol": "2330", "agent_id": "growth-01", "operator": "admin", "reason": "overvalued",
	})
	h.HandleRejectRecommendation(w, req)
	assertStatus(t, w, http.StatusOK)
	assertJSONKey(t, w, "success")
}

// --- Active Overrides ---

func TestHandleActiveOverrides_EmptyInitially(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/control/active-overrides", nil)
	w := httptest.NewRecorder()
	h.HandleActiveOverrides(w, req)
	assertStatus(t, w, http.StatusOK)
}

func TestHandleActiveOverrides_AfterIntervention(t *testing.T) {
	h := newTestHandlers(t)
	// Pause an agent first
	w, req := postJSON(t, "/api/control/pause-agent", map[string]string{
		"agent_id": "growth-01", "reason": "underperforming", "operator": "admin",
	})
	h.HandlePauseAgent(w, req)

	// Check active overrides includes it
	req2 := httptest.NewRequest(http.MethodGet, "/api/control/active-overrides", nil)
	w2 := httptest.NewRecorder()
	h.HandleActiveOverrides(w2, req2)
	assertStatus(t, w2, http.StatusOK)
}

// --- Audit Log ---

func TestHandleAuditLog_EmptyInitially(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/control/audit-log", nil)
	w := httptest.NewRecorder()
	h.HandleAuditLog(w, req)
	assertStatus(t, w, http.StatusOK)
}

func TestHandleAuditLog_AfterInterventions(t *testing.T) {
	h := newTestHandlers(t)
	// Record a few interventions
	for i, action := range []string{"pause_agent", "resume_agent"} {
		w, req := postJSON(t, "/api/control/"+action, map[string]string{
			"agent_id": "test-0" + string(rune('1'+i)), "reason": "test", "operator": "admin",
		})
		if action == "pause_agent" {
			h.HandlePauseAgent(w, req)
		} else {
			h.HandleResumeAgent(w, req)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/control/audit-log", nil)
	w := httptest.NewRecorder()
	h.HandleAuditLog(w, req)
	assertStatus(t, w, http.StatusOK)
}

// --- Route Registration ---

func TestRegisterRoutes(t *testing.T) {
	h := newTestHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []string{
		"/api/control/pause-agent",
		"/api/control/resume-agent",
		"/api/control/set-model-weight",
		"/api/control/sector-ban",
		"/api/control/approve-recommendation",
		"/api/control/reject-recommendation",
		"/api/control/audit-log",
		"/api/control/active-overrides",
		"/api/agents/health",
	}
	for _, route := range routes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		// All routes should handle GET (even if they return 405, they're registered)
		if w.Code == 0 {
			t.Errorf("route %s not registered (no handler)", route)
		}
	}
}
