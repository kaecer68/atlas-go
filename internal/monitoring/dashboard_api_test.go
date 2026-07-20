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
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
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
	t.Setenv("ATLAS_API_KEY", "")
	t.Setenv("ATLAS_ADMIN_KEY", "")
	t.Setenv("ATLAS_ENV", "")
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-calibration", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no risk gate, got %d", w.Code)
	}
}

// T4: RegisterAllRoutes must not capture riskGate by value at call time;
// SetRiskGate called later must still reach the registered handlers.
func TestDashboardAPI_RiskGate_WiredAfterRegisterAllRoutes(t *testing.T) {
	t.Setenv("ATLAS_API_KEY", "")
	t.Setenv("ATLAS_ADMIN_KEY", "")
	t.Setenv("ATLAS_ENV", "")
	d := NewDashboardAPIWithGateway(t.TempDir(), t.TempDir(), nil, NoopFetcher())

	mux := http.NewServeMux()
	d.RegisterAllRoutes(mux, RouteOptions{IncludeBacktest: false, IncludeSwagger: false})

	gate := risk.NewRiskGate(risk.NewPreTradeGate(), risk.NewInTradeGate(), risk.NewPostTradeGate())
	d.SetRiskGate(gate)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (RiskGate wired even after RegisterAllRoutes), got %d", w.Code)
	}
}

