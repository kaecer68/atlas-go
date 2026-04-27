package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

type Handlers struct {
	WorkDir       string
	LedgerDir     string
	HealthManager *portfolio.AgentHealthManager
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (h *Handlers) HandlePauseAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AgentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	store := ledger.NewStore(h.LedgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-pause-%s-%d", req.AgentID, time.Now().UnixNano()),
		Type:          "pause_agent",
		TargetAgentID: req.AgentID,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleResumeAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AgentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	store := ledger.NewStore(h.LedgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-resume-%s-%d", req.AgentID, time.Now().UnixNano()),
		Type:          "resume_agent",
		TargetAgentID: req.AgentID,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleAgentHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.HealthManager == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"agents":      []*portfolio.AgentHealth{},
			"total":       0,
			"muted_count": 0,
		})
		return
	}

	registryPath := filepath.Join(h.WorkDir, "configs/agents.json")
	registry, err := orchestrator.LoadRegistry(registryPath)
	if err != nil {
		registry = orchestrator.SeedRegistry()
	}

	agents := make([]*portfolio.AgentHealth, 0)
	mutedCount := 0

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		h := h.HealthManager.GetHealth(agent.ID)
		if h == nil {
			h = &portfolio.AgentHealth{
				AgentID: agent.ID,
				Status:  portfolio.HealthStatusHealthy,
			}
		}
		agents = append(agents, h)
		if h.Status == portfolio.HealthStatusMuted {
			mutedCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agents":      agents,
		"total":       len(agents),
		"muted_count": mutedCount,
	})
}

func (h *Handlers) HandleSetModelWeight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ModelID  string  `json:"model_id"`
		Weight   float64 `json:"weight"`
		Reason   string  `json:"reason"`
		Operator string  `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.ModelID == "" {
		writeJSONError(w, http.StatusBadRequest, "model_id required")
		return
	}
	store := ledger.NewStore(h.LedgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-model-%s-%d", req.ModelID, time.Now().UnixNano()),
		Type:          "set_model_weight",
		TargetModelID: req.ModelID,
		Value:         req.Weight,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleSectorBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Sector   string `json:"sector"`
		Banned   bool   `json:"banned"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Sector == "" {
		writeJSONError(w, http.StatusBadRequest, "sector required")
		return
	}
	store := ledger.NewStore(h.LedgerDir)
	interventionType := "sector_unban"
	if req.Banned {
		interventionType = "sector_ban"
	}
	intervention := domain.HumanIntervention{
		ID:           fmt.Sprintf("int-sector-%s-%d", req.Sector, time.Now().UnixNano()),
		Type:         interventionType,
		TargetSector: req.Sector,
		Reason:       req.Reason,
		Operator:     req.Operator,
		RecordedAt:   time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleApproveRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Symbol   string `json:"symbol"`
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	store := ledger.NewStore(h.LedgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-approve-%s-%d", req.Symbol, time.Now().UnixNano()),
		Type:          "approve_rec",
		TargetSymbol:  req.Symbol,
		TargetAgentID: req.AgentID,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleRejectRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Symbol   string `json:"symbol"`
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	store := ledger.NewStore(h.LedgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-reject-%s-%d", req.Symbol, time.Now().UnixNano()),
		Type:          "reject_rec",
		TargetSymbol:  req.Symbol,
		TargetAgentID: req.AgentID,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (h *Handlers) HandleAuditLog(w http.ResponseWriter, r *http.Request) {
	store := ledger.NewStore(h.LedgerDir)
	interventions, err := store.LoadHumanInterventions()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load interventions: %v", err))
		return
	}
	slices.Reverse(interventions)
	writeJSON(w, http.StatusOK, map[string]any{"interventions": interventions})
}

func (h *Handlers) HandleActiveOverrides(w http.ResponseWriter, r *http.Request) {
	store := ledger.NewStore(h.LedgerDir)
	interventions, err := store.LoadHumanInterventions()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load interventions: %v", err))
		return
	}

	pausedAgents := make(map[string]bool)
	bannedSectors := make(map[string]bool)
	modelWeights := make(map[string]float64)

	for _, iv := range interventions {
		switch iv.Type {
		case "pause_agent":
			pausedAgents[iv.TargetAgentID] = true
		case "resume_agent":
			delete(pausedAgents, iv.TargetAgentID)
		case "sector_ban":
			bannedSectors[iv.TargetSector] = true
		case "sector_unban":
			delete(bannedSectors, iv.TargetSector)
		case "set_model_weight":
			modelWeights[iv.TargetModelID] = iv.Value
		default:
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"paused_agents":  mapKeys(pausedAgents),
		"banned_sectors": mapKeys(bannedSectors),
		"model_weights":  modelWeights,
	})
}
