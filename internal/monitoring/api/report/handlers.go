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
	mux.HandleFunc("/api/report/list", h.HandleReportList)
	mux.HandleFunc("/api/dashboard/daily-summary", h.HandleDailySummary)
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

func (h *Handlers) HandleReportList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	reports, err := h.svc.LoadReportList()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

func (h *Handlers) HandleDailySummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	date := r.URL.Query().Get("date")
	report, err := h.svc.LoadDailySummary(date)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	shared.WriteJSON(w, http.StatusOK, report)
}
