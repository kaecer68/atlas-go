package main

import (
	"log"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// wireNarrativePipeline creates the narrative engine and adapter, then
// registers eventdriven routes so the predictor consumes both capital flow
// and narrative models. Returns the engine so callers can schedule
// UpdateModelWeights / EvaluateModels on a cron.
func wireNarrativePipeline(
	mux *http.ServeMux,
	cal *industry.EventCalendar,
	cfProvider eventdriven.CapitalFlowProvider,
) *narrative.NarrativeEngine {
	engine := narrative.NewNarrativeEngine()
	np := &eventdriven.NarrativeProvider{Engine: engine}
	eventdriven.RegisterRoutesWithNarrative(mux, cal, cfProvider, np)
	log.Printf("[Narrative] wired %d InvestmentModels into predictor",
		len(np.ListModels()))
	return engine
}
