package spawning

import (
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestAgentFactory_GenerateAgentID(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())

	tests := []struct {
		name     string
		gap      *KnowledgeGap
		contains string
	}{
		{
			name:     "sector gap",
			gap:      &KnowledgeGap{Type: GapTypeSector, Sector: "biotech"},
			contains: "spawn_biotech_",
		},
		{
			name:     "style gap",
			gap:      &KnowledgeGap{Type: GapTypeStyle, Style: "momentum"},
			contains: "spawn_momentum_",
		},
		{
			name:     "regime gap",
			gap:      &KnowledgeGap{Type: GapTypeRegime},
			contains: "spawn_regime_",
		},
		{
			name:     "correlation gap",
			gap:      &KnowledgeGap{Type: GapTypeCorrelation, Sector: "semiconductor"},
			contains: "spawn_alt_semiconductor_",
		},
		{
			name:     "default/unknown gap",
			gap:      &KnowledgeGap{Type: GapTypeMarketCap},
			contains: "spawn_auto_",
		},
	}

	for _, tt := range tests {
		got := factory.generateAgentID(tt.gap)
		if got == "" {
			t.Errorf("%s: expected non-empty ID", tt.name)
		}
		if len(got) < len(tt.contains) || got[:len(tt.contains)] != tt.contains {
			t.Errorf("%s: expected ID to start with %q, got %q", tt.name, tt.contains, got)
		}
	}
}

func TestAgentFactory_DetermineLayer(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())

	tests := []struct {
		name  string
		gap   *KnowledgeGap
		layer domain.AgentLayer
	}{
		{"sector", &KnowledgeGap{Type: GapTypeSector}, domain.LayerSector},
		{"style", &KnowledgeGap{Type: GapTypeStyle}, domain.LayerStyle},
		{"regime", &KnowledgeGap{Type: GapTypeRegime}, domain.LayerStyle},
		{"correlation", &KnowledgeGap{Type: GapTypeCorrelation}, domain.LayerStyle},
		{"default", &KnowledgeGap{Type: GapTypeSymbol}, domain.LayerStyle},
	}

	for _, tt := range tests {
		got := factory.determineLayer(tt.gap)
		if got != tt.layer {
			t.Errorf("%s: determineLayer() = %q, want %q", tt.name, got, tt.layer)
		}
	}
}

func TestAgentFactory_GenerateSkill(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())

	tests := []struct {
		name  string
		gap   *KnowledgeGap
		skill string
	}{
		{"sector", &KnowledgeGap{Type: GapTypeSector, Sector: "biotech"}, "sector_biotech_specialist"},
		{"style", &KnowledgeGap{Type: GapTypeStyle, Style: "momentum"}, "style_momentum"},
		{"regime", &KnowledgeGap{Type: GapTypeRegime}, "regime_specialist"},
		{"correlation", &KnowledgeGap{Type: GapTypeCorrelation, Sector: "semiconductor"}, "alternative_semiconductor"},
		{"default", &KnowledgeGap{Type: GapTypeMarketCap}, "adaptive_specialist"},
	}

	for _, tt := range tests {
		got := factory.generateSkill(tt.gap)
		if got != tt.skill {
			t.Errorf("%s: generateSkill() = %q, want %q", tt.name, got, tt.skill)
		}
	}
}

func TestAgentFactory_GenerateName(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())

	tests := []struct {
		name     string
		gap      *KnowledgeGap
		expected string
	}{
		{"sector", &KnowledgeGap{Type: GapTypeSector, Sector: "biotech"}, "Biotech Specialist (Auto)"},
		{"style", &KnowledgeGap{Type: GapTypeStyle, Style: "momentum"}, "Momentum Style Agent (Auto)"},
		{"regime", &KnowledgeGap{Type: GapTypeRegime}, "Regime Adaptive Agent (Auto)"},
	}

	for _, tt := range tests {
		got := factory.generateName(tt.gap)
		if got != tt.expected {
			t.Errorf("%s: generateName() = %q, want %q", tt.name, got, tt.expected)
		}
	}
}

func TestAgentFactory_DetermineUniverse(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())

	tests := []struct {
		name      string
		gap       *KnowledgeGap
		wantEmpty bool
		contains  string
	}{
		{"semiconductor", &KnowledgeGap{Sector: "semiconductor"}, false, "2330.TW"},
		{"electronics", &KnowledgeGap{Sector: "electronics"}, false, "2317.TW"},
		{"financial", &KnowledgeGap{Sector: "financial"}, false, "2881.TW"},
		{"shipping", &KnowledgeGap{Sector: "shipping"}, false, "2603.TW"},
		{"biotech", &KnowledgeGap{Sector: "biotech"}, false, "6456.TW"},
		{"automotive", &KnowledgeGap{Sector: "automotive"}, false, "2207.TW"},
		{"unknown sector", &KnowledgeGap{Sector: "aerospace"}, true, ""},
		{"empty sector", &KnowledgeGap{Sector: ""}, true, ""},
	}

	for _, tt := range tests {
		got := factory.determineUniverse(tt.gap)
		if tt.wantEmpty && len(got) != 0 {
			t.Errorf("%s: expected empty universe, got %d symbols", tt.name, len(got))
		}
		if !tt.wantEmpty && len(got) == 0 {
			t.Errorf("%s: expected non-empty universe, got empty", tt.name)
		}
		if tt.contains != "" {
			found := slices.Contains(got, tt.contains)
			if !found {
				t.Errorf("%s: expected universe to contain %s, got %v", tt.name, tt.contains, got)
			}
		}
	}
}

