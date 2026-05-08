package metrics

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	svc *service.MetricsService
}

func NewHandlers(svc *service.MetricsService) *Handlers {
	return &Handlers{svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/metrics", shared.Get(h.HandleMetrics))
	mux.Handle("GET /api/dashboard/metrics/trend", shared.Get(h.HandleMetricsTrend))
	mux.Handle("GET /api/dashboard/data-quality", shared.Get(h.HandleDataQuality))
}

func (h *Handlers) HandleMetrics(r *http.Request) (int, any) {
	metricType := r.URL.Query().Get("type")
	return http.StatusOK, h.svc.GetMetrics(metricType)
}

func (h *Handlers) HandleMetricsTrend(r *http.Request) (int, any) {
	metric := r.URL.Query().Get("metric")
	period := r.URL.Query().Get("period")
	if metric == "" {
		metric = "screening_rate"
	}
	if period == "" {
		period = "24h"
	}
	return http.StatusOK, h.svc.GetMetricsTrend(metric, period)
}

func (h *Handlers) HandleDataQuality(r *http.Request) (int, any) {
	return http.StatusOK, h.svc.CheckDataQuality("", "")
}
