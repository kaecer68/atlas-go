package eventlogic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kaecer68/atlas-go/internal/eventlogic"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

type Handlers struct {
	registry  *eventlogic.RuleRegistry
	validator *eventlogic.RuleValidator
	detector  *eventlogic.PatternDetector
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
	return http.StatusAccepted, map[string]any{"message": "validation queued", "rule_id": id, "hit_rate": ru.HitRate, "total_tests": ru.TotalTests, "total_hits": ru.TotalHits, "status": ru.Status}
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
	return http.StatusOK, map[string]any{"total_rules": total, "active_rules": active, "degraded_rules": total - active - exp, "expired_rules": exp, "average_hit_rate": avg}
}

func (h *Handlers) Discover(r *http.Request) (int, any) {
	return http.StatusAccepted, map[string]any{"message": "discovery triggered"}
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
		Pattern   *string  `json:"pattern"`
		HitRate   *float64 `json:"hit_rate"`
		Status    *string  `json:"status"`
		Direction *string  `json:"direction"`
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
