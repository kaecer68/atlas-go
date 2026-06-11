// Package strategies provides the HTTP API for the strategy techniques
// library. Endpoints under /api/strategies expose the 5-layer framework,
// 4 core leading indicators, and self-correction attribution history.
package strategies

import (
	"encoding/json"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// Handlers serves the strategies API.
type Handlers struct {
	registry *strategy_techniques.Registry
}

// NewHandlers builds a new Handlers backed by the given Registry.
// Passing a nil registry is allowed; handlers will respond 503 until
// a registry is wired in.
func NewHandlers(r *strategy_techniques.Registry) *Handlers {
	return &Handlers{registry: r}
}

// SetRegistry replaces the underlying registry. Used by main.go to wire
// the registry after construction.
func (h *Handlers) SetRegistry(r *strategy_techniques.Registry) {
	h.registry = r
}

// RegisterRoutes attaches the strategies routes to mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/strategies", shared.Get(h.listStrategies))
	mux.Handle("GET /api/strategies/active", shared.Get(h.listActive))
	mux.Handle("GET /api/strategies/layers", shared.Get(h.listLayers))
	mux.Handle("GET /api/strategies/{id}", shared.Get(h.getStrategy))
	mux.Handle("POST /api/strategies/{id}/validate", shared.Post(h.validateStrategy))
	mux.Handle("GET /api/strategies/{id}/attribution", shared.Get(h.getAttribution))
}

// StrategyFrameSummary is the API-facing representation of a StrategyFrame.
// JSON tags are snake_case per internal/monitoring/AGENTS.md.
type StrategyFrameSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Layer       string   `json:"layer"`
	Summary     string   `json:"summary"`
	Rationale   string   `json:"rationale"`
	Direction   string   `json:"direction"`
	Risk        string   `json:"risk"`
	Status      string   `json:"status"`
	Source      string   `json:"source"`
	HitRate     float64  `json:"hit_rate"`
	TotalTests  int      `json:"total_tests"`
	TotalHits   int      `json:"total_hits"`
	Themes      []string `json:"themes"`
	Sectors     []string `json:"sectors"`
	Regimes     []string `json:"regimes"`
	Attribution []string `json:"attribution"`
}

func toSummary(f strategy_techniques.StrategyFrame) StrategyFrameSummary {
	return StrategyFrameSummary{
		ID:          f.ID,
		Name:        f.Name,
		Layer:       string(f.Layer),
		Summary:     f.Summary,
		Rationale:   f.Rationale,
		Direction:   string(f.Direction),
		Risk:        string(f.Risk),
		Status:      string(f.Status),
		Source:      string(f.Source),
		HitRate:     f.HitRate,
		TotalTests:  f.TotalTests,
		TotalHits:   f.TotalHits,
		Themes:      f.Themes,
		Sectors:     f.Sectors,
		Regimes:     f.Regimes,
		Attribution: f.Attribution,
	}
}

// StrategiesListResponse is the response shape for list endpoints.
type StrategiesListResponse struct {
	Strategies []StrategyFrameSummary `json:"strategies"`
	Total      int                    `json:"total"`
}

// LayerCount is one row in the LayersResponse.
type LayerCount struct {
	Layer string `json:"layer"`
	Count int    `json:"count"`
}

// LayersResponse summarises how many strategies live in each of the 5 layers.
type LayersResponse struct {
	Layers []LayerCount `json:"layers"`
	Total  int          `json:"total"`
}

// ValidateRequest is the body of POST /api/strategies/{id}/validate.
type ValidateRequest struct {
	TotalTests int `json:"total_tests"`
	TotalHits  int `json:"total_hits"`
}

// ValidateResponse is the response of POST /api/strategies/{id}/validate.
type ValidateResponse struct {
	ID      string  `json:"id"`
	Status  string  `json:"status"`
	HitRate float64 `json:"hit_rate"`
	Message string  `json:"message"`
}

