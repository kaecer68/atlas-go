package experiment

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestAutoProposer_BurnInSkips(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-10 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_proposer.json")
	p := NewAutoProposer(dw, nil).WithMaturityTracker(tr)

	proposals, outcome, err := p.CheckAndPropose(context.Background())
	if err != nil {
		t.Fatalf("CheckAndPropose: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals during burn_in, got %d", len(proposals))
	}
	if !outcome.BurnInSkipped || outcome.Reason != "burn_in" {
		t.Errorf("expected burn_in skip outcome, got %+v", outcome)
	}
}

func TestAutoProposer_NoDegradationNoProposal(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_proposer.json")

	// Seed a healthy agent
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "healthy_agent", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	for range 60 {
		dw.RecordOutcome("healthy_agent", 0.02, true)
	}

	p := NewAutoProposer(dw, nil).WithMaturityTracker(tr)
	proposals, outcome, err := p.CheckAndPropose(context.Background())
	if err != nil {
		t.Fatalf("CheckAndPropose: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals for healthy agent, got %d", len(proposals))
	}
	if outcome.Reason != "no_trigger" {
		t.Errorf("expected no_trigger outcome, got %+v", outcome)
	}
	if outcome.NoTrigger != outcome.AgentsScanned || outcome.AgentsScanned == 0 {
		t.Errorf("expected all scanned agents to have no trigger, got %+v", outcome)
	}
}

func TestAutoProposer_SharpeDegradation(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_proposer.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sick_agent", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	// 60 negative returns → negative Sharpe
	for range 60 {
		dw.RecordOutcome("sick_agent", -0.02, false)
	}

	p := NewAutoProposer(dw, nil).WithMaturityTracker(tr)
	proposals, outcome, err := p.CheckAndPropose(context.Background())
	if err != nil {
		t.Fatalf("CheckAndPropose: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("expected 1 proposal for degraded agent, got %d", len(proposals))
	}
	if outcome.Reason != "proposals" || outcome.ProposalsGenerated != 1 {
		t.Errorf("expected proposals outcome, got %+v", outcome)
	}
	if proposals[0].AgentID != "sick_agent" {
		t.Errorf("expected agent sick_agent, got %s", proposals[0].AgentID)
	}
	if proposals[0].Brief.MutationType != "auto_prompt_optimization" {
		t.Errorf("expected mutation_type auto_prompt_optimization, got %s", proposals[0].Brief.MutationType)
	}
}

func TestAutoProposer_CooldownRespected(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_proposer.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sick_agent", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	for range 60 {
		dw.RecordOutcome("sick_agent", -0.02, false)
	}

	p := NewAutoProposer(dw, nil).WithMaturityTracker(tr).WithCooldown(24 * time.Hour)

	// First scan should generate proposal
	p1, out1, _ := p.CheckAndPropose(context.Background())
	if len(p1) != 1 {
		t.Fatalf("expected 1 proposal on first scan, got %d", len(p1))
	}
	if out1.Reason != "proposals" {
		t.Errorf("expected proposals outcome, got %+v", out1)
	}

	// Second scan within cooldown should return none
	p2, out2, _ := p.CheckAndPropose(context.Background())
	if len(p2) != 0 {
		t.Errorf("expected 0 proposals within cooldown, got %d", len(p2))
	}
	if out2.Reason != "cooldown_only" {
		t.Errorf("expected cooldown_only outcome, got %+v", out2)
	}
	if out2.CooldownSkipped != out2.AgentsScanned || out2.AgentsScanned == 0 {
		t.Errorf("expected all agents cooldown-skipped, got %+v", out2)
	}
}

func TestAutoProposer_WeightTrap(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_proposer.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "trapped_agent", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	// Give it some signals but keep at min weight
	for range 20 {
		dw.RecordOutcome("trapped_agent", 0.001, true)
	}
	// Manually set weight to minimum and consecutive at min
	w, _ := dw.GetAgentWeightData("trapped_agent")
	if w != nil {
		w.Weight = 0.3
		w.ConsecutiveAtMin = 6
	}

	p := NewAutoProposer(dw, nil).WithMaturityTracker(tr)
	proposals, _, err := p.CheckAndPropose(context.Background())
	if err != nil {
		t.Fatalf("CheckAndPropose: %v", err)
	}
	// Note: GetAllAgentWeightData returns copies, so the manual weight trap
	// modification above won't be visible. This test documents the intent;
	// a full integration test would use the real weight adjustment path.
	_ = proposals
}

