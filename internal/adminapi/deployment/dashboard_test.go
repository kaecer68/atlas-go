package deployment

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/fubonproxy"
)

func TestHandleDeploymentDashboard_503_WhenPMNil(t *testing.T) {
	h := NewHandlers(nil) // pm explicitly nil

	req := httptest.NewRequest(http.MethodGet, "/api/admin/live/deployment/dashboard", nil)
	rec := httptest.NewRecorder()

	h.HandleDeploymentDashboard(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for nil pm, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected error message in response body")
	}
}

func TestHandleDeploymentDashboard_200_WhenPMWired(t *testing.T) {
	pm := &fubonproxy.ProcessManager{}
	h := NewHandlers(pm)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/live/deployment/dashboard", nil)
	rec := httptest.NewRecorder()

	h.HandleDeploymentDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for wired pm, got %d", rec.Code)
	}

	var status fubonproxy.DeploymentStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode DeploymentStatus: %v", err)
	}

	// Zero-value is expected for a never-started PM.
	if status.SupervisorRunning {
		t.Error("expected SupervisorRunning=false for never-started PM")
	}
}

func TestHandleDeploymentDashboard_405_MethodNotAllowed(t *testing.T) {
	pm := &fubonproxy.ProcessManager{}
	h := NewHandlers(pm)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/admin/live/deployment/dashboard", nil)
		rec := httptest.NewRecorder()

		h.HandleDeploymentDashboard(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", method, rec.Code)
		}
	}
}
