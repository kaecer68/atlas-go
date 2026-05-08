package report

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	svc *service.ReportService
}

func NewHandlers(svc *service.ReportService) *Handlers {
	return &Handlers{svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/report/latest", h.HandleLatestReport)
	mux.Handle("GET /api/report/list", shared.Get(h.HandleReportList))
	mux.Handle("GET /api/dashboard/daily-summary", shared.Get(h.HandleDailySummary))
}

func (h *Handlers) HandleLatestReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	content, filename, err := h.svc.LoadLatestReport()
	if err != nil {
		if err.Error() == "no reports directory found" {
			shared.WriteJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		if err.Error() == "no backtest report found" {
			shared.WriteJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		shared.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("X-Filename", filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *Handlers) HandleReportList(r *http.Request) (int, any) {
	reports, err := h.svc.LoadReportList()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, map[string]any{"reports": reports}
}

func (h *Handlers) HandleDailySummary(r *http.Request) (int, any) {
	date := r.URL.Query().Get("date")
	report, err := h.svc.LoadDailySummary(date)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, report
}
