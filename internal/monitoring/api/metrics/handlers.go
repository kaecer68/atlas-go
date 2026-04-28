package metrics

import (
	"encoding/json"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	svc *service.MetricsService
}

func NewHandlers(svc *service.MetricsService) *Handlers {
	return &Handlers{svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/metrics", h.HandleMetrics)
	mux.HandleFunc("/api/dashboard/metrics/trend", h.HandleMetricsTrend)
	mux.HandleFunc("/api/dashboard/data-quality", h.HandleDataQuality)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// HandleMetrics handles GET /api/dashboard/metrics
func (h *Handlers) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metricType := r.URL.Query().Get("type")
	response := h.svc.GetMetrics(metricType)

	writeJSON(w, http.StatusOK, response)
}

// HandleMetricsTrend handles GET /api/dashboard/metrics/trend
func (h *Handlers) HandleMetricsTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metric := r.URL.Query().Get("metric")
	period := r.URL.Query().Get("period")

	if metric == "" {
		metric = "screening_rate"
	}
	if period == "" {
		period = "24h"
	}

	result := h.svc.GetMetricsTrend(metric, period)

	writeJSON(w, http.StatusOK, result)
}

// HandleDataQuality handles GET /api/dashboard/data-quality
func (h *Handlers) HandleDataQuality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Note: workDir and ledgerDir should be configured or obtained from the service
	// For now, we use empty strings and let the service handle defaults
	report := h.svc.CheckDataQuality("", "")

	writeJSON(w, http.StatusOK, report)
}
