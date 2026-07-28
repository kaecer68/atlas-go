package eventdriven

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// PredictionCacheTTL controls how long /api/events/prediction responses
// are cached in memory. Set to 60s so the frontend auto-refresh (30s)
// usually hits the warm cache, while stale data never exceeds 1 minute.
const PredictionCacheTTL = 60 * time.Second

// Handler serves event-driven prediction endpoints.
type Handler struct {
	eventCal      *industry.EventCalendar
	predictor     *Predictor
	macroProvider marketdata.MacroDataProvider

	cacheMu      sync.RWMutex
	cachedReport *PredictionReport
	cachedKey    string // "YYYY-MM-DD"
	cachedAt     time.Time
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

// SetMacroProvider wires a macro data provider so sector predictions use
// fresh macro snapshots on each request. nil disables macro-driven sector
// adjustments.
func (h *Handler) SetMacroProvider(mp marketdata.MacroDataProvider) {
	h.macroProvider = mp
}

// SetSectorPredictor wires a custom sector predictor. nil disables sector
// predictions (the API still returns an empty slice).
func (h *Handler) SetSectorPredictor(sp *SectorPredictor) {
	h.predictor.SetSectorPredictor(sp)
}

// SetScanStore injects a detector scan store into the predictor so detected
// themes are considered alongside event-calendar data.
func (h *Handler) SetScanStore(ss DetectorScanStore) {
	h.predictor.SetScanStore(ss)
}

// Predictor returns the underlying Predictor for external wiring (F04).
func (h *Handler) Predictor() *Predictor {
	return h.predictor
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
func RegisterRoutesWithDetectors(mux *http.ServeMux, cal *industry.EventCalendar, cf CapitalFlowProvider, np NarrativeModelProvider, ss DetectorScanStore) *Handler {
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
	return h
}

// HandleCalendar returns the upcoming event timeline with full calendar metadata.
// Since P2 cleanup (2026-07-26), this is the canonical event calendar endpoint
// replacing the duplicate /api/dashboard/calendar-events.
func (h *Handler) HandleCalendar(r *http.Request) (int, any) {
	now := time.Now()
	timeline := h.eventCal.GetEventTimeline(now, 14)
	if timeline == nil {
		timeline = []industry.CalendarEvent{}
	}

	items := make([]EventCalendarItem, 0, len(timeline))
	for _, e := range timeline {
		items = append(items, EventCalendarItem{
			ID:                  e.ID,
			Name:                e.Name,
			NameEN:              e.NameEN,
			EventType:           e.EventType,
			Description:         e.Description,
			Direction:           e.Direction,
			StartDate:           e.StartDate,
			EndDate:             e.EndDate,
			PeakDate:            e.PeakDate,
			DecayDays:           e.DecayDays,
			AffectedIndustries:  e.AffectedIndustries,
			ExpectedFlowImpact:  expectedFlow(e.EventType),
			Confidence:          e.BaseWeight,
			SentimentAdjustment: e.SentimentAdjustment,
			DataSource:          string(e.DataSource),
			EvidenceQuality:     string(e.EvidenceQuality),
			Backfilled:          e.Backfilled,
			CrossSourceStatus:   e.CrossSourceStatus,
			GeneratedAt:         e.GeneratedAt,
		})
	}

	return http.StatusOK, map[string]any{
		"events": items,
		"total":  len(items),
	}
}

// predictionCacheKey returns the cache key for the given time.
func predictionCacheKey(t time.Time) string {
	return t.Format("2006-01-02")
}

// HandlePrediction returns the 5-day event-driven capital flow prediction.
func (h *Handler) HandlePrediction(r *http.Request) (int, any) {
	now := time.Now()
	key := predictionCacheKey(now)

	h.cacheMu.RLock()
	if h.cachedReport != nil && h.cachedKey == key && time.Since(h.cachedAt) < PredictionCacheTTL {
		report := *h.cachedReport
		h.cacheMu.RUnlock()
		return http.StatusOK, report
	}
	h.cacheMu.RUnlock()

	h.cacheMu.Lock()
	if h.cachedReport != nil && h.cachedKey == key && time.Since(h.cachedAt) < PredictionCacheTTL {
		report := *h.cachedReport
		h.cacheMu.Unlock()
		return http.StatusOK, report
	}
	h.cacheMu.Unlock()

	// Cache miss: rebuild sector predictor if macro provider is wired.
	// FetchSnapshot is now backed by MacroSnapshotCacheTTL so this
	// call is fast when the snapshot is warm.
	if h.macroProvider != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		snap, err := h.macroProvider.FetchSnapshot(ctx)
		if err == nil {
			h.predictor.SetSectorPredictor(NewSectorPredictor(&snap, nil))
		}
	}

	report := h.predictor.Predict(now)

	h.cacheMu.Lock()
	h.cachedReport = &report
	h.cachedKey = key
	h.cachedAt = time.Now()
	h.cacheMu.Unlock()

	logging.Info("eventdriven", "prediction_generated",
		"events", len(report.ActiveEvents),
		"summary", report.Summary)

	return http.StatusOK, report
}
