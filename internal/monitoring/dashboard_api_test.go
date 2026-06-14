package monitoring

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	apistrategies "github.com/kaecer68/atlas-go/internal/monitoring/api/strategies"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/storage"
	"github.com/kaecer68/atlas-go/internal/taskexec"
)

func TestDashboardAPI_SetStorageReporter(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mgr := storage.NewLifecycleManager(t.TempDir())

	d.SetStorageReporter(mgr)

	if d.storageReport == nil {
		t.Fatal("SetStorageReporter did not set storageReport field")
	}

	// Verify type assertion works
	if _, ok := d.storageReport.(*storage.LifecycleManager); !ok {
		t.Fatal("storageReport is not a *storage.LifecycleManager")
	}
}

func TestDashboardAPI_RegisterRoutes_WithStorageReporter(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mgr := storage.NewLifecycleManager(t.TempDir())
	d.SetStorageReporter(mgr)

	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	// Verify /api/metrics/storage route exists
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/storage", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatal("/api/metrics/storage route not registered when storageReport is set")
	}
}

func TestDashboardAPI_RegisterRoutes_WithoutStorageReporter(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())

	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	// Verify /api/metrics/storage route does NOT exist
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/storage", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected /api/metrics/storage to return 404 when no reporter set, got %d", w.Code)
	}
}

func TestNewWiredIndustryServiceWithoutReplay(t *testing.T) {
	t.Setenv("ATLAS_REPLAY_DATA_PATH", "")

	eng := narrative.NewNarrativeEngine()
	svc := newWiredIndustryService(eng, nil)
	if svc == nil {
		t.Fatal("expected non-nil industry service")
	}
}

func TestNewWiredIndustryServiceWithReplay(t *testing.T) {
	tmpDir := t.TempDir()

	replayCSV := filepath.Join(tmpDir, "replay.csv")
	f, err := os.Create(replayCSV)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(f)
	w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for i := 0; i < 20; i++ {
		date := fmt.Sprintf("2026-01-%02d", 1+i)
		w.Write([]string{date, "2330", "TSMC", "10000", "100", "105", "99", "104"})
		w.Write([]string{date, "2303", "UMC", "5000", "50", "52", "49", "51"})
	}
	w.Flush()
	f.Close()

	t.Setenv("ATLAS_REPLAY_DATA_PATH", replayCSV)

	eng := narrative.NewNarrativeEngine()
	svc := newWiredIndustryService(eng, nil)
	if svc == nil {
		t.Fatal("expected non-nil industry service")
	}
}

func TestHandleRiskCalibration_NoGate(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-calibration", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK when no risk gate (returns not_available), got %d", w.Code)
	}
}

func TestHandleRiskCalibration_NoReportYet(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	gate := risk.NewRiskGate(risk.NewPreTradeGate(), risk.NewInTradeGate(), risk.NewPostTradeGate())
	d.SetRiskGate(gate)

	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-calibration", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleRiskCalibration_WithReport(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	gate := risk.NewRiskGate(risk.NewPreTradeGate(), risk.NewInTradeGate(), risk.NewPostTradeGate())
	gate.SetLastCalibration(&risk.CalibrationReport{
		Verdict:   "stable",
		Summary:   "thresholds optimal",
		Evaluated: 50,
	})
	d.SetRiskGate(gate)

	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-calibration", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleIndustryLinkage_ReturnsLeoAndMining(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Skipf("cannot chdir to project root: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterAllRoutes(mux, RouteOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-linkage?industry=leo_satellite", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for leo_satellite linkage, got %d", w.Code)
	}

	var resp struct {
		LinkageScore struct {
			SystemicImportance    float64 `json:"systemic_importance"`
			ShockPropagationSpeed float64 `json:"shock_propagation_speed"`
		} `json:"linkage_score"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode leo_satellite response: %v", err)
	}
	if resp.LinkageScore.SystemicImportance == 0 {
		t.Errorf("leo_satellite systemic_importance = 0, want > 0")
	}
	if resp.LinkageScore.ShockPropagationSpeed == 0 {
		t.Errorf("leo_satellite shock_propagation_speed = 0, want > 0")
	}
	t.Logf("leo_satellite: systemic=%.4f, propagation=%.4f",
		resp.LinkageScore.SystemicImportance, resp.LinkageScore.ShockPropagationSpeed)

	req2 := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-linkage?industry=mining", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for mining linkage, got %d", w2.Code)
	}
	var resp2 struct {
		LinkageScore struct {
			SystemicImportance    float64 `json:"systemic_importance"`
			ShockPropagationSpeed float64 `json:"shock_propagation_speed"`
		} `json:"linkage_score"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode mining response: %v", err)
	}
	if resp2.LinkageScore.SystemicImportance == 0 {
		t.Errorf("mining systemic_importance = 0, want > 0")
	}
	if resp2.LinkageScore.ShockPropagationSpeed == 0 {
		t.Errorf("mining shock_propagation_speed = 0, want > 0")
	}
	t.Logf("mining: systemic=%.4f, propagation=%.4f",
		resp2.LinkageScore.SystemicImportance, resp2.LinkageScore.ShockPropagationSpeed)
}

