package control

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Svc *service.ControlService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/control/pause-agent", h.HandlePauseAgent)
	mux.HandleFunc("/api/control/resume-agent", h.HandleResumeAgent)
	mux.HandleFunc("/api/control/set-model-weight", h.HandleSetModelWeight)
	mux.HandleFunc("/api/control/sector-ban", h.HandleSectorBan)
	mux.HandleFunc("/api/control/approve-recommendation", h.HandleApproveRecommendation)
	mux.HandleFunc("/api/control/reject-recommendation", h.HandleRejectRecommendation)
	mux.HandleFunc("/api/control/audit-log", h.HandleAuditLog)
	mux.HandleFunc("/api/control/active-overrides", h.HandleActiveOverrides)
	mux.HandleFunc("/api/agents/health", h.HandleAgentHealth)
}

func (h *Handlers) HandlePauseAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AgentID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	intervention := h.Svc.CreateIntervention("pause_agent", req.AgentID, req.Reason, req.Operator, 0)
	if err := h.Svc.RecordIntervention(intervention); err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleResumeAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AgentID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	intervention := h.Svc.CreateIntervention("resume_agent", req.AgentID, req.Reason, req.Operator, 0)
	if err := h.Svc.RecordIntervention(intervention); err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleAgentHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	agents, mutedCount, err := h.Svc.GetAgentHealth()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("get agent health: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"agents":      agents,
		"total":       len(agents),
		"muted_count": mutedCount,
	})
}

func (h *Handlers) HandleSetModelWeight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ModelID  string  `json:"model_id"`
		Weight   float64 `json:"weight"`
		Reason   string  `json:"reason"`
		Operator string  `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.ModelID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "model_id required")
		return
	}
	intervention := h.Svc.CreateIntervention("set_model_weight", req.ModelID, req.Reason, req.Operator, req.Weight)
	if err := h.Svc.RecordIntervention(intervention); err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleSectorBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Sector   string `json:"sector"`
		Banned   bool   `json:"banned"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Sector == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "sector required")
		return
	}
	interventionType := "sector_unban"
	if req.Banned {
		interventionType = "sector_ban"
	}
	intervention := h.Svc.CreateIntervention(interventionType, req.Sector, req.Reason, req.Operator, 0)
	if err := h.Svc.RecordIntervention(intervention); err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleApproveRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Symbol   string `json:"symbol"`
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	intervention := h.Svc.CreateIntervention("approve_rec", req.Symbol, req.Reason, req.Operator, 0)
	if err := h.Svc.RecordIntervention(intervention); err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleRejectRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Symbol   string `json:"symbol"`
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	intervention := h.Svc.CreateIntervention("reject_rec", req.Symbol, req.Reason, req.Operator, 0)
	if err := h.Svc.RecordIntervention(intervention); err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleAuditLog(w http.ResponseWriter, r *http.Request) {
	interventions, err := h.Svc.LoadInterventions()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load interventions: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"interventions": interventions})
}

func (h *Handlers) HandleActiveOverrides(w http.ResponseWriter, r *http.Request) {
	pausedAgents, bannedSectors, modelWeights := h.Svc.GetActiveOverrides()
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"paused_agents":  pausedAgents,
		"banned_sectors": bannedSectors,
		"model_weights":  modelWeights,
	})
}
