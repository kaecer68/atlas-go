package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
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
	mux.Handle("POST /api/alerts/acknowledge", shared.Post(a.handleAcknowledge))
}

func (a *AlertAPI) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records, err := a.store.LoadAll()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load alerts: %v", err))
		return
	}
	if records == nil {
		records = []domain.AlertRecord{}
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"alerts": records, "total": len(records)})
}

func (a *AlertAPI) handleUnacknowledged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records, err := a.store.LoadUnacknowledged()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load unacknowledged alerts: %v", err))
		return
	}
	if records == nil {
		records = []domain.AlertRecord{}
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"alerts": records, "total": len(records)})
}

func (a *AlertAPI) handleAcknowledge(r *http.Request) (int, any) {
	var req struct {
		AlertID string `json:"alert_id"`
		User    string `json:"user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if req.AlertID == "" {
		return http.StatusBadRequest, map[string]string{"error": "alert_id required"}
	}
	if req.User == "" {
		return http.StatusBadRequest, map[string]string{"error": "user required"}
	}

	if err := a.store.Acknowledge(req.AlertID, req.User); err != nil {
		return http.StatusNotFound, map[string]string{"error": fmt.Sprintf("acknowledge: %v", err)}
	}
	return http.StatusOK, map[string]any{"success": true, "alert_id": req.AlertID}
}