func (h *Handlers) listStrategies(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "registry not initialized"}
	}
	layerParam := r.URL.Query().Get("layer")
	var frames []strategy_techniques.StrategyFrame
	if layerParam != "" {
		if !strategy_techniques.Layer(layerParam).IsValid() {
			return http.StatusBadRequest, map[string]string{"error": "invalid layer: " + layerParam}
		}
		frames = h.registry.FindByLayer(strategy_techniques.Layer(layerParam))
	} else {
		frames = h.registry.All()
	}
	summaries := make([]StrategyFrameSummary, 0, len(frames))
	for _, f := range frames {
		summaries = append(summaries, toSummary(f))
	}
	return http.StatusOK, StrategiesListResponse{Strategies: summaries, Total: len(summaries)}
}

func (h *Handlers) listActive(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "registry not initialized"}
	}
	all := h.registry.All()
	summaries := make([]StrategyFrameSummary, 0, len(all))
	for _, f := range all {
		if f.Status == strategy_techniques.StatusActive {
			summaries = append(summaries, toSummary(f))
		}
	}
	return http.StatusOK, StrategiesListResponse{Strategies: summaries, Total: len(summaries)}
}

func (h *Handlers) listLayers(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "registry not initialized"}
	}
	// Always enumerate all 5 canonical layers (L1~L5), with count=0 for empty layers.
	// This makes the dashboard "investment techniques library" visually complete:
	// users see L4 count=0 and can decide to author new techniques for that layer.
	counts := make([]LayerCount, 0, len(strategy_techniques.AllLayers))
	total := 0
	for _, layer := range strategy_techniques.AllLayers {
		c := len(h.registry.FindByLayer(layer))
		total += c
		counts = append(counts, LayerCount{Layer: string(layer), Count: c})
	}
	return http.StatusOK, LayersResponse{Layers: counts, Total: total}
}

func (h *Handlers) getStrategy(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "registry not initialized"}
	}
	id := r.PathValue("id")
	if err := shared.ValidatePathComponent(id); err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}
	f, err := h.registry.FindByID(id)
	if err != nil {
		return http.StatusNotFound, map[string]any{"error": "strategy not found"}
	}
	return http.StatusOK, toSummary(*f)
}

func (h *Handlers) validateStrategy(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "registry not initialized"}
	}
	id := r.PathValue("id")
	if err := shared.ValidatePathComponent(id); err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}
	f, err := h.registry.FindByID(id)
	if err != nil {
		return http.StatusNotFound, map[string]any{"error": "strategy not found"}
	}
	var req ValidateRequest
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()}
	}
	if req.TotalTests <= 0 {
		return http.StatusBadRequest, map[string]string{"error": "total_tests must be positive"}
	}
	if req.TotalHits < 0 || req.TotalHits > req.TotalTests {
		return http.StatusBadRequest, map[string]string{"error": "total_hits out of range"}
	}
	hitRate := float64(req.TotalHits) / float64(req.TotalTests)
	var newStatus strategy_techniques.Status
	var message string
	switch {
	case req.TotalTests < 10:
		newStatus = strategy_techniques.StatusDegraded
		message = "insufficient sample size"
	case hitRate < 0.4:
		newStatus = strategy_techniques.StatusDegraded
		message = "hit rate below 0.4"
	case hitRate >= 0.6:
		newStatus = strategy_techniques.StatusActive
		message = "strategy reactivated"
	default:
		newStatus = strategy_techniques.Status(f.Status)
		message = "strategy stable"
	}
	return http.StatusOK, ValidateResponse{
		ID:      f.ID,
		Status:  string(newStatus),
		HitRate: hitRate,
		Message: message,
	}
}

func (h *Handlers) getAttribution(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "registry not initialized"}
	}
	id := r.PathValue("id")
	if err := shared.ValidatePathComponent(id); err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}
	f, err := h.registry.FindByID(id)
	if err != nil {
		return http.StatusNotFound, map[string]any{"error": "strategy not found"}
	}
	return http.StatusOK, map[string]any{
		"id":          f.ID,
		"attribution": f.Attribution,
	}
}
