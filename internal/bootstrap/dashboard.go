package bootstrap

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func NewDashboardAPI(workDir, ledgerDir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
	return monitoring.NewDashboardAPI(workDir, ledgerDir, collector)
}

// NewDashboardAPIWithGateway creates a DashboardAPI backed by the Gateway data fetcher.
// Prefer this constructor in production; it routes macro data through apigateway.Gateway.
func NewDashboardAPIWithGateway(workDir, ledgerDir string, collector *monitoring.MetricsCollector, fetcher monitoring.DataFetcher) *monitoring.DashboardAPI {
	return monitoring.NewDashboardAPIWithGateway(workDir, ledgerDir, collector, fetcher)
}

func RegisterDashboardRoutes(mux *http.ServeMux, dashboard *monitoring.DashboardAPI, enableSwagger, includeLive bool, rt *Runtime) {
	if rt.Repository != nil {
		dashboard.SetRepository(rt.Repository)
		logging.Info("repository", "injected_to_dashboard")
	}

	dashboard.RegisterRoutes(mux)

	if rt.Stores.AlertStore != nil {
		alertAPI := monitoring.NewAlertAPI(rt.Stores.AlertStore)
		alertAPI.RegisterRoutes(mux)
	}

	if rt.TaskManager != nil {
		dashboard.SetTaskManager(rt.TaskManager)
		dashboard.RegisterTaskExecRoutes(mux)
		logging.Info("taskexec", "injected_to_dashboard")
	}

	dashboard.RegisterNarrativeRoutes(mux)
	dashboard.RegisterControlRoutes(mux)
	dashboard.RegisterMacroRoutes(mux)
	dashboard.RegisterExperimentRoutes(mux)

	dashboard.RegisterBacktestRoutes(mux)
	dashboard.RegisterIndustryRoutes(mux)
	if includeLive {
		dashboard.RegisterLiveRoutes(mux)
	}

	if enableSwagger {
		dashboard.RegisterSwaggerRoutes(mux)
	}
}
