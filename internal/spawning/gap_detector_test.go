package spawning

import (
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestInferSectorFromAgent(t *testing.T) {
	tests := []struct {
		agent  domain.AgentSpec
		sector string
	}{
		{domain.AgentSpec{ID: "chip_mfg", Skill: "semiconductor"}, "semiconductor"},
		{domain.AgentSpec{ID: "chip_maker", Skill: ""}, "semiconductor"},
		{domain.AgentSpec{ID: "financial_analyst", Skill: "banking"}, "financial"},
		{domain.AgentSpec{ID: "shipping_expert", Skill: ""}, "shipping"},
		{domain.AgentSpec{ID: "bio_pharma", Skill: "pharma"}, "biotech"},
		{domain.AgentSpec{ID: "assembly_line", Skill: "ev"}, "automotive"},
		{domain.AgentSpec{ID: "industrial_expert", Skill: ""}, "industrials"},
		{domain.AgentSpec{ID: "consumer_goods", Skill: "retail"}, "consumer"},
		{domain.AgentSpec{ID: "reit_specialist", Skill: ""}, "real_estate"},
		{domain.AgentSpec{ID: "material_analyst", Skill: ""}, "materials"},
		{domain.AgentSpec{ID: "energy_trader", Skill: ""}, "energy"},
		{domain.AgentSpec{ID: "electricity_sector", Skill: "ai"}, "electronics"},
		{domain.AgentSpec{ID: "generic_agent", Skill: "random_skill"}, ""},
		{domain.AgentSpec{ID: "", Skill: ""}, ""},
	}
	for _, tt := range tests {
		got := inferSectorFromAgent(tt.agent)
		if got != tt.sector {
			t.Errorf("inferSectorFromAgent(%+v) = %q, want %q", tt.agent, got, tt.sector)
		}
	}
}

func TestInferStyleFromAgent(t *testing.T) {
	tests := []struct {
		agent domain.AgentSpec
		style string
	}{
		{domain.AgentSpec{ID: "value_investor", Skill: "yield"}, "value"},
		{domain.AgentSpec{ID: "growth_hunter", Skill: ""}, "growth"},
		{domain.AgentSpec{ID: "momentum_trader", Skill: "breakout"}, "momentum"},
		{domain.AgentSpec{ID: "quality_seeker", Skill: ""}, "quality"},
		{domain.AgentSpec{ID: "contrarian_bet", Skill: ""}, "contrarian"},
		{domain.AgentSpec{ID: "trend_follower", Skill: ""}, "trend_following"},
		{domain.AgentSpec{ID: "mean_reversion_bot", Skill: "reversal"}, "mean_reversion"},
		{domain.AgentSpec{ID: "reversal_expert", Skill: ""}, "mean_reversion"},
		{domain.AgentSpec{ID: "generic_agent", Skill: "unknown"}, ""},
		{domain.AgentSpec{ID: "", Skill: ""}, ""},
	}
	for _, tt := range tests {
		got := inferStyleFromAgent(tt.agent)
		if got != tt.style {
			t.Errorf("inferStyleFromAgent(%+v) = %q, want %q", tt.agent, got, tt.style)
		}
	}
}

func TestAverage(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"empty", nil, 0},
		{"empty slice", []float64{}, 0},
		{"single", []float64{5.0}, 5.0},
		{"positive", []float64{1.0, 2.0, 3.0}, 2.0},
		{"negative", []float64{-1.0, -2.0, -3.0}, -2.0},
		{"mixed", []float64{-1.0, 0.0, 1.0}, 0.0},
		{"decimals", []float64{1.1, 2.2, 3.3}, 2.2},
	}
	for _, tt := range tests {
		got := average(tt.values)
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if diff > 1e-9 {
			t.Errorf("%s: average(%v) = %v, want %v", tt.name, tt.values, got, tt.want)
		}
	}
}

func TestCalculateGapPriorityScore(t *testing.T) {
	tests := []struct {
		severity      GapSeverity
		hoursAgo      float64
		wantBaseScore float64
	}{
		{GapSeverityCritical, 0, 100},
		{GapSeverityHigh, 0, 70},
		{GapSeverityMedium, 0, 40},
		{GapSeverityLow, 0, 20},
	}
	for _, tt := range tests {
		gap := &KnowledgeGap{
			Severity:   tt.severity,
			DetectedAt: time.Now().Add(-time.Duration(tt.hoursAgo) * time.Hour),
		}
		score := CalculateGapPriorityScore(gap)
		if score < tt.wantBaseScore-0.1 || score > tt.wantBaseScore+0.1 {
			t.Errorf("CalculateGapPriorityScore({severity=%s, hoursAgo=%.0f}) = %.2f, want ~%.0f",
				tt.severity, tt.hoursAgo, score, tt.wantBaseScore)
		}
	}
}