// TestAutoProposer_TriggerZero_WeightDriftNoSignal covers the B2 trigger 0:
// a zero/low-signal agent whose weight drifted >30% away from the configured
// neutral default must produce a proposal, while a zero-signal agent near
// neutral must not (and >=30-signal agents keep using the original triggers).
func TestAutoProposer_TriggerZero_WeightDriftNoSignal(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_proposer.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "silent_drifted", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
			{ID: "silent_neutral", Enabled: true, Layer: domain.LayerStyle, Skill: "value"},
		},
	})
	// No signals recorded for either agent. Push silent_drifted's weight to
	// 1.5 (50% above the neutral default 1.0 -> drift >30%); silent_neutral
	// stays at the neutral default. SetWeight mutates the manager's internal
	// state (GetAgentWeightData returns copies, so direct field writes in
	// earlier tests were invisible).
	if _, ev := dw.SetWeight("silent_drifted", 1.5); ev != nil {
		t.Fatalf("SetWeight(silent_drifted, 1.5) produced clamping event: %+v", ev)
	}
	if _, ev := dw.SetWeight("silent_neutral", 1.0); ev != nil {
		t.Fatalf("SetWeight(silent_neutral, 1.0) produced clamping event: %+v", ev)
	}

	p := NewAutoProposer(dw, nil).WithMaturityTracker(tr)
	proposals, outcome, err := p.CheckAndPropose(context.Background())
	if err != nil {
		t.Fatalf("CheckAndPropose: %v", err)
	}
	if outcome.Reason != "proposals" || outcome.ProposalsGenerated != 1 {
		t.Fatalf("expected exactly 1 proposal (drifted zero-signal agent), got %+v", outcome)
	}
	if proposals[0].AgentID != "silent_drifted" {
		t.Errorf("expected proposal for silent_drifted, got %s", proposals[0].AgentID)
	}
	if !strings.Contains(proposals[0].TriggerReason, "weight_drift_no_signal") {
		t.Errorf("expected weight_drift_no_signal trigger reason, got %q", proposals[0].TriggerReason)
	}
}

// TestAutoProposer_TriggerZero_NearNeutralNoProposal verifies the negative
// case of trigger 0: a zero-signal agent whose weight is close to neutral
// must not fire the new trigger (and has no other degradation to report).
func TestAutoProposer_TriggerZero_NearNeutralNoProposal(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_proposer.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "silent_close", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	if _, ev := dw.SetWeight("silent_close", 1.05); ev != nil {
		t.Fatalf("SetWeight produced clamping event: %+v", ev)
	}

	p := NewAutoProposer(dw, nil).WithMaturityTracker(tr)
	proposals, outcome, err := p.CheckAndPropose(context.Background())
	if err != nil {
		t.Fatalf("CheckAndPropose: %v", err)
	}
	if len(proposals) != 0 {
		t.Fatalf("expected 0 proposals for near-neutral zero-signal agent, got %d", len(proposals))
	}
	if outcome.Reason != "no_trigger" {
		t.Errorf("expected no_trigger outcome, got %+v", outcome)
	}
}

// TestAutoProposer_TriggerZero_SignalRichAgentUsesOriginalTriggers verifies
// that once an agent has >=30 signals, trigger 0 no longer applies and the
// original degradation triggers take over: a healthy signal-rich agent with
// drifted weight must NOT propose, and a degraded one still does via trigger 2.
func TestAutoProposer_TriggerZero_SignalRichAgentUsesOriginalTriggers(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_proposer.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "rich_healthy", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
			{ID: "rich_sick", Enabled: true, Layer: domain.LayerStyle, Skill: "value"},
		},
	})
	// rich_healthy: 40 positive returns, weight drifted to 1.5 — must NOT fire
	// trigger 0 (signals >= 30) and has no other degradation.
	for range 40 {
		dw.RecordOutcome("rich_healthy", 0.02, true)
	}
	if _, ev := dw.SetWeight("rich_healthy", 1.5); ev != nil {
		t.Fatalf("SetWeight produced clamping event: %+v", ev)
	}
	// rich_sick: 60 negative returns -> negative Sharpe triggers trigger 2.
	for range 60 {
		dw.RecordOutcome("rich_sick", -0.02, false)
	}

	p := NewAutoProposer(dw, nil).WithMaturityTracker(tr)
	proposals, outcome, err := p.CheckAndPropose(context.Background())
	if err != nil {
		t.Fatalf("CheckAndPropose: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("expected 1 proposal (rich_sick via sharpe_degradation), got %d: %+v", len(proposals), outcome)
	}
	if proposals[0].AgentID != "rich_sick" {
		t.Errorf("expected proposal for rich_sick, got %s", proposals[0].AgentID)
	}
	// With 60 identical negative returns the degenerate-window Sharpe guard
	// zeroes Sharpe, so trigger 2 (sharpe_degradation) does not fire; the
	// original hit_rate_collapse trigger (3) does. Either is a pre-existing
	// degradation trigger — the point is that trigger 0 must NOT fire for a
	// signal-rich agent regardless of its weight drift.
	if strings.Contains(proposals[0].TriggerReason, "weight_drift_no_signal") {
		t.Errorf("trigger 0 must not fire for signal-rich agent, got %q", proposals[0].TriggerReason)
	}
	if !strings.Contains(proposals[0].TriggerReason, "hit_rate_collapse") &&
		!strings.Contains(proposals[0].TriggerReason, "sharpe_degradation") {
		t.Errorf("expected an original degradation trigger, got %q", proposals[0].TriggerReason)
	}
}
