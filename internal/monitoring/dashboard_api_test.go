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
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	apistrategies "github.com/kaecer68/atlas-go/internal/monitoring/api/strategies"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
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
	svc := newWiredIndustryService(eng, nil, os.TempDir())
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
	for i := range 20 {
		date := fmt.Sprintf("2026-01-%02d", 1+i)
		w.Write([]string{date, "2330", "TSMC", "10000", "100", "105", "99", "104"})
		w.Write([]string{date, "2303", "UMC", "5000", "50", "52", "49", "51"})
	}
	w.Flush()
	f.Close()

	t.Setenv("ATLAS_REPLAY_DATA_PATH", replayCSV)

	eng := narrative.NewNarrativeEngine()
	svc := newWiredIndustryService(eng, nil, os.TempDir())
	if svc == nil {
		t.Fatal("expected non-nil industry service")
	}
}

// stubMacroProvider returns a fixed snapshot. Used to feed the narrative
// adapter's real-data path in newWiredIndustryService.
type stubMacroProvider struct {
	snap marketdata.MacroDataSnapshot
}

func (s *stubMacroProvider) Name() string { return "stub" }

func (s *stubMacroProvider) FetchSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, error) {
	return s.snap, nil
}

// findLogEntry extracts a "name=value" entry from an adjustment log.
func findLogEntry(log []string, name string) (float64, bool) {
	for _, l := range log {
		if strings.HasPrefix(l, name+"=") {
			var v float64
			if _, err := fmt.Sscanf(strings.TrimPrefix(l, name+"="), "%f", &v); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

// TestNewWiredIndustryService_NarrativeFnSectorBias verifies the dashboard
// narrative adapter is driven by SectorBias over real macro data: with a
// TSMC revenue surge snapshot, semiconductor gets a multiplier > 1 and an
// uncovered sector stays neutral.
func TestNewWiredIndustryService_NarrativeFnSectorBias(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	provider := &stubMacroProvider{
		snap: marketdata.MacroDataSnapshot{
			TSMCRevenue: marketdata.MacroDataPoint{
				Symbol:    "2330",
				Value:     100,
				ChangePct: 50, // above TSMCRevenueYoYThreshold → AI_capex_surge
			},
		},
	}
	svc := newWiredIndustryService(eng, provider, os.TempDir())
	if svc == nil || svc.WeightEngine == nil {
		t.Fatal("expected non-nil industry service with weight engine")
	}

	sw, err := svc.WeightEngine.ComputeWeight(context.Background(), "semiconductor", time.Now())
	if err != nil {
		t.Fatalf("ComputeWeight: %v", err)
	}
	narrativeLog, ok := findLogEntry(sw.AdjustmentLog, "narrative")
	if !ok {
		t.Fatalf("expected narrative entry in adjustment log, got %v", sw.AdjustmentLog)
	}
	if narrativeLog <= 1.0 {
		t.Fatalf("expected narrative multiplier > 1 for favored semiconductor, got %f", narrativeLog)
	}

	// Uncovered canonical sector stays neutral (safe no-op).
	zero, err := svc.WeightEngine.ComputeWeight(context.Background(), "energy", time.Now())
	if err != nil {
		t.Fatalf("ComputeWeight(energy): %v", err)
	}
	tourismNarrative, ok := findLogEntry(zero.AdjustmentLog, "narrative")
	if !ok {
		t.Fatalf("expected narrative entry for energy, got %v", zero.AdjustmentLog)
	}
	if tourismNarrative != 1.0 {
		t.Fatalf("expected neutral narrative multiplier for uncovered sector energy, got %f", tourismNarrative)
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

func TestConfigMasking_Unit(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  any
		want any
	}{
		{"api key long masked", "FubonAPIKey", "fubon-key-1234567890", "fub••••890"},
		{"api key short fully masked", "FugleAPIKey", "short", "••••"},
		{"exactly 8 chars fully masked", "FinMindAPIKey", "12345678", "••••"},
		{"secret masked", "BrokerAPISecret", "bkr-secret-abcdef", "bkr••••def"},
		{"personal id masked", "FubonDMAPersonalID", "A123456789", "A12••••789"},
		{"key suffix catches XxxKey", "FugleKey", "abcdefghijklmnop", "abc••••nop"},
		{"empty secret stays empty", "TWSEAPISecret", "", ""},
		{"normal string unchanged", "WorkDir", "/srv/atlas", "/srv/atlas"},
		{"store backend unchanged", "StoreBackend", "postgres", "postgres"},
		{"url with creds stripped", "DatabaseURL", "postgres://alice:s3cr3t@db.example.com:5432/atlas?sslmode=require", "postgres://db.example.com:5432"},
		{"url without creds unchanged", "TWSEAPIURL", "https://api.twse.com.tw/v1", "https://api.twse.com.tw/v1"},
		{"bool unchanged", "YahooEnabled", true, true},
		{"int unchanged", "BrokerMaxRetries", 3, 3},
		{"redis url with creds stripped", "BrokerNonceRedisURL", "redis://:pw@redis.internal:6379/0", "redis://redis.internal:6379"},
		{"redis key prefix not masked", "BrokerNonceRedisKeyPrefix", "atlas:nonce", "atlas:nonce"},
		{"non-secret keyid unchanged", "BrokerKeyID", "2026-key-01", "2026-key-01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskedConfigValue(tc.key, tc.val); got != tc.want {
				t.Errorf("maskedConfigValue(%q, %v) = %#v, want %#v", tc.key, tc.val, got, tc.want)
			}
		})
	}
}

func TestMaskedConfigMap_KeysPreservedAndSensitiveRedacted(t *testing.T) {
	cfg := config.Config{
		WorkDir:                    "/srv/atlas",
		StoreBackend:               "postgres",
		PrimaryMarket:              "TW",
		DatabaseURL:                "postgres://alice:s3cr3t@db.example.com:5432/atlas",
		FubonAPIKey:                "fubon-key-1234567890",
		FugleAPIKey:                "fugle",
		FinMindAPIKey:              "fin-0123456789",
		BrokerAPISecret:            "broker-top-secret-99",
		TWSEAPISecret:              "",
		FubonDMAPersonalID:         "A123456789",
		BrokerMode:                 "dry-run",
		YahooEnabled:               true,
		BrokerHTTPRetryStatusCodes: []int{408, 429, 503},
	}
	m := MaskedConfigMap(cfg)
	if len(m) == 0 {
		t.Fatal("MaskedConfigMap returned empty map")
	}
	// Masking must not drop or rename any key: compare against the unmasked
	// struct serialization (62 fields, unset ones serialize as ""/false/0).
	var plain map[string]any
	rawPlain, _ := json.Marshal(cfg)
	if err := json.Unmarshal(rawPlain, &plain); err != nil {
		t.Fatalf("unmarshal plain config: %v", err)
	}
	if len(m) != len(plain) {
		t.Errorf("key count = %d, want %d (masking must not drop keys)", len(m), len(plain))
	}
	for k := range plain {
		if _, ok := m[k]; !ok {
			t.Errorf("masked map missing key %q", k)
		}
	}
	if got := m["FubonAPIKey"]; got != "fub••••890" {
		t.Errorf("FubonAPIKey = %#v, want masked fub••••890", got)
	}
	if got := m["FugleAPIKey"]; got != "••••" {
		t.Errorf("FugleAPIKey = %#v, want full-masked (<=8 chars)", got)
	}
	if got := m["BrokerAPISecret"]; got != "bro••••-99" {
		t.Errorf("BrokerAPISecret = %#v, want masked bro••••-99", got)
	}
	if got := m["FubonDMAPersonalID"]; got != "A12••••789" {
		t.Errorf("FubonDMAPersonalID = %#v, want masked A12••••789", got)
	}
	if got := m["TWSEAPISecret"]; got != "" {
		t.Errorf("TWSEAPISecret = %#v, want unchanged empty string", got)
	}
	if got := m["DatabaseURL"]; got != "postgres://db.example.com:5432" {
		t.Errorf("DatabaseURL = %#v, want scheme://host only (creds stripped)", got)
	}
	if got := m["WorkDir"]; got != "/srv/atlas" {
		t.Errorf("WorkDir = %#v, want unchanged /srv/atlas", got)
	}
	if got := m["StoreBackend"]; got != "postgres" {
		t.Errorf("StoreBackend = %#v, want unchanged postgres", got)
	}
	if got := m["YahooEnabled"]; got != true {
		t.Errorf("YahooEnabled = %#v, want true", got)
	}
	// Round-trip through real JSON to prove the wire body carries masked values.
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal masked map: %v", err)
	}
	s := string(raw)
	for _, leak := range []string{"s3cr3t", "fubon-key-1234567890", "broker-top-secret-99", "A123456789"} {
		if strings.Contains(s, leak) {
			t.Errorf("wire JSON leaks plaintext %q", leak)
		}
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

// TestDashboardAPI_ChannelHealthEndpoint_SurfacesFullRecord verifies that
// /api/dashboard/channel-health returns the full ChannelHealthRecord surface
// (latency, last_error, records_fetched, etc.) instead of just channel_id/status.
func TestDashboardAPI_ChannelHealthEndpoint_SurfacesFullRecord(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "data/state"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	now := time.Now().UTC().Round(time.Second)
	payload := map[string]any{
		"channels": map[string]any{
			"twse_capital_flow": map[string]any{
				"status":               "error",
				"last_fetch_at":        now.Format(time.RFC3339),
				"last_data_at":         now.Add(-time.Hour).Format(time.RFC3339),
				"last_error":           "rate limit exceeded",
				"last_success_at":      now.Add(-2 * time.Hour).Format(time.RFC3339),
				"latency_ms":           int64(1234),
				"rate_limit_remaining": 0,
				"records_fetched":      42,
				"symbols_processed":    150,
				"errors":               []string{"timeout", "rate limit exceeded"},
			},
			"fugle": map[string]any{
				"status":        "ok",
				"last_fetch_at": now.Format(time.RFC3339),
				"latency_ms":    int64(56),
			},
		},
		"updated_at": now.Format(time.RFC3339),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "data/state", "channel_health.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	d := NewDashboardAPIWithGateway(tmpDir, tmpDir, nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Channels  []map[string]any `json:"channels"`
		UpdatedAt string           `json:"updated_at"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(resp.Channels))
	}

	var twse map[string]any
	for _, ch := range resp.Channels {
		if ch["channel_id"] == "twse_capital_flow" {
			twse = ch
			break
		}
	}
	if twse == nil {
		t.Fatal("twse_capital_flow channel not found")
	}
	for _, key := range []string{"last_error", "latency_ms", "records_fetched", "symbols_processed", "errors"} {
		if _, ok := twse[key]; !ok {
			t.Errorf("expected %s in response", key)
		}
	}
	if twse["last_error"] != "rate limit exceeded" {
		t.Errorf("expected last_error=rate limit exceeded, got %v", twse["last_error"])
	}
}

func TestDashboardAPI_ChannelHealthEndpoint_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDashboardAPIWithGateway(tmpDir, tmpDir, nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"channels":[]`) {
		t.Errorf("expected empty channels array, got %s", body)
	}
}

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
	upserted       []ledger.StressRow
	upsertedGeo    []ledger.GeopoliticalRow
	upsertedRegime []ledger.RegimeRow
	upsertedPeriod []ledger.PeriodRow
	geoRows        []ledger.GeopoliticalRow
	returnErr      error
}

func (m *mockStressStore) UpsertRegime(ctx context.Context, row ledger.RegimeRow) error {
	if m.returnErr != nil {
		return m.returnErr
	}
	m.upsertedRegime = append(m.upsertedRegime, row)
	return nil
}

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
	return m.geoRows, nil
}

func (m *mockStressStore) CountSynthetic(ctx context.Context) (map[string]int64, error) {
	return nil, nil
}
func (m *mockStressStore) UpsertPeriod(_ context.Context, row ledger.PeriodRow) error {
	m.upsertedPeriod = append(m.upsertedPeriod, row)
	return m.returnErr
}
func (m *mockStressStore) LoadPeriodByDate(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockStressStore) LoadPeriodByDateAll(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockStressStore) LoadPeriodHistory(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

func (m *mockStressStore) LoadPeriodHistoryAll(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
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

func (m *mockGeopoliticalStore) UpsertPeriod(_ context.Context, _ ledger.PeriodRow) error { return nil }

func (m *mockGeopoliticalStore) LoadPeriodByDate(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockGeopoliticalStore) LoadPeriodByDateAll(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockGeopoliticalStore) LoadPeriodHistory(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

func (m *mockGeopoliticalStore) LoadPeriodHistoryAll(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

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
	geo := geopolitical.GeopoliticalRiskScore{Intensity: 30}
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

// TestDashboardAPI_PersistRegimeHistory_HappyPath covers E8: every live macro
// tick must persist a canonical regime row derived from the current stress
// index so /api/regime/history can serve recent dates.
func TestDashboardAPI_PersistRegimeHistory_HappyPath(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{}
	d.WithHistoricalStore(store)

	snap := validStressMacroSnapshot()
	geo := geopolitical.GeopoliticalRiskScore{Intensity: 30}
	d.NarrativeEngine().UpdateMacro(snap, geo)
	d.persistRegimeHistory(context.Background())

	if len(store.upsertedRegime) != 1 {
		t.Fatalf("expected 1 regime upsert, got %d", len(store.upsertedRegime))
	}
	row := store.upsertedRegime[0]
	if row.Date != "2024-04-13" {
		t.Errorf("Date = %q, want 2024-04-13", row.Date)
	}
	if row.Regime == "" {
		t.Errorf("expected non-empty canonical regime")
	}
	if row.Source != "macro_ingest" {
		t.Errorf("Source = %q, want macro_ingest", row.Source)
	}
	if row.IsSynthetic != 0 {
		t.Errorf("IsSynthetic = %v, want 0", row.IsSynthetic)
	}
	if row.RecordedAt.IsZero() {
		t.Errorf("RecordedAt should be set from stress index timestamp")
	}
	if row.CapturedAt.IsZero() {
		t.Errorf("CapturedAt should be set")
	}
}

func TestDashboardAPI_PersistRegimeHistory_NilStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	snap := validStressMacroSnapshot()
	d.NarrativeEngine().UpdateMacro(snap, geopolitical.GeopoliticalRiskScore{Intensity: 30})
	d.persistRegimeHistory(context.Background())
}

func TestDashboardAPI_PersistRegimeHistory_ZeroTimestamp(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{}
	d.WithHistoricalStore(store)
	d.persistRegimeHistory(context.Background())
	if len(store.upsertedRegime) != 0 {
		t.Errorf("expected no regime upsert when timestamp is zero, got %d", len(store.upsertedRegime))
	}
}

func TestDashboardAPI_PersistStressIndex_NilStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	snap := validStressMacroSnapshot()
	d.NarrativeEngine().UpdateMacro(snap, geopolitical.GeopoliticalRiskScore{Intensity: 30})
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
	d.NarrativeEngine().UpdateMacro(snap, geopolitical.GeopoliticalRiskScore{Intensity: 30})
	d.persistStressIndex(context.Background())
	if len(store.upserted) != 1 {
		t.Errorf("expected 1 upsert attempt even on error, got %d", len(store.upserted))
	}
}

func TestDashboardAPI_PersistGeopolitical_HappyPath(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockGeopoliticalStore{}
	d.WithHistoricalStore(store)

	geo := geopolitical.GeopoliticalRiskScore{
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
	geo := geopolitical.GeopoliticalRiskScore{
		Intensity: 10,
		Timestamp: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
	}
	d.persistGeopolitical(context.Background(), geo)
}

func TestDashboardAPI_PersistGeopolitical_ZeroTimestamp(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockGeopoliticalStore{}
	d.WithHistoricalStore(store)
	d.persistGeopolitical(context.Background(), geopolitical.GeopoliticalRiskScore{Intensity: 5})
	if len(store.upserted) != 0 {
		t.Errorf("expected no upsert when timestamp is zero, got %d", len(store.upserted))
	}
}

func TestDashboardAPI_PersistGeopolitical_UpsertError(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockGeopoliticalStore{returnErr: fmt.Errorf("db down")}
	d.WithHistoricalStore(store)
	geo := geopolitical.GeopoliticalRiskScore{
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
	geo := geopolitical.GeopoliticalRiskScore{
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
	geo := geopolitical.GeopoliticalRiskScore{Intensity: 1}
	d.applyMacroUpdate(context.Background(), snap, geo)

	if len(store.upserted) != 0 || len(store.upsertedGeo) != 0 || len(store.upsertedRegime) != 0 {
		t.Errorf("helper should no-op when narrativeEngine nil; got stress=%d geo=%d regime=%d",
			len(store.upserted), len(store.upsertedGeo), len(store.upsertedRegime))
	}
}

// errGeoProvider is a GeopoliticalRiskProvider that always fails.
type errGeoProvider struct{ err error }

func (p *errGeoProvider) Name() string { return "errGeoProvider" }

func (p *errGeoProvider) FetchScore(ctx context.Context) (geopolitical.GeopoliticalRiskScore, error) {
	return geopolitical.GeopoliticalRiskScore{}, p.err
}

// TestDashboardAPI_ResolveGeoScore_FallsBackToHistoricalStore covers E5:
// when the live geo provider fails, resolveGeoScore must return the latest
// persisted intensity from the historical ledger instead of zero.
func TestDashboardAPI_ResolveGeoScore_FallsBackToHistoricalStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	store := &mockStressStore{
		geoRows: []ledger.GeopoliticalRow{
			{
				Date:        "2026-07-21",
				Intensity:   39.0,
				Source:      "macro_ingest",
				CapturedAt:  time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
				Sources:     []string{"gdelt"},
				IsSynthetic: 0,
			},
		},
	}
	d.WithHistoricalStore(store)
	d.geoProvider = &errGeoProvider{err: context.DeadlineExceeded}

	score := d.resolveGeoScore(context.Background())
	if score.Intensity != 39.0 {
		t.Errorf("Intensity = %v, want 39.0 (fallback from historical store)", score.Intensity)
	}
	if len(score.Sources) != 1 || score.Sources[0] != "gdelt" {
		t.Errorf("Sources = %v, want [gdelt]", score.Sources)
	}
	if score.Timestamp.IsZero() {
		t.Errorf("Timestamp should be set from historical row")
	}
}

// TestDashboardAPI_ResolveGeoScore_FallsBackToFileStore covers E5: when both
// the live provider and the historical store are unavailable, resolveGeoScore
// should fall back to the on-disk geopolitical file store if it has a
// non-zero intensity.
func TestDashboardAPI_ResolveGeoScore_FallsBackToFileStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	d.geoProvider = &errGeoProvider{err: context.DeadlineExceeded}

	geoFile := filepath.Join(d.workDir, constants.StateGeopolitical, "latest.json")
	if err := os.MkdirAll(filepath.Dir(geoFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	score := geopolitical.GeopoliticalRiskScore{
		Intensity: 25.0,
		Timestamp: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
	}
	data, _ := json.Marshal(score)
	if err := os.WriteFile(geoFile, data, 0o644); err != nil {
		t.Fatalf("write geo file: %v", err)
	}

	got := d.resolveGeoScore(context.Background())
	if got.Intensity != 25.0 {
		t.Errorf("Intensity = %v, want 25.0 (file fallback)", got.Intensity)
	}
}

// mockQuoteStore is a minimal QuoteStore for testing warmup paths.
type mockQuoteStore struct {
	recorded []domain.DailyBar
	err      error
}

func (m *mockQuoteStore) RecordQuotes(quotes []domain.DailyBar) error {
	if m.err != nil {
		return m.err
	}
	m.recorded = append(m.recorded, quotes...)
	return nil
}

func (m *mockQuoteStore) LoadQuotes(symbol string, start, end time.Time) ([]domain.DailyBar, error) {
	return nil, nil
}

func (m *mockQuoteStore) LoadLatestQuotes(symbols []string) (map[string]domain.DailyBar, error) {
	return nil, nil
}

func TestDashboardAPI_SetQuoteStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	qs := &mockQuoteStore{}
	d.SetQuoteStore(qs)
	d.quoteStoreMu.RLock()
	if d.quoteStore != qs {
		t.Error("SetQuoteStore did not set quoteStore")
	}
	d.quoteStoreMu.RUnlock()
}

func TestDashboardAPI_SetFugleAPIKey(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	d.SetFugleAPIKey("secret")
	d.fugleAPIKeyMu.RLock()
	if d.fugleAPIKey != "secret" {
		t.Errorf("SetFugleAPIKey = %q, want secret", d.fugleAPIKey)
	}
	d.fugleAPIKeyMu.RUnlock()
}

func TestDashboardAPI_WarmupQuotes_NoQuoteStore(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	d.SetFugleAPIKey("secret")
	d.warmupQuotes() // should not panic
}

func TestDashboardAPI_WarmupQuotes_NoFugleKey(t *testing.T) {
	d := NewDashboardAPIWithGateway(".", ".", nil, NoopFetcher())
	d.SetQuoteStore(&mockQuoteStore{})
	d.warmupQuotes() // should not panic
}

// ---------------------------------------------------------------------------
// R9: persistPeriodHistory / persistRegimeHistory unit tests.
// ---------------------------------------------------------------------------

// writeFixtureMacroSnapshots writes `n` dated snapshot files starting at
// startDate into dir, each with a flat TAIEX series so rolling indicators
// (TAIEXMA20 etc.) become computable once >= MinDaysTAIEXMA20 files exist.
func writeFixtureMacroSnapshots(t *testing.T, dir, startDate string, n int) {
	t.Helper()
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		day := start.AddDate(0, 0, i)
		snap := marketdata.MacroDataSnapshot{
			VIX:                marketdata.MacroDataPoint{Symbol: "^VIX", Value: 14},
			DXY:                marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 100},
			US10Y:              marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.2},
			SOXIndex:           marketdata.MacroDataPoint{Symbol: "^SOX", Value: 6000},
			TSMADR:             marketdata.MacroDataPoint{Symbol: "TSM", Value: 250},
			TAIEX:              marketdata.MacroDataPoint{Symbol: "TAIEX", Value: 25000},
			ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "TAIWAN_FOREIGN", Value: 10},
			MarketVolume:       marketdata.MacroDataPoint{Symbol: "TSE_VOLUME", Value: 8000},
			RecordedAt:         day.Unix(),
		}
		data, err := json.Marshal(snap)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, day.Format("2006-01-02")+".json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDashboardAPI_PersistPeriodHistory covers R9: the live persist path must
// upsert one period_history row with the snapshot's own date (not today) and
// run the documented enrich pipeline against on-disk fixtures.
func TestDashboardAPI_PersistPeriodHistory(t *testing.T) {
	tmp := t.TempDir()
	// persistPeriodHistory reads enrich dirs from a.workDir/data/state/*
	for _, d := range []string{"margin", "sector_index", "government_flow"} {
		if err := os.MkdirAll(filepath.Join(tmp, "data", "state", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	macroDir := filepath.Join(tmp, "macro_fixture")
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 30 days of history so TAIEXMA20 becomes computable for the target date.
	writeFixtureMacroSnapshots(t, macroDir, "2026-07-01", 30)

	store := &mockStressStore{}
	d := NewDashboardAPIWithGateway(tmp, tmp, nil, NoopFetcher())
	d.historicalStore = store
	d.macroIngestor = narrative.NewMacroIngestor(nil, macroDir)

	target, _ := time.Parse("2006-01-02", "2026-08-01")
	snap := marketdata.MacroDataSnapshot{
		VIX:                marketdata.MacroDataPoint{Value: 14},
		DXY:                marketdata.MacroDataPoint{Value: 100},
		US10Y:              marketdata.MacroDataPoint{Value: 4.2},
		SOXIndex:           marketdata.MacroDataPoint{Value: 6000},
		TSMADR:             marketdata.MacroDataPoint{Value: 250},
		TAIEX:              marketdata.MacroDataPoint{Value: 26000},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 15},
		MarketVolume:       marketdata.MacroDataPoint{Value: 8500},
		RecordedAt:         target.Unix(),
	}

	d.persistPeriodHistory(context.Background(), snap, geopolitical.GeopoliticalRiskScore{})

	if len(store.upsertedPeriod) != 1 {
		t.Fatalf("UpsertPeriod calls = %d, want 1", len(store.upsertedPeriod))
	}
	row := store.upsertedPeriod[0]
	if row.Date != "2026-08-01" {
		t.Errorf("row.Date = %q, want 2026-08-01 (snapshot date, not today)", row.Date)
	}
	if row.Period == "" {
		t.Error("row.Period is empty")
	}
	if row.IsSynthetic != 0 {
		t.Errorf("row.IsSynthetic = %d, want 0 (live ingest)", row.IsSynthetic)
	}
	if row.Source != "macro_ingest" {
		t.Errorf("row.Source = %q, want macro_ingest", row.Source)
	}
	if row.RecordedAt.IsZero() || row.CapturedAt.IsZero() {
		t.Error("row timestamps must be set")
	}

	// The persisted period must equal the documented pipeline output on the
	// same fixtures (proves enrichment ran and the row is not a hardcoded value).
	ind := SnapshotToPeriodIndicators(snap)
	calc := portfolio.NewCalculator()
	_ = calc.EnrichFromDir(&ind, "2026-08-01", macroDir)
	want := string(portfolio.NewPeriodDetectorWithDefaults().DetectPeriod(ind))
	if row.Period != want {
		t.Errorf("row.Period = %q, pipeline DetectPeriod = %q", row.Period, want)
	}
}

// TestDashboardAPI_PersistPeriodHistory_NoEnrichDirNoPanic covers the honest
// degradation path: missing enrich dirs must leave indicators at zero and
// still produce a row (consolidation fallback) instead of panicking.
func TestDashboardAPI_PersistPeriodHistory_NoEnrichDirNoPanic(t *testing.T) {
	tmp := t.TempDir() // no data/state subdirs at all
	store := &mockStressStore{}
	d := NewDashboardAPIWithGateway(tmp, tmp, nil, NoopFetcher())
	d.historicalStore = store
	d.macroIngestor = narrative.NewMacroIngestor(nil, filepath.Join(tmp, "missing_macro"))

	snap := marketdata.MacroDataSnapshot{RecordedAt: unixTimeForTest("2026-08-01")}
	d.persistPeriodHistory(context.Background(), snap, geopolitical.GeopoliticalRiskScore{})

	if len(store.upsertedPeriod) != 1 {
		t.Fatalf("UpsertPeriod calls = %d, want 1", len(store.upsertedPeriod))
	}
	row := store.upsertedPeriod[0]
	if row.Date != "2026-08-01" {
		t.Errorf("row.Date = %q, want 2026-08-01", row.Date)
	}
	if row.Period != "consolidation" {
		t.Errorf("row.Period = %q, want consolidation (all-zero no-data fallback)", row.Period)
	}
}

// TestDashboardAPI_PersistRegimeHistory covers R9: the live persist path must
// upsert one regime_history row derived from the narrative engine's current
// stress index, normalized to the canonical regime vocabulary.
func TestDashboardAPI_PersistRegimeHistory(t *testing.T) {
	store := &mockStressStore{}
	d := NewDashboardAPIWithGateway(t.TempDir(), t.TempDir(), nil, NoopFetcher())
	d.historicalStore = store

	snap := marketdata.MacroDataSnapshot{
		VIX:                marketdata.MacroDataPoint{Value: 45},
		DXY:                marketdata.MacroDataPoint{Value: 105},
		US10Y:              marketdata.MacroDataPoint{Value: 4.2},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -8},
		RecordedAt:         unixTimeForTest("2026-08-01"),
	}
	d.narrativeEngine.UpdateMacro(snap, geopolitical.GeopoliticalRiskScore{Intensity: 25})

	d.persistRegimeHistory(context.Background())

	if len(store.upsertedRegime) != 1 {
		t.Fatalf("UpsertRegime calls = %d, want 1", len(store.upsertedRegime))
	}
	row := store.upsertedRegime[0]
	if row.Date != "2026-08-01" {
		t.Errorf("row.Date = %q, want 2026-08-01", row.Date)
	}
	idx := d.narrativeEngine.GetCurrentStressIndex()
	wantRegime := narrative.NormalizeRegime(idx.Regime)
	if row.Regime != wantRegime {
		t.Errorf("row.Regime = %q, want %q (NormalizeRegime(%q))", row.Regime, wantRegime, idx.Regime)
	}
	if row.Regime != "RISK_ON" && row.Regime != "NEUTRAL" && row.Regime != "RISK_OFF" {
		t.Errorf("row.Regime = %q, want canonical regime vocabulary", row.Regime)
	}
	if row.IsSynthetic != 0 {
		t.Errorf("row.IsSynthetic = %d, want 0 (live ingest)", row.IsSynthetic)
	}
	if row.Source != "macro_ingest" {
		t.Errorf("row.Source = %q, want macro_ingest", row.Source)
	}
	if row.SourceSessionID != "macro_ingest:2026-08-01" {
		t.Errorf("row.SourceSessionID = %q, want macro_ingest:2026-08-01", row.SourceSessionID)
	}
}

func unixTimeForTest(date string) int64 {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return t.Unix()
}

// TestDashboardAPI_PersistPeriodHistory_NationalFundActive covers A1 (R8 接線):
// 國安基金護盤期間（2025-04-09~2026-01-12 川普關稅窗口內）→ NationalFundActive=true
// → 黑天鵝條件 4「國安基金宣布進場護盤」觸發 → period=black_swan。
func TestDashboardAPI_PersistPeriodHistory_NationalFundActive(t *testing.T) {
	tmp := t.TempDir()
	store := &mockStressStore{}
	d := NewDashboardAPIWithGateway(tmp, tmp, nil, NoopFetcher())
	d.historicalStore = store
	d.macroIngestor = narrative.NewMacroIngestor(nil, filepath.Join(tmp, "missing_macro"))

	snap := marketdata.MacroDataSnapshot{RecordedAt: unixTimeForTest("2025-04-09")}
	d.persistPeriodHistory(context.Background(), snap, geopolitical.GeopoliticalRiskScore{})

	if len(store.upsertedPeriod) != 1 {
		t.Fatalf("UpsertPeriod calls = %d, want 1", len(store.upsertedPeriod))
	}
	if got := store.upsertedPeriod[0].Period; got != string(domain.PeriodBlackSwan) {
		t.Errorf("row.Period = %q, want black_swan (國安基金護盤中, A1/R8)", got)
	}
}

// TestDashboardAPI_PersistPeriodHistory_NoNSFOutOfWindow is the control for A1:
// 護盤窗口外日期不得因國安基金誤判黑天鵝（2026-08-01 無護盤）。
func TestDashboardAPI_PersistPeriodHistory_NoNSFOutOfWindow(t *testing.T) {
	tmp := t.TempDir()
	store := &mockStressStore{}
	d := NewDashboardAPIWithGateway(tmp, tmp, nil, NoopFetcher())
	d.historicalStore = store
	d.macroIngestor = narrative.NewMacroIngestor(nil, filepath.Join(tmp, "missing_macro"))

	snap := marketdata.MacroDataSnapshot{RecordedAt: unixTimeForTest("2026-08-01")}
	d.persistPeriodHistory(context.Background(), snap, geopolitical.GeopoliticalRiskScore{})

	if len(store.upsertedPeriod) != 1 {
		t.Fatalf("UpsertPeriod calls = %d, want 1", len(store.upsertedPeriod))
	}
	if got := store.upsertedPeriod[0].Period; got == string(domain.PeriodBlackSwan) {
		t.Errorf("row.Period = %q, want NOT black_swan (無護盤, A1 control)", got)
	}
}
