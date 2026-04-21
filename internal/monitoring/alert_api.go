package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type AlertAPI struct {
	store *AlertStore
}

func NewAlertAPI(store *AlertStore) *AlertAPI {
	return &AlertAPI{store: store}
}

func (a *AlertAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/alerts", a.handleListAlerts)
	mux.HandleFunc("/api/alerts/unacknowledged", a.handleUnacknowledged)
	mux.HandleFunc("/api/alerts/acknowledge", a.handleAcknowledge)
}

func (a *AlertAPI) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records, err := a.store.LoadAll()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load alerts: %v", err))
		return
	}
	if records == nil {
		records = []domain.AlertRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": records, "total": len(records)})
}

func (a *AlertAPI) handleUnacknowledged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records, err := a.store.LoadUnacknowledged()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load unacknowledged alerts: %v", err))
		return
	}
	if records == nil {
		records = []domain.AlertRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": records, "total": len(records)})
}

func (a *AlertAPI) handleAcknowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		AlertID string `json:"alert_id"`
		User    string `json:"user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AlertID == "" {
		writeJSONError(w, http.StatusBadRequest, "alert_id required")
		return
	}
	if req.User == "" {
		writeJSONError(w, http.StatusBadRequest, "user required")
		return
	}

	if err := a.store.Acknowledge(req.AlertID, req.User); err != nil {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("acknowledge: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "alert_id": req.AlertID})
}
