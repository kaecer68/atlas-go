package metrics

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// StorageReporter provides the latest storage cleanup report.
type StorageReporter interface {
	LastReport() any
}

type Handlers struct {
	svc            *service.MetricsService
	storageReport  StorageReporter
	qualityChecker service.DataQualityCheckerInterface
}

func NewHandlers(svc *service.MetricsService) *Handlers {
	return &Handlers{svc: svc}
}

// WithQualityChecker attaches a data quality checker so HandleDataQuality
// reports real checks instead of an empty placeholder report.
func (h *Handlers) WithQualityChecker(c service.DataQualityCheckerInterface) *Handlers {
	h.qualityChecker = c
	return h
}

// WithStorageReporter attaches a storage reporter for the /api/metrics/storage endpoint.
func (h *Handlers) WithStorageReporter(r StorageReporter) *Handlers {
	h.storageReport = r
	return h
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/metrics", shared.Get(h.HandleMetrics))
	mux.Handle("GET /api/dashboard/metrics/trend", shared.Get(h.HandleMetricsTrend))
	mux.Handle("GET /api/dashboard/metrics/thresholds", shared.Get(h.HandleThresholds))
	mux.Handle("GET /api/dashboard/data-quality", shared.Get(h.HandleDataQuality))
	if h.storageReport != nil {
		mux.Handle("GET /api/metrics/storage", shared.Get(h.HandleStorage))
	}
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
	return http.StatusOK, h.svc.CheckDataQuality(h.qualityChecker)
}

func (h *Handlers) HandleThresholds(r *http.Request) (int, any) {
	return http.StatusOK, h.svc.GetThresholds()
}

func (h *Handlers) HandleStorage(r *http.Request) (int, any) {
	return http.StatusOK, h.storageReport.LastReport()
}
