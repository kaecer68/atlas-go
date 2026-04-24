package industry

import (
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
	for _, industry := range chain {
		if industry == "semiconductor" {
			hasSemiconductor = true
			break
		}
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
