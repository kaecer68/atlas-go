package monitoring

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/storage"
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
