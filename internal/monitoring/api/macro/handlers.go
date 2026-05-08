package macro

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Service *service.MacroService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/macro/ingest", shared.Post(h.HandleMacroIngest))
	mux.Handle("POST /api/channels/ingest", shared.Post(h.HandleChannelsIngest))
	mux.Handle("GET /api/macro/capital-flow/latest", shared.Get(h.HandleCapitalFlowLatest))
	mux.Handle("GET /api/taiwan/stress-index", shared.Get(h.HandleTaiwanStressIndex))
	// Raw-write handlers (cannot use Adapt — write raw bytes)
	mux.HandleFunc("/api/macro/snapshot/latest", h.HandleMacroSnapshotLatest)
	mux.HandleFunc("/api/macro/snapshot/history", h.HandleMacroSnapshotHistory)
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

func (h *Handlers) HandleMacroSnapshotLatest(w http.ResponseWriter, r *http.Request) {
	data, err := h.Service.GetLatestSnapshot()
	if err != nil {
		shared.WriteJSONError(w, http.StatusNotFound, "no macro snapshot available")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (h *Handlers) HandleMacroSnapshotHistory(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "date query param required (YYYY-MM-DD)")
		return
	}
	data, err := h.Service.GetSnapshotByDate(date)
	if err != nil {
		shared.WriteJSONError(w, http.StatusNotFound, "snapshot not found for date")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
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
