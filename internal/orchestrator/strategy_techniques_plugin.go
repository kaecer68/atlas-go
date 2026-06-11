// Package orchestrator - strategy_techniques plugin.
//
// strategyTechniquesPlugin is the StrategyFrame-based replacement for
// eventlogicPlugin. It subscribes to narrative events on eventbus,
// maintains a rolling event buffer per-attached System, and exposes a
// PostSimulation hook that will be wired to detector + corrector in
// Wave 4 (self-correction phase).
//
// The plugin implements the Plugin interface (Name / Attach /
// ProcessRecommendations / PostSimulation) registered via
// PluginHost.Register from system_plugins.go WithStrategyTechniques.
//
// Wave 2 delivers the wiring only — full detector (auto-discovery of
// candidate StrategyFrames) and corrector (hybrid attribution:
// rule-based + LLM annotation) arrive in Wave 4. Until then
// PostSimulation is a no-op that records the last seen narrative
// event buffer size, while ProcessRecommendations is intentionally a
// pure pass-through so the system can validate wiring without
// observable side effects.
package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// strategyTechniquesPlugin is the Wave 2 wiring layer for the
// strategy_techniques package. Field semantics:
//
//   - registry: source of truth for StrategyFrames. Read-only here;
//     Wave 4 adds write paths for auto-discovered candidates.
//   - savePath: on-disk JSON path used by Wave 4 corrector to
//     persist attributed rules. Empty disables persistence.
//   - core: ServiceRegistry captured at Attach time. Used to read
//     outcomes and narrative bus subscriptions.
//   - mu/evtBuf: thread-safe ring of the most recent 200 narrative
//     events consumed by onNarrativeEvent. Capped to bound memory.
type strategyTechniquesPlugin struct {
	registry *strategy_techniques.Registry
	savePath string
	core     ServiceRegistry

	mu     sync.Mutex
	evtBuf []strategyTechniquesNarrativeEvent
}

// strategyTechniquesNarrativeEvent is the minimal projection of an
// eventbus.NarrativeEventPayload that the plugin retains in its
// rolling buffer. It mirrors eventlogic.NarrativeEventSnapshot but
// lives in this file to avoid a hard import cycle with
// internal/eventlogic (which is being retired).
type strategyTechniquesNarrativeEvent struct {
	Theme      string
	DetectedAt time.Time
}

// Name satisfies the Plugin interface. Stable identifier used by
// PluginHost routing and by main.go logging.
func (p *strategyTechniquesPlugin) Name() string { return "strategy_techniques" }

// Attach subscribes the plugin to eventbus.EventNarrative on the
// provided ServiceRegistry. The narrative buffer is cleared on
// every Attach to avoid leaking state across SystemCore restarts.
//
// Pre-condition: core must expose Bus() returning an event bus with
// a Subscribe(eventType, handler) method (see eventbus package).
// Post-condition: p.evtBuf is empty and p.core is set.
func (p *strategyTechniquesPlugin) Attach(core ServiceRegistry) {
	if core == nil {
		logging.Warn("strategy_techniques", "attach_nil_core", "msg", "skipping subscription")
		return
	}
	p.core = core
	p.mu.Lock()
	p.evtBuf = p.evtBuf[:0]
	p.mu.Unlock()
	core.EventBus().Subscribe(eventbus.EventNarrative, p.onNarrativeEvent)
	logging.Info("strategy_techniques", "attached", "event", string(eventbus.EventNarrative))
}

// onNarrativeEvent is the eventbus callback invoked on every
// narrative event. It performs defensive type assertion, drops
// malformed payloads (logged at debug level), and appends the
// event to a capped ring buffer. The buffer is used by Wave 4
// detector logic and by the diagnostic log line in PostSimulation.
//
// Thread-safety: mu protects evtBuf. The function may be called
// concurrently from the event bus dispatch goroutine.
func (p *strategyTechniquesPlugin) onNarrativeEvent(_ context.Context, event eventbus.BusEvent) error {
	evt, ok := event.Payload.(eventbus.NarrativeEventPayload)
	if !ok {
		logging.Debug("strategy_techniques", "payload_type_mismatch", "msg", "dropping non-NarrativeEventPayload")
		return nil
	}
	p.mu.Lock()
	p.evtBuf = append(p.evtBuf, strategyTechniquesNarrativeEvent{
		Theme:      evt.Theme,
		DetectedAt: event.Timestamp,
	})
	if len(p.evtBuf) > 200 {
		// Drop oldest entries to keep the buffer bounded.
		p.evtBuf = p.evtBuf[len(p.evtBuf)-200:]
	}
	p.mu.Unlock()
	return nil
}

// ProcessRecommendations is a no-op pass-through. Wave 4 will
// implement attribution-aware filtering (e.g. drop recommendations
// whose sector matches a degraded StrategyFrame).
//
// Returning the input slice unchanged preserves the contract that
// Plugin hosts are non-destructive: callers can opt to skip this
// plugin without changing the recommendation stream.
func (p *strategyTechniquesPlugin) ProcessRecommendations(_ domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	return recs
}

// PostSimulation is the post-cycle hook called by SystemCore after
// every simulation tick. The Wave 2 implementation is intentionally
// minimal: it logs a single info-level line with the current event
// buffer size so operators can confirm the plugin is receiving
// events. Wave 4 will replace this with:
//
//  1. Pull outcomes from p.core.GetLastOutcomes().
//  2. Build a snapshot of MarketSnapshot for cross-checking.
//  3. Run detector.DiscoverCandidates(snapshot, p.evtBuf) to
//     propose new StrategyFrames.
//  4. For each new candidate, call corrector.AssignAttribution
//     (rule-based + optional LLM) and registry.Upsert.
//
// Pre-condition: quotes may be nil on dry-run ticks.
// Post-condition: no observable side effects in Wave 2.
func (p *strategyTechniquesPlugin) PostSimulation(_ []domain.Quote, _ domain.Regime, ts time.Time) {
	p.mu.Lock()
	bufSize := len(p.evtBuf)
	p.mu.Unlock()
	if p.registry == nil {
		logging.Warn("strategy_techniques plugin PostSimulation called with nil registry; ts=%s", ts.Format(time.RFC3339))
		return
	}
	logging.With("strategy_techniques_plugin").Info("PostSimulation",
		"ts", ts.Format(time.RFC3339),
		"active_strategies", p.registry.Count(),
		"evt_buf", bufSize)
}