func TestHandleIndustryOverview_ReturnsLeoAndMining(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Skipf("cannot chdir to project root: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterAllRoutes(mux, RouteOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-overview", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for overview, got %d", w.Code)
	}

	var resp struct {
		Industries []struct {
			ID           string `json:"id"`
			LinkageScore *struct {
				SystemicImportance    float64 `json:"systemic_importance"`
				ShockPropagationSpeed float64 `json:"shock_propagation_speed"`
			} `json:"linkage_score"`
		} `json:"industries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}

	var foundLeo, foundMining bool
	for _, ind := range resp.Industries {
		if ind.ID == "leo_satellite" {
			foundLeo = true
			if ind.LinkageScore == nil {
				t.Error("leo_satellite linkage_score is nil")
			} else if ind.LinkageScore.SystemicImportance == 0 {
				t.Errorf("leo_satellite systemic_importance = 0, want > 0")
			}
		}
		if ind.ID == "mining" {
			foundMining = true
			if ind.LinkageScore == nil {
				t.Error("mining linkage_score is nil")
			} else if ind.LinkageScore.SystemicImportance == 0 {
				t.Errorf("mining systemic_importance = 0, want > 0")
			}
		}
	}
	if !foundLeo {
		t.Error("leo_satellite not found in industry overview")
	}
	if !foundMining {
		t.Error("mining not found in industry overview")
	}
}

func TestDashboardAPI_RegisterAllRoutes(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Skipf("cannot chdir to project root: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterAllRoutes(mux, RouteOptions{IncludeBacktest: true, IncludeSwagger: true})

	paths := []string{
		"/api/dashboard/macro-radar",
		"/api/narrative/events",
		"/api/control/audit-log",
		"/api/macro/snapshot/latest",
		"/api/cross-market/status",
		"/api/dashboard/experiment-inbox",
		"/api/dashboard/industry-overview",
		"/api/dashboard/live-status",
		"/api/backtest/status",
		"/api/docs",
	}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodPost, p, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("route %s not registered (got %d)", p, w.Code)
		}
	}
}

func TestDashboardAPI_RegisterStrategiesRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()

	d.RegisterStrategiesRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/strategies", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when handlers not set, got %d", w.Code)
	}

	d.SetStrategiesHandlers(apistrategies.NewHandlers(nil))
	mux2 := http.NewServeMux()
	d.RegisterStrategiesRoutes(mux2)
	req2 := httptest.NewRequest(http.MethodGet, "/api/strategies", nil)
	w2 := httptest.NewRecorder()
	mux2.ServeHTTP(w2, req2)
	if w2.Code == http.StatusNotFound {
		t.Error("/api/strategies route not registered after setting handlers")
	}
}

type fakeTaskStore struct{}

func (fakeTaskStore) CreateExecution(context.Context, domain.TaskExecution) error { return nil }
func (fakeTaskStore) UpdateExecution(context.Context, domain.TaskExecution) error { return nil }
func (fakeTaskStore) GetExecution(context.Context, string) (*domain.TaskExecution, error) {
	return nil, nil
}
func (fakeTaskStore) ListExecutions(context.Context, domain.ExecutionFilter) ([]domain.TaskExecution, error) {
	return nil, nil
}
func (fakeTaskStore) AppendEvent(context.Context, domain.TaskExecutionEvent) error { return nil }
func (fakeTaskStore) ListEventsAfter(context.Context, string, int64) ([]domain.TaskExecutionEvent, error) {
	return nil, nil
}
func (fakeTaskStore) UpsertLineage(context.Context, domain.ExperimentLineageRecord) error { return nil }
func (fakeTaskStore) GetLineage(context.Context, string) (*domain.ExperimentLineageRecord, error) {
	return nil, nil
}
func (fakeTaskStore) GetLineageChildren(context.Context, string) ([]domain.ExperimentLineageRecord, error) {
	return nil, nil
}
func (fakeTaskStore) InsertBaselineHistory(context.Context, domain.BaselineHistoryRecord) error {
	return nil
}
func (fakeTaskStore) ListBaselineHistory(context.Context, int) ([]domain.BaselineHistoryRecord, error) {
	return nil, nil
}
func (fakeTaskStore) InsertMetricPoints(context.Context, []domain.MetricTrendPoint) error { return nil }
func (fakeTaskStore) QueryMetricTrends(context.Context, domain.MetricTrendFilter) ([]domain.MetricTrendPoint, error) {
	return nil, nil
}

func TestDashboardAPI_RegisterTaskExecRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterTaskExecRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when taskManager nil, got %d", w.Code)
	}

	d.SetTaskManager(taskexec.NewManager(fakeTaskStore{}))
	mux2 := http.NewServeMux()
	d.RegisterTaskExecRoutes(mux2)
	req2 := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	w2 := httptest.NewRecorder()
	mux2.ServeHTTP(w2, req2)
	if w2.Code == http.StatusNotFound {
		t.Error("/api/tasks route not registered after setting taskManager")
	}
}

func TestDashboardAPI_ConfigHandler(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /api/config", configHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestDashboardAPI_SetGateway_InitializesProviders(t *testing.T) {
	d := NewDashboardAPI(".", ".", nil)
	called := make(map[string]bool)
	fetcher := func(ctx context.Context, channelID string) ([]byte, error) {
		called[channelID] = true
		return []byte(`{}`), nil
	}
	d.SetGateway(fetcher)
	if d.dataFetcher == nil {
		t.Fatal("dataFetcher not set")
	}
	if d.macroProvider == nil {
		t.Fatal("macroProvider not initialized")
	}
	if d.geoProvider == nil {
		t.Fatal("geoProvider not initialized")
	}
}

func TestDashboardAPI_SetLatestDrawdown(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	dd := &portfolio.DrawdownResult{MaxDrawdown: 0.15}
	d.SetLatestDrawdown(dd)
	got := d.GetLatestDrawdown()
	if got == nil {
		t.Fatal("expected non-nil drawdown")
	}
	if got.MaxDrawdown != 0.15 {
		t.Errorf("MaxDrawdown = %v, want 0.15", got.MaxDrawdown)
	}
}

func TestDashboardAPI_CrisisModeSetter(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	var active bool
	d.SetCrisisModeSetter(func(a bool) { active = a })
	d.InvokeCrisisModeSetter(true)
	if !active {
		t.Error("crisis mode setter was not invoked")
	}
	d.InvokeCrisisModeSetter(false)
	if active {
		t.Error("crisis mode setter did not set false")
	}
}

func TestDashboardAPI_CorrelationSetter(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	var rho float64
	d.SetCorrelationSetter(func(r float64) { rho = r })
	d.InvokeCorrelationSetter(0.75)
	if rho != 0.75 {
		t.Errorf("rho = %v, want 0.75", rho)
	}
}

func TestDashboardAPI_SetContext_NilStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	d.SetContext(context.TODO())
	if d.outcomeStore != nil {
		t.Error("outcomeStore should remain nil when repo nil")
	}
}

func TestDashboardAPI_GetLatestMacroSnapshot_Missing(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	snap, ok := d.GetLatestMacroSnapshot()
	if ok {
		t.Error("expected false when latest.json missing")
	}
	if snap.RecordedAt != 0 {
		t.Error("expected zero snapshot")
	}
}

func TestDashboardAPI_GetLatestMacroSnapshot_Present(t *testing.T) {
	tmp := t.TempDir()
	d := NewDashboardAPIWithGateway(tmp, tmp, nil, NoopFetcher())
	macroDir := filepath.Join(tmp, "data", "state", "macro")
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatal(err)
	}
	snap := marketdata.MacroDataSnapshot{RecordedAt: 1700000000, VIX: marketdata.MacroDataPoint{Symbol: "VIX", Value: 20.0}}
	b, _ := json.Marshal(snap)
	if err := os.WriteFile(filepath.Join(macroDir, "latest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := d.GetLatestMacroSnapshot()
	if !ok {
		t.Fatal("expected true when latest.json exists")
	}
	if got.RecordedAt != 1700000000 {
		t.Errorf("RecordedAt = %d, want 1700000000", got.RecordedAt)
	}
	if got.VIX.Value != 20.0 {
		t.Errorf("VIX.Value = %v, want 20.0", got.VIX.Value)
	}
}

func TestDashboardAPI_GetEventBus(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	if d.GetEventBus() != nil {
		t.Error("expected nil event bus initially")
	}
}

func TestDashboardAPI_GetMacroIngestor(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	if d.GetMacroIngestor() == nil {
		t.Error("expected non-nil macro ingestor")
	}
}

func TestDashboardAPI_GetEventLifecycleManager(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	if d.GetEventLifecycleManager() == nil {
		t.Error("expected non-nil lifecycle manager")
	}
}

func TestDashboardAPI_GetCrossMarketService(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	if d.GetCrossMarketService() != nil {
		t.Error("expected nil cross-market service before route registration")
	}
}

func TestDashboardAPI_GetIndustryService(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	if d.GetIndustryService() == nil {
		t.Error("expected non-nil industry service")
	}
}

func TestDashboardAPI_SetHealthManager(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mgr := portfolio.NewAgentHealthManager()
	d.SetHealthManager(mgr)
	if d.healthManager != mgr {
		t.Error("SetHealthManager did not set field")
	}
}

func TestDashboardAPI_SetJanusEngine(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	eng := janus.NewEngine()
	d.SetJanusEngine(eng)
	if d.janusEngine != eng {
		t.Error("SetJanusEngine did not set field")
	}
}

func TestDashboardAPI_SetPool(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	d.SetPool(nil)
	if d.pool != nil {
		t.Error("expected nil pool")
	}
}

func TestDashboardAPI_RegisterControlRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterControlRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/control/audit-log", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/control/audit-log route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterCrossMarketRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterCrossMarketRoutes(mux)

	if d.GetCrossMarketService() == nil {
		t.Error("expected cross-market service to be created")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cross-market/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/cross-market/status route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterMacroRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterMacroRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/macro/snapshot/latest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/macro/snapshot/latest route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterNarrativeRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterNarrativeRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/narrative/events", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/narrative/events route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterExperimentRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterExperimentRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/experiment-inbox", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/dashboard/experiment-inbox route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterIndustryRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterIndustryRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/industry-overview", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/dashboard/industry-overview route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterLiveRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterLiveRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/live-status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/dashboard/live-status route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterBacktestRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterBacktestRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/backtest/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/backtest/status route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterCircuitBreakerRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterCircuitBreakerRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/circuit-breaker", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/dashboard/circuit-breaker route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_RegisterPerformanceRoutes(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterPerformanceRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/performance-report", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/dashboard/performance-report route not registered (got %d)", w.Code)
	}
}

func TestDashboardAPI_SetRiskGate(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	gate := risk.NewRiskGate(risk.NewPreTradeGate(), risk.NewInTradeGate(), risk.NewPostTradeGate())
	d.SetRiskGate(gate)
	if d.riskGate != gate {
		t.Error("SetRiskGate did not set field")
	}
}

func TestDashboardAPI_SetCalibrationTask(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	_, err := d.RunCalibration()
	if err == nil {
		t.Error("expected error when calibration task nil")
	}
}

func TestDashboardAPI_CalibrateNarrative_NoEngine(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	d.narrativeEngine = nil
	_, err := d.CalibrateNarrative("")
	if err == nil {
		t.Error("expected error when narrative engine nil")
	}
}
