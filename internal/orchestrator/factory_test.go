package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/janus"
)

func testProductionSystemConfig(t *testing.T) config.Config {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))

	return config.Config{
		WorkDir:              root,
		PrimaryMarket:        "TW",
		AgentRegistryPath:    filepath.Join(root, "configs", "agents.json"),
		BaselinePolicyPath:   filepath.Join(root, "data", "state", "baseline_policy.json"),
		ParametersConfigPath: filepath.Join(root, "configs", "parameters.json"),
		LedgerDir:            t.TempDir(),
		ReplayDataPath:       filepath.Join(root, "data", "replay", "tw_extended_90days.csv"),
	}
}

func findJANUSPlugin(t *testing.T, sys *System) *janusPlugin {
	t.Helper()

	if sys == nil || sys.host == nil {
		t.Fatal("expected system plugin host to be initialized")
	}
	for _, plugin := range sys.host.plugins {
		jp, ok := plugin.(*janusPlugin)
		if ok {
			return jp
		}
	}
	t.Fatal("expected janus plugin to be registered")
	return nil
}

func TestNewProductionSystemWithEventBus_UsesProvidedJANUSEngine(t *testing.T) {
	sharedJANUS := janus.NewEngine()

	sys, err := NewProductionSystemWithEventBus(testProductionSystemConfig(t), nil, sharedJANUS)
	if err != nil {
		t.Fatalf("NewProductionSystemWithEventBus() error = %v", err)
	}

	plugin := findJANUSPlugin(t, sys)
	if plugin.engine != sharedJANUS {
		t.Fatalf("expected shared JANUS engine %p, got %p", sharedJANUS, plugin.engine)
	}
}
