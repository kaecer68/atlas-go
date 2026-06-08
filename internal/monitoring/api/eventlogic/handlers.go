package eventlogic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventlogic"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

type Handlers struct {
	registry  *eventlogic.RuleRegistry
	validator *eventlogic.RuleValidator
	detector  *eventlogic.PatternDetector
}

type statsResponse struct {
	TotalRules     int     `json:"total_rules"`
	ActiveRules    int     `json:"active_rules"`
	DegradedRules  int     `json:"degraded_rules"`
	ExpiredRules   int     `json:"expired_rules"`
	AverageHitRate float64 `json:"average_hit_rate"`
}

type validateRuleResponse struct {
	Message    string  `json:"message"`
	RuleID     string  `json:"rule_id"`
	HitRate    float64 `json:"hit_rate"`
	TotalTests int     `json:"total_tests"`
	TotalHits  int     `json:"total_hits"`
	Status     string  `json:"status"`
}

func NewHandlers(r *eventlogic.RuleRegistry, v *eventlogic.RuleValidator, d *eventlogic.PatternDetector) *Handlers {
	return &Handlers{registry: r, validator: v, detector: d}
}

// Registry returns the underlying event logic rule registry.
func (h *Handlers) Registry() *eventlogic.RuleRegistry {
	return h.registry
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/eventlogic/rules", shared.Get(h.ListRules))
	mux.Handle("GET /api/eventlogic/rules/active", shared.Get(h.ListActive))
	mux.Handle("GET /api/eventlogic/rules/expired", shared.Get(h.ListExpired))
	mux.Handle("GET /api/eventlogic/rules/{id}", shared.Get(h.GetRule))
	mux.Handle("POST /api/eventlogic/rules/{id}/validate", shared.Post(h.ValidateRule))
	mux.Handle("GET /api/eventlogic/stats", shared.Get(h.Stats))
	mux.Handle("POST /api/eventlogic/discover", shared.Post(h.Discover))
	mux.Handle("POST /api/eventlogic/rules", shared.AdminPost(h.CreateRule))
	mux.Handle("PUT /api/eventlogic/rules/{id}", shared.Adapt(h.UpdateRule))
	mux.Handle("DELETE /api/eventlogic/rules/{id}", shared.Adapt(h.DeleteRule))
}

func (h *Handlers) ListRules(r *http.Request) (int, any) {
	rl := h.registry.List()
	return http.StatusOK, map[string]any{"rules": rl, "total": len(rl)}
}

func (h *Handlers) ListActive(r *http.Request) (int, any) {
	rl := h.registry.ListActive()
	return http.StatusOK, map[string]any{"rules": rl, "total": len(rl)}
}

func (h *Handlers) ListExpired(r *http.Request) (int, any) {
	rl := h.registry.ListExpired()
	return http.StatusOK, map[string]any{"rules": rl, "total": len(rl)}
}

func (h *Handlers) GetRule(r *http.Request) (int, any) {
	id := r.PathValue("id")
	if id == "" {
		return http.StatusBadRequest, map[string]string{"error": "missing id"}
	}
	ru, ok := h.registry.GetByID(id)
	if !ok {
		return http.StatusNotFound, map[string]string{"error": "not found"}
	}
	return http.StatusOK, ru
}

func (h *Handlers) ValidateRule(r *http.Request) (int, any) {
	id := strings.TrimSuffix(r.PathValue("id"), "/validate")
	if id == "" {
		return http.StatusBadRequest, map[string]string{"error": "missing id"}
	}
	ru, ok := h.registry.GetByID(id)
	if !ok {
		return http.StatusNotFound, map[string]string{"error": "not found"}
	}

	// 執行規則健康檢查：基於歷史命中率自動升降級
	var msg string
	if ru.TotalTests >= 10 {
		switch {
		case ru.HitRate < 0.4 && ru.Status == eventlogic.StatusActive:
			ru.Status = eventlogic.StatusDegraded
			msg = fmt.Sprintf("hit rate below 40%% over %d tests — status degraded", ru.TotalTests)
			_ = h.registry.Update(ru)
		case ru.HitRate >= 0.6 && ru.Status == eventlogic.StatusDegraded:
			ru.Status = eventlogic.StatusActive
			msg = "hit rate recovered above 60% — status reactivated"
			_ = h.registry.Update(ru)
		default:
			msg = fmt.Sprintf("hit rate stable at %.1f%%", ru.HitRate*100)
		}
	} else {
		msg = fmt.Sprintf("insufficient sample size (%d tests) — needs >= 10 for auto-calibration", ru.TotalTests)
	}

	return http.StatusOK, validateRuleResponse{
		Message:    msg,
		RuleID:     id,
		HitRate:    ru.HitRate,
		TotalTests: ru.TotalTests,
		TotalHits:  ru.TotalHits,
		Status:     ru.Status,
	}
}

