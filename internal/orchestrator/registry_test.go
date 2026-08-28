package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/screener"
)

func TestSeedRegistryIsValid(t *testing.T) {
	reg := SeedRegistry()
	if err := ValidateRegistry(reg, ""); err != nil {
		t.Fatalf("registry validation failed: %v", err)
	}
	if len(reg.Agents) < 5 {
		t.Fatalf("expected multiple seeded agents")
	}
}

// TestLoadRegistryStockpickerWinrate verifies the stockpicker agent (PR 2d)
// is registered in configs/agents.json and loads through LoadRegistry with
// the expected id / layer / skill contract:
//   - id:       stockpicker-winrate-01 (existing <domain>-<descriptor>-<nn> pattern)
//   - layer:    style — the recommendation-producing layer set
//     (sector/style/superinvestor) that collectRecommendations and
//     Darwinian tracking iterate; a stock-level picker is a
//     cross-sectional selection style over the full quotes universe.
//   - skill:    stockpicker_winrate (new skill)
//   - prompt:   prompts/agents/stockpicker_winrate.md must exist on disk
func TestLoadRegistryStockpickerWinrate(t *testing.T) {
	reg, err := LoadRegistry("../../configs/agents.json")
	if err != nil {
		t.Fatalf("LoadRegistry(configs/agents.json): %v", err)
	}

	var spec *domain.AgentSpec
	for i := range reg.Agents {
		if reg.Agents[i].ID == "stockpicker-winrate-01" {
			spec = &reg.Agents[i]
			break
		}
	}
	if spec == nil {
		t.Fatal("stockpicker-winrate-01 not found in configs/agents.json")
	}
	if spec.Layer != domain.LayerStyle {
		t.Errorf("layer = %q, want %q (style — recommendation-producing layer)", spec.Layer, domain.LayerStyle)
	}
	if spec.Skill != "stockpicker_winrate" {
		t.Errorf("skill = %q, want %q", spec.Skill, "stockpicker_winrate")
	}
	if !spec.Enabled {
		t.Error("stockpicker-winrate-01 must be enabled")
	}
	if spec.PromptFile != "prompts/agents/stockpicker_winrate.md" {
		t.Errorf("promptFile = %q, want %q", spec.PromptFile, "prompts/agents/stockpicker_winrate.md")
	}
	if _, err := os.Stat(filepath.Join("..", "..", spec.PromptFile)); err != nil {
		t.Errorf("prompt file missing: %v", err)
	}
}

func TestPluginRegistryScreenDetailed(t *testing.T) {
	r := NewPluginRegistry().WithScreener(screener.NewEngine(nil, nil))
	minVol := int64(1000000)
	agent := domain.AgentSpec{
		ID: "test-agent",
		ScreeningCriteria: domain.ScreeningCriteria{
			VolumeIntraday: &domain.MinFilter{Min: &minVol},
		},
	}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Volume: 500000, IsTradable: true}}
	res, err := r.ScreenDetailed(context.Background(), agent, "2330.TW", quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion == "" {
		t.Fatal("expected criterion")
	}
}

func TestPluginRegistryScreenDetailedNoScreener(t *testing.T) {
	r := NewPluginRegistry()
	agent := domain.AgentSpec{
		ID: "test-agent",
		ScreeningCriteria: domain.ScreeningCriteria{
			VolumeIntraday: &domain.MinFilter{Min: int64Ptr(1000000)},
		},
	}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Volume: 500000, IsTradable: true}}
	res, err := r.ScreenDetailed(context.Background(), agent, "2330.TW", quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatal("expected pass when no screener attached")
	}
}

func int64Ptr(i int64) *int64 {
	return &i
}
