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

// RegisterRoutes registers event-driven endpoints on the mux.
func RegisterRoutes(mux *http.ServeMux, cal *industry.EventCalendar) {
	h := NewHandler(cal)
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
