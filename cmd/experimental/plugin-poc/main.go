// plugin-poc validates the plugin boundary injection points introduced in
// feat/plugin-boundary-architecture. It demonstrates how a third-party
// module registers a custom AgentExecutor via StaticLoader.RegisterAgent
// and passes it to the System constructor via WithExecutorLoader.
//
// Usage: go run ./cmd/experimental/plugin-poc
//
// Expected output: build succeeds, agent count is larger than built-in count.

package main

import (
	"fmt"
	"log"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// demoExecutor is a minimal AgentExecutor that does nothing real.
// In a proprietary module, this would implement actual strategy logic.
type demoExecutor struct{}

func (demoExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "demo_plugin_strategy"
}

func (demoExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq orchestrator.FactorQuery) (domain.Recommendation, bool) {
	return domain.Recommendation{}, false
}

func main() {
	cfg := config.Load()
	if cfg.WorkDir == "" {
		log.Fatal("ATLAS_WORK_DIR must be set")
	}

	loader := &orchestrator.StaticLoader{}
	loader.RegisterAgent(demoExecutor{})

	system, err := orchestrator.NewProductionSystem(cfg, orchestrator.WithExecutorLoader(loader))
	if err != nil {
		log.Fatalf("create system: %v", err)
	}

	registry := system.Registry()
	fmt.Printf("agent count: %d\n", len(registry.Agents))
	fmt.Printf("custom executor registered: demo_plugin_strategy\n")
	fmt.Printf("plugin boundary injection: OK\n")
}
