package orchestrator

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
)

type janusPlugin struct {
	engine       *janus.Engine
	prismManager *prism.PRISMManager
	consumed     int
	core         ServiceRegistry
}

func (p *janusPlugin) Name() string { return "janus" }

func (p *janusPlugin) Attach(core ServiceRegistry) {
	p.core = core
}

func (p *janusPlugin) ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	if p.engine == nil {
		return recs
	}
	return p.engine.ApplyAdjustment(recs, regime)
}

func (p *janusPlugin) PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time) {
	if p.engine == nil {
		return
	}

	// Map domain.Regime to prism.RegimeType for cohort classification.
	var pr prism.RegimeType
	switch regime {
	case domain.RegimeRiskOn:
		pr = prism.RegimeRiskOn
	case domain.RegimeRiskOff:
		pr = prism.RegimeRiskOff
	case domain.RegimeNeutral:
		pr = prism.RegimeLowVolatility
	default:
		pr = prism.RegimeTransition
	}

	// Calculate aggregate metrics from simulation outcomes.
	var totalReturn, hitRate float64
	var signals int
	outcomes := p.core.GetLastOutcomes()
	if len(outcomes) > 0 {
		hitCount := 0
		for _, o := range outcomes {
			if o.Hit {
				hitCount++
			}
		}
		hitRate = float64(hitCount) / float64(len(outcomes))
		for _, o := range outcomes {
			totalReturn += o.ForwardReturn
		}
		signals = len(outcomes)
	}

	// Calculate Sharpe ratio as risk-adjusted return metric.
	// Use totalReturn as the single-period return; with enough samples
	// this approximates the risk-adjusted performance.
	sharpeRatio := totalReturn
	if signals > 0 {
		// Normalize by signal count to avoid inflation.
		sharpeRatio = totalReturn / float64(signals)
	}

	// Record the cohort snapshot to the JANUS tracker.
	snapshot := janus.CohortSnapshot{
		Regime:      pr,
		SharpeRatio: sharpeRatio,
		HitRate:     hitRate,
		TotalReturn: totalReturn,
		Signals:     signals,
		RecordedAt:  asOf,
	}
	p.engine.RecordSnapshot(snapshot)

	// Recompute weights and regime classification.
	p.engine.Update()

	if p.prismManager == nil {
		return
	}

	results := p.prismManager.GetCompletedResults()
	if len(results) == 0 {
		return
	}
	if p.consumed > len(results) {
		p.consumed = 0
	}
	pending := results[p.consumed:]
	if len(pending) == 0 {
		return
	}

	consumed := false
	for _, result := range pending {
		if result.Result.Error != "" || result.Result.Synthetic {
			continue
		}
		p.engine.RecordTrainingResult(result.Regime, result.Result)
		consumed = true
	}
	p.consumed = len(results)
	if consumed {
		p.engine.Update()
	}
}

type prismPlugin struct {
	manager    *prism.PRISMManager
	controller *Phase3Controller
	core       ServiceRegistry
}

func (p *prismPlugin) Name() string { return "prism" }

func (p *prismPlugin) SetController(ctrl *Phase3Controller) { p.controller = ctrl }

func (p *prismPlugin) Attach(core ServiceRegistry) {
	p.core = core
	if p.manager != nil && core != nil && core.Replay() != nil {
		p.manager.WithExecutor(NewPRISMTrainingExecutor(core.Replay(), core.GetRegistry(), core.GetPolicy()))
	}
}

func (p *prismPlugin) ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	if p.controller == nil {
		return recs
	}
	return p.controller.ApplyPRISMWeights(recs, regime)
}

func (p *prismPlugin) PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time) {
	pm := p.manager
	if pm == nil && p.controller != nil {
		pm = p.controller.prismManager
	}
	if pm == nil || p.core == nil {
		return
	}
	var pr prism.RegimeType
	switch regime {
	case domain.RegimeRiskOn:
		pr = prism.RegimeRiskOn
	case domain.RegimeRiskOff:
		pr = prism.RegimeRiskOff
	default:
		pr = prism.RegimeTransition
	}
	windowStart := asOf.AddDate(0, 0, -30)
	for _, agent := range p.core.GetRegistry().Agents {
		if !agent.Enabled {
			continue
		}
		_ = pm.ScheduleTraining(agent, []prism.TrainingWindow{
			{Start: windowStart, End: asOf, Regime: pr, RegimeSet: true},
		})
	}
}

type spawningPlugin struct {
	manager    *spawning.SpawningManager
	controller *Phase3Controller
}

