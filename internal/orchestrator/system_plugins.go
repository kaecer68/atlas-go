package orchestrator

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/charter"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/macroflow"
	"github.com/kaecer68/atlas-go/internal/methodology"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// WithCharterMode attaches per-arm charter options to the system (Phase C3).
// It wires the charter components lazily when the given options enable at
// least one switch (even with cfg.CharterMode=false — the A/B harness keeps
// the global flag off and controls each arm explicitly). When options are
// zero or the system already wired full charter via cfg.CharterMode, the
// existing wiring is kept and only the switch set is updated:
//
//   - cfg.CharterMode=true + no WithCharterMode → AllOn (Phase C2 behavior)
//   - cfg.CharterMode=false + WithCharterMode(opts) → only opts' layers
//   - cfg.CharterMode=false + no WithCharterMode → Phase A (no charter)
func (s *System) WithCharterMode(options charter.Options) *System {
	if s.charter == nil {
		if !options.Enabled() {
			return s
		}
		s.charter = &charterConfig{
			periodDetector: portfolio.NewPeriodDetectorWithDefaults(),
			macroflow:      macroflow.NewEngine(0),
			advisor:        methodology.NewAdvisor(nil),
			options:        options,
		}
		logging.Info("orchestrator", "charter_mode_attached",
			"switches", strings.Join(options.Names(), "+"))
		return s
	}
	// Charter already wired (cfg.CharterMode=true): keep components, update
	// the active switch set (union semantics — existing switches are never
	// disabled by a later WithCharterMode call).
	s.charter.options = mergeCharterOptions(s.charter.options, options)
	return s
}

// mergeCharterOptions combines two option sets (union of enabled switches).
func mergeCharterOptions(base, add charter.Options) charter.Options {
	merged := charter.Options{
		PeriodOnly:      base.PeriodOnly || add.PeriodOnly,
		StrategyFilter:  base.StrategyFilter || add.StrategyFilter,
		MacroFlow:       base.MacroFlow || add.MacroFlow,
		CashReserve:     base.CashReserve || add.CashReserve,
		ConvictionFloor: base.ConvictionFloor || add.ConvictionFloor,
	}
	return merged
}

// WithJANUS attaches a JANUS engine to the system for backtest validation.
func (s *System) WithJANUS(j *janus.Engine, pm *prism.PRISMManager) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&janusPlugin{engine: j, prismManager: pm}, s.SystemCore)
	return s
}

// WithPersistentState enables cross-day simulation state carry-over for backtests.
func (s *System) WithPersistentState(state *domain.SimulationState) *System {
	s.Sim().persistentState = state
	return s
}

// WithPRISM attaches a PRISM training manager to the system.
// If replay data is available, a real training executor is automatically wired.
func (s *System) WithPRISM(pm *prism.PRISMManager) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&prismPlugin{manager: pm}, s.SystemCore)
	return s
}

// WithSpawning attaches a spawning manager for automated agent creation.
func (s *System) WithSpawning(sm *spawning.SpawningManager) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&spawningPlugin{manager: sm}, s.SystemCore)
	return s
}

// WithDarwinian attaches a Darwinian weight manager to the system for dynamic
// agent weight adjustment based on performance.
func (s *System) WithDarwinian(dw *portfolio.DarwinianWeightManager) *System {
	s.Port().darwinian = dw
	return s
}

// WithPhase3Controller attaches the advanced Phase 3 optimization controller.
// If replay data is available, an adversarial scenario runner is automatically wired.
func (s *System) WithPhase3Controller(ctrl *Phase3Controller) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	for _, p := range s.host.plugins {
		if ca, ok := p.(ControllerAware); ok {
			ca.SetController(ctrl)
		}
	}
	s.host.Register(&phase3Plugin{controller: ctrl}, s.SystemCore)
	s.phase3Ctrl = ctrl
	return s
}

// WithStrategyTechniques wires the new 5-layer strategy techniques library
// into the plugin host. This is the StrategyFrame-based replacement for the
// legacy eventlogic plugin.
//
// Framework:
//   - L1 Global Liquidity, L2 Foreign Capital Behavior, L3 Industry Catalysts,
//     L4 FX & Chips, L5 Geopolitics
//   - 4 core leading indicators: ForeignInvestorNet, TSMADR, NVDA, DXY
//   - Hybrid self-correction: rule-based attribution + LLM annotation
//     (LLM path is filled in by Wave 4; this scaffold is event-loop safe)
//
// Migration: this is the new code path. Existing eventlogicPlugin remains
// wired only for backward compatibility during the migration window
// (eventlogic is retired in Wave 5 cleanup after the production seeds
// migrate and the eventbus references move).
//
// Parameter savePath is the on-disk location for the 9 production seeds
// (data/seeds/strategy_techniques.json). It is read once at boot via
// strategy_techniques.LoadFromFile; live writes are queued but flushed by
// the registry's own background task, not by this plugin.
func (s *System) WithStrategyTechniques(
	registry *strategy_techniques.Registry,
	savePath string,
) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&strategyTechniquesPlugin{
		registry: registry,
		savePath: savePath,
	}, s.SystemCore)
	return s
}

// WithLLMSectorAgents wires the LLM-driven sector agent plugin into the
// system. The driver argument provides the LLM backend (typically backed
// by llm.Router via dependency injection). A nil driver is a valid state:
// the plugin returns recs unchanged so the deterministic sector agent
// path remains active during the observation window.
//
// Issue #719 (Wave 11 L2.1 wiring): the factory calls this option only
// when config.LLMSectorAgentsEnabled is true (env LLM_SECTOR_AGENTS_ENABLED).
// Production callers that need a real PlanDriver + ReflectDriver pair
// should pass a driver implementation here rather than relying on the
// default no-op path.
func (s *System) WithLLMSectorAgents(driver *SectorAgentLLMDriver) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&llmSectorAgentsPlugin{
		driver: driver,
	}, s.SystemCore)
	return s
}
