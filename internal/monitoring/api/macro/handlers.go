package macro

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Service *service.MacroService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	// Deprecated: manual ingest trigger; use the BackgroundTaskManager. See docs/operations/tier-boundary.md.
	mux.Handle("POST /api/macro/ingest", shared.AdminPost(h.HandleMacroIngest))
	mux.Handle("POST /api/channels/ingest", shared.AdminPost(h.HandleChannelsIngest))
	// Deprecated: covered by /api/macro/snapshot/latest. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/macro/capital-flow/latest", shared.Get(h.HandleCapitalFlowLatest))
	mux.Handle("GET /api/taiwan/stress-index", shared.Get(h.HandleTaiwanStressIndex))
	mux.Handle("GET /api/macro/snapshot/latest", shared.Get(h.HandleMacroSnapshotLatest))
	mux.Handle("GET /api/macro/snapshot/history", shared.Get(h.HandleMacroSnapshotHistory))
	mux.Handle("GET /api/macro/snapshot/timeline", shared.Get(h.HandleMacroSnapshotTimeline))
	mux.Handle("GET /api/dashboard/macro-data-health", shared.Get(h.HandleMacroDataHealth))
}

func (h *Handlers) HandleMacroIngest(r *http.Request) (int, any) {
	events, snap, err := h.Service.Ingest(r.Context())
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("ingest failed: %v", err)}
	}
	return http.StatusOK, map[string]any{
		"events":   events,
		"snapshot": snap,
	}
}

func (h *Handlers) HandleMacroSnapshotLatest(r *http.Request) (int, any) {
	snap, err := h.Service.GetLatestSnapshot()
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "no macro snapshot available"}
	}
	return http.StatusOK, snap
}

func (h *Handlers) HandleMacroSnapshotHistory(r *http.Request) (int, any) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		return http.StatusBadRequest, map[string]string{"error": "date query param required (YYYY-MM-DD)"}
	}
	if err := shared.ValidateDateParam(date); err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}
	snap, err := h.Service.GetSnapshotByDate(date)
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "snapshot not found for date"}
	}
	return http.StatusOK, snap
}

// HandleMacroSnapshotTimeline returns a range of macro snapshots for time-series
// queries. Backed by Service.ListSnapshotsInRange.
//
// Query params (mutually exclusive `from` vs `days`; `from/to` override `days`):
//
//	from=YYYY-MM-DD  range start (inclusive); defaults to no lower bound
//	to=YYYY-MM-DD    range end (inclusive); defaults to today (UTC)
//	days=N          relative window (default 30, max 365); translated to (to-days+1, to)
//
// Behavior follows CF-MS-01/02/03/04 invariants (see docs/specs/macro-snapshot-history-spec.md).
func (h *Handlers) HandleMacroSnapshotTimeline(r *http.Request) (int, any) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	daysStr := strings.TrimSpace(r.URL.Query().Get("days"))

	if from != "" {
		if err := shared.ValidateDateParam(from); err != nil {
			return http.StatusBadRequest, map[string]string{"error": err.Error()}
		}
	}
	if to != "" {
		if err := shared.ValidateDateParam(to); err != nil {
			return http.StatusBadRequest, map[string]string{"error": err.Error()}
		}
	}

	var days int
	var hasDays bool
	if daysStr != "" {
		parsed, err := strconv.Atoi(daysStr)
		if err != nil {
			return http.StatusBadRequest, map[string]string{"error": "days must be integer"}
		}
		if parsed <= 0 {
			return http.StatusBadRequest, map[string]string{"error": "days must be positive"}
		}
		if parsed > 365 {
			return http.StatusBadRequest, map[string]string{"error": "days exceeds capacity (max 365)"}
		}
		days = parsed
		hasDays = true
	}

	if from != "" && hasDays {
		return http.StatusBadRequest, map[string]string{"error": "from and days are mutually exclusive"}
	}

	if !hasDays && from == "" {
		days = 30
		hasDays = true
	}

	if hasDays {
		today := time.Now().UTC()
		to = today.Format("2006-01-02")
		from = today.AddDate(0, 0, -days+1).Format("2006-01-02")
	}

	if from != "" && to != "" && from > to {
		return http.StatusBadRequest, map[string]string{"error": "from must be on or before to"}
	}

	snapshots, missingDates, capacityLimitHit, err := h.Service.ListSnapshotsInRange(r.Context(), from, to, 365)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list snapshots: %v", err)}
	}

	returnedCount := len(snapshots)
	missingCount := len(missingDates)
	return http.StatusOK, map[string]any{
		"snapshots": snapshots,
		"range": map[string]string{
			"from": from,
			"to":   to,
		},
		"capacity_limit_hit": capacityLimitHit,
		"missing_dates":      missingDates,
		"stats": map[string]int{
			"requested_count": returnedCount + missingCount,
			"returned_count":  returnedCount,
			"missing_count":   missingCount,
		},
	}
}

func (h *Handlers) HandleCapitalFlowLatest(r *http.Request) (int, any) {
	snap, err := h.Service.GetCapitalFlow()
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "no macro snapshot available"}
	}
	return http.StatusOK, map[string]any{
		"foreign_investor_net": snap.ForeignInvestorNet,
		"domestic_fund_net":    snap.DomesticFundNet,
		"dealer_net":           snap.DealerNet,
		"recorded_at":          snap.RecordedAt,
	}
}

func (h *Handlers) HandleTaiwanStressIndex(r *http.Request) (int, any) {
	index, err := h.Service.CalculateStressIndex(r.Context())
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("calculate stress index: %v", err)}
	}
	return http.StatusOK, index
}

func (h *Handlers) HandleChannelsIngest(r *http.Request) (int, any) {
	_, _, err := h.Service.Ingest(r.Context())
	if err != nil {
		return http.StatusOK, map[string]any{
			"macro_ok":    false,
			"macro_error": err.Error(),
			"geo_ok":      false,
			"geo_error":   "geo ingest not yet wired to macro service",
		}
	}
	return http.StatusOK, map[string]any{
		"macro_ok":    true,
		"macro_error": "",
		"geo_ok":      false,
		"geo_error":   "geo ingest not yet wired to macro service",
	}
}

func (h *Handlers) HandleMacroDataHealth(r *http.Request) (int, any) {
	health, err := h.Service.GetMacroDataHealth()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("macro data health: %v", err)}
	}
	return http.StatusOK, health
}
