package orchestrator

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

type janusPlugin struct {
	engine *janus.Engine
	core   ServiceRegistry
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
}

type swarmPlugin struct {
	swarm      *swarm.MiroFishSwarm
	controller *Phase3Controller
}

func (p *swarmPlugin) Name() string { return "swarm" }

func (p *swarmPlugin) Attach(core ServiceRegistry) {}

func (p *swarmPlugin) SetController(ctrl *Phase3Controller) { p.controller = ctrl }

func (p *swarmPlugin) ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	if len(recs) == 0 {
		return recs
	}
	var result swarm.SimulationResult
	var ok bool
	if p.controller != nil {
		result, ok = p.controller.GetSwarmConsensus()
	}
	if !ok && p.swarm != nil {
		result, ok = p.swarm.GetLatestResult()
	}
	if !ok || len(result.Consensus) == 0 {
		return recs
	}
	adjusted := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		adjusted[i] = rec
		cp, ok := result.Consensus[rec.Symbol]
		if !ok {
			continue
		}
		switch cp.ConsensusDirection {
		case "bullish":
			if rec.Side == domain.SideBuy {
				adjusted[i].Conviction = min(100, rec.Conviction+5)
			} else {
				adjusted[i].Conviction = max(0, rec.Conviction-5)
			}
		case "bearish":
			if rec.Side == domain.SideSell {
				adjusted[i].Conviction = min(100, rec.Conviction+5)
			} else {
				adjusted[i].Conviction = max(0, rec.Conviction-5)
			}
		}
	}
	return adjusted
}

func (p *swarmPlugin) PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time) {}

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
	baseState := swarm.MarketState{
		Timestamp: asOf,
		Prices:    make(map[string]float64, len(quotes)),
		Volumes:   make(map[string]float64, len(quotes)),
	}
	for _, q := range quotes {
		baseState.Prices[q.Symbol] = q.Last
		baseState.Volumes[q.Symbol] = float64(q.Volume)
	}
	p.controller.RunParallelOptimization(baseState, regime)
}
