package taskexec

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/taskexec"
)

type Handlers struct {
	manager *taskexec.Manager
}

func NewHandlers(manager *taskexec.Manager) *Handlers {
	return &Handlers{manager: manager}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	log.Printf("[TaskExec] registering routes: /api/tasks (POST/GET), /api/tasks/:id (GET/POST), /api/tasks/:id/events (SSE)")
	mux.HandleFunc("/api/tasks", h.handleTasks)
	mux.HandleFunc("/api/tasks/", h.handleTaskPath)
}

func (h *Handlers) handleTaskPath(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TaskExec] handleTaskPath: method=%s path=%s", r.Method, r.URL.Path)
	if strings.HasSuffix(r.URL.Path, "/events") {
		h.handleTaskEvents(w, r)
		return
	}
	h.handleTaskDetail(w, r)
}

type submitTaskRequest struct {
	TaskType       string                 `json:"task_type"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	Confirmed      bool                   `json:"confirmed,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
}

type submitTaskResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (h *Handlers) handleTasks(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TaskExec] handleTasks: method=%s path=%s", r.Method, r.URL.Path)
	switch r.Method {
	case http.MethodPost:
		var req submitTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
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
			http.Error(w, fmt.Sprintf("submit failed: %v", err), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(submitTaskResponse{
			ID:     exec.ID,
			Status: string(exec.Status),
		})

	case http.MethodGet:
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
			http.Error(w, fmt.Sprintf("list failed: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(executions)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) handleTaskDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/tasks/"):]
	if id == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		exec, err := h.manager.Get(r.Context(), id)
		if err != nil {
			http.Error(w, fmt.Sprintf("not found: %v", err), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(exec)

	case http.MethodPost:
		action := r.URL.Query().Get("action")
		switch action {
		case "cancel":
			if err := h.manager.Cancel(r.Context(), id); err != nil {
				http.Error(w, fmt.Sprintf("cancel failed: %v", err), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "retry":
			actor := r.Header.Get("X-User-ID")
			if actor == "" {
				actor = "anonymous"
			}
			exec, err := h.manager.Retry(r.Context(), id, actor)
			if err != nil {
				http.Error(w, fmt.Sprintf("retry failed: %v", err), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(submitTaskResponse{
				ID:     exec.ID,
				Status: string(exec.Status),
			})
		case "confirm":
			exec, err := h.manager.Get(r.Context(), id)
			if err != nil {
				http.Error(w, fmt.Sprintf("not found: %v", err), http.StatusNotFound)
				return
			}
			if exec.Status != domain.TaskStatusQueued || !exec.RequiresConfirmation {
				http.Error(w, "task does not require confirmation", http.StatusBadRequest)
				return
			}
			actor := r.Header.Get("X-User-ID")
			if actor == "" {
				actor = "anonymous"
			}
			newExec, err := h.manager.Submit(r.Context(), taskexec.SubmitRequest{
				TaskType:    string(exec.TaskType),
				Actor:       actor,
				ActorSource: "web_ui",
				Payload:     make(map[string]interface{}),
				Confirmed:   true,
			})
			if err != nil {
				http.Error(w, fmt.Sprintf("confirm failed: %v", err), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(submitTaskResponse{
				ID:     newExec.ID,
				Status: string(newExec.Status),
			})
		default:
			http.Error(w, "invalid action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) handleTaskEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path
	if !strings.HasSuffix(path, "/events") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	id := path[len("/api/tasks/") : len(path)-len("/events")]
	if id == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	ch, unsubscribe := h.manager.Subscribe(id)
	defer unsubscribe()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}
