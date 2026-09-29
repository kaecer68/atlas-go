package portfolio

// darwinian_weights_unregistered_warn_test.go — issue #1944 G3 / FU-20260929-08.
//
// recordOutcome() used to drop an outcome silently when the agent ID was not in
// the weights map: no line, no counter, nothing. That is indistinguishable from
// "no outcome ever arrived", so an audit looking for where an agent's signal went
// had no evidence at all. This pins the replacement — a warning, exactly once per
// (manager, agent ID) — and pins that adding it changed no computation.

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const unregisteredAgentEvent = "outcome_for_unregistered_agent"

// captureDarwinianLogs swaps the global logger for the duration of the test.
func captureDarwinianLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logging.SetLogger(prev) })
	return buf
}

// managerWithAgent builds a manager whose weights map contains exactly the given
// agent, using the production seeding path.
func managerWithAgent(t *testing.T, agentID string) *DarwinianWeightManager {
	t.Helper()
	m := NewDarwinianWeightManager(t.TempDir() + "/darwinian_weights.json")
	m.InitializeFromRegistry(domain.AgentRegistry{Agents: []domain.AgentSpec{{
		ID:      agentID,
		Skill:   "ai_supply_chain_desk",
		Layer:   domain.LayerSector,
		Enabled: true,
	}}})
	if _, ok := m.GetAgentWeightData(agentID); !ok {
		t.Fatalf("fixture: agent %q was not seeded", agentID)
	}
	return m
}

// TestRecordOutcome_UnregisteredAgentWarnsOnce pins the warning AND its bound.
func TestRecordOutcome_UnregisteredAgentWarnsOnce(t *testing.T) {
	buf := captureDarwinianLogs(t)
	m := managerWithAgent(t, "ai-desk-01")

	for range 5 { // five outcomes for an ID nobody registered
		m.RecordOutcome("ghost-desk-01", 0.01, true)
	}

	logs := buf.String()
	if n := strings.Count(logs, unregisteredAgentEvent); n != 1 {
		t.Fatalf("warning lines = %d, want exactly 1 for five outcomes (throttled per agent)\n--- log ---\n%s", n, logs)
	}
	if !strings.Contains(logs, "ghost-desk-01") {
		t.Errorf("the warning must name the offending agent id\n--- log ---\n%s", logs)
	}
	// The drop itself is unchanged: no entry may be created for the unknown ID.
	if _, ok := m.GetAgentWeightData("ghost-desk-01"); ok {
		t.Error("an unregistered agent must not be added to the weights map by the warning path")
	}
}

// TestRecordOutcome_ThrottleIsPerAgentNotPerProcess proves the bound is per agent
// ID: a second unknown ID gets its own line.
func TestRecordOutcome_ThrottleIsPerAgentNotPerProcess(t *testing.T) {
	buf := captureDarwinianLogs(t)
	m := managerWithAgent(t, "ai-desk-01")

	m.RecordOutcome("ghost-a", 0.01, true)
	m.RecordOutcome("ghost-a", 0.01, true)
	m.RecordOutcome("ghost-b", 0.01, true)

	logs := buf.String()
	if n := strings.Count(logs, unregisteredAgentEvent); n != 2 {
		t.Fatalf("warning lines = %d, want 2 (one per distinct unknown agent)\n--- log ---\n%s", n, logs)
	}
	for _, want := range []string{"ghost-a", "ghost-b"} {
		if !strings.Contains(logs, want) {
			t.Errorf("the warning must name %q", want)
		}
	}
}

// TestRecordOutcome_RegisteredAgentDoesNotWarn is the negative half: no warning
// may appear for the normal path, otherwise the signal is worthless.
func TestRecordOutcome_RegisteredAgentDoesNotWarn(t *testing.T) {
	buf := captureDarwinianLogs(t)
	m := managerWithAgent(t, "ai-desk-01")

	m.RecordOutcome("ai-desk-01", 0.02, true)
	m.RecordOutcome("ai-desk-01", -0.01, false)

	if logs := buf.String(); strings.Contains(logs, unregisteredAgentEvent) {
		t.Fatalf("a registered agent must not warn\n--- log ---\n%s", logs)
	}
}

// TestRecordOutcome_ComputationUnchanged pins the arithmetic by VALUE, not by
// calling the code twice: the expected numbers come from the documented behavior
// (one increment per outcome, hit rate = wins/total, EMA with Darwinian.EMAAlpha),
// so an accidental edit inside the touched branch shows up here immediately.
func TestRecordOutcome_ComputationUnchanged(t *testing.T) {
	m := managerWithAgent(t, "ai-desk-01")
	m.RecordOutcome("ai-desk-01", 0.05, true)   // first outcome: AvgReturn = return
	m.RecordOutcome("ai-desk-01", -0.03, false) // EMA: alpha*(-0.03) + (1-alpha)*0.05

	w, ok := m.GetAgentWeightData("ai-desk-01")
	if !ok {
		t.Fatal("agent record disappeared")
	}
	if w.TotalSignals != 2 {
		t.Errorf("TotalSignals = %d, want 2", w.TotalSignals)
	}
	if w.WinCount != 1 || w.LossCount != 1 {
		t.Errorf("WinCount/LossCount = %d/%d, want 1/1", w.WinCount, w.LossCount)
	}
	if w.HitRate != 0.5 {
		t.Errorf("HitRate = %v, want 0.5", w.HitRate)
	}
	alpha := m.params.Darwinian.EMAAlpha
	wantAvg := alpha*(-0.03) + (1-alpha)*0.05
	if diff := w.AvgReturn - wantAvg; diff > 1e-12 || diff < -1e-12 {
		t.Errorf("AvgReturn = %v, want %v (EMA alpha=%v)", w.AvgReturn, wantAvg, alpha)
	}
	// And the unknown-ID path must not have altered the population either.
	if got := len(m.GetAllAgentWeightData()); got != 1 {
		t.Errorf("weights map size = %d, want 1", got)
	}
}
