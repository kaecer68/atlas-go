package eventlogic

import (
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
