package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// SchedulerManager is the minimal interface for managing scheduled tasks.
type SchedulerManager interface {
	Status() []apigateway.TaskStatus
	Get(name string) (*apigateway.ScheduledTask, bool)
}

// SchedulerService wraps background task management for monitoring APIs.
type SchedulerService struct {
	mgr SchedulerManager
}

// NewSchedulerService creates a new SchedulerService.
func NewSchedulerService(mgr SchedulerManager) *SchedulerService {
	return &SchedulerService{mgr: mgr}
}

// status returns runtime status for all scheduled tasks.
func (s *SchedulerService) status() []apigateway.TaskStatus {
	if s == nil || s.mgr == nil {
		return nil
	}
	return s.mgr.Status()
}

// toggle enables or disables a named scheduled task.
func (s *SchedulerService) toggle(name string, enabled bool) error {
	if s == nil || s.mgr == nil {
		return fmt.Errorf("toggle scheduler task %s: manager not configured", name)
	}
	task, ok := s.mgr.Get(name)
	if !ok {
		return fmt.Errorf("toggle scheduler task %s: task not found", name)
	}
	task.SetEnabled(enabled)
	return nil
}

type Handlers struct {
	Svc *SchedulerService
}

func NewHandlers(svc *SchedulerService) *Handlers {
	return &Handlers{Svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/scheduler/status", shared.Get(h.HandleStatus))
	mux.Handle("POST /api/scheduler/toggle", shared.AdminPost(h.HandleToggle))
}

func (h *Handlers) HandleStatus(r *http.Request) (int, any) {
	return http.StatusOK, h.Svc.status()
}

func (h *Handlers) HandleToggle(r *http.Request) (int, any) {
	var req struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode request: %v", err)}
	}
	if req.Name == "" {
		return http.StatusBadRequest, map[string]string{"error": "name is required"}
	}
	if err := h.Svc.toggle(req.Name, req.Enabled); err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, map[string]any{"name": req.Name, "enabled": req.Enabled, "status": "ok"}
}