func TestCalculateGapPriorityScore_AgeBonus(t *testing.T) {
	// Test age bonus: use a recent time and a 10-day-old time, ensure older gets higher
	recentGap := &KnowledgeGap{
		Severity:   GapSeverityHigh,
		DetectedAt: time.Now(),
	}
	oldGap := &KnowledgeGap{
		Severity:   GapSeverityHigh,
		DetectedAt: time.Now().Add(-10 * 24 * time.Hour),
	}

	recentScore := CalculateGapPriorityScore(recentGap)
	oldScore := CalculateGapPriorityScore(oldGap)

	if oldScore <= recentScore {
		t.Errorf("old gap (%.2f) should score higher than recent gap (%.2f)", oldScore, recentScore)
	}
}

func TestCalculateGapPriorityScore_AgeBonusCapped(t *testing.T) {
	// Age bonus caps at 10
	gap := &KnowledgeGap{
		Severity:   GapSeverityLow,
		DetectedAt: time.Now().Add(-100 * 24 * time.Hour), // >10 days old
	}
	score := CalculateGapPriorityScore(gap)
	// Base 20 + max 10 = 30
	if score > 30.1 {
		t.Errorf("age bonus should be capped at 10, got %.2f total (base=20)", score)
	}
}

func TestGapDetector_DetectGaps_WithScorecards_RegimeGaps(t *testing.T) {
	detector := NewGapDetector()
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent_a", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
			{ID: "agent_b", Enabled: true, Layer: domain.LayerSector, Skill: "finance"},
		},
	}
	scorecards := map[string]*domain.Scorecard{
		"agent_a": {Observations: 50, SharpeLike: -0.8, MaxDrawdown: -0.25},
		"agent_b": {Observations: 40, SharpeLike: -1.2, MaxDrawdown: -0.30},
		"agent_c": {Observations: 10, SharpeLike: -1.5, MaxDrawdown: -0.50}, // too few observations
	}

	gaps := detector.DetectGaps(registry, scorecards, nil)

	// Check for regime gaps
	regimeGaps := 0
	for _, g := range gaps {
		if g.Type == GapTypeRegime {
			regimeGaps++
		}
	}
	if regimeGaps != 2 {
		t.Errorf("expected 2 regime gaps (agents with Sharpe < -0.5 and >= 30 observations), got %d", regimeGaps)
	}
}

func TestGapDetector_DetectGaps_CorrelationGaps(t *testing.T) {
	detector := NewGapDetector()
	// Create 6 agents all in the same sector to trigger correlation gap (>5)
	agents := make([]domain.AgentSpec, 6)
	for i := range 6 {
		agents[i] = domain.AgentSpec{
			ID:      "semi_agent_" + strings.Repeat("x", i),
			Enabled: true,
			Layer:   domain.LayerSector,
			Skill:   "semiconductor",
		}
	}
	registry := domain.AgentRegistry{Agents: agents}

	gaps := detector.DetectGaps(registry, nil, nil)

	// Check for correlation gaps (>=6 agents in semiconductor sector)
	corrGaps := 0
	for _, g := range gaps {
		if g.Type == GapTypeCorrelation {
			corrGaps++
			if g.Sector != "semiconductor" {
				t.Errorf("expected semiconductor sector in correlation gap, got %s", g.Sector)
			}
			if g.Severity != GapSeverityLow {
				t.Errorf("expected low severity for correlation gap, got %s", g.Severity)
			}
		}
	}
	if corrGaps != 1 {
		t.Fatalf("expected 1 correlation gap for oversaturated semiconductor sector, got %d", corrGaps)
	}
}

func TestGapDetector_DetectGaps_CorrelationGapsFiveOrFewer(t *testing.T) {
	detector := NewGapDetector()
	agents := make([]domain.AgentSpec, 5)
	for i := range 5 {
		agents[i] = domain.AgentSpec{
			ID:      "semi_agent_" + strings.Repeat("x", i),
			Enabled: true,
			Layer:   domain.LayerSector,
			Skill:   "semiconductor",
		}
	}
	registry := domain.AgentRegistry{Agents: agents}

	gaps := detector.DetectGaps(registry, nil, nil)

	for _, g := range gaps {
		if g.Type == GapTypeCorrelation {
			t.Errorf("expected no correlation gap with 5 agents, but got one")
		}
	}
}

func TestGapDetector_UpdateGapStatus(t *testing.T) {
	detector := NewGapDetector()
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sector_tech", Enabled: true, Layer: domain.LayerSector, Skill: "technology"},
		},
	}
	universe := []string{"2330.TW"}

	gaps := detector.DetectGaps(registry, nil, universe)
	if len(gaps) == 0 {
		t.Fatal("expected gaps to be detected")
	}

	// Update status of first gap
	gapID := gaps[0].ID
	detector.UpdateGapStatus(gapID, GapStatusSpawning)

	// Gap should no longer be open
	openGaps := detector.GetOpenGaps()
	for _, g := range openGaps {
		if g.ID == gapID {
			t.Errorf("gap %s should not be in open gaps after updating to spawning", gapID)
		}
	}

	// Update non-existent gap (should not panic)
	detector.UpdateGapStatus("nonexistent", GapStatusResolved)
}

