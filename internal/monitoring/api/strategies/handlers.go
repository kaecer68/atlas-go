// Package strategies provides the HTTP API for the strategy techniques
// library. Endpoints under /api/strategies expose the 5-layer framework,
// 4 core leading indicators, and self-correction attribution history.
package strategies

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/llm"
	llmcapabilities "github.com/kaecer68/atlas-go/internal/llm/capabilities"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// Handlers serves the strategies API.
type Handlers struct {
	registry       *strategy_techniques.Registry
	annotator      llm_annotator.Annotator
	summaryHandler *llmcapabilities.StrategySummaryHandler
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

// SetAnnotator wires the LLM annotator for the /annotate endpoint. nil is
// permitted; the annotate handler will return 503 until a real annotator
// is wired in. The annotator is decoupled from the registry so a mock can
// be used in tests without loading the production registry.
func (h *Handlers) SetAnnotator(a llm_annotator.Annotator) {
	h.annotator = a
}

// SetSummaryHandler wires the LLM strategy summary handler for the
// /{id}/summary endpoint. nil is permitted; the getSummary handler will
// return 503 until a real handler is wired in.
func (h *Handlers) SetSummaryHandler(sh *llmcapabilities.StrategySummaryHandler) {
	h.summaryHandler = sh
}

// RegisterRoutes attaches the strategies routes to mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/strategies", shared.Get(h.listStrategies))
	// Deprecated: same data as /api/strategies. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/strategies/active", shared.Get(h.listActive))
	mux.Handle("GET /api/strategies/layers", shared.Get(h.listLayers))
	mux.Handle("GET /api/strategies/{id}", shared.Get(h.getStrategy))
	mux.Handle("POST /api/strategies/{id}/validate", shared.Post(h.validateStrategy))
	mux.Handle("GET /api/strategies/{id}/attribution", shared.Get(h.getAttribution))
	mux.Handle("POST /api/strategies/{id}/annotate", shared.Post(h.annotate))
	// Deprecated: covered by /api/strategies/{id} and /attribution. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/strategies/{id}/summary", shared.Get(h.getSummary))
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

// LayersResponse summarizes how many strategies live in each of the 5 layers.
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
		newStatus = f.Status
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

// annotate handles POST /api/strategies/{id}/annotate. It calls the
// configured LLM annotator to produce a natural-language explanation of
// why the strategy would not hit under the current macro data. The
// annotator is opt-in: if no annotator is wired (SetAnnotator was not
// called), the endpoint returns 503 with a fallback to rule_based.
//
// The request body is optional and accepts:
//   - "macro":       map[string]float64 of MacroSnapshot fields
//   - "actual":      map[string]float64 of actual condition values
//   - "occurred_at": RFC3339 string (defaults to time.Now())
func (h *Handlers) annotate(r *http.Request) (int, any) {
	id := r.PathValue("id")
	if err := shared.ValidatePathComponent(id); err != nil {
		return http.StatusBadRequest, map[string]any{"error": err.Error()}
	}
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]any{"error": "registry not initialized"}
	}
	frame, err := h.registry.FindByID(id)
	if err != nil {
		return http.StatusNotFound, map[string]any{"error": "strategy not found"}
	}
	if h.annotator == nil {
		return http.StatusServiceUnavailable, map[string]any{
			"error":    "llm annotator not configured (set LLM_ANNOTATOR_API_KEY)",
			"fallback": frame.Attribution,
		}
	}
	var body struct {
		Macro      map[string]float64 `json:"macro"`
		Actual     map[string]float64 `json:"actual"`
		OccurredAt string             `json:"occurred_at"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	}
	occurredAt, _ := time.Parse(time.RFC3339, body.OccurredAt)
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	fc := buildFailureContext(frame, body.Macro, body.Actual, occurredAt)
	text, err := h.annotator.Annotate(r.Context(), fc)
	if err != nil {
		return http.StatusBadGateway, map[string]any{
			"error":    err.Error(),
			"fallback": frame.Attribution,
			"backend":  h.annotator.Name(),
		}
	}
	return http.StatusOK, map[string]any{
		"id":         id,
		"annotation": text,
		"backend":    h.annotator.Name(),
	}
}

// getSummary handles GET /api/strategies/{id}/summary. It calls the
// configured StrategySummaryHandler to produce a human-readable Chinese
// summary of the strategy frame and its key conditions. The handler is
// opt-in: if no summary handler is wired (SetSummaryHandler was not
// called), the endpoint returns 503.
func (h *Handlers) getSummary(r *http.Request) (int, any) {
	id := r.PathValue("id")
	if err := shared.ValidatePathComponent(id); err != nil {
		return http.StatusBadRequest, map[string]any{"error": err.Error()}
	}
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]any{"error": "registry not initialized"}
	}
	frame, err := h.registry.FindByID(id)
	if err != nil {
		return http.StatusNotFound, map[string]any{"error": "strategy not found"}
	}
	if h.summaryHandler == nil {
		return http.StatusServiceUnavailable, map[string]any{
			"error": "llm summary handler not configured (set LLM_*_API_KEY)",
		}
	}
	input := schemas.StrategySummaryInput{
		Frame:     *frame,
		DataClass: llm.DataClassRegulated,
	}
	resp, err := h.summaryHandler.Handle(r.Context(), input)
	if err != nil {
		return http.StatusBadGateway, map[string]any{
			"error": err.Error(),
		}
	}
	return http.StatusOK, map[string]any{
		"id":             id,
		"summary":        resp.Summary,
		"key_conditions": resp.KeyConditions,
		"backend":        "llm",
	}
}

// buildFailureContext converts a StrategyFrame + macro/actual maps into
// the FailureContext that llm_annotator expects. Missing macro fields
// default to 0; missing actual values default to 0 (the LLM prompt
// surfaces the gap so the response can explain what was missing).
func buildFailureContext(frame *strategy_techniques.StrategyFrame, macro, actual map[string]float64, occurredAt time.Time) llm_annotator.FailureContext {
	snap := llm_annotator.MacroSnapshot{
		ForeignInvestorNet:  macro["foreign_capital_net_twd"],
		TSMADR:              macro["tsm_adr_pct"],
		NVDA:                macro["nvda_pct"],
		DXY:                 macro["dxy_pct"],
		USD_TWD:             macro["usd_twd"],
		RetailMarginBalance: macro["retail_margin_balance"],
		DomesticFundNet:     macro["domestic_fund_net"],
		DealerNet:           macro["dealer_net"],
		VIX:                 macro["vix"],
		US10Y:               macro["us10y"],
	}
	conds := make([]llm_annotator.ConditionSnapshot, len(frame.Conditions))
	for i, c := range frame.Conditions {
		conds[i] = llm_annotator.ConditionSnapshot{
			Field:       c.Field,
			Operator:    c.Operator,
			Threshold:   c.Value,
			ActualValue: actual[c.Field],
			Timeframe:   c.Timeframe,
		}
	}
	return llm_annotator.FailureContext{
		FrameID:    frame.ID,
		FrameName:  frame.Name,
		Layer:      string(frame.Layer),
		Conditions: conds,
		Snap:       snap,
		OccurredAt: occurredAt,
	}
}
