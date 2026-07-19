package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	monitoringservice "github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/recommender"
	"github.com/kaecer68/atlas-go/internal/strategy"
)

// 構造規則 (per .omo/research/2026-07-08-recommender-wiring-gaps.md):
//
//	narrative:     no-deps constructor (always wire)
//	capitalflow:   needs macroProvider; nil → graceful fallback
//	event-driven:  needs *industry.EventCalendar; nil → graceful fallback
//	comparison:    needs *strategy.ComparisonEngine; default fresh per-call
//
// BK-15: CapitalFlowStore is the production-side handle for the rolling
// Z-score window. When non-nil, WireRecommenderDeps routes the
// capitalflow Service through NewServiceWithStore so the HTTP handler,
// eventdriven adapter, and operations_tasks refresh closure all share
// the same persisted window. When nil (graceful-fallback tests), the
// service uses the default in-memory store and reads/writes only
// affect the process lifetime.

// WireDeps bundles the inputs needed to construct production HandlerDeps.
// nil for any field = that producer isn't wired (handlers fall back to safe defaults).
type WireDeps struct {
	WorkDir       string
	MacroProvider marketdata.MacroDataProvider
	EventCalendar *industry.EventCalendar
	// CapitalFlowStore is the date-keyed rolling sample store (BK-15).
	// Production wiring passes a FileRollingSampleStore rooted under
	// cfg.LedgerDir; tests may pass a MemoryRollingSampleStore to assert
	// wired-path behavior, or nil to exercise the in-memory fallback
	// used by older harness code that never needs persistence.
	CapitalFlowStore capitalflow.RollingSampleStore
}

// WireRecommenderDeps constructs the 4 producer adapters per WireDeps.
// Returns zero-value HandlerDeps if all inputs are unavailable (rare in prod).
func WireRecommenderDeps(in WireDeps) recommender.HandlerDeps {
	deps, _ := wireForTest(in)
	return deps
}

// wireForTest is the internal builder shared with the wire_recommender
// tests. It returns the underlying *capitalflow.Service alongside the
// HandlerDeps so tests can assert which RollingSampleStore was wired
// through (NewService vs NewServiceWithStore). Production code must
// call WireRecommenderDeps, which discards the service handle.
func wireForTest(in WireDeps) (recommender.HandlerDeps, *capitalflow.Service) {
	deps := recommender.HandlerDeps{}
	var cfsvc *capitalflow.Service

	// 1. capitalflow: needs macroProvider for FetchSnapshot.
	if in.MacroProvider != nil {
		// BK-15: production passes a shared RollingSampleStore
		// (FileRollingSampleStore rooted at cfg.LedgerDir) so the
		// HTTP handler, eventdriven adapter, and operations_tasks
		// refresh closure all see the same date-keyed window across
		// restarts. Nil falls back to the legacy in-memory store so
		// harness / older wiring paths still work.
		if in.CapitalFlowStore != nil {
			// Production path: pass nil calendar because wireForTest is
			// only invoked via WireRecommenderDeps (test harness), not
			// from the live process. The live Refresh path uses
			// main.go:733 NewHandlerWithStore which passes the real
			// eventCalendar instance.
			cfsvc = capitalflow.NewServiceWithStore(in.MacroProvider, 0, in.CapitalFlowStore, nil)
		} else {
			cfsvc = capitalflow.NewService(in.MacroProvider, 0, nil)
		}
		deps.CapitalFlow = recommender.NewCapitalFlowFunc(cfsvc.LatestDaily, cfsvc.Summary, cfsvc.LatestAssessment)
	}

	// 2. event-driven Predictor: needs event calendar.
	if in.EventCalendar != nil {
		predictor := eventdriven.NewPredictor(in.EventCalendar)
		deps.EventPredictor = recommender.NewEventPredictorAdapter(predictor)
	}

	// 3. narrative: no external deps; always wire (NewNarrativeEngine + NewReportGenerator are no-arg).
	// NOTE: a fresh NarrativeEngine has no stressCalc, so its
	// GetCurrentStressIndex() always returns zero. When a macro provider is
	// available, compute the stress index through the same
	// TaiwanStressCalculator path that backs /api/taiwan/stress-index.
	narrativeEng := narrative.NewNarrativeEngine()
	reportGen := narrative.NewReportGenerator()
	narrativeSvc := monitoringservice.NewNarrativeService(in.WorkDir, narrativeEng, reportGen)
	getStress := func() narrative.TaiwanStressIndex { return narrative.TaiwanStressIndex{} }
	if in.MacroProvider != nil {
		stressCalc := narrative.NewTaiwanStressCalculator(nil, in.WorkDir)
		geoStore := narrative.NewGeopoliticalStore(filepath.Join(in.WorkDir, constants.StateGeopolitical))
		getStress = func() narrative.TaiwanStressIndex {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			snap, err := in.MacroProvider.FetchSnapshot(ctx)
			if err != nil {
				return narrative.TaiwanStressIndex{}
			}
			idx, err := stressCalc.CalculateFromSnapshotWithStore(ctx, snap, marketdata.MacroDataSnapshot{}, geoStore)
			if err != nil {
				return narrative.TaiwanStressIndex{}
			}
			return idx
		}
	}
	if narrativeSvc != nil {
		deps.Narrative = recommender.NewNarrativeAdapterFunc(
			getStress,
			narrativeSvc.BuildMarketNarrativeData,
		)
	}

	// 4. comparison engine: use file-backed store for persistence across restarts (F06).
	store := strategy.NewFileComparisonStore(filepath.Join(in.WorkDir, "data", "state", "comparison_days.json"), 60)
	cmpEng := strategy.NewComparisonEngine(30, store)
	if cmpEng != nil {
		deps.StrategyComp = recommender.NewComparisonEngineAdapter(cmpEng)
	}

	return deps, cfsvc
}
