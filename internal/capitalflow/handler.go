package capitalflow

import (
	"context"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// Handler serves capital flow analysis endpoints.
type Handler struct {
	provider  marketdata.MacroDataProvider
	extractor *ForceExtractor
}

// NewHandler creates a capital flow HTTP handler.
func NewHandler(provider marketdata.MacroDataProvider) *Handler {
	return &Handler{
		provider:  provider,
		extractor: NewForceExtractor(),
	}
}

func RegisterRoutes(mux *http.ServeMux, provider marketdata.MacroDataProvider) {
	h := NewHandler(provider)
	mux.Handle("GET /api/capital-flow/daily", shared.Get(h.HandleDaily))
	mux.Handle("GET /api/capital-flow/summary", shared.Get(h.HandleSummary))
}

// HandleDaily returns the full daily capital flow report.
func (h *Handler) HandleDaily(r *http.Request) (int, any) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	snap, err := h.provider.FetchSnapshot(ctx)
	if err != nil {
		logging.Warn("capitalflow", "fetch_failed", logging.Err(err))
		return http.StatusServiceUnavailable, map[string]string{
			"error": "failed to fetch market data: " + err.Error(),
		}
	}

	date := time.Unix(snap.RecordedAt, 0)
	forces := h.extractor.Extract(snap)
	resonance := ComputeResonance(forces)
	report := GenerateDailyReport(date, forces, resonance)

	return http.StatusOK, report
}

// HandleSummary returns a condensed capital flow summary.
//
//	GET /api/capital-flow/summary
func (h *Handler) HandleSummary(r *http.Request) (int, any) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	snap, err := h.provider.FetchSnapshot(ctx)
	if err != nil {
		logging.Warn("capitalflow", "fetch_failed", logging.Err(err))
		return http.StatusServiceUnavailable, map[string]string{
			"error": "failed to fetch market data: " + err.Error(),
		}
	}

	date := time.Unix(snap.RecordedAt, 0)
	forces := h.extractor.Extract(snap)
	resonance := ComputeResonance(forces)
	summary := GenerateSummaryReport(date, forces, resonance)

	return http.StatusOK, summary
}
