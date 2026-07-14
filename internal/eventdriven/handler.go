package eventdriven

import (
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// Handler serves event-driven prediction endpoints.
type Handler struct {
	eventCal  *industry.EventCalendar
	predictor *Predictor
}

// NewHandler creates an event-driven flow prediction handler.
func NewHandler(cal *industry.EventCalendar) *Handler {
	return &Handler{
		eventCal:  cal,
		predictor: NewPredictor(cal),
	}
}

// SetCapitalFlow wires the predictor's capital flow provider so predictions
// deviate from the default neutral/staticCF baseline.
func (h *Handler) SetCapitalFlow(cf CapitalFlowProvider) {
	h.predictor.SetCapitalFlow(cf)
}

// SetNarrativeProvider wires the predictor's narrative model provider,
// typically *narrative.NarrativeEngine via a thin adapter.
func (h *Handler) SetNarrativeProvider(np NarrativeModelProvider) {
	h.predictor.SetNarrativeProvider(np)
}

// SetScanStore injects a detector scan store into the predictor so detected
// themes are considered alongside event-calendar data.
func (h *Handler) SetScanStore(ss DetectorScanStore) {
	h.predictor.SetScanStore(ss)
}

// RegisterRoutes registers event-driven endpoints using the default static
// capital flow provider. Preserves v0.0.0.32 API.
func RegisterRoutes(mux *http.ServeMux, cal *industry.EventCalendar) {
	RegisterRoutesWithNarrative(mux, cal, &staticCF{score: 0, label: "neutral"}, nil)
}

// RegisterRoutesWithCapitalFlow registers event-driven endpoints with a
// real capital flow provider (typically *capitalflow.Service). nil cf
// falls back to the staticCF baseline so tests can omit it.
func RegisterRoutesWithCapitalFlow(mux *http.ServeMux, cal *industry.EventCalendar, cf CapitalFlowProvider) {
	RegisterRoutesWithNarrative(mux, cal, cf, nil)
}

// RegisterRoutesWithNarrative is the full production wiring: real capital
// flow + Darwinian narrative models + detector scan themes. nil providers
// fall back to event-only predictions.
func RegisterRoutesWithNarrative(mux *http.ServeMux, cal *industry.EventCalendar, cf CapitalFlowProvider, np NarrativeModelProvider) {
	RegisterRoutesWithDetectors(mux, cal, cf, np, nil)
}

// RegisterRoutesWithDetectors extends RegisterRoutesWithNarrative with a
// detector scan store for run-time detected theme data.
func RegisterRoutesWithDetectors(mux *http.ServeMux, cal *industry.EventCalendar, cf CapitalFlowProvider, np NarrativeModelProvider, ss DetectorScanStore) {
	h := NewHandler(cal)
	if cf != nil {
		h.SetCapitalFlow(cf)
	}
	if np != nil {
		h.SetNarrativeProvider(np)
	}
	if ss != nil {
		h.SetScanStore(ss)
	}
	mux.Handle("GET /api/events/prediction", shared.Adapt(shared.Handler(h.HandlePrediction)))
	mux.Handle("GET /api/events/calendar", shared.Adapt(shared.Handler(h.HandleCalendar)))
}

// HandleCalendar returns the upcoming event timeline.
func (h *Handler) HandleCalendar(r *http.Request) (int, any) {
	now := time.Now()
	timeline := h.eventCal.GetEventTimeline(now, 14)
	if timeline == nil {
		timeline = []industry.CalendarEvent{}
	}

	items := make([]EventCalendarItem, 0, len(timeline))
	for _, e := range timeline {
		items = append(items, EventCalendarItem{
			Name:               e.Name,
			EventType:          e.EventType,
			Direction:          e.Direction,
			StartDate:          e.StartDate,
			EndDate:            e.EndDate,
			AffectedIndustries: e.AffectedIndustries,
			ExpectedFlowImpact: expectedFlow(e.EventType),
			Confidence:         e.BaseWeight,
		})
	}

	return http.StatusOK, map[string]any{
		"events": items,
		"total":  len(items),
	}
}

// HandlePrediction returns the 5-day event-driven capital flow prediction.
func (h *Handler) HandlePrediction(r *http.Request) (int, any) {
	now := time.Now()
	report := h.predictor.Predict(now)

	logging.Info("eventdriven", "prediction_generated",
		"events", len(report.ActiveEvents),
		"summary", report.Summary)

	return http.StatusOK, report
}
