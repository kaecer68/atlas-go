package industry

import (
	"math"
	"slices"
	"testing"
)

func TestDefaultSupplyChainGraph(t *testing.T) {
	graph := DefaultSupplyChainGraph()

	// Test semiconductor node
	node, ok := graph.GetNode("semiconductor")
	if !ok {
		t.Fatal("semiconductor node not found")
	}
	if node.Tier != 1 {
		t.Errorf("expected tier 1, got %d", node.Tier)
	}
	if len(node.UpstreamOf) == 0 {
		t.Error("expected upstream connections for semiconductor")
	}
	if len(node.DownstreamOf) == 0 {
		t.Error("expected downstream connections for semiconductor")
	}
}

func TestGetUpstreamDownstream(t *testing.T) {
	graph := DefaultSupplyChainGraph()

	// Test upstream of AI supply chain
	upstream := graph.GetUpstream("ai_supply_chain")
	if len(upstream) == 0 {
		t.Error("expected upstream suppliers for ai_supply_chain")
	}

	// Test downstream of semiconductor
	downstream := graph.GetDownstream("semiconductor")
	if len(downstream) == 0 {
		t.Error("expected downstream customers for semiconductor")
	}

	// Test non-existent industry
	up := graph.GetUpstream("nonexistent")
	if up != nil {
		t.Error("expected nil for non-existent industry")
	}
}

func TestGetUpstreamChain(t *testing.T) {
	graph := DefaultSupplyChainGraph()

	// Get full upstream chain for AI supply chain
	chain := graph.GetUpstreamChain("ai_supply_chain", 3)
	if len(chain) == 0 {
		t.Error("expected upstream chain for ai_supply_chain")
	}

	// Should include semiconductor and its suppliers
	var hasSemiconductor bool
	if slices.Contains(chain, "semiconductor") {
		hasSemiconductor = true
	}
	if !hasSemiconductor {
		t.Error("expected semiconductor in upstream chain")
	}
}

func TestGetDownstreamChain(t *testing.T) {
	graph := DefaultSupplyChainGraph()

	// Get full downstream chain for semiconductor
	chain := graph.GetDownstreamChain("semiconductor", 3)
	if len(chain) == 0 {
		t.Error("expected downstream chain for semiconductor")
	}
}

func TestCorrelationMatrix(t *testing.T) {
	cm := DefaultCorrelationMatrix()

	// Test known correlation
	corr, ok := cm.GetCorrelation("semiconductor", "ai_supply_chain")
	if !ok {
		t.Fatal("expected correlation between semiconductor and ai_supply_chain")
	}
	if corr != 0.85 {
		t.Errorf("expected correlation 0.85, got %f", corr)
	}

	// Test symmetry
	corrReverse, ok := cm.GetCorrelation("ai_supply_chain", "semiconductor")
	if !ok {
		t.Fatal("expected symmetric correlation")
	}
	if corr != corrReverse {
		t.Error("expected symmetric correlation")
	}

	// Test non-existent correlation
	_, ok = cm.GetCorrelation("semiconductor", "nonexistent")
	if ok {
		t.Error("expected no correlation for non-existent industry")
	}
}

func TestGetCorrelatedIndustries(t *testing.T) {
	cm := DefaultCorrelationMatrix()

	// Get industries correlated with semiconductor (min 0.5)
	correlated := cm.GetCorrelatedIndustries("semiconductor", 0.5)
	if len(correlated) == 0 {
		t.Error("expected correlated industries")
	}

	// Should include ai_supply_chain
	if _, ok := correlated["ai_supply_chain"]; !ok {
		t.Error("expected ai_supply_chain to be correlated with semiconductor")
	}

	// Should not include shipping (negative correlation)
	if _, ok := correlated["shipping"]; ok {
		t.Error("expected shipping not to be correlated with semiconductor")
	}
}

