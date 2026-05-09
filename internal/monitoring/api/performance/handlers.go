package performance

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

// Handlers holds the dependencies for performance report handlers.
type Handlers struct {
	Svc *service.PerformanceService
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(svc *service.PerformanceService) *Handlers {
	return &Handlers{Svc: svc}
}

// RegisterRoutes mounts performance report endpoints.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/performance-report", shared.Get(h.HandleReport))
	mux.Handle("GET /api/dashboard/performance-report/export", shared.GetRaw(h.HandleExport))
}

// HandleReport returns a JSON performance report for the requested period.
func (h *Handlers) HandleReport(r *http.Request) (int, any) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}

	report, err := h.Svc.GetPerformanceReport(period)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}

	return http.StatusOK, report
}

// HandleExport returns a Markdown performance report.
func (h *Handlers) HandleExport(w http.ResponseWriter, r *http.Request) (int, any) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}

	report, err := h.Svc.GetPerformanceReport(period)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}

	md := reporting.GenerateMarkdownReport(report)

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(md)); err != nil {
		return http.StatusInternalServerError, map[string]string{"error": "failed to write response"}
	}
	return 0, nil
}
