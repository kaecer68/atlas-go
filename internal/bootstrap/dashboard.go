package bootstrap

import (
	"log"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func NewDashboardAPI(workDir, ledgerDir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
	return monitoring.NewDashboardAPI(workDir, ledgerDir, collector)
}

func RegisterDashboardRoutes(mux *http.ServeMux, dashboard *monitoring.DashboardAPI, enableSwagger, includeLive bool, rt *Runtime) {
	dashboard.RegisterRoutes(mux)

	if rt.Stores.AlertStore != nil {
		alertAPI := monitoring.NewAlertAPI(rt.Stores.AlertStore)
		alertAPI.RegisterRoutes(mux)
	}

	if rt.Repository != nil {
		dashboard.SetRepository(rt.Repository)
		log.Printf("[Repository] injected into dashboard API")
	}

	if rt.TaskManager != nil {
		dashboard.SetTaskManager(rt.TaskManager)
		dashboard.RegisterTaskExecRoutes(mux)
		log.Printf("[TaskExec] injected into dashboard API")
	}

	dashboard.RegisterNarrativeRoutes(mux)
	dashboard.RegisterControlRoutes(mux)
	dashboard.RegisterMacroRoutes(mux)
	dashboard.RegisterExperimentRoutes(mux)

	dashboard.RegisterPhase3Routes(mux)
	dashboard.RegisterBacktestRoutes(mux)
	dashboard.RegisterIndustryRoutes(mux)
	if includeLive {
		dashboard.RegisterLiveRoutes(mux)
	}

	if enableSwagger {
		dashboard.RegisterSwaggerRoutes(mux)
	}
}
