package capitalflow

import (
	"net/http"
	"strconv"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// Handler serves capital flow analysis endpoints.
//
// Handler is a thin HTTP layer over Service: HandleDaily and HandleSummary
// each delegate to a single Service method, which already runs the full
// FetchSnapshot -> Extract -> ComputeResonance -> GenerateX pipeline
// with its own timeout. The Service is the source of truth for pipeline
// behavior; Handler only owns HTTP concerns (request, error-to-status
// mapping, response write).
type Handler struct {
	service *Service
}

// NewHandler creates a capital flow HTTP handler backed by a
// Service with an in-memory rolling store (capacity
// defaultHistoryLimit). Callers that need a persistent rolling
// store should use NewHandlerWithStore directly; that is the
// wiring Task 5 will adopt in cmd/atlas/main.go.
func NewHandler(provider marketdata.MacroDataProvider) *Handler {
	return &Handler{service: NewService(provider, 0)}
}

// NewHandlerWithStore wires a custom rolling sample store into
// the Service underlying this handler. The store is what Refresh
// writes to and what LatestDaily / Summary read from for the
// rolling-history Z-score window. Production callers (Task 5
// wiring in cmd/atlas/main.go) pass a FileRollingSampleStore
// here so the window survives process restart (spec §8.5).
func NewHandlerWithStore(provider marketdata.MacroDataProvider, store RollingSampleStore) *Handler {
	return &Handler{service: NewServiceWithStore(provider, 0, store)}
}

func ServiceFromHandler(h *Handler) *Service { return h.service }

func RegisterRoutes(mux *http.ServeMux, provider marketdata.MacroDataProvider) {
	h := NewHandler(provider)
	mux.Handle("GET /api/capital-flow/daily", shared.Get(h.HandleDaily))
	mux.Handle("GET /api/capital-flow/summary", shared.Get(h.HandleSummary))
	mux.Handle("GET /api/capital-flow/history", shared.Get(h.HandleHistory))
}

// HandleDaily returns the full daily capital flow report.
//
//	GET /api/capital-flow/daily
func (h *Handler) HandleDaily(r *http.Request) (int, any) {
	report, err := h.service.LatestDaily(r.Context())
	if err != nil {
		logging.Warn("capitalflow", "fetch_failed", logging.Err(err))
		return http.StatusServiceUnavailable, map[string]string{
			"error": "failed to fetch market data: " + err.Error(),
		}
	}
	return http.StatusOK, report
}

// HandleSummary returns a condensed capital flow summary.
//
//	GET /api/capital-flow/summary
func (h *Handler) HandleSummary(r *http.Request) (int, any) {
	summary, err := h.service.Summary(r.Context())
	if err != nil {
		logging.Warn("capitalflow", "fetch_failed", logging.Err(err))
		return http.StatusServiceUnavailable, map[string]string{
			"error": "failed to fetch market data: " + err.Error(),
		}
	}
	return http.StatusOK, summary
}

// HandleHistory returns multi-day rolling samples for each capital force
// dimension. Accepts optional `days` query param (default 60, max 60).
//
//	GET /api/capital-flow/history?days=60
//
// Response shape:
//
//	{
//	  "foreign":        [{"trading_date":"...","raw_value":...},...],
//	  "institutional":  [...],
//	  "dealer":         [...],
//	  "government":     [...],
//	  "retail":         [...],
//	  "futures":        [...],
//	  "tsm_adr":        [...]
//	}
func (h *Handler) HandleHistory(r *http.Request) (int, any) {
	days := 60
	if d := r.URL.Query().Get("days"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n <= 0 {
			return http.StatusBadRequest, map[string]string{
				"error": "days must be a positive integer",
			}
		}
		if n > 60 {
			n = 60
		}
		if n < days {
			days = n
		}
	}

	// Use a far-future sentinel (2099-12-31) so all stored samples are
	// returned regardless of trading date. This is a read-only endpoint;
	// the "beforeDate" constraint matters for Z-score computation, not
	// for a history display.
	const sentinel = "2099-12-31"
	store := h.service.Store()
	if store == nil {
		return http.StatusServiceUnavailable, map[string]string{
			"error": "rolling store not available",
		}
	}

	result := make(map[ForceName][]RollingSample, 7)
	for _, dim := range []ForceName{
		ForceForeign, ForceInstitutional, ForceDealer,
		ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR,
	} {
		samples, err := store.History(r.Context(), dim, sentinel, days)
		if err != nil {
			logging.Warn("capitalflow", "history_failed", "dim", string(dim), "err", err.Error())
			// Degrade gracefully — return empty slice for this dimension
			result[dim] = []RollingSample{}
			continue
		}
		if samples == nil {
			samples = []RollingSample{}
		}
		result[dim] = samples
	}
	return http.StatusOK, result
}
