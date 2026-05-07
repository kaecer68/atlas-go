package experiment

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// ExperimentInboxItem represents a single experiment in the inbox.
type ExperimentInboxItem struct {
	ExperimentID    string                  `json:"experiment_id"`
	TargetAgentID   string                  `json:"target_agent_id"`
	Skill           string                  `json:"skill"`
	MutationType    string                  `json:"mutation_type"`
	MutationSummary string                  `json:"mutation_summary,omitempty"`
	Status          domain.ExperimentStatus `json:"status"`
	BaselineValue   float64                 `json:"baseline_value"`
	CandidateValue  float64                 `json:"candidate_value"`
	CandidatePath   string                  `json:"candidate_path"`
	RejectReason    string                  `json:"reject_reason,omitempty"`
	RecordedAt      time.Time               `json:"recorded_at"`
}

// ExperimentInboxResponse groups experiments by actionable state.
type ExperimentInboxResponse struct {
	PendingJudges   []ExperimentInboxItem `json:"pending_judges"`
	PendingPromotes []ExperimentInboxItem `json:"pending_promotes"`
	RecentHistory   []ExperimentInboxItem `json:"recent_history"`
	BaselineVersion int                   `json:"baseline_version"`
}

// Handlers holds the dependencies for experiment lifecycle handlers.
type Handlers struct {
	BaselinePath string
	LedgerDir    string
	WorkDir      string
}

// RegisterRoutes mounts experiment lifecycle endpoints.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/experiment/promote", h.HandlePromote)
	mux.HandleFunc("/api/experiment/revert", h.HandleRevert)
	mux.HandleFunc("/api/experiment/history", h.HandleHistory)
	mux.HandleFunc("/api/experiment/judge", h.HandleJudge)
	mux.HandleFunc("/api/experiment/diff", h.HandleDiff)
	mux.HandleFunc("/api/dashboard/experiment-inbox", h.HandleInbox)
}

func buildMutationSummary(policy baseline.Policy, result domain.PromptExperimentResult) string {
	baselinePrompt := baseline.ResolvePromptOverride(policy, result.Experiment.TargetAgentID, result.Experiment.Skill)
	if baselinePrompt == "" {
		sourcePrompt, err := os.ReadFile(result.Brief.PromptFile)
		if err == nil {
			baselinePrompt = string(sourcePrompt)
		}
	}

	baselineCtrl, _ := domain.ExtractPromptControl(baselinePrompt)
	candidateBytes, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		return result.Experiment.MutationType
	}
	candidateCtrl, _ := domain.ExtractPromptControl(string(candidateBytes))

	parts := make([]string, 0, 4)
	add := func(name string, base, cand int64) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %d→%d", name, base, cand))
		}
	}
	addInt := func(name string, base, cand int) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %d→%d", name, base, cand))
		}
	}
	addBool := func(name string, base, cand bool) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %t→%t", name, base, cand))
		}
	}

	add("volume_floor", baselineCtrl.VolumeFloor, candidateCtrl.VolumeFloor)
	addInt("volume_downgrade", baselineCtrl.VolumeDowngrade, candidateCtrl.VolumeDowngrade)
	addInt("close_strength_boost", baselineCtrl.CloseStrengthBoost, candidateCtrl.CloseStrengthBoost)
	add("hard_reject_volume", baselineCtrl.HardRejectVolume, candidateCtrl.HardRejectVolume)
	addInt("conviction_floor", baselineCtrl.ConvictionFloor, candidateCtrl.ConvictionFloor)
	addInt("volume_boost", baselineCtrl.VolumeBoost, candidateCtrl.VolumeBoost)
	addInt("neutral_penalty_reduction", baselineCtrl.NeutralPenaltyReduction, candidateCtrl.NeutralPenaltyReduction)
	addBool("require_trend", baselineCtrl.RequireTrend, candidateCtrl.RequireTrend)

	if len(parts) == 0 {
		return result.Experiment.MutationType
	}
	return strings.Join(parts, ", ")
}

