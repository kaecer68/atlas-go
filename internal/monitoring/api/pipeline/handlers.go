package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type Handlers struct {
	Svc              *service.PipelineService
	ReasoningHandler *ReasoningHandler
}

func NewHandlers(svc *service.PipelineService) *Handlers {
	return &Handlers{Svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/macro-radar", shared.Get(h.HandleMacroRadar))
	mux.Handle("GET /api/dashboard/agent-observatory", shared.Get(h.HandleAgentObservatory))
	mux.Handle("GET /api/dashboard/forecast-vs-reality", shared.Get(h.HandleForecastVsReality))
	mux.Handle("GET /api/dashboard/recommendation-pipeline", shared.Get(h.HandleRecommendationPipeline))
	mux.Handle("GET /api/dashboard/sessions", shared.Get(h.HandleSessions))
	mux.Handle("GET /api/dashboard/sessions/{id}", shared.Get(h.HandleSessionDetail))
	mux.Handle("GET /api/dashboard/universe-overlap", shared.Get(h.HandleUniverseOverlap))
	mux.Handle("GET /api/dashboard/reasoning-trace", shared.Get(h.ReasoningHandler.HandleReasoningTrace))
	mux.Handle("GET /api/synergy/darwinian/status", shared.Get(h.HandleDarwinianStatus))
	mux.Handle("GET /api/synergy/darwinian/trend", shared.Get(h.HandleDarwinianTrend))
	// Canonical: /api/regime/history (public, per isPublicPath).
	// Deprecated alias: /api/dashboard/regime-history (same handler, auth-required).
	// R-02: consumers should use /api/regime/history going forward.
	mux.Handle("GET /api/regime/history", shared.Get(h.HandleRegimeHistory))
	mux.Handle("GET /api/dashboard/regime-history", shared.Get(h.HandleRegimeHistory))
	mux.Handle("GET /api/dashboard/baseline-info", shared.Get(h.HandleBaselineInfo))
}

// parseLimit extracts an integer limit from the query string.
//
// Accepts both `limit` and `days` parameter names so that callers — most
// notably the MCP briefing tool which calls /api/regime/history?days=5 —
// can use either convention. `limit` wins when both are present (more
// explicit / canonical). The returned value is clamped to (0, maxValue].
// Returns defaultValue when neither parameter is set.
func parseLimit(r *http.Request, defaultValue, maxValue int) (int, error) {
	q := r.URL.Query()
	raw := strings.TrimSpace(q.Get("limit"))
	if raw == "" {
		raw = strings.TrimSpace(q.Get("days"))
	}
	if raw == "" {
		return defaultValue, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: must be integer")
	}
	if v <= 0 {
		return 0, fmt.Errorf("invalid limit: must be > 0")
	}
	if v > maxValue {
		return maxValue, nil
	}
	return v, nil
}

// HandleMacroRadar handles GET /api/dashboard/macro-radar.
func (h *Handlers) HandleMacroRadar(r *http.Request) (int, any) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	data, err := h.Svc.LoadMacroRadar(sessionID)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load macro radar data: %v", err)}
	}
	if data == nil {
		return http.StatusOK, MacroRadarResponse{}
	}

	resp := MacroRadarResponse{
		SessionID:     data.SessionID,
		Regime:        data.Regime,
		GuardOutcomes: data.GuardOutcomes,
		BrokerRuntime: data.BrokerRuntime,
		RecordedAt:    data.RecordedAt,
	}
	return http.StatusOK, resp
}

// MacroRadarResponse is the API response for macro radar.
type MacroRadarResponse struct {
	SessionID     string                    `json:"session_id"`
	Regime        domain.Regime             `json:"regime"`
	GuardOutcomes []domain.GuardOutcome     `json:"guard_outcomes"`
	BrokerRuntime domain.BrokerRuntimeAudit `json:"broker_runtime"`
	RecordedAt    time.Time                 `json:"recorded_at"`
}

