package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type mockAgentExecutor struct {
	id string
}

func (mockAgentExecutor) Supports(domain.AgentSpec) bool { return true }
func (mockAgentExecutor) Recommend(domain.AgentSpec, domain.Quote, string, domain.Regime, FactorQuery) (domain.Recommendation, bool) {
	return domain.Recommendation{}, false
}

type mockRegimeExecutor struct {
	id string
}

func (mockRegimeExecutor) Supports(domain.AgentSpec) bool                              { return true }
func (mockRegimeExecutor) Score(domain.AgentSpec, map[string]domain.Quote, string) int { return 0 }

type mockControlExecutor struct {
	id string
}

func (mockControlExecutor) Supports(domain.AgentSpec) bool { return true }
func (mockControlExecutor) Apply(domain.AgentSpec, []domain.Recommendation, domain.ExecutionPolicy, domain.Regime) []domain.Recommendation {
	return nil
}

// TestStaticLoader_RegisterAgent tests that custom agents are appended to the builtin list.
func TestStaticLoader_RegisterAgent(t *testing.T) {
	loader := &StaticLoader{}
	customAgent := mockAgentExecutor{id: "custom-agent-1"}

	loader.RegisterAgent(customAgent)

	agents, err := loader.LoadAgentExecutors()
	if err != nil {
		t.Fatalf("LoadAgentExecutors() error = %v", err)
	}

	// Should have all builtin agents + our custom one
	found := false
	for _, a := range agents {
		if _, ok := a.(mockAgentExecutor); ok {
			found = true
			break
		}
	}
	if !found {
		t.Error("custom agent not found in loaded agents")
	}
}

// TestStaticLoader_RegisterRegime tests that custom regime executors are appended.
func TestStaticLoader_RegisterRegime(t *testing.T) {
	loader := &StaticLoader{}
	customRegime := mockRegimeExecutor{id: "custom-regime-1"}

	loader.RegisterRegime(customRegime)

	regimes, err := loader.LoadRegimeExecutors()
	if err != nil {
		t.Fatalf("LoadRegimeExecutors() error = %v", err)
	}

	found := false
	for _, r := range regimes {
		if _, ok := r.(mockRegimeExecutor); ok {
			found = true
			break
		}
	}
	if !found {
		t.Error("custom regime not found in loaded regimes")
	}
}

// TestStaticLoader_RegisterControl tests that custom control executors are appended.
func TestStaticLoader_RegisterControl(t *testing.T) {
	loader := &StaticLoader{}
	customControl := mockControlExecutor{id: "custom-control-1"}

	loader.RegisterControl(customControl)

	controls, err := loader.LoadControlExecutors()
	if err != nil {
		t.Fatalf("LoadControlExecutors() error = %v", err)
	}

	found := false
	for _, c := range controls {
		if _, ok := c.(mockControlExecutor); ok {
			found = true
			break
		}
	}
	if !found {
		t.Error("custom control not found in loaded controls")
	}
}

// TestStaticLoader_MultipleRegistrations tests that multiple registrations accumulate.
func TestStaticLoader_MultipleRegistrations(t *testing.T) {
	loader := &StaticLoader{}

	loader.RegisterAgent(mockAgentExecutor{id: "agent-1"})
	loader.RegisterAgent(mockAgentExecutor{id: "agent-2"})
	loader.RegisterRegime(mockRegimeExecutor{id: "regime-1"})
	loader.RegisterControl(mockControlExecutor{id: "control-1"})

	agents, err := loader.LoadAgentExecutors()
	if err != nil {
		t.Fatalf("LoadAgentExecutors() error = %v", err)
	}
	customAgents := 0
	for _, a := range agents {
		if _, ok := a.(mockAgentExecutor); ok {
			customAgents++
		}
	}
	if customAgents != 2 {
		t.Errorf("expected 2 custom agents, got %d", customAgents)
	}

	regimes, err := loader.LoadRegimeExecutors()
	if err != nil {
		t.Fatalf("LoadRegimeExecutors() error = %v", err)
	}
	customRegimes := 0
	for _, r := range regimes {
		if _, ok := r.(mockRegimeExecutor); ok {
			customRegimes++
		}
	}
	if customRegimes != 1 {
		t.Errorf("expected 1 custom regime, got %d", customRegimes)
	}

	controls, err := loader.LoadControlExecutors()
	if err != nil {
		t.Fatalf("LoadControlExecutors() error = %v", err)
	}
	customControls := 0
	for _, c := range controls {
		if _, ok := c.(mockControlExecutor); ok {
			customControls++
		}
	}
	if customControls != 1 {
		t.Errorf("expected 1 custom control, got %d", customControls)
	}
}

// TestStaticLoader_ZeroValueWorks tests that zero-value StaticLoader produces builtins.
func TestStaticLoader_ZeroValueWorks(t *testing.T) {
	loader := StaticLoader{}

	agents, err := loader.LoadAgentExecutors()
	if err != nil {
		t.Fatalf("LoadAgentExecutors() error = %v", err)
	}
	if len(agents) == 0 {
		t.Error("expected non-empty agent list from zero-value loader")
	}

	regimes, err := loader.LoadRegimeExecutors()
	if err != nil {
		t.Fatalf("LoadRegimeExecutors() error = %v", err)
	}
	if len(regimes) == 0 {
		t.Error("expected non-empty regime list from zero-value loader")
	}

	controls, err := loader.LoadControlExecutors()
	if err != nil {
		t.Fatalf("LoadControlExecutors() error = %v", err)
	}
	if len(controls) == 0 {
		t.Error("expected non-empty control list from zero-value loader")
	}
}

// TestStaticLoader_BuiltinsNotDuplicated tests that builtins are not duplicated when registering.
func TestStaticLoader_BuiltinsNotDuplicated(t *testing.T) {
	loader := &StaticLoader{}

	// Register one custom agent
	loader.RegisterAgent(mockAgentExecutor{id: "custom-agent"})

	agents, err := loader.LoadAgentExecutors()
	if err != nil {
		t.Fatalf("LoadAgentExecutors() error = %v", err)
	}

	// Count non-builtin agents
	customCount := 0
	for _, a := range agents {
		if _, ok := a.(mockAgentExecutor); ok {
			customCount++
		}
	}

	if customCount != 1 {
		t.Errorf("expected exactly 1 custom agent, got %d", customCount)
	}
}
