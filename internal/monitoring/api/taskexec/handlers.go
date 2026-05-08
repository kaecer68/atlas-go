package taskexec

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/taskexec"
)

type Handlers struct {
	manager *taskexec.Manager
}

func NewHandlers(manager *taskexec.Manager) *Handlers {
	return &Handlers{manager: manager}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	log.Printf("[TaskExec] registering RESTful routes")
	mux.Handle("POST /api/tasks", shared.Post(h.HandleCreateTask))
	mux.Handle("GET /api/tasks", shared.Get(h.HandleListTasks))
	mux.Handle("GET /api/tasks/{id}", shared.Get(h.HandleGetTask))
	mux.Handle("POST /api/tasks/{id}/cancel", shared.Post(h.HandleCancelTask))
	mux.Handle("POST /api/tasks/{id}/retry", shared.Post(h.HandleRetryTask))
	mux.Handle("POST /api/tasks/{id}/confirm", shared.Post(h.HandleConfirmTask))
	mux.Handle("GET /api/tasks/{id}/events", shared.GetRaw(h.HandleTaskEvents))
}

type submitTaskRequest struct {
	TaskType       string         `json:"task_type"`
	Payload        map[string]any `json:"payload,omitempty"`
	Confirmed      bool           `json:"confirmed,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
}

type submitTaskResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (h *Handlers) HandleCreateTask(r *http.Request) (int, any) {
	var req submitTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid request: %v", err)}
	}

	actor := r.Header.Get("X-User-ID")
	if actor == "" {
		actor = "anonymous"
	}

	submitReq := taskexec.SubmitRequest{
		TaskType:       req.TaskType,
		Actor:          actor,
		ActorSource:    "web_ui",
		Payload:        req.Payload,
		Confirmed:      req.Confirmed,
		IdempotencyKey: req.IdempotencyKey,
	}

	exec, err := h.manager.Submit(r.Context(), submitReq)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("submit failed: %v", err)}
	}

	return http.StatusOK, submitTaskResponse{
		ID:     exec.ID,
		Status: string(exec.Status),
	}
}

func (h *Handlers) HandleListTasks(r *http.Request) (int, any) {
	filter := domain.ExecutionFilter{}
	if taskType := r.URL.Query().Get("task_type"); taskType != "" {
		filter.TaskType = taskType
	}
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = status
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}

	executions, err := h.manager.List(r.Context(), filter)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list failed: %v", err)}
	}
	return http.StatusOK, executions
}

func (h *Handlers) HandleGetTask(r *http.Request) (int, any) {
	id := r.PathValue("id")
	exec, err := h.manager.Get(r.Context(), id)
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": fmt.Sprintf("not found: %v", err)}
	}
	return http.StatusOK, exec
}

func (h *Handlers) HandleCancelTask(r *http.Request) (int, any) {
	id := r.PathValue("id")
	if err := h.manager.Cancel(r.Context(), id); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cancel failed: %v", err)}
	}
	return http.StatusNoContent, nil
}

func (h *Handlers) HandleRetryTask(r *http.Request) (int, any) {
	id := r.PathValue("id")
	actor := r.Header.Get("X-User-ID")
	if actor == "" {
		actor = "anonymous"
	}
	exec, err := h.manager.Retry(r.Context(), id, actor)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("retry failed: %v", err)}
	}
	return http.StatusOK, submitTaskResponse{
		ID:     exec.ID,
		Status: string(exec.Status),
	}
}

func (h *Handlers) HandleConfirmTask(r *http.Request) (int, any) {
	id := r.PathValue("id")
	exec, err := h.manager.Get(r.Context(), id)
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": fmt.Sprintf("not found: %v", err)}
	}
	if exec.Status != domain.TaskStatusQueued || !exec.RequiresConfirmation {
		return http.StatusBadRequest, map[string]string{"error": "task does not require confirmation"}
	}
	actor := r.Header.Get("X-User-ID")
	if actor == "" {
		actor = "anonymous"
	}
	newExec, err := h.manager.Submit(r.Context(), taskexec.SubmitRequest{
		TaskType:    string(exec.TaskType),
		Actor:       actor,
		ActorSource: "web_ui",
		Payload:     make(map[string]any),
		Confirmed:   true,
	})
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("confirm failed: %v", err)}
	}
	return http.StatusOK, submitTaskResponse{
		ID:     newExec.ID,
		Status: string(newExec.Status),
	}
}

func (h *Handlers) HandleTaskEvents(w http.ResponseWriter, r *http.Request) (int, any) {
	id := r.PathValue("id")
	if id == "" {
		return http.StatusBadRequest, map[string]string{"error": "task id required"}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return http.StatusInternalServerError, map[string]string{"error": "streaming not supported"}
	}

	ctx := r.Context()
	ch, unsubscribe := h.manager.Subscribe(id)
	defer unsubscribe()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return 0, nil
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ctx.Done():
			return 0, nil
		}
	}
}