func TestAgentFactory_DetermineMetrics(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())

	tests := []struct {
		name string
		gap  *KnowledgeGap
		len  int
	}{
		{"sector", &KnowledgeGap{Type: GapTypeSector}, 3},
		{"value style", &KnowledgeGap{Type: GapTypeStyle, Style: "value"}, 3},
		{"growth style", &KnowledgeGap{Type: GapTypeStyle, Style: "growth"}, 3},
		{"momentum style", &KnowledgeGap{Type: GapTypeStyle, Style: "momentum"}, 3},
		{"unknown style", &KnowledgeGap{Type: GapTypeStyle, Style: "unknown"}, 2},
		{"regime", &KnowledgeGap{Type: GapTypeRegime}, 3},
		{"default", &KnowledgeGap{Type: GapTypeMarketCap}, 3},
	}

	for _, tt := range tests {
		got := factory.determineMetrics(tt.gap)
		if len(got) != tt.len {
			t.Errorf("%s: expected %d metrics, got %d: %v", tt.name, tt.len, len(got), got)
		}
	}
}

func TestAgentFactory_CreateAgentForGap_StyleGap(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	gap := &KnowledgeGap{
		ID:       "gap-style-value-1",
		Type:     GapTypeStyle,
		Style:    "value",
		Severity: GapSeverityMedium,
	}

	agent, prompt := factory.CreateAgentForGap(gap, "")

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.Layer != domain.LayerStyle {
		t.Errorf("expected style layer, got %s", agent.Layer)
	}
	if agent.Skill != "style_value" {
		t.Errorf("expected skill 'style_value', got %s", agent.Skill)
	}
	if prompt == "" {
		t.Error("expected non-empty prompt content")
	}
	if agent.Enabled {
		t.Error("new spawned agent should start disabled")
	}
}

func TestAgentFactory_CreateAgentForGap_RegimeGap(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	gap := &KnowledgeGap{
		ID:       "gap-regime-1",
		Type:     GapTypeRegime,
		Severity: GapSeverityHigh,
	}

	agent, prompt := factory.CreateAgentForGap(gap, "")

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.Skill != "regime_specialist" {
		t.Errorf("expected skill 'regime_specialist', got %s", agent.Skill)
	}
	if prompt == "" {
		t.Error("expected non-empty prompt content")
	}
}

func TestAgentFactory_CreateAgentForGap_CorrelationGap(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	gap := &KnowledgeGap{
		ID:       "gap-correlation-1",
		Type:     GapTypeCorrelation,
		Sector:   "semiconductor",
		Severity: GapSeverityLow,
	}

	agent, prompt := factory.CreateAgentForGap(gap, "")

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.Skill != "alternative_semiconductor" {
		t.Errorf("expected skill 'alternative_semiconductor', got %s", agent.Skill)
	}
	if prompt == "" {
		t.Error("expected non-empty prompt content")
	}
}

func TestAgentFactory_CloneAgentWithVariation_AllTypes(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	parent := domain.AgentSpec{
		ID:    "parent_001",
		Name:  "Parent Agent",
		Layer: domain.LayerSector,
		Skill: "tech_sector",
	}

	variations := []string{"conservative", "aggressive", "contrarian", "technical", "fundamental"}
	for _, vt := range variations {
		clone, prompt := factory.CloneAgentWithVariation(parent, vt)
		if clone == nil {
			t.Errorf("%s: expected non-nil clone", vt)
			continue
		}
		if clone.Layer != parent.Layer {
			t.Errorf("%s: clone should inherit parent layer, got %s", vt, clone.Layer)
		}
		if clone.DarwinianWeight != 1.0 {
			t.Errorf("%s: clone should have neutral weight 1.0, got %f", vt, clone.DarwinianWeight)
		}
		if clone.Enabled {
			t.Errorf("%s: clone should start as disabled", vt)
		}
		if prompt == "" {
			t.Errorf("%s: expected non-empty prompt", vt)
		}
	}
}

