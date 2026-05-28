package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/eventlogic"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type eventlogicPlugin struct {
	detector        *eventlogic.PatternDetector
	corrector       *eventlogic.SelfCorrector
	core            ServiceRegistry
	saveRulesPath   string
	historyRecorder *eventlogic.HistoryRecorder
	mu              sync.Mutex
	evtBuf          []eventlogic.NarrativeEventSnapshot
}

func (p *eventlogicPlugin) Name() string { return "eventlogic" }

func (p *eventlogicPlugin) Attach(core ServiceRegistry) {
	p.core = core
	if bus := core.EventBus(); bus != nil {
		bus.Subscribe(eventbus.EventNarrative, p.onNarrativeEvent)
	}
}

func (p *eventlogicPlugin) onNarrativeEvent(_ context.Context, ev eventbus.BusEvent) error {
	payload, ok := ev.Payload.(eventbus.NarrativeEventPayload)
	if !ok || payload.Theme == "" {
		return nil
	}
	p.mu.Lock()
	p.evtBuf = append(p.evtBuf, eventlogic.NarrativeEventSnapshot{
		Theme: payload.Theme, DetectedAt: ev.Timestamp,
	})
	if len(p.evtBuf) > 200 {
		p.evtBuf = p.evtBuf[len(p.evtBuf)-200:]
	}
	p.mu.Unlock()
	return nil
}

func (p *eventlogicPlugin) ProcessRecommendations(_ domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	return recs
}

func (p *eventlogicPlugin) PostSimulation(_ []domain.Quote, _ domain.Regime, _ time.Time) {
	if p.core == nil {
		return
	}
	outcomes := p.core.GetLastOutcomes()
	if len(outcomes) == 0 {
		return
	}
	p.mu.Lock()
	narrativeEvents := make([]eventlogic.NarrativeEventSnapshot, len(p.evtBuf))
	copy(narrativeEvents, p.evtBuf)
	p.mu.Unlock()
	if len(narrativeEvents) == 0 {
		return
	}
	priceChanges := make([]eventlogic.PriceChangeSnapshot, 0, len(outcomes))
	for _, o := range outcomes {
		priceChanges = append(priceChanges, eventlogic.PriceChangeSnapshot{
			Symbol: o.Symbol, ChangePct: o.ForwardReturn, RecordedAt: o.RecordedAt,
		})
	}
	candidates := p.detector.DiscoverPatterns(&eventlogic.DiscoveryInput{
		NarrativeEvents: narrativeEvents, PriceChanges: priceChanges,
	})
	var promoted int
	for i := range candidates {
		rule, err := p.detector.PromoteCandidate(&candidates[i])
		if err != nil {
			continue
		}
		promoted++
		logging.Info("eventlogic_plugin", "rule_promoted",
			logging.FStr("rule_id", rule.ID),
			logging.FFloat64("hit_rate", rule.HitRate),
			logging.FInt("total_tests", rule.TotalTests),
		)
		if p.corrector != nil {
			p.corrector.Evaluate(rule.ID, candidates[i].HitRate > 0.5)
		}
	}
	if promoted > 0 && p.saveRulesPath != "" {
		p.detector.Registry.MustSave(p.saveRulesPath)
		if p.historyRecorder != nil {
			p.historyRecorder.SnapshotAll(p.detector.Registry)
		}
	}
}
