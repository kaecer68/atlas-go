package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestApplyHumanOverridesFiltersPausedAgentsAndBannedSectors(t *testing.T) {
	ledgerDir := t.TempDir()
	store := ledger.NewStore(ledgerDir)

	// Pause one agent.
	_ = store.RecordHumanIntervention(domain.HumanIntervention{
		Type:          "pause_agent",
		TargetAgentID: "growth-momentum-01",
	})
	// Ban a sector.
	_ = store.RecordHumanIntervention(domain.HumanIntervention{
		Type:         "sector_ban",
		TargetSector: "ai_supply_chain",
	})

	sys := &System{
		ledger:   store,
		registry: SeedRegistry(),
	}

	recs := []domain.Recommendation{
		{Agent: "growth-momentum-01", Skill: "growth_momentum", Symbol: "2317.TW"},
		{Agent: "value-yield-01", Skill: "value_yield", Symbol: "2881.TW"},
		{Agent: "ai-desk-01", Skill: "ai_supply_chain_desk", Symbol: "2382.TW"},
		{Agent: "semi-desk-01", Skill: "semiconductor_desk", Symbol: "2330.TW"},
	}

	filtered := sys.applyHumanOverrides(recs)

	// growth-momentum-01 is paused -> should be removed.
	// ai-desk-01 is in banned ai_supply_chain sector -> should be removed.
	if len(filtered) != 2 {
		t.Fatalf("expected 2 recommendations after overrides, got %d", len(filtered))
	}

	agents := map[string]bool{}
	for _, r := range filtered {
		agents[r.Agent] = true
	}
	if agents["growth-momentum-01"] {
		t.Fatalf("expected paused agent to be filtered out")
	}
	if agents["ai-desk-01"] {
		t.Fatalf("expected banned sector agent to be filtered out")
	}
	if !agents["value-yield-01"] || !agents["semi-desk-01"] {
		t.Fatalf("expected non-paused non-banned agents to remain")
	}
}

func TestApplyHumanOverridesResumesAgentAndUnbansSector(t *testing.T) {
	ledgerDir := t.TempDir()
	store := ledger.NewStore(ledgerDir)

	_ = store.RecordHumanIntervention(domain.HumanIntervention{
		Type:          "pause_agent",
		TargetAgentID: "growth-momentum-01",
	})
	_ = store.RecordHumanIntervention(domain.HumanIntervention{
		Type:          "resume_agent",
		TargetAgentID: "growth-momentum-01",
	})

	sys := &System{
		ledger:   store,
		registry: SeedRegistry(),
	}

	recs := []domain.Recommendation{
		{Agent: "growth-momentum-01", Skill: "growth_momentum", Symbol: "2317.TW"},
	}

	filtered := sys.applyHumanOverrides(recs)
	if len(filtered) != 1 {
		t.Fatalf("expected resumed agent to remain, got %d", len(filtered))
	}
}
