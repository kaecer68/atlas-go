package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/alertscanner"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

type AlertAPI struct {
	store        *AlertStore
	multiScanner *alertscanner.MultiScanner // optional cross-source aggregation
}

func NewAlertAPI(store *AlertStore) *AlertAPI {
	return &AlertAPI{store: store}
}

// WithAlertSources enables cross-source alert aggregation. When set,
// /api/alerts/active merges results from the AlertStore AND the
// provided sources (e.g. Prometheus Alertmanager webhook, Wave9 events).
// Sources should be started before the first request (e.g. Wave9Source
// needs Start() to subscribe to the eventbus).
func (a *AlertAPI) WithAlertSources(sources ...alertscanner.AlertSource) *AlertAPI {
	a.multiScanner = alertscanner.NewMultiScanner(
		alertscanner.NewStoreAdapter(a.store),
	)
	// Append additional sources after the store adapter.
	for _, src := range sources {
		a.multiScanner = alertscanner.NewMultiScanner(
			append(a.multiScanner.Sources(), src)...,
		)
	}
	return a
}

func (a *AlertAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/alerts", a.handleListAlerts)
	mux.HandleFunc("/api/alerts/unacknowledged", a.handleUnacknowledged)
	mux.HandleFunc("/api/alerts/active", a.handleActiveSnapshot)
	mux.HandleFunc("/api/alerts/stats", a.handleStats)
	mux.HandleFunc("/api/alerts/rules", a.handleRules)
	mux.Handle("POST /api/alerts/acknowledge", shared.Post(a.handleAcknowledge))
	mux.Handle("POST /api/alerts/acknowledge-bulk", shared.Post(a.handleAcknowledgeBulk))
	mux.Handle("POST /api/alerts/resolve", shared.Post(a.handleResolve))
	mux.Handle("POST /api/alerts/silence", shared.Post(a.handleSilence))
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

	// Apply sort (P1-1: alert-redesign-v2.md Part 6.1). Default is
	// timestamp_desc (newest first). Stable sort by ID for tie-breaking
	// so tests are deterministic.
	sortParam := q.Get("sort")
	sortAlerts(filtered, sortParam)

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

func (a *AlertAPI) handleActiveSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var snap *alertscanner.Snapshot
	var err error
	if a.multiScanner != nil {
		snap, err = a.multiScanner.Snapshot(r.Context())
	} else {
		scanner := alertscanner.New(a.store)
		snap, err = scanner.Snapshot(r.Context())
	}
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("scan active alerts: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, snap)
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

// handleAcknowledgeBulk acknowledges multiple alerts in one request.
// Body: {"ids": ["uuid1", "uuid2", ...]}. Returns the count of
// acknowledged vs failed (id not found). Per alert-redesign-v2.md Part 6.3.
func (a *AlertAPI) handleAcknowledgeBulk(r *http.Request) (int, any) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if len(req.IDs) == 0 {
		return http.StatusBadRequest, map[string]string{"error": "ids required"}
	}

	ack := 0
	for _, id := range req.IDs {
		if err := a.store.Acknowledge(id, "bulk"); err == nil {
			ack++
		}
	}
	return http.StatusOK, map[string]any{
		"acknowledged": ack,
		"failed":       len(req.IDs) - ack,
	}
}

// handleSilence silences all non-resolved alerts matching a rule for the
// requested duration. Each matching alert's status is set to SILENCED and
// SilencedUntil is persisted. Returns the silenced_until timestamp and
// count of affected alerts.
func (a *AlertAPI) handleSilence(r *http.Request) (int, any) {
	var req struct {
		Rule        string `json:"rule"`
		DurationMin int    `json:"duration_minutes"`
		Reason      string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if req.Rule == "" {
		return http.StatusBadRequest, map[string]string{"error": "rule required"}
	}
	if req.DurationMin <= 0 {
		return http.StatusBadRequest, map[string]string{"error": "duration_minutes must be > 0"}
	}
	silencedUntil := time.Now().Add(time.Duration(req.DurationMin) * time.Minute)

	// Find and silence matching non-resolved alerts.
	all, err := a.store.LoadAll()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load alerts: %v", err)}
	}
	var silenced int
	for _, rec := range all {
		if rec.Rule != req.Rule {
			continue
		}
		if rec.Status == domain.AlertStatusResolved {
			continue
		}
		if err := a.store.Update(rec.ID, func(r *domain.AlertRecord) {
			r.Status = domain.AlertStatusSilenced
			silencedCopy := silencedUntil
			r.SilencedUntil = &silencedCopy
		}); err != nil {
			// Continue silencing other alerts even if one fails.
			continue
		}
		silenced++
	}

	return http.StatusOK, map[string]any{
		"rule":           req.Rule,
		"silenced_until": silencedUntil.UTC().Format(time.RFC3339),
		"reason":         req.Reason,
		"silenced_count": silenced,
	}
}

// handleRules returns a list of distinct rules with their active count
// (status=triggered) and last seen timestamp. Per alert-redesign-v2.md
// Part 6.5.
func (a *AlertAPI) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records, err := a.store.LoadAll()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load alerts: %v", err))
		return
	}

	ruleStats := map[string]*struct {
		ActiveCount int
		LastSeen    time.Time
	}{}
	for _, rec := range records {
		if _, ok := ruleStats[rec.Rule]; !ok {
			ruleStats[rec.Rule] = &struct {
				ActiveCount int
				LastSeen    time.Time
			}{}
		}
		if rec.Status == domain.AlertStatusTriggered {
			ruleStats[rec.Rule].ActiveCount++
		}
		if rec.Timestamp.After(ruleStats[rec.Rule].LastSeen) {
			ruleStats[rec.Rule].LastSeen = rec.Timestamp
		}
	}

	type ruleEntry struct {
		Rule        string `json:"rule"`
		ActiveCount int    `json:"active_count"`
		LastSeen    string `json:"last_seen"`
	}
	out := make([]ruleEntry, 0, len(ruleStats))
	for rule, s := range ruleStats {
		lastSeen := ""
		if !s.LastSeen.IsZero() {
			lastSeen = s.LastSeen.UTC().Format(time.RFC3339)
		}
		out = append(out, ruleEntry{Rule: rule, ActiveCount: s.ActiveCount, LastSeen: lastSeen})
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"rules": out})
}