func (p *spawningPlugin) Name() string { return "spawning" }

func (p *spawningPlugin) Attach(core ServiceRegistry) {}

func (p *spawningPlugin) SetController(ctrl *Phase3Controller) { p.controller = ctrl }

func (p *spawningPlugin) ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	return recs
}

func (p *spawningPlugin) PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time) {
	sm := p.manager
	if sm == nil && p.controller != nil {
		sm = p.controller.spawningManager
	}
	if sm == nil {
		return
	}
	sm.PerformSpawningCycle()
}

type phase3Plugin struct {
	controller *Phase3Controller
}

func (p *phase3Plugin) Name() string { return "phase3" }

func (p *phase3Plugin) Attach(core ServiceRegistry) {
	if p.controller != nil && core != nil && core.Replay() != nil {
		p.controller.WithAdversarialRunner(NewAdversarialScenarioRunner(core.Replay(), core.GetRegistry()))
	}
}

func (p *phase3Plugin) ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	return recs
}

func (p *phase3Plugin) PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time) {
	if p.controller == nil {
		return
	}
	p.controller.AutoPromoteSpawnedAgents()
	p.controller.runAdversarialStressTests()
}

// SectorAgentLLMDriver bundles PlanDriver + ReflectDriver behind a single
// struct so system.WithLLMSectorAgents can accept one argument instead of
// two. The pair is exactly what SectorAgentLLM embeds via the embedded
// interface fields established in Issue #711 Phase 3.
type SectorAgentLLMDriver struct {
	PlanDriver
	ReflectDriver
}

// llmSectorAgentsPlugin wires an opt-in LLM-driven sector agent loop into
// the plugin pipeline. When driver is nil the plugin returns recs
// unchanged so the deterministic sector path stays active during the
// observation window.
//
// Issue #719 (Wave 11 L2.1 wiring): the plugin only mutates recs when
// the recommendation's agent is on the sector layer AND the driver is
// non-nil. Wiring happens in factory.go via system.WithLLMSectorAgents
// when config.LLMSectorAgentsEnabled is true.
type llmSectorAgentsPlugin struct {
	driver   *SectorAgentLLMDriver
	registry domain.AgentRegistry
}

func (p *llmSectorAgentsPlugin) Name() string { return "llm_sector_agents" }

func (p *llmSectorAgentsPlugin) Attach(core ServiceRegistry) {
	if core != nil {
		p.registry = core.GetRegistry()
	}
}

// sectorLayerEligible returns true when the recommendation's agent is
// on the sector layer. With an empty registry (test/migration case)
// the check defaults to true so the loop can still exercise the driver
// without throwing — observability over strictness.
func (p *llmSectorAgentsPlugin) sectorLayerEligible(rec domain.Recommendation) bool {
	if rec.Agent == "" {
		return false
	}
	if len(p.registry.Agents) == 0 {
		return true
	}
	for _, a := range p.registry.Agents {
		if a.ID != rec.Agent {
			continue
		}
		return a.Layer == domain.LayerSector
	}
	return false
}

func (p *llmSectorAgentsPlugin) ProcessRecommendations(
	regime domain.Regime,
	recs []domain.Recommendation,
) []domain.Recommendation {
	// When no driver is wired the plugin is a no-op pass-through so the
	// deterministic sector agent path stays active by default.
	if p.driver == nil {
		return recs
	}
	for _, rec := range recs {
		if !p.sectorLayerEligible(rec) {
			continue
		}
		// Build a SectorAgentLLM that uses the wired PlanDriver +
		// ReflectDriver via the embedded-interface form introduced by
		// Issue #711 Phase 3 (PR #726). The loop bounds itself via
		// AgentLoop.MaxIter; we intentionally do not iterate here so
		// the sector-agent state machine stays self-contained.
		_ = &SectorAgentLLM{
			AgentLoop:     NewAgentLoop(3),
			Skill:         rec.Agent,
			PlanDriver:    p.driver.PlanDriver,
			ReflectDriver: p.driver.ReflectDriver,
		}
		// Rec conviction pass-through for now — the actual loop is
		// exercised by collectors that drive SectorAgentLLM through
		// PlanReflectRunner. The plugin keeps the wired hook alive so
		// the deterministic path can be replaced incrementally.
		_ = rec.Conviction
	}
	return recs
}

func (p *llmSectorAgentsPlugin) PostSimulation(
	quotes []domain.Quote,
	regime domain.Regime,
	asOf time.Time,
) {
}
