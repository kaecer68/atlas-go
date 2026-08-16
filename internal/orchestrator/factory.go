package orchestrator

import (
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/spawning"
)

// NewProductionSystem builds a fully-wired System for dependency-graph visibility
// with an internally-created EventBus.
func NewProductionSystem(cfg config.Config, opts ...SystemOption) (*System, error) {
	return NewProductionSystemWithJANUS(cfg, nil, nil, opts...)
}

// NewProductionSystemWithJANUS builds a fully-wired System using the provided
// EventBus and JANUS engine. If janusEngine is nil, a new internal engine is
// created for backward compatibility.
func NewProductionSystemWithJANUS(
	cfg config.Config,
	eventBus *eventbus.ChannelEventBus,
	janusEngine *janus.Engine,
	opts ...SystemOption,
) (*System, error) {
	return NewProductionSystemWithEventBus(cfg, eventBus, janusEngine, opts...)
}

// NewProductionSystemWithEventBus builds a fully-wired System, passing the
// provided EventBus to NewSystemWithEventBus. If eventBus is nil, an internal
// EventBus is created (backward-compatible).
func NewProductionSystemWithEventBus(cfg config.Config, eventBus *eventbus.ChannelEventBus, janusEngine *janus.Engine, opts ...SystemOption) (*System, error) {
	system, err := NewSystemWithEventBus(cfg, eventBus, opts...)
	if err != nil {
		return nil, err
	}

	// Create and wire MaturityTracker.
	// Seeded constructor: ATLAS_MATURITY_FIRST_START carries the original
	// first_start across data-dir loss so burn-in never silently resets.
	maturityTracker, err := domain.NewMaturityTrackerSeeded(filepath.Join(cfg.WorkDir, "data/state/maturity_tracker.json"), cfg.MaturityFirstStart)
	if err != nil {
		// Non-fatal: system can run without maturity tracking.
		// Log loudly — a fallback reset of the burn-in clock would freeze
		// experiments/calibration evolution.
		logging.Error("orchestrator", "maturity_tracker_load_failed", "err", err)
		maturityTracker = domain.NewMaturityTrackerWithStart(time.Now().UTC())
	}
	system.WithMaturityTracker(maturityTracker)

	// Inject maturity tracker into DarwinianWeightManager.
	if system.Port().darwinian != nil {
		system.Port().darwinian.WithMaturityTracker(maturityTracker)
	}

	// Inject maturity tracker into sim engine's PreTradeGate.
	if system.Sim().engine != nil && maturityTracker != nil {
		system.Sim().engine.WithPreTradeGate(risk.NewPreTradeGate().WithMaturityTracker(maturityTracker))
	}

	system.WithDarwinian(system.Port().darwinian)

	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	system.WithPRISM(pm)

	if janusEngine == nil {
		janusEngine = janus.NewEngineWithConfig(janus.DefaultJANUSConfig())
	}
	system.WithJANUS(janusEngine, pm)

	spawnCfg := spawning.DefaultSpawningConfig()
	spawnCfg.PromptsDir = filepath.Join(cfg.WorkDir, "prompts")
	sm := spawning.NewSpawningManager(&system.Sim().registry, spawnCfg)
	system.WithSpawning(sm)

	re := reflexivity.NewReflexivityEngine()
	ctrl := NewPhase3Controller(&system.Sim().registry, pm, sm, re, system.Sim().ledger)
	system.WithPhase3Controller(ctrl)

	system.WithStrategyEvolver(NewStrategyEvolver())
	pm.Start()

	// Wave 11 L2.1 (Issue #719): opt-in LLM-driven sector agent wiring.
	// driver is intentionally nil — the plugin falls back to the
	// deterministic sector path. Production code that needs the LLM
	// loop should call system.WithLLMSectorAgents(driver) explicitly
	// with a real SectorAgentLLMDriver implementation.
	if cfg.LLMSectorAgentsEnabled {
		system.WithLLMSectorAgents(nil)
	}

	return system, nil
}