func promotionHistoryToAPI(history []baseline.PromotionRecordWithVersion) []map[string]any {
	result := make([]map[string]any, len(history))
	for i, h := range history {
		result[i] = map[string]any{
			"experiment_id":   h.ExperimentID,
			"target_agent_id": h.TargetAgentID,
			"target_skill":    h.TargetSkill,
			"mutation_type":   h.MutationType,
			"candidate_path":  h.CandidatePath,
			"promoted_at":     h.PromotedAt,
			"status":          h.Status,
			"version_after":   h.VersionAfter,
			"version":         h.Version,
		}
	}
	return result
}

// HandlePromote promotes an experiment result to baseline.
func (h *Handlers) HandlePromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ResultPath string `json:"result_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.ResultPath == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "result_path required")
		return
	}
	mgr := baseline.NewManager(h.BaselinePath)
	policy, err := mgr.PromoteResult(req.ResultPath)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("promote failed: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "version": policy.Version})
}

// HandleRevert reverts baseline to a previous version.
func (h *Handlers) HandleRevert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Type         string `json:"type"`
		Version      int    `json:"version"`
		ExperimentID string `json:"experiment_id"`
		Reason       string `json:"reason"`
		DryRun       bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	target := baseline.RevertTarget{Type: baseline.RevertType(req.Type), Version: req.Version, ExperimentID: req.ExperimentID}
	mgr := baseline.NewManager(h.BaselinePath)
	result, err := mgr.Revert(target, req.Reason, req.DryRun)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("revert failed: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, result)
}

// HandleHistory returns the promotion history.
func (h *Handlers) HandleHistory(w http.ResponseWriter, r *http.Request) {
	mgr := baseline.NewManager(h.BaselinePath)
	history, err := mgr.GetPromotionHistory()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load history: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"history": promotionHistoryToAPI(history)})
}

// HandleJudge evaluates an experiment result.
func (h *Handlers) HandleJudge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ExperimentID string `json:"experiment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.ExperimentID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "experiment_id required")
		return
	}

	resultPath := filepath.Join(h.LedgerDir, "experiments", req.ExperimentID+".json")
	if _, err := os.Stat(resultPath); err != nil {
		shared.WriteJSONError(w, http.StatusNotFound, "experiment result not found")
		return
	}

	replayPath := filepath.Join(h.WorkDir, "data/replay/tw_extended_90days.csv")
	judge := experiment.NewJudge(ledger.NewStore(h.LedgerDir), replayPath, h.BaselinePath)
	result, err := judge.Evaluate(resultPath)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("judge failed: %v", err))
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"status":     result.Experiment.Status,
		"baseline":   result.Experiment.BaselineValue,
		"candidate":  result.Experiment.CandidateValue,
		"experiment": result.Experiment,
	})
}

// HandleDiff returns the prompt diff for an experiment.
func (h *Handlers) HandleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	experimentID := strings.TrimSpace(r.URL.Query().Get("experiment_id"))
	if experimentID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "experiment_id required")
		return
	}

	resultPath := filepath.Join(h.LedgerDir, "experiments", experimentID+".json")
	bytes, err := os.ReadFile(resultPath)
	if err != nil {
		shared.WriteJSONError(w, http.StatusNotFound, "experiment result not found")
		return
	}
	var result domain.PromptExperimentResult
	if err := json.Unmarshal(bytes, &result); err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, "invalid experiment result")
		return
	}

	promptFile := result.Brief.PromptFile
	if !filepath.IsAbs(promptFile) {
		promptFile = filepath.Join(h.WorkDir, promptFile)
	}
	baselineBytes, err := os.ReadFile(promptFile)
	baselinePrompt := ""
	if err == nil {
		baselinePrompt = string(baselineBytes)
	}

	candidateBytes, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, "cannot read candidate prompt")
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"baseline_prompt":  baselinePrompt,
		"candidate_prompt": string(candidateBytes),
		"target_agent_id":  result.Experiment.TargetAgentID,
		"skill":            result.Experiment.Skill,
	})
}

