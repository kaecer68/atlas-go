package industry

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func TestIndustryHandlersSmoke(t *testing.T) {
	_ = config.GetParametersConfig()

	classifier := industry.DefaultClassification()
	seasonalEngine := industry.NewSeasonalEngine()
	cycleTracker := industry.NewCycleTracker()
	linkageAnalyzer := industry.NewLinkageAnalyzer()
	riskMonitor := industry.NewRiskMonitor()
	siliconTracker := industry.NewSiliconCycleTracker()

	// Wire as in dashboard_api.go
	cycleTracker.SetExternalValidators(seasonalEngine, linkageAnalyzer)
	seasonalEngine.SetLinkageGraph(linkageAnalyzer.GetSupplyChainGraph())
	linkageAnalyzer.SetCycleProvider(cycleTracker)

	baseline := marketdata.MacroDataSnapshot{
		Oil: marketdata.MacroDataPoint{Value: 75.0},
		DXY: marketdata.MacroDataPoint{Value: 103.0},
	}
	modulator := industry.NewDynamicEnvModulator(baseline, baseline)
	modulator.RecordSnapshot(baseline)
	seasonalEngine.SetDynamicEnv(modulator)

	calendar := industry.NewEventCalendar()
	calendar.RefreshEvents(time.Now())

	svc := service.NewIndustryService(
		classifier,
		seasonalEngine,
		cycleTracker,
		linkageAnalyzer,
		riskMonitor,
		siliconTracker,
		calendar,
	)

	handlers := &Handlers{Svc: svc}

	apis := []struct {
		name string
		req  *http.Request
		fn   func(*http.Request) (int, any)
	}{
		{"classification", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-classification", nil), handlers.HandleIndustryClassification},
		{"overview", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-overview", nil), handlers.HandleIndustryOverview},
		{"seasonality", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-seasonality", nil), handlers.HandleIndustrySeasonality},
		{"seasonality-calendar", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-seasonality-calendar", nil), handlers.HandleIndustrySeasonalityCalendar},
		{"graph", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-graph", nil), handlers.HandleIndustryGraph},
		{"cycle-status-card", httptest.NewRequest(http.MethodGet, "/api/dashboard/cycle-status-card", nil), handlers.HandleCycleStatusCard},
	}

	for _, api := range apis {
		status, body := api.fn(api.req)
		t.Logf("%s status: %d", api.name, status)
		if status != 200 {
			t.Errorf("%s returned status %d, body: %v", api.name, status, body)
		}
	}
}