func TestGapDetector_GetOpenGaps_Sorted(t *testing.T) {
	detector := NewGapDetector()
	// Directly inject gaps with different severities
	detector.gaps["a"] = &KnowledgeGap{ID: "a", Severity: GapSeverityLow, Status: GapStatusOpen}
	detector.gaps["b"] = &KnowledgeGap{ID: "b", Severity: GapSeverityCritical, Status: GapStatusOpen}
	detector.gaps["c"] = &KnowledgeGap{ID: "c", Severity: GapSeverityMedium, Status: GapStatusOpen}
	detector.gaps["d"] = &KnowledgeGap{ID: "d", Severity: GapSeverityHigh, Status: GapStatusOpen}
	detector.gaps["e"] = &KnowledgeGap{ID: "e", Severity: GapSeverityHigh, Status: GapStatusSpawning} // not open

	openGaps := detector.GetOpenGaps()
	if len(openGaps) != 4 {
		t.Fatalf("expected 4 open gaps, got %d", len(openGaps))
	}

	expectedOrder := []GapSeverity{GapSeverityCritical, GapSeverityHigh, GapSeverityMedium, GapSeverityLow}
	for i, gap := range openGaps {
		if gap.Severity != expectedOrder[i] {
			t.Errorf("position %d: expected severity %s, got %s (id=%s)", i, expectedOrder[i], gap.Severity, gap.ID)
		}
	}
}

func TestGapDetector_DetectGaps_FullIntegration(t *testing.T) {
	detector := NewGapDetector()
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sector_tech", Enabled: true, Layer: domain.LayerSector, Skill: "semiconductor"},
			{ID: "sector_finance", Enabled: true, Layer: domain.LayerSector, Skill: "financial"},
			{ID: "style_value", Enabled: true, Layer: domain.LayerStyle, Skill: "value"},
			{ID: "style_growth", Enabled: true, Layer: domain.LayerStyle, Skill: "growth"},
			{ID: "style_momentum", Enabled: true, Layer: domain.LayerStyle, Skill: "momentum"},
			{ID: "style_quality", Enabled: true, Layer: domain.LayerStyle, Skill: "quality"},
		},
	}
	scorecards := map[string]*domain.Scorecard{
		"sector_finance": {Observations: 40, SharpeLike: 0.1, MaxDrawdown: -0.05}, // low sharpe, single coverage → medium gap
	}

	gaps := detector.DetectGaps(registry, scorecards, nil)

	// Should detect: uncovered sectors (shipping, biotech, etc.), uncovered styles (contrarian, trend_following, mean_reversion),
	// possible weak finance coverage
	t.Logf("Detected %d gaps:", len(gaps))
	for _, g := range gaps {
		t.Logf("  type=%s sector=%s style=%s severity=%s", g.Type, g.Sector, g.Style, g.Severity)
	}

	if len(gaps) == 0 {
		t.Error("expected multiple gaps to be detected")
	}
}

func TestGapDetector_DetectGaps_DisabledAgentsIgnored(t *testing.T) {
	detector := NewGapDetector()
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sector_tech", Enabled: true, Layer: domain.LayerSector, Skill: "semiconductor"},
			{ID: "sector_finance_disabled", Enabled: false, Layer: domain.LayerSector, Skill: "financial"}, // disabled
		},
	}

	gaps := detector.DetectGaps(registry, nil, nil)

	// Finance sector should have a gap since the only finance agent is disabled
	hasFinanceGap := false
	for _, g := range gaps {
		if g.Sector == "financial" && g.Type == GapTypeSector {
			hasFinanceGap = true
			break
		}
	}
	if !hasFinanceGap {
		t.Error("expected finance sector gap since only finance agent is disabled")
	}
}

func TestGapDetector_GetOpenGaps_Empty(t *testing.T) {
	detector := NewGapDetector()
	gaps := detector.GetOpenGaps()
	if gaps == nil {
		t.Error("expected non-nil slice, got nil")
	}
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps, got %d", len(gaps))
	}
}

func TestGapDetector_DetectGaps_DeduplicatesStored(t *testing.T) {
	detector := NewGapDetector()
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{},
	}

	// First call stores gaps
	gaps1 := detector.DetectGaps(registry, nil, nil)
	// Second call should not re-store the same gaps (only new ones)
	gaps2 := detector.DetectGaps(registry, nil, nil)

	// Open gaps after second call should match first call
	openGaps := detector.GetOpenGaps()
	if len(openGaps) != len(gaps1) {
		t.Errorf("expected %d stored open gaps, got %d", len(gaps1), len(openGaps))
	}
	_ = gaps2 // gaps2 may be empty because no new gaps beyond stored ones
}
