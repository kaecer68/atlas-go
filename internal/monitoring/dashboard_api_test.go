package monitoring

import (
	"encoding/csv"
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
	d := NewDashboardAPI(".", ".", nil)
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
	d := NewDashboardAPI(".", ".", nil)
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
	d := NewDashboardAPI(".", ".", nil)

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
	d := NewDashboardAPI(".", ".", nil)
	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-calibration", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when no risk gate, got %d", w.Code)
	}
}

func TestHandleRiskCalibration_NoReportYet(t *testing.T) {
	d := NewDashboardAPI(".", ".", nil)
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
	d := NewDashboardAPI(".", ".", nil)
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

func TestHandleRiskCalibration_WrongMethod(t *testing.T) {
	d := NewDashboardAPI(".", ".", nil)
	gate := risk.NewRiskGate(risk.NewPreTradeGate(), risk.NewInTradeGate(), risk.NewPostTradeGate())
	d.SetRiskGate(gate)

	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/risk-calibration", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
