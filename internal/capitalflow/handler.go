package capitalflow

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kaecer68/atlas-go/internal/industry"
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
// wiring Task 5 will adopt in cmd/atlas/main.go. The calendar
// is nil here — handler-only test paths do not invoke Refresh.
func NewHandler(provider marketdata.MacroDataProvider) *Handler {
	return &Handler{service: NewService(provider, 0, nil)}
}

// NewHandlerWithStore wires a custom rolling sample store and
// the shared trading-day calendar into the Service underlying
// this handler. The store is what Refresh writes to and what
// LatestDaily / Summary read from for the rolling-history Z-score
// window. Production callers (cmd/atlas/main.go:733) pass a
// FileRollingSampleStore and the shared *industry.EventCalendar
// instance so the window survives process restart (spec §8.5)
// and Refresh's non-trading-day skip-and-log guard (CF-INV-16)
// is active.
func NewHandlerWithStore(provider marketdata.MacroDataProvider, store RollingSampleStore, cal *industry.EventCalendar) *Handler {
	return &Handler{service: NewServiceWithStore(provider, 0, store, cal)}
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
// dimension. Accepts optional `days` query param (default 252, max 252;
// raised from 60 per spec §10 H-CF-05 — A01 in
// docs/manifests/2026-07-20-cl5-capital-flow-handlehistory.md).
//
//	GET /api/capital-flow/history?days=252
//
// Response shape (default — backward compatible with H02 frontend):
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
//
// Response shape with ?include_meta=true (opt-in, A02):
//
//	{
//	  "samples": { ...same 7 keys as above... },
//	  "meta": {
//	    "status": "complete" | "partial" | "missing",
//	    "missing_dimensions": ["government", ...],
//	    "days_requested": 60,
//	    "days_returned": 60,
//	    "data_status": {
//	      "government": {"data_available": false},
//	      "foreign":    {"data_available": true},
//	      ...
//	    }
//	  }
//	}
func (h *Handler) HandleHistory(r *http.Request) (int, any) {
	days := defaultHistoryLimit
	if d := r.URL.Query().Get("days"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n <= 0 {
			return http.StatusBadRequest, map[string]string{
				"error": "days must be a positive integer",
			}
		}
		if n > defaultHistoryLimit {
			n = defaultHistoryLimit
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

	if !shouldIncludeMeta(r) {
		return http.StatusOK, result
	}
	return http.StatusOK, buildHistoryWithMeta(result, days)
}

// shouldIncludeMeta parses the opt-in ?include_meta=true query param.
// Per A02 spec §18.3.1: default behavior is the legacy flat shape; the
// wrapper with status / missing_dimensions / data_status is opt-in to
// preserve H02 frontend compatibility.
func shouldIncludeMeta(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("include_meta"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// buildHistoryWithMeta wraps the legacy result map with CF-INV-17 metadata.
// status is "missing" if all 7 dims are empty, "complete" if none are empty,
// "partial" otherwise. data_status reports per-dimension data_available flag
// (no missing_reason inference — per AGENTS.md "PublicBank 欄位歷史較短"
// we do not guess at provider-side reasons).
func buildHistoryWithMeta(result map[ForceName][]RollingSample, daysRequested int) map[string]any {
	const totalDims = 7
	missing := make([]any, 0, totalDims)
	dataStatus := make(map[string]any, totalDims)
	emptyCount := 0
	for _, dim := range []ForceName{
		ForceForeign, ForceInstitutional, ForceDealer,
		ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR,
	} {
		samples := result[dim]
		available := len(samples) > 0
		dataStatus[string(dim)] = map[string]any{"data_available": available}
		if !available {
			missing = append(missing, string(dim))
			emptyCount++
		}
	}
	status := "complete"
	switch {
	case emptyCount == totalDims:
		status = "missing"
	case emptyCount > 0:
		status = "partial"
	}
	return map[string]any{
		"samples": result,
		"meta": map[string]any{
			"status":             status,
			"missing_dimensions": missing,
			"days_requested":     daysRequested,
			"days_returned":      len(result[ForceForeign]),
			"data_status":        dataStatus,
		},
	}
}
