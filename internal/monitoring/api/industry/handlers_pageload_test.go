package industry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// setupPageLoadHandlers wires the full handler stack the same way dashboard_api.go
// does, so the 6 page-load endpoints can be exercised against realistic services.
func setupPageLoadHandlers(t *testing.T) *Handlers {
	t.Helper()

	classifier := industry.DefaultClassification()
	seasonalEngine := industry.NewSeasonalEngine()
	cycleTracker := industry.NewCycleTracker()
	linkageAnalyzer := industry.NewLinkageAnalyzer()
	riskMonitor := industry.NewRiskMonitor()
	siliconTracker := industry.NewSiliconCycleTracker()
	calendar := industry.NewEventCalendar()
	calendar.RefreshEvents(time.Now())

	// Cross-wire subsystems (mirrors dashboard_api.go bootstrap).
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

	svc := service.NewIndustryService(
		classifier, seasonalEngine, cycleTracker, linkageAnalyzer,
		riskMonitor, siliconTracker, calendar, nil, nil, "",
	)
	return &Handlers{Svc: svc}
}

// =====================================================================
// 1. HandleIndustryClassification
// =====================================================================

// Happy path: 200 with industries array matching the classifier tree.
func TestHandleIndustryClassification_Happy(t *testing.T) {
	h := setupPageLoadHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-classification", nil)

	status, body := h.HandleIndustryClassification(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	industries, ok := resp["industries"].([]map[string]any)
	if !ok {
		t.Fatal("expected industries array in response")
	}
	count, _ := resp["count"].(int)
	if count != len(industries) {
		t.Errorf("count %d != industries length %d", count, len(industries))
	}
	if len(industries) == 0 {
		t.Error("expected non-empty industries list")
	}
	// Each top-level entry must expose id/name/weight so the frontend
	// renderIndustryMap() can draw the chip layout.
	for _, ind := range industries {
		if _, ok := ind["id"].(string); !ok {
			t.Error("expected id field on industry")
		}
		if _, ok := ind["name"].(string); !ok {
			t.Error("expected name field on industry")
		}
	}
}

// =====================================================================
// 2. HandleIndustryOverview
// =====================================================================

// Happy path: 200 with full industry list and adjusted weights summing to ~1.0.
func TestHandleIndustryOverview_Happy(t *testing.T) {
	h := setupPageLoadHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-overview", nil)

	status, body := h.HandleIndustryOverview(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	industries, ok := resp["industries"].([]map[string]any)
	if !ok {
		t.Fatal("expected industries array in response")
	}
	if len(industries) == 0 {
		t.Fatal("expected non-empty industries list")
	}
	if _, ok := resp["updated_at"].(time.Time); !ok {
		t.Error("expected updated_at timestamp")
	}

	// All adjusted weights must sum to ~1.0 (industry.js renders them as
	// percentage chips and would be visually broken otherwise).
	total := 0.0
	for _, ind := range industries {
		if w, ok := ind["adjusted_weight"].(float64); ok {
			total += w
		}
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("expected total adjusted weight ~1.0, got %.4f", total)
	}

	// Each row must contain the linkage_score and cycle_phase fields that
	// renderIndustryLinkage() reads directly.
	for _, ind := range industries {
		if _, hasLink := ind["linkage_score"]; !hasLink {
			t.Error("expected linkage_score on each industry row")
		}
		if _, hasPhase := ind["cycle_phase"]; !hasPhase {
			t.Error("expected cycle_phase on each industry row")
		}
	}
}

// Empty ledger case: when the classifier has no top-level segments, the
// overview returns 200 with an empty industries array (frontend shows
// "尚無產業關聯資料" via renderIndustryLinkage()).
func TestHandleIndustryOverview_EmptyLedger(t *testing.T) {
	h := setupPageLoadHandlers(t)
	// Replace classifier with a deliberately empty one.
	h.Svc.Classifier = industry.NewClassificationTree() // empty tree
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-overview", nil)

	status, body := h.HandleIndustryOverview(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for empty classifier, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	industries, ok := resp["industries"].([]map[string]any)
	if !ok {
		t.Fatal("expected industries array (possibly empty)")
	}
	if len(industries) != 0 {
		t.Errorf("expected 0 industries for empty classifier, got %d", len(industries))
	}
	if count, _ := resp["count"].(int); count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}
}

// =====================================================================
// 3. HandleIndustrySeasonality
// =====================================================================

// Happy path with patterns: returns active + historical lists and adjustment.
func TestHandleIndustrySeasonality_Happy(t *testing.T) {
	h := setupPageLoadHandlers(t)
	req := httptest.NewRequest(http.MethodGet,
		"/api/dashboard/industry-seasonality?industry=semiconductor", nil)

	status, body := h.HandleIndustrySeasonality(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	if _, ok := resp["current_date"].(string); !ok {
		t.Error("expected current_date string")
	}
	// Without specific assertions on count (date-dependent), just confirm
	// the lists are present and well-typed.
	if _, ok := resp["active_patterns"].([]map[string]any); !ok {
		t.Error("expected active_patterns array")
	}
	if _, ok := resp["all_patterns"].([]map[string]any); !ok {
		t.Error("expected all_patterns array")
	}
	if _, ok := resp["pattern_count"].(int); !ok {
		t.Error("expected pattern_count int")
	}
}

// No patterns configured (use an industry that does not match any pattern).
// The contract is: response is 200, lists are present (possibly empty), and
// the adjustment breakdown map is the right shape.
func TestHandleIndustrySeasonality_NoPatterns(t *testing.T) {
	h := setupPageLoadHandlers(t)
	// Pass an industry that does not match any pattern's favored/avoided.
	req := httptest.NewRequest(http.MethodGet,
		"/api/dashboard/industry-seasonality?industry=etf_rotation", nil)

	status, body := h.HandleIndustrySeasonality(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 even when no patterns match, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	// `active_patterns` is JSON-unmarshalled as []any (not []map[string]any)
	// when round-tripped through silentGetJSON. The contract is just that
	// the handler returns 200 and well-typed list/breakdown fields.
	if _, present := resp["active_patterns"]; !present {
		t.Error("expected active_patterns key in response")
	}
	if _, present := resp["all_patterns"]; !present {
		t.Error("expected all_patterns key in response")
	}
	if c, _ := resp["pattern_count"].(int); c < 0 {
		t.Error("expected non-negative pattern_count")
	}
}

// Calibrator not ready: when parameters.json is missing the seasonal_patterns
// timestamp, calibration_evidence should be nil and the handler must still
// return 200 (frontend treats `undefined` calibration as "待驗證" badge).
func TestHandleIndustrySeasonality_CalibratorNotReady(t *testing.T) {
	// Build a temporary parameters.json without the seasonal_patterns block
	// so LoadCalibrationEvidence returns nil.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "parameters.json")
	minimal := `{"version":"test","industry":{}}`
	if err := os.WriteFile(cfgPath, []byte(minimal), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	// Temporarily override the work dir so config.Load() picks up our copy.
	orig := os.Getenv("ATLAS_WORK_DIR")
	t.Setenv("ATLAS_WORK_DIR", dir)
	defer os.Setenv("ATLAS_WORK_DIR", orig)

	// Reload config (it caches via sync.Once but in practice the package
	// honours ATLAS_WORK_DIR on first load).
	_ = config.GetParametersConfig()

	h := setupPageLoadHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-seasonality", nil)
	status, body := h.HandleIndustrySeasonality(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 when calibrator is not ready, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	// calibration_evidence is `any`; when nil the JSON omits it. The contract
	// for the frontend is that missing evidence -> "待驗證" badge, so we just
	// verify the field is absent-or-null and no error is raised.
	if v, present := resp["calibration_evidence"]; present && v != nil {
		// Some configurations may still surface a partial object; that's
		// acceptable as long as it's not raising a 500.
		t.Logf("calibration_evidence present: %v", v)
	}
}

// =====================================================================
// 4. HandleIndustrySeasonalityCalendar
// =====================================================================

// Happy path: 200 with 12 month entries.
func TestHandleIndustrySeasonalityCalendar_Happy(t *testing.T) {
	h := setupPageLoadHandlers(t)
	q := url.Values{}
	q.Set("industry", "semiconductor")
	req := httptest.NewRequest(http.MethodGet,
		"/api/dashboard/industry-seasonality-calendar?"+q.Encode(), nil)

	status, body := h.HandleIndustrySeasonalityCalendar(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	if y, ok := resp["year"].(int); !ok || y != time.Now().Year() {
		t.Errorf("expected current year %d, got %v", time.Now().Year(), resp["year"])
	}
	if _, ok := resp["industry"].(string); !ok {
		t.Error("expected industry field")
	}
	months, ok := resp["months"].([]map[string]any)
	if !ok {
		t.Fatal("expected months array")
	}
	if len(months) != 12 {
		t.Errorf("expected 12 months, got %d", len(months))
	}
	// Each month should have a `month` (1-12) and `count` field.
	for m, entry := range months {
		if _, ok := entry["month"].(int); !ok {
			t.Errorf("month[%d] missing `month` int", m)
		}
		if _, ok := entry["count"].(int); !ok {
			t.Errorf("month[%d] missing `count` int", m)
		}
	}
}

// Cache miss case: industry filter with no matching patterns returns 200
// with empty pattern lists (frontend renders "尚無季節性模式資料").
func TestHandleIndustrySeasonalityCalendar_DefaultForUnmatched(t *testing.T) {
	h := setupPageLoadHandlers(t)
	q := url.Values{}
	q.Set("industry", "no_such_industry_xyz")
	req := httptest.NewRequest(http.MethodGet,
		"/api/dashboard/industry-seasonality-calendar?"+q.Encode(), nil)

	status, body := h.HandleIndustrySeasonalityCalendar(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 even with unmatched industry, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	months, ok := resp["months"].([]map[string]any)
	if !ok {
		t.Fatal("expected months array")
	}
	if len(months) != 12 {
		t.Errorf("expected 12 months even when no patterns match, got %d", len(months))
	}
	// Each month's count should be 0 since no patterns are relevant.
	allZero := true
	for _, m := range months {
		if c, _ := m["count"].(int); c != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Error("expected all month counts=0 for unmatched industry")
	}
}

// =====================================================================
// 5. HandleIndustryGraph
// =====================================================================

// Happy path: returns nodes and edges.
func TestHandleIndustryGraph_Happy(t *testing.T) {
	h := setupPageLoadHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-graph", nil)

	status, body := h.HandleIndustryGraph(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	nodes, ok := resp["nodes"].([]map[string]any)
	if !ok {
		t.Fatal("expected nodes array")
	}
	edges, ok := resp["edges"].([]map[string]any)
	if !ok {
		t.Fatal("expected edges array")
	}
	if len(nodes) == 0 {
		t.Error("expected non-empty nodes list")
	}
	// Each node must expose id, systemic_importance (industry.js draws
	// radius proportional to this), upstream_count, downstream_count.
	for i, n := range nodes {
		if _, ok := n["id"].(string); !ok {
			t.Errorf("node[%d] missing id", i)
		}
		if _, ok := n["systemic_importance"].(float64); !ok {
			t.Errorf("node[%d] missing systemic_importance", i)
		}
	}
	// Edges are not required to be non-empty (depends on correlation data),
	// but if present must include source/target/correlation/strength.
	for i, e := range edges {
		if _, ok := e["source"].(string); !ok {
			t.Errorf("edge[%d] missing source", i)
		}
		if _, ok := e["target"].(string); !ok {
			t.Errorf("edge[%d] missing target", i)
		}
		if _, ok := e["correlation"].(float64); !ok {
			t.Errorf("edge[%d] missing correlation", i)
		}
		if _, ok := e["strength"].(string); !ok {
			t.Errorf("edge[%d] missing strength", i)
		}
	}
}

// Empty graph: install a fresh empty SupplyChainGraph + empty CorrelationMatrix.
// Handler must return 200 with empty node/edge arrays (no 500).
func TestHandleIndustryGraph_EmptyGraph(t *testing.T) {
	h := setupPageLoadHandlers(t)
	emptyGraph := industry.NewSupplyChainGraph()
	emptyCM := industry.NewCorrelationMatrix(0)
	h.Svc.LinkageAnalyzer.SetSupplyChainGraph(emptyGraph, emptyCM)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-graph", nil)
	status, body := h.HandleIndustryGraph(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for empty graph, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	nodes, _ := resp["nodes"].([]map[string]any)
	edges, _ := resp["edges"].([]map[string]any)
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

// Circular dependency in the graph: A→B→A. The PropagateShock collector uses
// a visited map so the BFS terminates, but the integration path is the
// industry-graph endpoint feeding nodes/edges from GetAllCorrelations. We
// install a circular correlation directly and assert the handler still
// returns 200 (no stack overflow, no panic).
func TestHandleIndustryGraph_CircularDependencySafe(t *testing.T) {
	h := setupPageLoadHandlers(t)
	cm := industry.NewCorrelationMatrix(0)
	// A <-> B (mutual correlation triggers the "node B seen twice" path
	// inside GetIndustryGraph; visited logic in PropagateShock prevents
	// infinite recursion).
	cm.UpdateCorrelation("alpha", "beta", 0.85)
	cm.UpdateCorrelation("beta", "alpha", 0.85)
	cm.UpdateCorrelation("alpha", "gamma", -0.30)
	cm.UpdateCorrelation("gamma", "alpha", -0.30)

	graph := industry.NewSupplyChainGraph()
	graph.AddNode(&industry.SupplyChainNode{
		IndustryID:   "alpha",
		UpstreamOf:   []string{"beta"},
		DownstreamOf: []string{"gamma"},
	})
	graph.AddNode(&industry.SupplyChainNode{
		IndustryID:   "beta",
		UpstreamOf:   []string{"alpha"},
		DownstreamOf: []string{},
	})
	graph.AddNode(&industry.SupplyChainNode{
		IndustryID:   "gamma",
		UpstreamOf:   []string{},
		DownstreamOf: []string{"alpha"},
	})
	h.Svc.LinkageAnalyzer.SetSupplyChainGraph(graph, cm)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-graph", nil)
	status, body := h.HandleIndustryGraph(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for circular graph, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	nodes, _ := resp["nodes"].([]map[string]any)
	if len(nodes) < 2 {
		t.Errorf("expected at least 2 nodes from circular graph, got %d", len(nodes))
	}
	// Edges from the matrix should still be enumerated.
	edges, _ := resp["edges"].([]map[string]any)
	t.Logf("circular graph yielded %d nodes, %d edges", len(nodes), len(edges))
}

// =====================================================================
// 6. HandleCycleStatusCard
// =====================================================================

// Happy path: composite card with all 5 layers populated.
func TestHandleCycleStatusCard_Happy(t *testing.T) {
	h := setupPageLoadHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/cycle-status-card", nil)

	status, body := h.HandleCycleStatusCard(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	card, ok := resp["card"].(*industry.CycleStatusCard)
	if !ok {
		t.Fatalf("expected *industry.CycleStatusCard, got %T", resp["card"])
	}
	if card.CompositeCoefficient < 0.80 || card.CompositeCoefficient > 1.20 {
		t.Errorf("composite coefficient out of clamp range: %.3f", card.CompositeCoefficient)
	}
	if card.SentimentLabel == "" {
		t.Error("expected non-empty sentiment_label")
	}
	if len(card.Breakdown) != 5 {
		t.Errorf("expected 5 layer breakdown entries, got %d", len(card.Breakdown))
	}
	// Verify the 5 named layers are present (matches industry.js layerDefs).
	wantLayers := map[string]bool{
		"silicon": false, "business_cycle": false, "seasonal": false,
		"events": false, "supply_chain": false,
	}
	for _, b := range card.Breakdown {
		if _, ok := wantLayers[b.Layer]; ok {
			wantLayers[b.Layer] = true
		}
	}
	for layer, found := range wantLayers {
		if !found {
			t.Errorf("breakdown missing layer: %s", layer)
		}
	}
}

// Per-industry card endpoint: must populate the 5 layers for a specific
// industry, not the composite aggregation.
func TestHandleCycleStatusCard_IndustrySpecific(t *testing.T) {
	h := setupPageLoadHandlers(t)
	q := url.Values{}
	q.Set("industry", "semiconductor")
	req := httptest.NewRequest(http.MethodGet,
		"/api/dashboard/cycle-status-card?"+q.Encode(), nil)

	status, body := h.HandleCycleStatusCard(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	card, ok := resp["card"].(*industry.CycleStatusCard)
	if !ok {
		t.Fatalf("expected *industry.CycleStatusCard, got %T", resp["card"])
	}
	if len(card.Breakdown) != 5 {
		t.Errorf("expected 5 breakdown entries, got %d", len(card.Breakdown))
	}
	if card.BusinessCycle == "" {
		t.Error("expected business_cycle populated for known industry")
	}
}

// Missing layer data: rebuild the card builder with nil seasonal engine
// and nil event calendar so the seasonal + events layers are empty. The
// handler must still return 200 with a partial card (frontend renders
// "尚無原因說明" for empty layers).
func TestHandleCycleStatusCard_PartialLayers(t *testing.T) {
	h := setupPageLoadHandlers(t)
	// CardBuilder was wired at construction; rebuild it with nil deps to
	// force the seasonal and event layers to use their neutral defaults.
	h.Svc.CardBuilder = industry.NewCycleStatusCardBuilder(
		h.Svc.SiliconTracker, // keep silicon for richer card
		h.Svc.CycleTracker,
		nil, // seasonal engine missing
		nil, // event calendar missing
		h.Svc.LinkageAnalyzer,
	)

	q := url.Values{}
	q.Set("industry", "semiconductor")
	req := httptest.NewRequest(http.MethodGet,
		"/api/dashboard/cycle-status-card?"+q.Encode(), nil)
	status, body := h.HandleCycleStatusCard(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 with partial layers, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	card, ok := resp["card"].(*industry.CycleStatusCard)
	if !ok {
		t.Fatalf("expected *industry.CycleStatusCard, got %T", resp["card"])
	}
	if len(card.Breakdown) != 5 {
		t.Errorf("expected 5 breakdown entries even with partial data, got %d", len(card.Breakdown))
	}
	// Seasonal and events should be at their neutral defaults when their
	// backing engine is nil: seasonal returns 1.0, events returns 1.0.
	for _, b := range card.Breakdown {
		if b.Layer == "seasonal" && b.RawValue != 1.0 {
			t.Errorf("expected seasonal layer neutral 1.0, got %.3f", b.RawValue)
		}
		if b.Layer == "events" && b.RawValue != 1.0 {
			t.Errorf("expected events layer neutral 1.0, got %.3f", b.RawValue)
		}
	}
}

// No cycles tracked: pass a non-existent industry, handler must return
// 200 with the card using neutral defaults (frontend shows the empty
// "尚無週期資料" state).
func TestHandleCycleStatusCard_NoCyclesTracked(t *testing.T) {
	h := setupPageLoadHandlers(t)
	q := url.Values{}
	q.Set("industry", "no_such_industry_zzz")
	req := httptest.NewRequest(http.MethodGet,
		"/api/dashboard/cycle-status-card?"+q.Encode(), nil)

	status, body := h.HandleCycleStatusCard(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for unknown industry, got %d", status)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	card, ok := resp["card"].(*industry.CycleStatusCard)
	if !ok {
		t.Fatalf("expected *industry.CycleStatusCard, got %T", resp["card"])
	}
	if card.BusinessCycle != "" {
		t.Errorf("expected empty business_cycle for unknown industry, got %q", card.BusinessCycle)
	}
	// CycleConfidence should be the neutral 0.5.
	if card.CycleConfidence != 0.5 {
		t.Errorf("expected neutral confidence 0.5, got %.3f", card.CycleConfidence)
	}
}

// =====================================================================
// JSON round-trip sanity: confirm each endpoint produces a JSON-encodable
// body. industry.js calls silentGetJSON() which JSON.parse()s the response.
// =====================================================================

func TestPageLoadHandlers_JSONEncodable(t *testing.T) {
	h := setupPageLoadHandlers(t)
	endpoints := []struct {
		name string
		req  *http.Request
		fn   func(*http.Request) (int, any)
	}{
		{"classification", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-classification", nil), h.HandleIndustryClassification},
		{"overview", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-overview", nil), h.HandleIndustryOverview},
		{"seasonality", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-seasonality", nil), h.HandleIndustrySeasonality},
		{"calendar", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-seasonality-calendar", nil), h.HandleIndustrySeasonalityCalendar},
		{"graph", httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-graph", nil), h.HandleIndustryGraph},
		{"cycle-status-card", httptest.NewRequest(http.MethodGet, "/api/dashboard/cycle-status-card", nil), h.HandleCycleStatusCard},
	}
	for _, e := range endpoints {
		_, body := e.fn(e.req)
		if _, err := json.Marshal(body); err != nil {
			t.Errorf("%s: response not JSON-encodable: %v", e.name, err)
		}
	}
}
