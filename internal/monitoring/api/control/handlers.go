package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Svc *service.ControlService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/control/pause-agent", shared.AdminPost(h.HandlePauseAgent))
	mux.Handle("POST /api/control/resume-agent", shared.AdminPost(h.HandleResumeAgent))
	mux.Handle("POST /api/control/set-model-weight", shared.AdminPost(h.HandleSetModelWeight))
	mux.Handle("POST /api/control/sector-ban", shared.AdminPost(h.HandleSectorBan))
	mux.Handle("POST /api/control/approve-recommendation", shared.Post(h.HandleApproveRecommendation))
	mux.Handle("POST /api/control/reject-recommendation", shared.Post(h.HandleRejectRecommendation))
	mux.Handle("GET /api/control/audit-log", shared.Get(h.HandleAuditLog))
	mux.Handle("GET /api/control/active-overrides", shared.Get(h.HandleActiveOverrides))
	mux.Handle("GET /api/agents/health", shared.Get(h.HandleAgentHealth))
}

func decodeInterventionBody(r *http.Request) (agentID, reason, operator string, _ error) {
	var req struct {
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", "", "", err
	}
	return req.AgentID, req.Reason, req.Operator, nil
}

func (h *Handlers) recordIntervention(interventionType, targetID, reason, operator string) (int, any) {
	if h.Svc == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "control service unavailable"}
	}
	cleanedReason, status, errResp := validateReason(reason)
	if status != 0 {
		return status, errResp
	}
	intervention := h.Svc.CreateIntervention(interventionType, targetID, cleanedReason, operator, 0)
	if err := h.Svc.RecordIntervention(intervention); err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("record intervention: %v", err)}
	}
	return http.StatusOK, map[string]any{"success": true, "intervention": intervention}
}

func validateReason(reason string) (string, int, any) {
	trimmed := trimReasonWhitespace(reason)
	if trimmed == "" {
		return "", http.StatusBadRequest, map[string]string{"error": "reason required"}
	}
	if utf8.RuneCountInString(trimmed) < 4 {
		return "", http.StatusBadRequest, map[string]string{
			"error": "reason must be at least 4 characters (got " + strconv.Itoa(utf8.RuneCountInString(trimmed)) + ")",
		}
	}
	return trimmed, 0, nil
}

func trimReasonWhitespace(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\u3000'
	})
}

func (h *Handlers) HandlePauseAgent(r *http.Request) (int, any) {
	agentID, reason, operator, err := decodeInterventionBody(r)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if agentID == "" {
		return http.StatusBadRequest, map[string]string{"error": "agent_id required"}
	}
	return h.recordIntervention("pause_agent", agentID, reason, operator)
}

func (h *Handlers) HandleResumeAgent(r *http.Request) (int, any) {
	agentID, reason, operator, err := decodeInterventionBody(r)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if agentID == "" {
		return http.StatusBadRequest, map[string]string{"error": "agent_id required"}
	}
	return h.recordIntervention("resume_agent", agentID, reason, operator)
}

func (h *Handlers) HandleSetModelWeight(r *http.Request) (int, any) {
	var req struct {
		ModelID  string  `json:"model_id"`
		Weight   float64 `json:"weight"`
		Reason   string  `json:"reason"`
		Operator string  `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if req.ModelID == "" {
		return http.StatusBadRequest, map[string]string{"error": "model_id required"}
	}
	return h.recordIntervention("set_model_weight", req.ModelID, req.Reason, req.Operator)
}

func (h *Handlers) HandleSectorBan(r *http.Request) (int, any) {
	var req struct {
		Sector   string `json:"sector"`
		Banned   bool   `json:"banned"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if req.Sector == "" {
		return http.StatusBadRequest, map[string]string{"error": "sector required"}
	}
	interventionType := "sector_unban"
	if req.Banned {
		interventionType = "sector_ban"
	}
	return h.recordIntervention(interventionType, req.Sector, req.Reason, req.Operator)
}

func (h *Handlers) HandleApproveRecommendation(r *http.Request) (int, any) {
	var req struct {
		Symbol   string `json:"symbol"`
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	return h.recordIntervention("approve_rec", req.AgentID+":"+req.Symbol, req.Reason, req.Operator)
}

func (h *Handlers) HandleRejectRecommendation(r *http.Request) (int, any) {
	var req struct {
		Symbol   string `json:"symbol"`
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	return h.recordIntervention("reject_rec", req.AgentID+":"+req.Symbol, req.Reason, req.Operator)
}

func (h *Handlers) HandleAgentHealth(r *http.Request) (int, any) {
	if h.Svc == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "control service unavailable"}
	}
	agents, mutedCount, err := h.Svc.GetAgentHealth()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("get agent health: %v", err)}
	}
	return http.StatusOK, map[string]any{
		"agents":      agents,
		"total":       len(agents),
		"muted_count": mutedCount,
	}
}

func (h *Handlers) HandleAuditLog(r *http.Request) (int, any) {
	if h.Svc == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "control service unavailable"}
	}
	interventions, err := h.Svc.LoadInterventions()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load interventions: %v", err)}
	}
	return http.StatusOK, map[string]any{"interventions": interventions}
}

func (h *Handlers) HandleActiveOverrides(r *http.Request) (int, any) {
	if h.Svc == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "control service unavailable"}
	}
	pausedAgents, bannedSectors, modelWeights := h.Svc.GetActiveOverrides()
	return http.StatusOK, map[string]any{
		"paused_agents":  pausedAgents,
		"banned_sectors": bannedSectors,
		"model_weights":  modelWeights,
	}
}
