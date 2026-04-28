package pipeline

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Svc *service.PipelineService
}

func NewHandlers(svc *service.PipelineService) *Handlers {
	return &Handlers{Svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/macro-radar", h.HandleMacroRadar)
	mux.HandleFunc("/api/dashboard/agent-observatory", h.HandleAgentObservatory)
	mux.HandleFunc("/api/dashboard/forecast-vs-reality", h.HandleForecastVsReality)
	mux.HandleFunc("/api/dashboard/recommendation-pipeline", h.HandleRecommendationPipeline)
	mux.HandleFunc("/api/dashboard/sessions", h.HandleSessions)
	mux.HandleFunc("/api/dashboard/universe-overlap", h.HandleUniverseOverlap)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseLimit(r *http.Request, defaultValue, maxValue int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
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
func (h *Handlers) HandleMacroRadar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	data, err := h.Svc.LoadMacroRadar(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load macro radar data: %v", err))
		return
	}
	if data == nil {
		writeJSON(w, http.StatusOK, MacroRadarResponse{})
		return
	}

	resp := MacroRadarResponse{
		SessionID:     data.SessionID,
		Regime:        data.Regime,
		GuardOutcomes: data.GuardOutcomes,
		BrokerRuntime: data.BrokerRuntime,
		RecordedAt:    data.RecordedAt,
	}
	writeJSON(w, http.StatusOK, resp)
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
func (h *Handlers) HandleAgentObservatory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	limit, err := parseLimit(r, 5, 50)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := h.Svc.LoadAgentObservatory(sessionID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load agent observatory: %v", err))
		return
	}

	resp := AgentObservatoryResponse{
		WeakestAgentScorecards: data.WeakestAgentScorecards,
		SessionID:              data.SessionID,
		NextExperimentAgentID:  data.NextExperimentAgentID,
		BrokerRuntime:          data.BrokerRuntime,
		RecordedAt:             data.RecordedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

// AgentObservatoryResponse is the API response for agent observatory.
type AgentObservatoryResponse struct {
	SessionID              string                    `json:"session_id"`
	NextExperimentAgentID  string                    `json:"next_experiment_agent_id"`
	WeakestAgentScorecards []domain.Scorecard        `json:"weakest_agent_scorecards"`
	BrokerRuntime          domain.BrokerRuntimeAudit `json:"broker_runtime"`
	RecordedAt             time.Time                 `json:"recorded_at"`
}

// HandleForecastVsReality handles GET /api/dashboard/forecast-vs-reality.
func (h *Handlers) HandleForecastVsReality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit, err := parseLimit(r, 20, 100)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))

	data, err := h.Svc.LoadForecastVsReality(agentID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load forecast-vs-reality data: %v", err))
		return
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

	resp := ForecastVsRealityResponse{
		Items:         items,
		BrokerRuntime: data.BrokerRuntime,
	}
	writeJSON(w, http.StatusOK, resp)
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
	Items         []ForecastVsRealityItem   `json:"items"`
	BrokerRuntime domain.BrokerRuntimeAudit `json:"broker_runtime"`
}

// HandleRecommendationPipeline handles GET /api/dashboard/recommendation-pipeline.
func (h *Handlers) HandleRecommendationPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	showAll := r.URL.Query().Get("show_all") == "true"
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))

	data, err := h.Svc.LoadRecommendationPipeline(sessionID, showAll)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load recommendation pipeline: %v", err))
		return
	}
	if data == nil {
		writeJSON(w, http.StatusOK, RecommendationPipelineResponse{})
		return
	}

	items := make([]PipelineItem, len(data.Items))
	for i, item := range data.Items {
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
			Reason:              item.Reason,
			Price:               item.Price,
			PassedGuards:        item.PassedGuards,
			GuardReason:         item.GuardReason,
			Tags:                item.Tags,
			RecordedAt:          item.RecordedAt,
			FactorScores:        item.FactorScores,
			ConvictionBreakdown: item.ConvictionBreakdown,
		}
	}

	resp := RecommendationPipelineResponse{
		SessionID:     data.SessionID,
		Regime:        data.Regime,
		Items:         items,
		GuardOutcomes: data.GuardOutcomes,
		ScreenedItems: data.ScreenedItems,
		RecordedAt:    data.RecordedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

// PipelineItem is the API response item for recommendation pipeline.
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
	FactorScores        domain.FactorScores         `json:"factor_scores,omitempty"`
	ConvictionBreakdown *domain.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
}

// RecommendationPipelineResponse is the API response for recommendation pipeline.
type RecommendationPipelineResponse struct {
	SessionID     string                   `json:"session_id"`
	Regime        domain.Regime            `json:"regime"`
	Items         []PipelineItem           `json:"items"`
	GuardOutcomes []domain.GuardOutcome    `json:"guard_outcomes"`
	ScreenedItems []domain.ScreeningReject `json:"screened_items"`
	RecordedAt    time.Time                `json:"recorded_at"`
}

// HandleSessions handles GET /api/dashboard/sessions.
func (h *Handlers) HandleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessions, err := h.Svc.LoadSessions()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load sessions: %v", err))
		return
	}
	if sessions == nil {
		writeJSON(w, http.StatusOK, map[string]any{"sessions": []any{}})
		return
	}

	result := make([]map[string]any, len(sessions))
	for i, s := range sessions {
		result[i] = map[string]any{
			"session_id":    s.SessionID,
			"recorded_at":   s.RecordedAt,
			"regime":        s.Regime,
			"outcome_count": s.OutcomeCount,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": result})
}

// HandleUniverseOverlap handles GET /api/dashboard/universe-overlap.
func (h *Handlers) HandleUniverseOverlap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	data, err := h.Svc.LoadUniverseOverlap()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load universe overlap: %v", err))
		return
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
	writeJSON(w, http.StatusOK, resp)
}

// AgentUniverseView is the API response for agent universe view.
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