func TestHandleRiskCalibration_NoReportYet(t *testing.T) {
	t.Setenv("ATLAS_API_KEY", "")
	t.Setenv("ATLAS_ADMIN_KEY", "")
	t.Setenv("ATLAS_ENV", "")
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
	t.Setenv("ATLAS_API_KEY", "")
	t.Setenv("ATLAS_ADMIN_KEY", "")
	t.Setenv("ATLAS_ENV", "")
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
	t.Setenv("ATLAS_API_KEY", "")
	t.Setenv("ATLAS_ADMIN_KEY", "")
	t.Setenv("ATLAS_ENV", "")
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
	t.Setenv("ATLAS_API_KEY", "")
	t.Setenv("ATLAS_ADMIN_KEY", "")
	t.Setenv("ATLAS_ENV", "")
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

	d.SetStrategiesHandlers(apistrategies.NewHandlers(nil, nil))
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
	fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
		called[channelID] = true
		return []byte(`{}`), FetchMeta{}, nil
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

// TestHandleAgentNames verifies the /api/dashboard/agent-names endpoint returns
// the agent registry from configs/agents.json with id/name/skill/layer fields,
// and gracefully returns an empty array when the file is missing.
func TestHandleAgentNames(t *testing.T) {
	tmpDir := t.TempDir()
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	registryJSON := `{
		"version": 1,
		"agents": [
			{"id":"agent-a","name":"Alpha","skill":"tech","layer":"sector","prompt_file":"x.md","enabled":true,"universe":[],"primary_metrics":[]},
			{"id":"agent-b","name":"Beta","skill":"value","layer":"stock","prompt_file":"y.md","enabled":true,"universe":[],"primary_metrics":[]}
		]
	}`
	if err := os.WriteFile(filepath.Join(configsDir, "agents.json"), []byte(registryJSON), 0o644); err != nil {
		t.Fatalf("write agents.json: %v", err)
	}

	d := NewDashboardAPIWithGateway(tmpDir, ".", nil, NoopFetcher())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/agent-names", nil)
	d.handleAgentNames(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Agents []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Skill string `json:"skill"`
			Layer string `json:"layer"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(resp.Agents))
	}
	if resp.Agents[0].ID != "agent-a" || resp.Agents[0].Name != "Alpha" || resp.Agents[0].Skill != "tech" {
		t.Errorf("agent-a mismatch: %+v", resp.Agents[0])
	}
	if resp.Agents[1].ID != "agent-b" || resp.Agents[1].Name != "Beta" {
		t.Errorf("agent-b mismatch: %+v", resp.Agents[1])
	}
}

// TestHandleAgentNames_MissingFile verifies graceful degradation when
// configs/agents.json is absent (returns 200 + empty array).
func TestHandleAgentNames_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDashboardAPIWithGateway(tmpDir, ".", nil, NoopFetcher())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/agent-names", nil)
	d.handleAgentNames(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 even with missing file, got %d", rec.Code)
	}
	var resp struct {
		Agents []map[string]string `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Agents) != 0 {
		t.Errorf("expected empty agents array, got %d entries", len(resp.Agents))
	}
}

func TestRegisterCrossMarketRoutes_DegradedMetricsOnIncWired(t *testing.T) {
	collector := NewMetricsCollector()
	d := NewDashboardAPIWithGateway(".", ".", collector, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterCrossMarketRoutes(mux)

	svc := d.GetCrossMarketService()
	if svc == nil {
		t.Fatal("expected cross-market service to be created")
	}

	dm := svc.GetDegradedMetrics()
	if dm == nil {
		t.Fatal("expected degraded metrics to be set on cross-market service")
	}

	dm.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Inc()

	m, ok := collector.GetMetric("degraded_activations", map[string]string{"crossmarket": "snapshot_stale"})
	if !ok {
		t.Fatalf("expected degraded_activations metric to be recorded in collector")
	}
	if m.Value != 1.0 {
		t.Fatalf("expected metric value 1.0, got %v", m.Value)
	}
}

func TestRegisterCrossMarketRoutes_DegradedEndpointRegistered(t *testing.T) {
	collector := NewMetricsCollector()
	d := NewDashboardAPIWithGateway(".", ".", collector, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterCrossMarketRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/degraded", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected /api/degraded status 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var snap map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&snap); err != nil {
		t.Fatalf("decode /degraded response: %v", err)
	}
	if _, ok := snap["degraded_activations"]; !ok {
		t.Error("expected 'degraded_activations' key in /degraded snapshot")
	}
	if _, ok := snap["provider_errors"]; !ok {
		t.Error("expected 'provider_errors' key in /degraded snapshot")
	}
	if _, ok := snap["timestamp"].(string); !ok {
		t.Error("expected 'timestamp' (string) key in /degraded snapshot")
	}
}

func TestDashboardAPI_NarrativeEngine(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	eng := d.NarrativeEngine()
	if eng == nil {
		t.Fatal("expected non-nil narrative engine from public getter")
	}
	if eng == d.NarrativeEngine() {
		// Same pointer should be returned (cacheable, no new instance per call).
	} else {
		t.Errorf("expected stable pointer across calls")
	}
}

type mockStressStore struct {
	upserted    []ledger.StressRow
	upsertedGeo []ledger.GeopoliticalRow
	returnErr   error
}

func (m *mockStressStore) UpsertRegime(ctx context.Context, row ledger.RegimeRow) error { return nil }
func (m *mockStressStore) LoadRegimeByDate(ctx context.Context, date string) (ledger.RegimeRow, bool, error) {
	return ledger.RegimeRow{}, false, nil
}
func (m *mockStressStore) LoadRegimeByDateAll(ctx context.Context, date string) (ledger.RegimeRow, bool, error) {
	return ledger.RegimeRow{}, false, nil
}
func (m *mockStressStore) LoadRegimeHistory(ctx context.Context, limit int) ([]ledger.RegimeRow, error) {
	return nil, nil
}
func (m *mockStressStore) LoadRegimeHistoryAll(ctx context.Context, limit int) ([]ledger.RegimeRow, error) {
	return nil, nil
}
func (m *mockStressStore) UpsertStress(ctx context.Context, row ledger.StressRow) error {
	m.upserted = append(m.upserted, row)
	return m.returnErr
}
func (m *mockStressStore) LoadStressByDate(ctx context.Context, date string) (ledger.StressRow, bool, error) {
	return ledger.StressRow{}, false, nil
}
func (m *mockStressStore) LoadStressByDateAll(ctx context.Context, date string) (ledger.StressRow, bool, error) {
	return ledger.StressRow{}, false, nil
}
func (m *mockStressStore) LoadStressHistory(ctx context.Context, limit int) ([]ledger.StressRow, error) {
	return nil, nil
}
func (m *mockStressStore) LoadStressHistoryAll(ctx context.Context, limit int) ([]ledger.StressRow, error) {
	return nil, nil
}
func (m *mockStressStore) UpsertEventCalendar(ctx context.Context, row ledger.EventCalendarRow) error {
	return nil
}
func (m *mockStressStore) LoadEventCalendarByDate(ctx context.Context, date string) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}
func (m *mockStressStore) LoadEventCalendarByDateAll(ctx context.Context, date string) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}
func (m *mockStressStore) LoadEventCalendarRange(ctx context.Context, startDate, endDate string, limit int) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}
func (m *mockStressStore) LoadEventCalendarRangeAll(ctx context.Context, startDate, endDate string, limit int) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}
func (m *mockStressStore) UpsertPredictionBacktest(ctx context.Context, row ledger.PredictionBacktestRow) error {
	return nil
}
func (m *mockStressStore) LoadPredictionBacktestRange(ctx context.Context, startDate, endDate string, limit int) ([]ledger.PredictionBacktestRow, error) {
	return nil, nil
}
func (m *mockStressStore) LoadPredictionBacktestRangeAll(ctx context.Context, startDate, endDate string, limit int) ([]ledger.PredictionBacktestRow, error) {
	return nil, nil
}
func (m *mockStressStore) UpsertGeopolitical(ctx context.Context, row ledger.GeopoliticalRow) error {
	m.upsertedGeo = append(m.upsertedGeo, row)
	return nil
}
func (m *mockStressStore) LoadGeopoliticalByDate(ctx context.Context, date string) (ledger.GeopoliticalRow, bool, error) {
	return ledger.GeopoliticalRow{}, false, nil
}
func (m *mockStressStore) LoadGeopoliticalByDateAll(ctx context.Context, date string) (ledger.GeopoliticalRow, bool, error) {
	return ledger.GeopoliticalRow{}, false, nil
}
func (m *mockStressStore) LoadGeopoliticalHistory(ctx context.Context, limit int) ([]ledger.GeopoliticalRow, error) {
	return nil, nil
}
func (m *mockStressStore) LoadGeopoliticalHistoryAll(ctx context.Context, limit int) ([]ledger.GeopoliticalRow, error) {
	return nil, nil
}
func (m *mockStressStore) CountSynthetic(ctx context.Context) (map[string]int64, error) {
	return nil, nil
}