func (h *Handlers) Stats(r *http.Request) (int, any) {
	all := h.registry.List()
	total := len(all)
	active := h.registry.CountActive()
	exp := len(h.registry.ListExpired())
	var s float64
	for _, ru := range all {
		s += ru.HitRate
	}
	avg := 0.0
	if total > 0 {
		avg = s / float64(total)
	}
	return http.StatusOK, statsResponse{
		TotalRules:     total,
		ActiveRules:    active,
		DegradedRules:  total - active - exp,
		ExpiredRules:   exp,
		AverageHitRate: avg,
	}
}

func (h *Handlers) Discover(r *http.Request) (int, any) {
	// Pattern discovery requires post-simulation narrative events and price changes,
	// which are fed to the detector automatically by the eventlogic plugin after
	// each simulation run.  This endpoint returns the current auto-discovered
	// rules so the UI can show what the system has found so far.
	all := h.registry.List()
	autoDiscovered := make([]*eventlogic.EventRule, 0)
	for _, ru := range all {
		if ru.ConfidenceSource == eventlogic.SourceAutoDiscovered {
			autoDiscovered = append(autoDiscovered, ru)
		}
	}
	return http.StatusOK, map[string]any{
		"message":               "discovery runs automatically after each simulation; current auto-discovered rules listed below",
		"auto_discovered_count": len(autoDiscovered),
		"auto_discovered_rules": autoDiscovered,
		"total_rules":           len(all),
	}
}

func (h *Handlers) CreateRule(r *http.Request) (int, any) {
	var rule eventlogic.EventRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()}
	}
	if rule.ID == "" || rule.Pattern == "" {
		return http.StatusBadRequest, map[string]string{"error": "id and pattern are required"}
	}
	if rule.Status == "" {
		rule.Status = "active"
	}
	if rule.HitRate == 0 {
		rule.HitRate = 0.5
	}
	now := time.Now()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = now
	}
	if err := h.registry.Add(&rule); err != nil {
		return http.StatusConflict, map[string]string{"error": err.Error()}
	}
	return http.StatusCreated, rule
}

func (h *Handlers) UpdateRule(r *http.Request) (int, any) {
	id := r.PathValue("id")
	if id == "" {
		return http.StatusBadRequest, map[string]string{"error": "missing id"}
	}
	existing, ok := h.registry.GetByID(id)
	if !ok {
		return http.StatusNotFound, map[string]string{"error": "not found"}
	}
	var u struct {
		Pattern         *string                `json:"pattern"`
		HitRate         *float64               `json:"hit_rate"`
		Status          *string                `json:"status"`
		Direction       *string                `json:"direction"`
		AffectedSectors []string               `json:"affected_sectors"`
		AffectedStocks  []string               `json:"affected_stocks"`
		Conditions      []eventlogic.Condition `json:"conditions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid JSON: %v", err)}
	}
	if u.Pattern != nil {
		existing.Pattern = *u.Pattern
	}
	if u.HitRate != nil {
		existing.HitRate = *u.HitRate
	}
	if u.Status != nil {
		existing.Status = *u.Status
	}
	if u.Direction != nil {
		existing.Direction = *u.Direction
	}
	if u.AffectedSectors != nil {
		existing.AffectedSectors = u.AffectedSectors
	}
	if u.AffectedStocks != nil {
		existing.AffectedStocks = u.AffectedStocks
	}
	if u.Conditions != nil {
		existing.Conditions = u.Conditions
	}
	if err := h.registry.Update(existing); err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, existing
}

func (h *Handlers) DeleteRule(r *http.Request) (int, any) {
	id := r.PathValue("id")
	if id == "" {
		return http.StatusBadRequest, map[string]string{"error": "missing id"}
	}
	if err := h.registry.Delete(id); err != nil {
		return http.StatusNotFound, map[string]string{"error": err.Error()}
	}
	return http.StatusNoContent, nil
}