// HandleInbox returns the experiment inbox with pending judges, promotes, and recent history.
func (h *Handlers) HandleInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	policy, err := baseline.Load(h.BaselinePath)
	if err != nil {
		policy = baseline.DefaultPolicy()
	}

	experimentsDir := filepath.Join(h.LedgerDir, "experiments")
	if _, err := experiment.ExpireOldExperiments(experimentsDir, experiment.DefaultExperimentTTL); err != nil {
		// Silently continue
	}

	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			shared.WriteJSON(w, http.StatusOK, ExperimentInboxResponse{BaselineVersion: policy.Version})
			return
		}
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read experiments dir: %v", err))
		return
	}

	pendingJudges := make([]ExperimentInboxItem, 0)
	pendingPromotes := make([]ExperimentInboxItem, 0)
	recentHistory := make([]ExperimentInboxItem, 0)

	promotedIDs := make(map[string]bool)
	for _, pr := range policy.Promotions {
		promotedIDs[pr.ExperimentID] = true
	}

	var allAccepted []ExperimentInboxItem
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(experimentsDir, entry.Name())
		bytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var result domain.PromptExperimentResult
		if err := json.Unmarshal(bytes, &result); err != nil {
			continue
		}

		item := ExperimentInboxItem{
			ExperimentID:    result.Experiment.ID,
			TargetAgentID:   result.Experiment.TargetAgentID,
			Skill:           result.Experiment.Skill,
			MutationType:    result.Experiment.MutationType,
			MutationSummary: buildMutationSummary(policy, result),
			Status:          result.Experiment.Status,
			BaselineValue:   result.Experiment.BaselineValue,
			CandidateValue:  result.Experiment.CandidateValue,
			CandidatePath:   result.CandidatePrompt,
			RejectReason:    result.Experiment.RevertReason,
			RecordedAt:      result.RecordedAt,
		}

		switch result.Experiment.Status {
		case domain.ExperimentRunning, domain.ExperimentPlanned:
			pendingJudges = append(pendingJudges, item)
		case domain.ExperimentAccepted:
			allAccepted = append(allAccepted, item)
		default:
			recentHistory = append(recentHistory, item)
		}
	}

	latestByAgent := make(map[string]ExperimentInboxItem)
	for _, item := range allAccepted {
		existing, ok := latestByAgent[item.TargetAgentID]
		if !ok || item.RecordedAt.After(existing.RecordedAt) {
			latestByAgent[item.TargetAgentID] = item
		}
	}
	for _, item := range allAccepted {
		latest := latestByAgent[item.TargetAgentID]
		if promotedIDs[item.ExperimentID] || item.ExperimentID != latest.ExperimentID {
			recentHistory = append(recentHistory, item)
		} else {
			pendingPromotes = append(pendingPromotes, item)
		}
	}

	slices.SortFunc(pendingJudges, func(a, b ExperimentInboxItem) int {
		if a.RecordedAt.After(b.RecordedAt) {
			return -1
		}
		return 1
	})
	slices.SortFunc(pendingPromotes, func(a, b ExperimentInboxItem) int {
		if a.RecordedAt.After(b.RecordedAt) {
			return -1
		}
		return 1
	})
	slices.SortFunc(recentHistory, func(a, b ExperimentInboxItem) int {
		if a.RecordedAt.After(b.RecordedAt) {
			return -1
		}
		return 1
	})
	if len(recentHistory) > 10 {
		recentHistory = recentHistory[:10]
	}

	shared.WriteJSON(w, http.StatusOK, ExperimentInboxResponse{
		PendingJudges:   pendingJudges,
		PendingPromotes: pendingPromotes,
		RecentHistory:   recentHistory,
		BaselineVersion: policy.Version,
	})
}
