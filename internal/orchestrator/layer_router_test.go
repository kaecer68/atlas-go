package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestLayerRouter(t *testing.T) {
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "macro-01", Layer: domain.LayerContext, Enabled: true},
			{ID: "semi-01", Layer: domain.LayerSector, Enabled: true},
			{ID: "growth-01", Layer: domain.LayerStyle, Enabled: true},
			{ID: "cro-01", Layer: domain.LayerControl, Enabled: true},
		},
	}
	router := DefaultLayerRouter{}

	ctx := router.GetContextAgents(registry)
	if len(ctx) != 1 || ctx[0].ID != "macro-01" {
		t.Errorf("GetContextAgents: got %d agents, expected 1 (macro-01)", len(ctx))
	}
	sector := router.GetSectorAgents(registry)
	if len(sector) != 1 || sector[0].ID != "semi-01" {
		t.Errorf("GetSectorAgents: got %d agents", len(sector))
	}
	style := router.GetStyleAgents(registry)
	if len(style) != 1 || style[0].ID != "growth-01" {
		t.Errorf("GetStyleAgents: got %d agents", len(style))
	}
	control := router.GetControlAgents(registry)
	if len(control) != 1 || control[0].ID != "cro-01" {
		t.Errorf("GetControlAgents: got %d agents", len(control))
	}
	filtered := router.FilterByLayer(registry, domain.LayerSector)
	if len(filtered) != 1 || filtered[0].ID != "semi-01" {
		t.Errorf("FilterByLayer sector: got %d agents", len(filtered))
	}
}
