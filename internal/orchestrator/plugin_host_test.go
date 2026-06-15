package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// mockPlugin implements the Plugin interface for testing.
type mockPlugin struct {
	name             string
	attachCalled     int
	processRecCalled int
	postSimCalled    int
	recsToReturn     []domain.Recommendation
	attachRegistry   ServiceRegistry
}

func (m *mockPlugin) Name() string { return m.name }

func (m *mockPlugin) Attach(registry ServiceRegistry) {
	m.attachCalled++
	m.attachRegistry = registry
}

func (m *mockPlugin) ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	m.processRecCalled++
	if m.recsToReturn != nil {
		return m.recsToReturn
	}
	return recs
}

func (m *mockPlugin) PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time) {
	m.postSimCalled++
}

// TestPluginHost_Register tests that plugins are registered and attached.
func TestPluginHost_Register(t *testing.T) {
	host := &PluginHost{}
	plugin := &mockPlugin{name: "test-plugin"}

	core := &SystemCore{}
	host.Register(plugin, core)

	if len(host.plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(host.plugins))
	}
	if plugin.attachCalled != 1 {
		t.Errorf("expected Attach called once, got %d", plugin.attachCalled)
	}
	if plugin.attachRegistry != core {
		t.Error("Attach was called with wrong registry")
	}
}

// TestPluginHost_RegisterNilCore tests that Register works with nil core.
func TestPluginHost_RegisterNilCore(t *testing.T) {
	host := &PluginHost{}
	plugin := &mockPlugin{name: "test-plugin"}

	host.Register(plugin, nil)

	if len(host.plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(host.plugins))
	}
	if plugin.attachCalled != 0 {
		t.Errorf("expected Attach called 0 times with nil core, got %d", plugin.attachCalled)
	}
}

// TestPluginHost_AttachAll tests that AttachAll calls Attach on all plugins.
func TestPluginHost_AttachAll(t *testing.T) {
	host := &PluginHost{}
	plugin1 := &mockPlugin{name: "plugin-1"}
	plugin2 := &mockPlugin{name: "plugin-2"}

	host.Register(plugin1, nil)
	host.Register(plugin2, nil)

	core := &SystemCore{}
	host.AttachAll(core)

	if plugin1.attachCalled != 1 {
		t.Errorf("plugin1 Attach call count: got %d, want 1", plugin1.attachCalled)
	}
	if plugin2.attachCalled != 1 {
		t.Errorf("plugin2 Attach call count: got %d, want 1", plugin2.attachCalled)
	}
}

// TestPluginHost_ProcessRecommendations tests that recommendations pass through all plugins.
func TestPluginHost_ProcessRecommendations(t *testing.T) {
	host := &PluginHost{}
	plugin1 := &mockPlugin{name: "plugin-1"}
	plugin2 := &mockPlugin{name: "plugin-2"}

	host.Register(plugin1, nil)
	host.Register(plugin2, nil)

	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Agent: "test-agent", Conviction: 50},
	}

	result := host.ProcessRecommendations(domain.RegimeRiskOn, recs)

	if len(result) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(result))
	}
	if plugin1.processRecCalled != 1 {
		t.Errorf("plugin1 ProcessRecommendations called %d times, want 1", plugin1.processRecCalled)
	}
	if plugin2.processRecCalled != 1 {
		t.Errorf("plugin2 ProcessRecommendations called %d times, want 1", plugin2.processRecCalled)
	}
}

// TestPluginHost_ProcessRecommendationsModifiesRecs tests that plugins can modify recommendations.
func TestPluginHost_ProcessRecommendationsModifiesRecs(t *testing.T) {
	host := &PluginHost{}
	plugin1 := &mockPlugin{
		name: "plugin-1",
		recsToReturn: []domain.Recommendation{
			{Symbol: "2330.TW", Agent: "plugin-1", Conviction: 60},
		},
	}

	host.Register(plugin1, nil)

	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Agent: "original", Conviction: 50},
	}

	result := host.ProcessRecommendations(domain.RegimeRiskOn, recs)

	// Plugin modified the recommendations
	if len(result) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(result))
	}
	if result[0].Agent != "plugin-1" {
		t.Errorf("expected agent 'plugin-1', got %q", result[0].Agent)
	}
}

// TestPluginHost_ProcessRecommendationsNil tests that nil host returns input unchanged.
func TestPluginHost_ProcessRecommendationsNil(t *testing.T) {
	var host *PluginHost

	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Agent: "test-agent", Conviction: 50},
	}

	result := host.ProcessRecommendations(domain.RegimeRiskOn, recs)

	if len(result) != 1 {
		t.Fatalf("expected 1 recommendation from nil host, got %d", len(result))
	}
}

// TestPluginHost_ProcessRecommendationsEmpty tests that empty host returns input unchanged.
func TestPluginHost_ProcessRecommendationsEmpty(t *testing.T) {
	host := &PluginHost{}

	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Agent: "test-agent", Conviction: 50},
	}

	result := host.ProcessRecommendations(domain.RegimeRiskOn, recs)

	if len(result) != 1 {
		t.Fatalf("expected 1 recommendation from empty host, got %d", len(result))
	}
}

// TestPluginHost_PostSimulation tests that PostSimulation is called on all plugins.
func TestPluginHost_PostSimulation(t *testing.T) {
	host := &PluginHost{}
	plugin1 := &mockPlugin{name: "plugin-1"}
	plugin2 := &mockPlugin{name: "plugin-2"}

	host.Register(plugin1, nil)
	host.Register(plugin2, nil)

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 1000},
	}
	asOf := time.Now()

	host.PostSimulation(quotes, domain.RegimeRiskOn, asOf)

	if plugin1.postSimCalled != 1 {
		t.Errorf("plugin1 PostSimulation called %d times, want 1", plugin1.postSimCalled)
	}
	if plugin2.postSimCalled != 1 {
		t.Errorf("plugin2 PostSimulation called %d times, want 1", plugin2.postSimCalled)
	}
}

// TestPluginHost_RegisterAllowsNilPlugin tests that nil plugin handling is safe.
func TestPluginHost_RegisterAllowsNilPlugin(t *testing.T) {
	host := &PluginHost{}
	host.Register(nil, nil)
	if len(host.plugins) != 1 {
		t.Errorf("expected 1 plugin (nil), got %d", len(host.plugins))
	}
}
