package orchestrator

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestNewSystem_UsesParametersConfigPathFromConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	parametersPath := filepath.Join(tempDir, "parameters.json")

	parameters := config.DefaultParametersConfig()
	parameters.Darwinian.WeightNeutral.Value = 1.23
	if err := parameters.Save(parametersPath); err != nil {
		t.Fatalf("save parameters config: %v", err)
	}

	sys, _ := NewSystem(config.Config{ParametersConfigPath: parametersPath})
	got := sys.Port().darwinian.GetWeight("missing-agent")

	if math.Abs(got-1.23) > 1e-9 {
		t.Fatalf("expected neutral weight from config path to be 1.23, got %.6f", got)
	}
}
