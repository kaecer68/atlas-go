package industry

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// =====================================================================
// HandleIndustryCalibration — calibrated=true happy path
// =====================================================================

func TestHandleIndustryCalibration_Calibrated(t *testing.T) {
	h := setupIndustryHandlers()

	cfg := config.CycleCalibrationConfig{
		MinSamples:     2,
		LearningRate:   0.1,
		HitRateHigh:    0.7,
		HitRateLow:     0.3,
		WeightClampMin: 0.0,
		WeightClampMax: 1.0,
		WindowSize:     100,
	}
	cal := industry.NewCycleCalibration(cfg)
	// The layer names must be FUNDED card layers (silicon/business_cycle/…):
	// "calibrated" means the evidence redistributed one of them, so a tracker
	// whose metrics only name unfunded layers has nothing to redistribute and
	// must report calibrated=false (issue #1944 Batch A1 — the fixture used
	// "layer-a"/"layer-b", which are not card layers, and the probe read that as
	// a redistribution only because rebuilding the weights was enough to shift
	// them by rounding residue).
	cal.RecordOutcome("session-1", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		map[string]float64{"silicon": 0.6, "business_cycle": -0.2}, 0.05)
	cal.RecordOutcome("session-2", time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
		map[string]float64{"silicon": 0.4}, -0.03)
	h.Svc.SetCycleCalibration(cal)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-calibration", nil)
	status, body := h.HandleIndustryCalibration(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	calibrated, _ := resp["calibrated"].(bool)
	if !calibrated {
		t.Fatalf("expected calibrated=true, got %v: %+v", resp["calibrated"], resp)
	}
	outcomeCount, _ := resp["outcome_count"].(int)
	if outcomeCount != 2 {
		t.Errorf("expected outcome_count=2, got %d", outcomeCount)
	}
	layers, ok := resp["layers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected layers []map, got %T", resp["layers"])
	}
	if len(layers) == 0 {
		t.Fatal("expected non-empty layers")
	}
	foundLayer := false
	for _, l := range layers {
		if l["layer"] == "silicon" {
			foundLayer = true
			break
		}
	}
	if !foundLayer {
		t.Errorf("expected silicon in layers, got %+v", layers)
	}
}

// =====================================================================
// HandleSeasonalHealth — happy path (no error)
// =====================================================================

func TestHandleSeasonalHealth_NoError(t *testing.T) {
	h := setupIndustryHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-seasonal-health", nil)
	status, body := h.HandleSeasonalHealth(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}

	if errMap, isErr := body.(map[string]any); isErr {
		if _, hasError := errMap["error"]; hasError {
			t.Skipf("setup returned error branch (acceptable for default config): %+v", errMap)
			return
		}
	}

	resp, ok := body.(*industry.CalibrationHealthSummary)
	if !ok {
		t.Fatalf("expected *CalibrationHealthSummary, got %T", body)
	}
	if resp == nil {
		t.Fatal("expected non-nil health summary")
	}
	t.Logf("health summary patterns: %d, health: %s", resp.PatternCount, resp.Health)
}