func TestUpdateCorrelation(t *testing.T) {
	cm := NewCorrelationMatrix(30)

	cm.UpdateCorrelation("a", "b", 0.75)

	corr, ok := cm.GetCorrelation("a", "b")
	if !ok {
		t.Fatal("expected correlation")
	}
	if corr != 0.75 {
		t.Errorf("expected 0.75, got %f", corr)
	}

	// Update existing correlation
	cm.UpdateCorrelation("a", "b", 0.80)
	corr, _ = cm.GetCorrelation("a", "b")
	if corr != 0.80 {
		t.Errorf("expected updated correlation 0.80, got %f", corr)
	}
}

func TestShockPropagation(t *testing.T) {
	graph := DefaultSupplyChainGraph()
	cm := DefaultCorrelationMatrix()
	sp := NewShockPropagation(graph, cm)

	// Propagate shock from semiconductor
	impacts := sp.PropagateShock("semiconductor", 0.10, 2)

	// Should impact semiconductor itself
	if impacts["semiconductor"] != 0.10 {
		t.Errorf("expected semiconductor impact 0.10, got %f", impacts["semiconductor"])
	}

	// Should impact downstream industries
	if len(impacts) <= 1 {
		t.Error("expected downstream impacts")
	}

	// Downstream impact should be smaller than source
	for industry, impact := range impacts {
		if industry != "semiconductor" && impact >= 0.10 {
			t.Errorf("expected downstream impact < 0.10, got %f for %s", impact, industry)
		}
	}
}

func TestCalculateLinkageScore(t *testing.T) {
	graph := DefaultSupplyChainGraph()
	cm := DefaultCorrelationMatrix()
	sp := NewShockPropagation(graph, cm)

	score := sp.CalculateLinkageScore("semiconductor")
	if score == nil {
		t.Fatal("expected non-nil linkage score")
	}

	if score.IndustryID != "semiconductor" {
		t.Errorf("expected industry semiconductor, got %s", score.IndustryID)
	}

	if score.UpstreamCount == 0 && score.DownstreamCount == 0 {
		t.Error("expected some connections for semiconductor")
	}

	if score.SystemicImportance < 0 || score.SystemicImportance > 1 {
		t.Errorf("expected systemic importance in [0,1], got %f", score.SystemicImportance)
	}
}

func TestLinkageScoreString(t *testing.T) {
	score := &IndustryLinkageScore{
		IndustryID:         "semiconductor",
		UpstreamCount:      2,
		DownstreamCount:    3,
		AvgCorrelation:     0.65,
		SystemicImportance: 0.50,
	}

	s := score.String()
	expected := "semiconductor: Upstream=2, Downstream=3, AvgCorr=0.65, Systemic=50%"
	if s != expected {
		t.Errorf("expected '%s', got '%s'", expected, s)
	}
}

func TestEmptyGraph(t *testing.T) {
	graph := NewSupplyChainGraph()

	chain := graph.GetUpstreamChain("anything", 3)
	if len(chain) != 0 {
		t.Error("expected empty chain for empty graph")
	}
}

func TestGetAllCorrelations(t *testing.T) {
	cm := DefaultCorrelationMatrix()
	all := cm.GetAllCorrelations()

	if len(all) == 0 {
		t.Error("expected correlations")
	}

	// Check semiconductor has correlations
	if _, ok := all["semiconductor"]; !ok {
		t.Error("expected semiconductor in correlations")
	}
}

type mockNarrativeProvider struct {
	themes      []string
	multipliers map[string]float64
}

func (m *mockNarrativeProvider) ActiveThemes() []string {
	return m.themes
}

func (m *mockNarrativeProvider) CorrelationMultiplier(theme, industryA, industryB string) float64 {
	key := theme + ":" + industryA + ":" + industryB
	if v, ok := m.multipliers[key]; ok {
		return v
	}
	key = theme + ":" + industryB + ":" + industryA
	if v, ok := m.multipliers[key]; ok {
		return v
	}
	return 1.0
}

