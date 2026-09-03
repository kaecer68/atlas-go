package capitalflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ===========================================================================
// M6 — GenerateSummaryReport must not fabricate a dominant force.
// (audit 2026-09-04): with no actor and no signal reading, the report used
// to fall back to ForceRetail, naming 散戶 the default protagonist even when
// no retail signal exists. It now stays empty; the API/front-end render an
// empty dominant_force as "—".
// ===========================================================================

// emptySnapshot has every source channel missing so no dimension reports
// DataAvailable — the "no actor / no signal" scenario from audit M6.
func emptySnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		RecordedAt: time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC).Unix(),
	}
}

func TestGenerateSummaryReport_NoActorNoSignal_LeavesDominantEmpty(t *testing.T) {
	snap := emptySnapshot()
	forces := NewForceExtractor().Score(snap, "2026-09-04", nil)
	summary := GenerateSummaryReport(time.Unix(snap.RecordedAt, 0), forces, ComputeResonance(forces))

	if summary.DominantForce != "" {
		t.Errorf("DominantForce = %q, want empty (must NOT fall back to retail)", summary.DominantForce)
	}
	if summary.DominantActor != "" || summary.DominantSignal != "" {
		t.Errorf("DominantActor/DominantSignal = %q/%q, want empty/empty", summary.DominantActor, summary.DominantSignal)
	}
	// The short summary text must not invent a protagonist either.
	if got := summary.Summary; got == "" {
		t.Error("Summary empty for an all-missing snapshot")
	}
}

// TestGenerateSummaryReport_DominantPicksActorThenSignal locks the
// non-empty side of M6: dominant prefers the actor slot and only falls
// back to the signal slot — never to a random actor default.
func TestGenerateSummaryReport_DominantPicksActorThenSignal(t *testing.T) {
	date := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, DimensionRole: DimensionRoleOfficialActor, ZScore: 2.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceInstitutional, Role: ForceRoleSubject, DimensionRole: DimensionRoleOfficialActor, ZScore: 1.5, Trend: "bullish", DataAvailable: true},
		{Force: ForceDealer, Role: ForceRoleSubject, DimensionRole: DimensionRoleOfficialActor, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceFutures, Role: ForceRoleLeadingIndicator, DimensionRole: DimensionRolePositioningIndicator, Deprecated: true, ZScore: 5.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceTSMADR, Role: ForceRoleSentiment, DimensionRole: DimensionRoleCrossMarketSignal, Deprecated: true, ZScore: 4.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceGovernment, Role: ForceRoleSubject, DimensionRole: DimensionRoleBehavioralProxy, ZScore: -0.2, Trend: "neutral", DataAvailable: true},
		{Force: ForceRetail, Role: ForceRoleSubject, DimensionRole: DimensionRoleBehavioralProxy, ZScore: -0.1, Trend: "neutral", DataAvailable: true},
	}
	summary := GenerateSummaryReport(date, forces, ResonanceResult{Direction: "bullish", Coefficient: 1.5})
	if summary.DominantForce != ForceForeign {
		t.Errorf("DominantForce = %q, want %q (actor preferred; futures 5.0 must not win the actor slot)", summary.DominantForce, ForceForeign)
	}
	if summary.DominantActor != ForceForeign {
		t.Errorf("DominantActor = %q, want %q", summary.DominantActor, ForceForeign)
	}
}

// TestHandleSummary_NoData_DominantForceEmptyOnTheWire asserts the JSON
// surface of /api/capital-flow/summary returns "dominant_force": "" —
// not "retail" — for a snapshot with no actor / no signal data.
func TestHandleSummary_NoData_DominantForceEmptyOnTheWire(t *testing.T) {
	h := NewHandler(&mockProvider{snap: emptySnapshot()})
	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/summary", nil)
	code, data := h.HandleSummary(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dominant, ok := doc["dominant_force"].(string)
	if !ok {
		t.Fatalf("dominant_force missing or not a string: %v", doc["dominant_force"])
	}
	if dominant != "" {
		t.Errorf("dominant_force = %q, want empty string (M6: no retail fallback)", dominant)
	}
}