// HandleAgentObservatory handles GET /api/dashboard/agent-observatory.
func (h *Handlers) HandleAgentObservatory(r *http.Request) (int, any) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	limit, err := parseLimit(r, 5, 50)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}

	data, err := h.Svc.LoadAgentObservatory(sessionID, limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load agent observatory: %v", err)}
	}

	// Sanitize scorecards: replace NaN/Inf values with 0
	for i := range data.Scorecards {
		sc := &data.Scorecards[i]
		if math.IsNaN(sc.SharpeLike) || math.IsInf(sc.SharpeLike, 0) {
			sc.SharpeLike = 0
		}
		if math.IsNaN(sc.HitRate) || math.IsInf(sc.HitRate, 0) {
			sc.HitRate = 0
		}
		if math.IsNaN(sc.MaxDrawdown) || math.IsInf(sc.MaxDrawdown, 0) {
			sc.MaxDrawdown = 0
		}
		if math.IsNaN(sc.AverageReturn) || math.IsInf(sc.AverageReturn, 0) {
			sc.AverageReturn = 0
		}
		// Phase 1: sanitize the new Darwinian-weight fields. A NaN/Inf
		// DarwinianWeight would poison the UI's "current allocation" view.
		if math.IsNaN(sc.DarwinianWeight) || math.IsInf(sc.DarwinianWeight, 0) {
			sc.DarwinianWeight = 0
		}
		if sc.DarwinianSharpe != nil && (math.IsNaN(*sc.DarwinianSharpe) || math.IsInf(*sc.DarwinianSharpe, 0)) {
			sc.DarwinianSharpe = nil
		}
	}

	resp := AgentObservatoryResponse{
		Scorecards:            data.Scorecards,
		SessionID:             data.SessionID,
		NextExperimentAgentID: data.NextExperimentAgentID,
		RecordedAt:            data.RecordedAt,
	}
	return http.StatusOK, resp
}

// AgentObservatoryResponse is the API response for agent observatory.
type AgentObservatoryResponse struct {
	SessionID             string             `json:"session_id"`
	NextExperimentAgentID string             `json:"next_experiment_agent_id"`
	Scorecards            []domain.Scorecard `json:"scorecards"`
	RecordedAt            time.Time          `json:"recorded_at"`
}

// HandleForecastVsReality handles GET /api/dashboard/forecast-vs-reality.
func (h *Handlers) HandleForecastVsReality(r *http.Request) (int, any) {
	limit, err := parseLimit(r, 20, 100)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))

	data, err := h.Svc.LoadForecastVsReality(agentID, limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load forecast-vs-reality data: %v", err)}
	}

	items := make([]ForecastVsRealityItem, len(data.Items))
	for i, item := range data.Items {
		items[i] = ForecastVsRealityItem{
			ExperimentID:   item.ExperimentID,
			ProposalID:     item.ProposalID,
			CommitID:       item.CommitID,
			ApprovalID:     item.ApprovalID,
			TargetAgentID:  item.TargetAgentID,
			Skill:          item.Skill,
			MutationType:   item.MutationType,
			Status:         item.Status,
			BaselineValue:  item.BaselineValue,
			CandidateValue: item.CandidateValue,
			RecordedAt:     item.RecordedAt,
		}
	}

	symbolPredictions := make([]SymbolPredictionItem, len(data.SymbolPredictions))
	for i, p := range data.SymbolPredictions {
		symbolPredictions[i] = SymbolPredictionItem{
			AgentID:       p.AgentID,
			Symbol:        p.Symbol,
			Side:          p.Side,
			Conviction:    p.Conviction,
			TargetPrice:   p.TargetPrice,
			ForwardReturn: p.ForwardReturn,
			Hit:           p.Hit,
			PassedGuards:  p.PassedGuards,
			RecordedAt:    p.RecordedAt,
			SessionID:     p.SessionID,
		}
	}

	resp := ForecastVsRealityResponse{
		Items:             items,
		SymbolPredictions: symbolPredictions,
		BrokerRuntime:     data.BrokerRuntime,
	}
	return http.StatusOK, resp
}