// sortAlerts sorts a slice of AlertRecord in-place by the given sort
// parameter. Stable sort by ID for tie-breaking (deterministic test output).
func sortAlerts(records []domain.AlertRecord, sortParam string) {
	switch strings.ToLower(sortParam) {
	case "timestamp_asc":
		sort.SliceStable(records, func(i, j int) bool {
			if records[i].Timestamp.Equal(records[j].Timestamp) {
				return records[i].ID < records[j].ID
			}
			return records[i].Timestamp.Before(records[j].Timestamp)
		})
	case "severity_desc":
		severityRank := map[string]int{"critical": 4, "error": 3, "warning": 2, "info": 1}
		sort.SliceStable(records, func(i, j int) bool {
			ri, rj := severityRank[strings.ToLower(records[i].Severity)], severityRank[strings.ToLower(records[j].Severity)]
			if ri == rj {
				return records[i].ID < records[j].ID
			}
			return ri > rj
		})
	case "first_seen_desc":
		sort.SliceStable(records, func(i, j int) bool {
			ai, aj := firstSeen(records[i]), firstSeen(records[j])
			if ai.Equal(aj) {
				return records[i].ID < records[j].ID
			}
			return ai.After(aj)
		})
	default: // timestamp_desc (also default for empty/missing)
		sort.SliceStable(records, func(i, j int) bool {
			if records[i].Timestamp.Equal(records[j].Timestamp) {
				return records[i].ID < records[j].ID
			}
			return records[i].Timestamp.After(records[j].Timestamp)
		})
	}
}

// firstSeen returns the earliest known occurrence for tie-breaking.
// Uses Timestamp if FirstSeen is nil (legacy records).
func firstSeen(r domain.AlertRecord) time.Time {
	if r.FirstSeen != nil {
		return *r.FirstSeen
	}
	return r.Timestamp
}