func TestAgentFactory_CloneAgentWithVariation_UnknownType(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	parent := domain.AgentSpec{
		ID:    "parent_001",
		Name:  "Parent Agent",
		Layer: domain.LayerSector,
		Skill: "tech_sector",
	}

	clone, prompt := factory.CloneAgentWithVariation(parent, "unknown")
	if clone == nil {
		t.Fatal("expected non-nil clone even for unknown type")
	}
	if prompt == "" {
		t.Error("expected non-empty prompt for unknown type")
	}
}

func TestAgentFactory_CloneAgentWithVariation_InheritsFields(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	parent := domain.AgentSpec{
		ID:               "parent_001",
		Name:             "Parent Agent",
		Layer:            domain.LayerSector,
		Skill:            "tech_sector",
		Universe:         []string{"2330.TW", "2454.TW"},
		PrimaryMetrics:   []string{"pe_ratio", "pb_ratio"},
		RequiredSkills:   []string{"finance"},
		ForbiddenActions: []string{"short"},
	}

	clone, _ := factory.CloneAgentWithVariation(parent, "conservative")

	if len(clone.Universe) != 2 {
		t.Errorf("expected to inherit universe, got %d items", len(clone.Universe))
	}
	if len(clone.PrimaryMetrics) != 2 {
		t.Errorf("expected to inherit primary metrics, got %d items", len(clone.PrimaryMetrics))
	}
	if len(clone.RequiredSkills) != 1 || clone.RequiredSkills[0] != "finance" {
		t.Errorf("expected to inherit required skills, got %v", clone.RequiredSkills)
	}
	if len(clone.ForbiddenActions) != 1 || clone.ForbiddenActions[0] != "short" {
		t.Errorf("expected to inherit forbidden actions, got %v", clone.ForbiddenActions)
	}
}

func TestAgentFactory_CounterIncrements(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	gap := &KnowledgeGap{Type: GapTypeSector, Sector: "tech", Severity: GapSeverityMedium}

	agent1, _ := factory.CreateAgentForGap(gap, "")
	agent2, _ := factory.CreateAgentForGap(gap, "")

	if agent1.ID == agent2.ID {
		t.Errorf("expected unique IDs for sequential spawns, got duplicate %s", agent1.ID)
	}
}

func TestDefaultPromptTemplate(t *testing.T) {
	template := defaultPromptTemplate()
	if template == "" {
		t.Error("expected non-empty default prompt template")
	}
	if len(template) < 100 {
		t.Errorf("expected substantial template, got %d chars", len(template))
	}
}

func TestAgentFactory_GeneratePromptContent(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	gap := &KnowledgeGap{
		ID:          "gap-test-1",
		Type:        GapTypeSector,
		Sector:      "semiconductor",
		Style:       "",
		Severity:    GapSeverityHigh,
		Description: "No coverage for semiconductor sector",
	}

	content := factory.generatePromptContent(gap, "agent_001")

	if content == "" {
		t.Error("expected non-empty prompt content")
	}
	if len(content) < 200 {
		t.Errorf("expected substantial prompt content, got %d chars", len(content))
	}
}

func TestAgentFactory_GenerateSpecialization(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())

	tests := []struct {
		name string
		gap  *KnowledgeGap
	}{
		{"sector", &KnowledgeGap{Type: GapTypeSector, Sector: "semiconductor"}},
		{"style", &KnowledgeGap{Type: GapTypeStyle, Style: "value"}},
		{"regime", &KnowledgeGap{Type: GapTypeRegime}},
		{"default", &KnowledgeGap{Type: GapTypeMarketCap}},
	}

	for _, tt := range tests {
		got := factory.generateSpecialization(tt.gap)
		if got == "" {
			t.Errorf("%s: expected non-empty specialization", tt.name)
		}
	}
}

func TestAgentFactory_GenerateCollaborationNotes(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())

	tests := []struct {
		name string
		gap  *KnowledgeGap
	}{
		{"no sector/style", &KnowledgeGap{Sector: "", Style: ""}},
		{"with sector", &KnowledgeGap{Sector: "semiconductor", Style: ""}},
		{"with style", &KnowledgeGap{Sector: "", Style: "value"}},
		{"both", &KnowledgeGap{Sector: "biotech", Style: "momentum"}},
	}

	for _, tt := range tests {
		got := factory.generateCollaborationNotes(tt.gap)
		if got == "" {
			t.Errorf("%s: expected non-empty collaboration notes", tt.name)
		}
	}
}

func TestAgentFactory_GenerateVariationPrompt(t *testing.T) {
	factory := NewAgentFactory(t.TempDir())
	parent := domain.AgentSpec{ID: "parent_001", Name: "Parent Agent"}

	tests := []struct {
		variation string
	}{
		{"conservative"},
		{"aggressive"},
		{"contrarian"},
		{"technical"},
		{"fundamental"},
		{"unknown_type"},
	}

	for _, tt := range tests {
		got := factory.generateVariationPrompt(parent, tt.variation, "clone_001")
		if got == "" {
			t.Errorf("%s: expected non-empty variation prompt", tt.variation)
		}
	}
}
