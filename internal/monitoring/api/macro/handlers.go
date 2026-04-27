package macro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Service *service.MacroService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/macro/ingest", h.HandleMacroIngest)
	mux.HandleFunc("/api/macro/snapshot/latest", h.HandleMacroSnapshotLatest)
	mux.HandleFunc("/api/macro/snapshot/history", h.HandleMacroSnapshotHistory)
	mux.HandleFunc("/api/macro/capital-flow/latest", h.HandleCapitalFlowLatest)
	mux.HandleFunc("/api/taiwan/stress-index", h.HandleTaiwanStressIndex)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (h *Handlers) HandleMacroIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	events, snap, err := h.Service.Ingest(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("ingest failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":   events,
		"snapshot": snap,
	})
}

func (h *Handlers) HandleMacroSnapshotLatest(w http.ResponseWriter, r *http.Request) {
	data, err := h.Service.GetLatestSnapshot()
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "no macro snapshot available")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (h *Handlers) HandleMacroSnapshotHistory(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		writeJSONError(w, http.StatusBadRequest, "date query param required (YYYY-MM-DD)")
		return
	}
	data, err := h.Service.GetSnapshotByDate(date)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "snapshot not found for date")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (h *Handlers) HandleCapitalFlowLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap, err := h.Service.GetCapitalFlow()
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "no macro snapshot available")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"foreign_investor_net": snap.ForeignInvestorNet,
		"domestic_fund_net":    snap.DomesticFundNet,
		"dealer_net":          snap.DealerNet,
		"recorded_at":         snap.RecordedAt,
	})
}

func (h *Handlers) HandleTaiwanStressIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	index, err := h.Service.CalculateStressIndex(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("calculate stress index: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, index)
}