// ForecastVsRealityItem is the API response item for forecast vs reality.
type ForecastVsRealityItem struct {
	ExperimentID   string                  `json:"experiment_id"`
	ProposalID     string                  `json:"proposal_id"`
	CommitID       string                  `json:"commit_id"`
	ApprovalID     string                  `json:"approval_id"`
	TargetAgentID  string                  `json:"target_agent_id"`
	Skill          string                  `json:"skill"`
	MutationType   string                  `json:"mutation_type"`
	Status         domain.ExperimentStatus `json:"status"`
	BaselineValue  float64                 `json:"baseline_value"`
	CandidateValue float64                 `json:"candidate_value"`
	RecordedAt     time.Time               `json:"recorded_at"`
}

// ForecastVsRealityResponse is the API response for forecast vs reality.
type ForecastVsRealityResponse struct {
	Items             []ForecastVsRealityItem   `json:"items"`
	SymbolPredictions []SymbolPredictionItem    `json:"symbol_predictions"`
	BrokerRuntime     domain.BrokerRuntimeAudit `json:"broker_runtime"`
}

// SymbolPredictionItem is the API response item for symbol-level predictions.
type SymbolPredictionItem struct {
	AgentID       string  `json:"agent_id"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	Conviction    int     `json:"conviction"`
	TargetPrice   float64 `json:"target_price"`
	ForwardReturn float64 `json:"forward_return"`
	Hit           bool    `json:"hit"`
	PassedGuards  bool    `json:"passed_guards"`
	RecordedAt    string  `json:"recorded_at"`
	SessionID     string  `json:"session_id"`
}

// HandleRecommendationPipeline handles GET /api/dashboard/recommendation-pipeline.
func (h *Handlers) HandleRecommendationPipeline(r *http.Request) (int, any) {
	showAll := r.URL.Query().Get("show_all") == "true"
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))

	data, err := h.Svc.LoadRecommendationPipeline(sessionID, showAll)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load recommendation pipeline: %v", err)}
	}
	if data == nil {
		return http.StatusOK, RecommendationPipelineResponse{}
	}

	items := make([]PipelineItem, len(data.Items))
	for i, item := range data.Items {
		var narCtx *NarrativeContextItem
		var indCtx *IndustryContextItem
		if item.NarrativeContext != nil {
			narCtx = &NarrativeContextItem{
				ActiveThemes:   item.NarrativeContext.ActiveThemes,
				PrimaryTheme:   item.NarrativeContext.PrimaryTheme,
				PrimaryHitRate: item.NarrativeContext.PrimaryHitRate,
				DirectionHint:  item.NarrativeContext.DirectionHint,
			}
		}
		if item.IndustryContext != nil {
			indCtx = &IndustryContextItem{
				IndustryID:         item.IndustryContext.IndustryID,
				BusinessCycle:      item.IndustryContext.BusinessCycle,
				CycleConfidence:    item.IndustryContext.CycleConfidence,
				SeasonalMultiplier: item.IndustryContext.SeasonalMultiplier,
				SystemicImportance: item.IndustryContext.SystemicImportance,
			}
		}
		items[i] = PipelineItem{
			Symbol:              item.Symbol,
			AgentID:             item.AgentID,
			Skill:               item.Skill,
			Layer:               item.Layer,
			Side:                item.Side,
			Conviction:          item.Conviction,
			TargetPrice:         item.TargetPrice,
			StopLossPrice:       item.StopLossPrice,
			ForwardReturn:       item.ForwardReturn,
			Hit:                 item.Hit,
			Reason:              narrative.TranslateReason(item.Reason),
			Price:               item.Price,
			PassedGuards:        item.PassedGuards,
			GuardReason:         narrative.TranslateReason(item.GuardReason),
			Tags:                item.Tags,
			RecordedAt:          item.RecordedAt,
			FactorScores:        item.FactorScores,
			ConvictionBreakdown: item.ConvictionBreakdown,
			NarrativeEventIDs:   item.NarrativeEventIDs,
			NarrativeContext:    narCtx,
			IndustryContext:     indCtx,
			Metrics: &PipelineItemMetrics{
				PriceToEarnings: item.Metrics.PriceToEarnings,
				PriceToBook:     item.Metrics.PriceToBook,
				DividendYield:   item.Metrics.DividendYield,
				BacktestReturn:  item.Metrics.BacktestReturn,
			},
		}
	}

	resp := RecommendationPipelineResponse{
		SessionID:         data.SessionID,
		Regime:            data.Regime,
		Items:             items,
		GuardOutcomes:     data.GuardOutcomes,
		ScreenedItems:     data.ScreenedItems,
		RecordedAt:        data.RecordedAt,
		IsFallbackSession: data.IsFallbackSession,
		FallbackMessage:   data.FallbackMessage,
		CycleStatus:       data.CycleStatus,
		Status:            data.Status,
		StatusMessage:     data.StatusMessage,
	}
	return http.StatusOK, resp
}

type NarrativeContextItem struct {
	ActiveThemes   []string `json:"active_themes"`
	PrimaryTheme   string   `json:"primary_theme,omitempty"`
	PrimaryHitRate float64  `json:"primary_hit_rate,omitempty"`
	DirectionHint  string   `json:"direction_hint,omitempty"` // "positive" / "negative" / "neutral"
}

type IndustryContextItem struct {
	IndustryID         string  `json:"industry_id"`
	BusinessCycle      string  `json:"business_cycle"`
	CycleConfidence    float64 `json:"cycle_confidence"`
	SeasonalMultiplier float64 `json:"seasonal_multiplier"`
	SystemicImportance float64 `json:"systemic_importance"`
}

type PipelineItem struct {
	Symbol              string                      `json:"symbol"`
	AgentID             string                      `json:"agent_id"`
	Skill               string                      `json:"skill"`
	Layer               string                      `json:"layer"`
	Side                string                      `json:"side"`
	Conviction          int                         `json:"conviction"`
	TargetPrice         float64                     `json:"target_price"`
	StopLossPrice       float64                     `json:"stop_loss_price"`
	ForwardReturn       float64                     `json:"forward_return"`
	Hit                 bool                        `json:"hit"`
	Reason              string                      `json:"reason"`
	Price               float64                     `json:"price"`
	PassedGuards        bool                        `json:"passed_guards"`
	GuardReason         string                      `json:"guard_reason"`
	Tags                []string                    `json:"tags"`
	RecordedAt          time.Time                   `json:"recorded_at"`
	FactorScores        domain.FactorScores         `json:"factor_scores"`
	ConvictionBreakdown *domain.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
	NarrativeEventIDs   []string                    `json:"narrative_event_ids,omitempty"`
	NarrativeContext    *NarrativeContextItem       `json:"narrative_context,omitempty"`
	IndustryContext     *IndustryContextItem        `json:"industry_context,omitempty"`
	Metrics             *PipelineItemMetrics        `json:"metrics,omitempty"`
}

type PipelineItemMetrics struct {
	PriceToEarnings *float64 `json:"price_to_earnings,omitempty"`
	PriceToBook     *float64 `json:"price_to_book,omitempty"`
	DividendYield   *float64 `json:"dividend_yield,omitempty"`
	BacktestReturn  *float64 `json:"backtest_return,omitempty"`
}

type RecommendationPipelineResponse struct {
	SessionID         string                       `json:"session_id"`
	Regime            domain.Regime                `json:"regime"`
	Items             []PipelineItem               `json:"items"`
	GuardOutcomes     []domain.GuardOutcome        `json:"guard_outcomes"`
	ScreenedItems     []domain.ScreeningReject     `json:"screened_items"`
	RecordedAt        time.Time                    `json:"recorded_at"`
	IsFallbackSession bool                         `json:"is_fallback_session"`
	FallbackMessage   string                       `json:"fallback_message"`
	CycleStatus       *service.CycleStatusResponse `json:"cycle_status,omitempty"`
	Status            service.PipelineLoadStatus   `json:"status,omitempty"`
	StatusMessage     string                       `json:"status_message,omitempty"`
}

// HandleSessions handles GET /api/dashboard/sessions.
//
// Per CL-4 §18.7.2, each session object now includes a `top_strategies`
// field containing the top-3 strategies from that session ranked by
// Conviction DESC. This is additive — existing clients that only read
// the original 4 fields are unaffected.
func (h *Handlers) HandleSessions(r *http.Request) (int, any) {
	// ?limit= caps the number of sessions returned (newest first).
	// Default 90; limit=0 keeps legacy all-sessions behavior. Guards
	// unbounded LoadSessionsWithTopStrategies growth (SK-22 audit v3).
	limit := 90
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	sessions, err := h.Svc.LoadSessionsWithTopStrategies(3, limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load sessions: %v", err)}
	}
	if sessions == nil {
		return http.StatusOK, map[string]any{"sessions": []any{}}
	}

	result := make([]map[string]any, len(sessions))
	for i, s := range sessions {
		entry := map[string]any{
			"session_id":    s.SessionID,
			"recorded_at":   s.RecordedAt,
			"regime":        s.Regime,
			"outcome_count": s.OutcomeCount,
		}
		// Only include top_strategies when the enrichment populated them.
		// Keep nil/missing to avoid changing the response shape for sessions
		// that never went through LoadSessionsWithTopStrategies.
		if s.TopStrategies != nil {
			entry["top_strategies"] = strategyEntriesForResponse(s.TopStrategies)
		}
		result[i] = entry
	}
	return http.StatusOK, map[string]any{"sessions": result}
}

// strategyEntriesForResponse projects RecommendationOutcome into the
// per-strategy shape that frontends and MCP consumers expect from
// /api/dashboard/sessions. Keeping this projection in the handler layer
// keeps the domain type clean and lets us evolve the public shape without
// touching internal/domain.
func strategyEntriesForResponse(outcomes []domain.RecommendationOutcome) []map[string]any {
	result := make([]map[string]any, len(outcomes))
	for i, o := range outcomes {
		result[i] = map[string]any{
			"agent_id":      o.AgentID,
			"symbol":        o.Symbol,
			"side":          string(o.Side),
			"conviction":    o.Conviction,
			"passed_guards": o.PassedGuards,
			"target_price":  o.TargetPrice,
			"stop_loss":     o.StopLossPrice,
		}
	}
	return result
}

// HandleSessionDetail handles GET /api/dashboard/sessions/{id}.
//
// Per CL-4 §18.7.3, returns the full session summary plus all
// recommendation outcomes (one record per strategy execution in the
// session). The summary is read from the filesystem; the outcomes come
// from the SQLite ledger store via LoadSessionOutcomes.
//
// 404 (not 500) when the sessionID is unknown — distinguishes "missing
// data" from "system error" and matches REST conventions.
func (h *Handlers) HandleSessionDetail(r *http.Request) (int, any) {
	sessionID := strings.TrimSpace(r.PathValue("id"))
	if sessionID == "" {
		return http.StatusBadRequest, map[string]string{"error": "session id is required"}
	}

	detail, err := h.Svc.LoadSessionDetail(sessionID)
	if errors.Is(err, service.ErrSessionNotFound) {
		return http.StatusNotFound, map[string]string{
			"error":      "session not found",
			"session_id": sessionID,
		}
	}
	if err != nil {
		return http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("load session detail: %v", err),
		}
	}
	return http.StatusOK, detail
}

func (h *Handlers) HandleUniverseOverlap(r *http.Request) (int, any) {
	data, err := h.Svc.LoadUniverseOverlap()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load universe overlap: %v", err)}
	}
	if data == nil {
		return http.StatusOK, UniverseOverlapResponse{Agents: []AgentUniverseView{}, Matrix: map[string]map[string]int{}, Warnings: []string{}}
	}

	agents := make([]AgentUniverseView, len(data.Agents))
	for i, a := range data.Agents {
		agents[i] = AgentUniverseView{
			AgentID:           a.AgentID,
			Name:              a.Name,
			Layer:             a.Layer,
			Universe:          a.Universe,
			ScreeningCriteria: a.ScreeningCriteria,
		}
	}

	resp := UniverseOverlapResponse{
		Agents:   agents,
		Matrix:   data.Matrix,
		Warnings: data.Warnings,
	}
	return http.StatusOK, resp
}

type AgentUniverseView struct {
	AgentID           string                   `json:"agent_id"`
	Name              string                   `json:"name"`
	Layer             string                   `json:"layer"`
	Universe          []string                 `json:"universe"`
	ScreeningCriteria domain.ScreeningCriteria `json:"screening_criteria"`
}

// UniverseOverlapResponse is the API response for universe overlap.
type UniverseOverlapResponse struct {
	Agents   []AgentUniverseView       `json:"agents"`
	Matrix   map[string]map[string]int `json:"matrix"`
	Warnings []string                  `json:"warnings"`
}

func (h *Handlers) HandleDarwinianStatus(r *http.Request) (int, any) {
	data, err := h.Svc.LoadDarwinianStatus()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load darwinian status: %v", err)}
	}
	return http.StatusOK, data
}

func (h *Handlers) HandleDarwinianTrend(r *http.Request) (int, any) {
	limit, err := parseLimit(r, 30, 200)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}
	points, err := h.Svc.LoadDarwinianHistory(limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load darwinian history: %v", err)}
	}
	return http.StatusOK, map[string]any{"points": points}
}

// HandleBaselineInfo returns baseline policy version and promotion history.
func (h *Handlers) HandleBaselineInfo(r *http.Request) (int, any) {
	baselineDir := filepath.Join(h.Svc.WorkDir, "data", "state")
	baselinePath := filepath.Join(baselineDir, "baseline_policy.json")
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return http.StatusOK, map[string]any{"version": "unknown", "updated_at": "", "history": []any{}}
	}
	var policy struct {
		Version   string `json:"version"`
		UpdatedAt string `json:"updated_at"`
		History   []any  `json:"history"`
	}
	_ = json.Unmarshal(data, &policy)
	return http.StatusOK, map[string]any{
		"version":    policy.Version,
		"updated_at": policy.UpdatedAt,
		"history":    policy.History,
	}
}

func (h *Handlers) HandleRegimeHistory(r *http.Request) (int, any) {
	q := r.URL.Query()
	// `limit` keeps the legacy row-limit semantics; `days` is interpreted as a
	// calendar window (today-days+1 .. today). This aligns the endpoint with
	// /api/narrative/stress-index/history and /api/geopolitical/history.
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return http.StatusBadRequest, map[string]string{"error": "invalid limit: must be positive integer"}
		}
		if limit > 365 {
			limit = 365
		}
		data, err := h.Svc.LoadRegimeHistory(limit)
		if err != nil {
			return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load regime history: %v", err)}
		}
		return http.StatusOK, data
	}

	days := 30
	if rawDays := strings.TrimSpace(q.Get("days")); rawDays != "" {
		v, err := strconv.Atoi(rawDays)
		if err != nil || v <= 0 {
			return http.StatusBadRequest, map[string]string{"error": "invalid days: must be positive integer"}
		}
		if v > 365 {
			v = 365
		}
		days = v
	}
	data, err := h.Svc.LoadRegimeHistoryDays(days)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load regime history: %v", err)}
	}
	return http.StatusOK, data
}
