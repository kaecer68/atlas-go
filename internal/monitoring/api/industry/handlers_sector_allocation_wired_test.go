package industry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// TestHandleSectorAllocationPlan_WiredReturns200 verifies the production
// wiring path: with 6 sectorallocation input providers connected and
// SectorAllocator assigned, the handler returns 200 with a non-empty
// industries array and an adjustment_log that records each factor.
//
// Companion test: TestHandleSectorAllocationPlan_EngineNotConfigured in
// handlers_zero_coverage_test.go verifies the 503 path (SectorAllocator=nil).
//
// Why this is the root-cause test for the "Industry Map empty" bug:
//   - The frontend's industry.js calls loadSectorAllocationPlan and feeds
//     the result to renderIndustryMap.
//   - When HandleSectorAllocationPlan returns 503 (because SectorAllocator
//     is nil), silentGetJSON converts the error to null and the UI shows
//     "尚無產業資料" (no data) silently.
//   - Wiring the production adapters (cycle/seasonal/linkage/narrative/macro/factor)
//     into Svc.WeightEngine AND assigning Svc.WeightEngine to
//     Handlers.SectorAllocator restores the multi-factor formula that
//     drives the industry map's adjusted weights.
func TestHandleSectorAllocationPlan_WiredReturns200(t *testing.T) {
	h := setupIndustryHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil)

	status, body := h.HandleSectorAllocationPlan(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 (sectorallocation wired), got %d: %v", status, body)
	}

	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}

	// Assert industries array is non-empty.
	weights, ok := resp["industries"].([]sectorallocation.SectorWeight)
	if !ok {
		t.Fatalf("expected []sectorallocation.SectorWeight, got %T", resp["industries"])
	}
	if len(weights) == 0 {
		t.Fatal("expected non-empty industries array (12 base weights in parameters.json)")
	}

	// Assert at least one industry ran the multi-factor formula. The engine
	// appends one log entry per factor (cycle, seasonal, linkage, narrative,
	// macro_tilt, factor_tilt) plus base/multiplier/clamped. If the log is
	// missing any of these, the wiring is incomplete.
	var sample *sectorallocation.SectorWeight
	for i := range weights {
		if len(weights[i].AdjustmentLog) >= 5 {
			sample = &weights[i]
			break
		}
	}
	if sample == nil {
		t.Fatalf("expected at least one industry with adjustment_log (5+ entries), got %d industries", len(weights))
	}

	logStr := strings.Join(sample.AdjustmentLog, " ")
	requiredKeys := []string{
		"base=",
		"cycle=",
		"seasonal=",
		"linkage=",
		"narrative=",
		"macro_tilt=",
		"factor_tilt=",
	}
	for _, key := range requiredKeys {
		if !strings.Contains(logStr, key) {
			t.Errorf("expected %q in adjustment log for %s, got %v", key, sample.ID, sample.AdjustmentLog)
		}
	}

	// Count metadata must match.
	if count, _ := resp["count"].(int); count != len(weights) {
		t.Errorf("count %d != industries length %d", count, len(weights))
	}

	// Config source must be parameter-system, not a fallback.
	if src, _ := resp["config_source"].(string); src == "" {
		t.Error("expected config_source to be set, got empty")
	}
}
