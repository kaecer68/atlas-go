package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	mux.HandleFunc("/api/alerts/stats", a.handleStats)
	mux.Handle("POST /api/alerts/acknowledge", shared.Post(a.handleAcknowledge))
	mux.Handle("POST /api/alerts/resolve", shared.Post(a.handleResolve))
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

	// Parse query parameters
	q := r.URL.Query()
	statusFilter := q.Get("status")
	severityFilter := q.Get("severity")
	ruleFilter := q.Get("rule")
	fromStr := q.Get("from")
	toStr := q.Get("to")
	pageStr := q.Get("page")
	pageSizeStr := q.Get("page_size")

	// Default page = 1, page_size = 50
	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	pageSize := 50
	if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 500 {
		pageSize = ps
	}

	// Apply filters
	var filtered []domain.AlertRecord
	var fromTime, toTime time.Time
	if fromStr != "" {
		fromTime, _ = time.Parse(time.RFC3339, fromStr)
	}
	if toStr != "" {
		toTime, _ = time.Parse(time.RFC3339, toStr)
	}

	for _, rec := range records {
		if statusFilter != "" && string(rec.Status) != statusFilter {
			continue
		}
		if severityFilter != "" && !strings.EqualFold(rec.Severity, severityFilter) {
			continue
		}
		if ruleFilter != "" && rec.Rule != ruleFilter {
			continue
		}
		if !fromTime.IsZero() && rec.Timestamp.Before(fromTime) {
			continue
		}
		if !toTime.IsZero() && rec.Timestamp.After(toTime) {
			continue
		}
		filtered = append(filtered, rec)
	}

	total := len(filtered)

	// Apply pagination
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	var paged []domain.AlertRecord
	if start < total {
		paged = filtered[start:end]
	} else {
		paged = []domain.AlertRecord{}
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"alerts":    paged,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
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

func (a *AlertAPI) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records, err := a.store.LoadAll()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load alerts: %v", err))
		return
	}

	stats := map[string]int{
		"total":        0,
		"triggered":    0,
		"acknowledged": 0,
		"resolved":     0,
		"silenced":     0,
		"info":         0,
		"warning":      0,
		"error":        0,
		"critical":     0,
	}

	cutoff24h := time.Now().Add(-24 * time.Hour)
	stats["last_24h"] = 0

	for _, rec := range records {
		stats["total"]++
		switch rec.Status {
		case domain.AlertStatusTriggered:
			stats["triggered"]++
		case domain.AlertStatusAcknowledged:
			stats["acknowledged"]++
		case domain.AlertStatusResolved:
			stats["resolved"]++
		case domain.AlertStatusSilenced:
			stats["silenced"]++
		}
		switch strings.ToUpper(rec.Severity) {
		case "INFO":
			stats["info"]++
		case "WARNING":
			stats["warning"]++
		case "ERROR":
			stats["error"]++
		case "CRITICAL":
			stats["critical"]++
		}
		if rec.Timestamp.After(cutoff24h) {
			stats["last_24h"]++
		}
	}

	shared.WriteJSON(w, http.StatusOK, stats)
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

func (a *AlertAPI) handleResolve(r *http.Request) (int, any) {
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

	if err := a.store.Resolve(req.AlertID, req.User); err != nil {
		return http.StatusNotFound, map[string]string{"error": fmt.Sprintf("resolve: %v", err)}
	}
	return http.StatusOK, map[string]any{"success": true, "alert_id": req.AlertID}
}