type mockGeopoliticalStore struct {
	upserted  []ledger.GeopoliticalRow
	returnErr error
}

func (m *mockGeopoliticalStore) UpsertGeopolitical(_ context.Context, row ledger.GeopoliticalRow) error {
	m.upserted = append(m.upserted, row)
	return m.returnErr
}
func (m *mockGeopoliticalStore) LoadGeopoliticalByDate(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	return ledger.GeopoliticalRow{}, false, nil
}
func (m *mockGeopoliticalStore) LoadGeopoliticalByDateAll(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	return ledger.GeopoliticalRow{}, false, nil
}
func (m *mockGeopoliticalStore) LoadGeopoliticalHistory(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) LoadGeopoliticalHistoryAll(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	return nil, nil
}

func (m *mockGeopoliticalStore) UpsertRegime(_ context.Context, _ ledger.RegimeRow) error { return nil }
func (m *mockGeopoliticalStore) LoadRegimeByDate(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	return ledger.RegimeRow{}, false, nil
}
func (m *mockGeopoliticalStore) LoadRegimeByDateAll(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	return ledger.RegimeRow{}, false, nil
}
func (m *mockGeopoliticalStore) LoadRegimeHistory(_ context.Context, _ int) ([]ledger.RegimeRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) LoadRegimeHistoryAll(_ context.Context, _ int) ([]ledger.RegimeRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) UpsertStress(_ context.Context, _ ledger.StressRow) error { return nil }
func (m *mockGeopoliticalStore) LoadStressByDate(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	return ledger.StressRow{}, false, nil
}
func (m *mockGeopoliticalStore) LoadStressByDateAll(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	return ledger.StressRow{}, false, nil
}
func (m *mockGeopoliticalStore) LoadStressHistory(_ context.Context, _ int) ([]ledger.StressRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) LoadStressHistoryAll(_ context.Context, _ int) ([]ledger.StressRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) UpsertEventCalendar(_ context.Context, _ ledger.EventCalendarRow) error {
	return nil
}
func (m *mockGeopoliticalStore) LoadEventCalendarByDate(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) LoadEventCalendarByDateAll(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) LoadEventCalendarRange(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) LoadEventCalendarRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) UpsertPredictionBacktest(_ context.Context, _ ledger.PredictionBacktestRow) error {
	return nil
}
func (m *mockGeopoliticalStore) LoadPredictionBacktestRange(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) LoadPredictionBacktestRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	return nil, nil
}
func (m *mockGeopoliticalStore) CountSynthetic(_ context.Context) (map[string]int64, error) {
	return nil, nil
}

func validStressMacroSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{ChangePct: 8.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		RecordedAt:         1713000000,
	}
}

func TestDashboardAPI_WithHistoricalStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{}
	got := d.WithHistoricalStore(store)
	if got != d {
		t.Errorf("WithHistoricalStore must return same DashboardAPI for chaining")
	}
	if d.historicalStore != store {
		t.Errorf("historicalStore not set")
	}
}

func TestDashboardAPI_PersistStressIndex_HappyPath(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{}
	d.WithHistoricalStore(store)

	snap := validStressMacroSnapshot()
	geo := narrative.GeopoliticalRiskScore{Intensity: 30}
	d.NarrativeEngine().UpdateMacro(snap, geo)
	d.persistStressIndex(context.Background())

	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}
	row := store.upserted[0]
	if row.Date != "2024-04-13" {
		t.Errorf("Date = %q, want 2024-04-13", row.Date)
	}
	if row.Score <= 0 {
		t.Errorf("expected positive score, got %v", row.Score)
	}
	if row.Regime == "" {
		t.Errorf("expected non-empty regime")
	}
	if row.Source != "macro_ingest" {
		t.Errorf("Source = %q, want macro_ingest", row.Source)
	}
	if row.IsSynthetic != 0 {
		t.Errorf("IsSynthetic = %v, want 0", row.IsSynthetic)
	}
	if len(row.Components) == 0 {
		t.Errorf("expected Components copied from stress index")
	}
}

func TestDashboardAPI_PersistStressIndex_NilStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	snap := validStressMacroSnapshot()
	d.NarrativeEngine().UpdateMacro(snap, narrative.GeopoliticalRiskScore{Intensity: 30})
	d.persistStressIndex(context.Background())
}

func TestDashboardAPI_PersistStressIndex_ZeroTimestamp(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{}
	d.WithHistoricalStore(store)
	d.persistStressIndex(context.Background())
	if len(store.upserted) != 0 {
		t.Errorf("expected no upsert when timestamp is zero, got %d", len(store.upserted))
	}
}

func TestDashboardAPI_PersistStressIndex_UpsertError(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{returnErr: fmt.Errorf("db down")}
	d.WithHistoricalStore(store)
	snap := validStressMacroSnapshot()
	d.NarrativeEngine().UpdateMacro(snap, narrative.GeopoliticalRiskScore{Intensity: 30})
	d.persistStressIndex(context.Background())
	if len(store.upserted) != 1 {
		t.Errorf("expected 1 upsert attempt even on error, got %d", len(store.upserted))
	}
}

func TestDashboardAPI_PersistGeopolitical_HappyPath(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockGeopoliticalStore{}
	d.WithHistoricalStore(store)

	geo := narrative.GeopoliticalRiskScore{
		Intensity: 42.5,
		Sources:   []string{"gdelt", "fugle"},
		Timestamp: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
	}
	d.persistGeopolitical(context.Background(), geo)

	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}
	row := store.upserted[0]
	if row.Date != "2026-07-20" {
		t.Errorf("Date = %q, want 2026-07-20", row.Date)
	}
	if row.Intensity != 42.5 {
		t.Errorf("Intensity = %v, want 42.5", row.Intensity)
	}
	if row.Source != "macro_ingest" {
		t.Errorf("Source = %q, want macro_ingest", row.Source)
	}
	if row.IsSynthetic != 0 {
		t.Errorf("IsSynthetic = %v, want 0", row.IsSynthetic)
	}
}

func TestDashboardAPI_PersistGeopolitical_NilStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	geo := narrative.GeopoliticalRiskScore{
		Intensity: 10,
		Timestamp: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
	}
	d.persistGeopolitical(context.Background(), geo)
}

