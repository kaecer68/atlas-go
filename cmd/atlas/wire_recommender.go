package main

import (
	"github.com/kaecer68/atlas-go/internal/capitalflow"
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

// WireDeps bundles the inputs needed to construct production HandlerDeps.
// nil for any field = that producer isn't wired (handlers fall back to safe defaults).
type WireDeps struct {
	WorkDir       string
	MacroProvider marketdata.MacroDataProvider
	EventCalendar *industry.EventCalendar
}

// WireRecommenderDeps constructs the 4 producer adapters per WireDeps.
// Returns zero-value HandlerDeps if all inputs are unavailable (rare in prod).
func WireRecommenderDeps(in WireDeps) recommender.HandlerDeps {
	deps := recommender.HandlerDeps{}

	// 1. capitalflow: needs macroProvider for FetchSnapshot.
	if in.MacroProvider != nil {
		cfsvc := capitalflow.NewService(in.MacroProvider, 0)
		deps.CapitalFlow = recommender.NewCapitalFlowFunc(cfsvc.LatestDaily, cfsvc.Summary)
	}

	// 2. event-driven Predictor: needs event calendar.
	if in.EventCalendar != nil {
		predictor := eventdriven.NewPredictor(in.EventCalendar)
		deps.EventPredictor = recommender.NewEventPredictorAdapter(predictor)
	}

	// 3. narrative: no external deps; always wire (NewNarrativeEngine + NewReportGenerator are no-arg).
	narrativeEng := narrative.NewNarrativeEngine()
	reportGen := narrative.NewReportGenerator()
	narrativeSvc := monitoringservice.NewNarrativeService(in.WorkDir, narrativeEng, reportGen)
	if narrativeSvc != nil {
		deps.Narrative = recommender.NewNarrativeAdapterFunc(
			narrativeSvc.GetCurrentStressIndex,
			narrativeSvc.BuildMarketNarrativeData,
		)
	}

	// 4. comparison engine: keep instance alive across requests; use 30-day window.
	cmpEng := strategy.NewComparisonEngine(30)
	if cmpEng != nil {
		deps.StrategyComp = recommender.NewComparisonEngineAdapter(cmpEng)
	}

	return deps
}