func TestGetNarrativeAdjustedCorrelation(t *testing.T) {
	graph := DefaultSupplyChainGraph()
	cm := DefaultCorrelationMatrix()
	sp := NewShockPropagation(graph, cm)

	baseCorr, _ := cm.GetCorrelation("semiconductor", "ai_supply_chain")

	mock := &mockNarrativeProvider{
		themes: []string{"AI_capex_surge"},
		multipliers: map[string]float64{
			"AI_capex_surge:semiconductor:ai_supply_chain": 1.12,
		},
	}
	sp.SetNarrativeProvider(mock)

	adj := sp.getNarrativeAdjustedCorrelation("semiconductor", "ai_supply_chain")
	want := baseCorr * 1.12
	if adj != want {
		t.Errorf("expected adjusted correlation %.4f, got %.4f", want, adj)
	}

	sp.SetNarrativeProvider(nil)
	adj = sp.getNarrativeAdjustedCorrelation("semiconductor", "ai_supply_chain")
	if adj != baseCorr {
		t.Errorf("expected base correlation %.4f without provider, got %.4f", baseCorr, adj)
	}
}

func TestNarrativeAwarePropagateShock(t *testing.T) {
	graph := DefaultSupplyChainGraph()
	cm := DefaultCorrelationMatrix()
	sp := NewShockPropagation(graph, cm)

	sp.SetNarrativeProvider(nil)
	baseline := sp.PropagateShock("semiconductor", 0.10, 2)

	mock := &mockNarrativeProvider{
		themes: []string{"AI_capex_surge"},
		multipliers: map[string]float64{
			"AI_capex_surge:semiconductor:ai_supply_chain": 1.50,
		},
	}
	sp.SetNarrativeProvider(mock)
	narrative := sp.PropagateShock("semiconductor", 0.10, 2)

	if baseline["semiconductor"] != narrative["semiconductor"] {
		t.Error("source industry impact should not change with narrative provider")
	}

	if baseline["ai_supply_chain"] == narrative["ai_supply_chain"] {
		t.Error("expected different shock impact on ai_supply_chain when narrative is active")
	}

	if narrative["ai_supply_chain"] <= baseline["ai_supply_chain"] {
		t.Errorf("expected larger impact with amplified correlation, got baseline=%f narrative=%f",
			baseline["ai_supply_chain"], narrative["ai_supply_chain"])
	}
}

func TestNarrativeAwareLinkageScore(t *testing.T) {
	graph := DefaultSupplyChainGraph()
	cm := DefaultCorrelationMatrix()
	sp := NewShockPropagation(graph, cm)

	sp.SetNarrativeProvider(nil)
	baseline := sp.CalculateLinkageScore("semiconductor")

	mock := &mockNarrativeProvider{
		themes: []string{"AI_capex_surge"},
		multipliers: map[string]float64{
			"AI_capex_surge:semiconductor:ai_supply_chain": 1.20,
			"AI_capex_surge:semiconductor:electronics":     1.20,
		},
	}
	sp.SetNarrativeProvider(mock)
	narrative := sp.CalculateLinkageScore("semiconductor")

	if narrative.UpstreamCount != baseline.UpstreamCount {
		t.Error("upstream count should not change with narrative provider")
	}
	if narrative.DownstreamCount != baseline.DownstreamCount {
		t.Error("downstream count should not change with narrative provider")
	}

	if narrative.SystemicImportance != baseline.SystemicImportance {
		t.Error("systemic importance should not change with narrative provider")
	}

	if narrative.AvgCorrelation == baseline.AvgCorrelation {
		t.Error("expected different avg_correlation when narrative is active")
	}

	if narrative.AvgCorrelation <= baseline.AvgCorrelation {
		t.Errorf("expected higher avg correlation with amplified narrative, got baseline=%f narrative=%f",
			baseline.AvgCorrelation, narrative.AvgCorrelation)
	}

	if narrative.ShockPropagationSpeed == baseline.ShockPropagationSpeed {
		t.Error("expected different shock_propagation_speed when narrative is active")
	}
}