func TestDashboardAPI_PersistGeopolitical_ZeroTimestamp(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockGeopoliticalStore{}
	d.WithHistoricalStore(store)
	d.persistGeopolitical(context.Background(), narrative.GeopoliticalRiskScore{Intensity: 5})
	if len(store.upserted) != 0 {
		t.Errorf("expected no upsert when timestamp is zero, got %d", len(store.upserted))
	}
}

func TestDashboardAPI_PersistGeopolitical_UpsertError(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockGeopoliticalStore{returnErr: fmt.Errorf("db down")}
	d.WithHistoricalStore(store)
	geo := narrative.GeopoliticalRiskScore{
		Intensity: 10,
		Timestamp: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
	}
	d.persistGeopolitical(context.Background(), geo)
	if len(store.upserted) != 1 {
		t.Errorf("expected 1 upsert attempt even on error, got %d", len(store.upserted))
	}
}

func TestDashboardAPI_StressComponentsToMap(t *testing.T) {
	if got := stressComponentsToMap(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
	in := map[string]float64{"dxy": 1.5, "vix": 20}
	got := stressComponentsToMap(in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got["dxy"] != 1.5 {
		t.Errorf("dxy = %v, want 1.5", got["dxy"])
	}
	if got["vix"] != 20.0 {
		t.Errorf("vix = %v, want 20.0", got["vix"])
	}
}

// TestDashboardAPI_ApplyMacroUpdate_HappyPath covers D08: the helper must
// persist both stress_index_history AND geopolitical_history in lockstep
// whenever it runs. This guards the fix's core invariant — the error
// fallback path of IngestAndUpdateMacro routes through this helper, so
// a successful call here proves both ledgers refresh on every tick.
func TestDashboardAPI_ApplyMacroUpdate_HappyPath(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{}
	d.WithHistoricalStore(store)

	snap := validStressMacroSnapshot()
	geo := narrative.GeopoliticalRiskScore{
		Intensity: 42,
		Timestamp: time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC),
	}
	d.applyMacroUpdate(context.Background(), snap, geo)

	if len(store.upserted) != 1 {
		t.Fatalf("stress upserts = %d, want 1", len(store.upserted))
	}
	if len(store.upsertedGeo) != 1 {
		t.Fatalf("geo upserts = %d, want 1", len(store.upsertedGeo))
	}
	stressRow := store.upserted[0]
	if stressRow.Source != "macro_ingest" {
		t.Errorf("stress Source = %q, want macro_ingest", stressRow.Source)
	}
	if stressRow.IsSynthetic != 0 {
		t.Errorf("stress IsSynthetic = %d, want 0", stressRow.IsSynthetic)
	}
	geoRow := store.upsertedGeo[0]
	if geoRow.Intensity != 42 {
		t.Errorf("geo Intensity = %v, want 42", geoRow.Intensity)
	}
	if geoRow.Date != "2026-07-20" {
		t.Errorf("geo Date = %q, want 2026-07-20", geoRow.Date)
	}
	if geoRow.Source != "macro_ingest" {
		t.Errorf("geo Source = %q, want macro_ingest", geoRow.Source)
	}
}

// TestDashboardAPI_ApplyMacroUpdate_NilNarrativeEngine covers D08: the
// helper must no-op when narrativeEngine is nil (defensive guard — the
// existing persistStressIndex/persistGeopolitical both also check
// a.narrativeEngine separately, but having the guard at the helper layer
// means a future caller can rely on "helper is safe to call").
func TestDashboardAPI_ApplyMacroUpdate_NilNarrativeEngine(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{}
	d.WithHistoricalStore(store)

	d.narrativeEngine = nil

	snap := validStressMacroSnapshot()
	geo := narrative.GeopoliticalRiskScore{Intensity: 1}
	d.applyMacroUpdate(context.Background(), snap, geo)

	if len(store.upserted) != 0 || len(store.upsertedGeo) != 0 {
		t.Errorf("helper should no-op when narrativeEngine nil; got stress=%d geo=%d",
			len(store.upserted), len(store.upsertedGeo))
	}
}
