package pipeline

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// ReportHandlers provides report-related endpoints (latest, list, daily-summary).
type ReportHandlers struct {
	svc *service.ReportService
}

// NewReportHandlers creates handlers for the report API.
func NewReportHandlers(svc *service.ReportService) *ReportHandlers {
	return &ReportHandlers{svc: svc}
}

func (h *ReportHandlers) RegisterRoutes(mux *http.ServeMux) {
	// Backtest report markdown (singular). Not deprecated — the admin reports
	// page consumes this via GET. Do not confuse with dailyreport's
	// /api/reports/* (plural, JSON). See docs/operations/tier-boundary.md §2.4.
	mux.Handle("GET /api/report/latest", shared.GetRaw(h.HandleLatestReport))
	mux.Handle("GET /api/report/list", shared.Get(h.HandleReportList))
	// Deprecated: daily-summary logic has been integrated into the dailyreport
	// module (GET /api/reports/latest). See docs/operations/tier-boundary.md §2.5.
	mux.Handle("GET /api/dashboard/daily-summary", shared.Get(h.HandleDailySummary))
}

func (h *ReportHandlers) HandleLatestReport(w http.ResponseWriter, r *http.Request) (int, any) {
	content, filename, err := h.svc.LoadLatestReport()
	if err != nil {
		switch err.Error() {
		case "no reports directory found", "no backtest report found", "no backtest window found":
			return http.StatusNotFound, map[string]string{"error": err.Error()}
		}
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("X-Filename", filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
	return 0, nil
}

func (h *ReportHandlers) HandleReportList(r *http.Request) (int, any) {
	reports, err := h.svc.LoadReportList()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, map[string]any{"reports": reports}
}

func (h *ReportHandlers) HandleDailySummary(r *http.Request) (int, any) {
	date := r.URL.Query().Get("date")
	report, err := h.svc.LoadDailySummary(date)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, report
}