func TestLoadSupplyChainGraph_FileNotFound(t *testing.T) {
	_, _, err := LoadSupplyChainGraph("/nonexistent/path/graph.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadSupplyChainGraph_ValidFile(t *testing.T) {
	graph, cm, err := LoadSupplyChainGraph("../../configs/supply_chain_graph.json")
	if err != nil {
		t.Fatalf("LoadSupplyChainGraph failed: %v", err)
	}
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
	if cm == nil {
		t.Fatal("expected non-nil correlation matrix")
	}

	node, ok := graph.GetNode("semiconductor")
	if !ok {
		t.Fatal("semiconductor node not found in loaded graph")
	}
	if node.Tier != 1 {
		t.Errorf("expected tier 1, got %d", node.Tier)
	}

	corr, ok := cm.GetCorrelation("semiconductor", "ai_supply_chain")
	if !ok {
		t.Fatal("expected correlation between semiconductor and ai_supply_chain")
	}
	if corr != 0.85 {
		t.Errorf("expected correlation 0.85, got %f", corr)
	}
}

func TestSetSupplyChainGraph(t *testing.T) {
	graph, cm, err := LoadSupplyChainGraph("../../configs/supply_chain_graph.json")
	if err != nil {
		t.Fatalf("LoadSupplyChainGraph failed: %v", err)
	}

	la := NewLinkageAnalyzer()
	la.SetSupplyChainGraph(graph, cm)

	score := la.CalculateLinkageScore("semiconductor")
	if score == nil {
		t.Fatal("expected non-nil linkage score")
	}
	if score.IndustryID != "semiconductor" {
		t.Errorf("expected industry semiconductor, got %s", score.IndustryID)
	}
}

func TestCorrelationMatrix_RegimeAdjustedCorrelation_NoProvider(t *testing.T) {
	cm := NewCorrelationMatrix(30)
	cm.UpdateCorrelation("semiconductor", "ai_supply_chain", 0.85)

	// Without cycle provider, should return raw correlation
	corr := cm.RegimeAdjustedCorrelation("semiconductor", "ai_supply_chain")
	if math.Abs(corr-0.85) > 0.0001 {
		t.Fatalf("expected 0.85 without provider, got %f", corr)
	}
}

func TestCorrelationMatrix_RegimeAdjustedCorrelation_RecessionBoost(t *testing.T) {
	cm := NewCorrelationMatrix(30)
	cm.UpdateCorrelation("semiconductor", "ai_supply_chain", 0.70)

	mock := &mockCycleProvider{
		phases: map[string]CyclePhase{
			"semiconductor":   CycleRecession,
			"ai_supply_chain": CycleExpansion,
		},
	}
	cm.SetCycleProvider(mock)

	corr := cm.RegimeAdjustedCorrelation("semiconductor", "ai_supply_chain")
	expected := 0.70 * 1.30 // 30% boost
	if math.Abs(corr-expected) > 0.0001 {
		t.Fatalf("expected %f during recession, got %f", expected, corr)
	}
}

func TestCorrelationMatrix_RegimeAdjustedCorrelation_MutualExpansionDampened(t *testing.T) {
	cm := NewCorrelationMatrix(30)
	cm.UpdateCorrelation("semiconductor", "ai_supply_chain", 0.60)

	mock := &mockCycleProvider{
		phases: map[string]CyclePhase{
			"semiconductor":   CycleExpansion,
			"ai_supply_chain": CycleExpansion,
		},
	}
	cm.SetCycleProvider(mock)

	corr := cm.RegimeAdjustedCorrelation("semiconductor", "ai_supply_chain")
	// Mutual expansion: correlation dampened by 10% (diversification benefit)
	expected := 0.60 * 0.90
	if math.Abs(corr-expected) > 0.0001 {
		t.Fatalf("expected %f during mutual expansion, got %f", expected, corr)
	}
}

func TestCorrelationMatrix_RegimeAdjustedCorrelation_MissingPhase(t *testing.T) {
	cm := NewCorrelationMatrix(30)
	cm.UpdateCorrelation("semiconductor", "ai_supply_chain", 0.50)

	mock := &mockCycleProvider{
		phases: map[string]CyclePhase{
			// intentionally empty for semiconductor/ai_supply_chain
		},
	}
	cm.SetCycleProvider(mock)

	corr := cm.RegimeAdjustedCorrelation("semiconductor", "ai_supply_chain")
	if math.Abs(corr-0.50) > 0.0001 {
		t.Fatalf("expected 0.50 passthrough, got %f", corr)
	}
}

func TestShockPropagation_CyclePlusNarrativeCombo(t *testing.T) {
	graph := DefaultSupplyChainGraph()
	cm := NewCorrelationMatrix(30)
	cm.UpdateCorrelation("semiconductor", "ai_supply_chain", 0.70)

	sp := NewShockPropagation(graph, cm)

	cycleProvider := &mockCycleProvider{
		phases: map[string]CyclePhase{
			"semiconductor": CycleRecession,
		},
	}
	cm.SetCycleProvider(cycleProvider)

	// cycle boost: 0.70 * 1.30 = 0.91
	// Without narrative provider, should return 0.91
	corr := sp.getNarrativeAdjustedCorrelation("semiconductor", "ai_supply_chain")
	expected := 0.70 * 1.30
	if math.Abs(corr-expected) > 0.0001 {
		t.Fatalf("expected %f with cycle boost only, got %f", expected, corr)
	}
}

func TestGetNarrativeAdjustedCorrelation_NegativeCorrelation(t *testing.T) {
	graph := DefaultSupplyChainGraph()
	cm := NewCorrelationMatrix(30)
	cm.UpdateCorrelation("semiconductor", "energy", -0.45)
	sp := NewShockPropagation(graph, cm)

	mock := &mockNarrativeProvider{
		themes: []string{"oil_price_shock"},
		multipliers: map[string]float64{
			"oil_price_shock:semiconductor:energy": 1.20,
		},
	}
	sp.SetNarrativeProvider(mock)

	adj := sp.getNarrativeAdjustedCorrelation("semiconductor", "energy")
	// -0.45 * 1.20 = -0.54 — must NOT be clamped to 0
	if adj >= 0 {
		t.Errorf("expected negative correlation after narrative adjustment, got %.4f", adj)
	}
	want := -0.45 * 1.20
	if math.Abs(adj-want) > 0.0001 {
		t.Errorf("expected %.4f, got %.4f", want, adj)
	}
}

func TestGetNarrativeAdjustedCorrelation_StrongNegativeCapped(t *testing.T) {
	graph := DefaultSupplyChainGraph()
	cm := NewCorrelationMatrix(30)
	cm.UpdateCorrelation("semiconductor", "energy", -0.80)
	sp := NewShockPropagation(graph, cm)

	mock := &mockNarrativeProvider{
		themes: []string{"geopolitical_risk_spike"},
		multipliers: map[string]float64{
			"geopolitical_risk_spike:semiconductor:energy": 2.0,
		},
	}
	sp.SetNarrativeProvider(mock)

	adj := sp.getNarrativeAdjustedCorrelation("semiconductor", "energy")
	// -0.80 * 2.0 = -1.60 → clamped to -1.0
	if adj != -1.0 {
		t.Errorf("expected clamped correlation -1.0, got %.4f", adj)
	}
}

// mockCycleProvider for testing
type mockCycleProvider struct {
	phases map[string]CyclePhase
}

func (m *mockCycleProvider) GetPhase(industryID string) (CyclePhase, bool) {
	p, ok := m.phases[industryID]
	return p, ok
}
