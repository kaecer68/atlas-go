package experiment

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// ExperimentInboxItem represents a single experiment in the inbox.
type ExperimentInboxItem struct {
	ExperimentID         string                  `json:"experiment_id"`
	TargetAgentID        string                  `json:"target_agent_id"`
	Skill                string                  `json:"skill"`
	MutationType         string                  `json:"mutation_type"`
	MutationSummary      string                  `json:"mutation_summary,omitempty"`
	Status               domain.ExperimentStatus `json:"status"`
	BaselineValue        float64                 `json:"baseline_value"`
	CandidateValue       float64                 `json:"candidate_value"`
	BaselineMonetaryNTD  float64                 `json:"baseline_monetary_ntd,omitempty"`
	CandidateMonetaryNTD float64                 `json:"candidate_monetary_ntd,omitempty"`
	CandidatePath        string                  `json:"candidate_path"`
	RejectReason         string                  `json:"reject_reason,omitempty"`
	RecordedAt           time.Time               `json:"recorded_at"`
}

// ExperimentInboxResponse groups experiments by actionable state.
type ExperimentInboxResponse struct {
	PendingJudges   []ExperimentInboxItem `json:"pending_judges"`
	PendingPromotes []ExperimentInboxItem `json:"pending_promotes"`
	RecentHistory   []ExperimentInboxItem `json:"recent_history"`
	BaselineVersion int                   `json:"baseline_version"`
	Items           []ExperimentInboxItem `json:"items"`
}

// Handlers holds the dependencies for experiment lifecycle handlers.
type Handlers struct {
	BaselinePath string
	LedgerDir    string
	WorkDir      string
}

// RegisterRoutes mounts experiment lifecycle endpoints.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/experiment/promote", shared.AdminPost(h.HandlePromote))
	mux.Handle("POST /api/experiment/revert", shared.AdminPost(h.HandleRevert))
	mux.Handle("POST /api/experiment/judge", shared.AdminPost(h.HandleJudge))
	mux.Handle("GET /api/experiment/diff", shared.Get(h.HandleDiff))
	mux.Handle("GET /api/dashboard/experiment-inbox", shared.Get(h.HandleInbox))
	mux.Handle("GET /api/experiment/history", shared.Get(h.HandleHistory))
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
func (h *Handlers) HandlePromote(r *http.Request) (int, any) {
	var req struct {
		ResultPath string `json:"result_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if req.ResultPath == "" {
		return http.StatusBadRequest, map[string]string{"error": "result_path required"}
	}
	mgr := baseline.NewManager(h.BaselinePath)
	policy, err := mgr.PromoteResult(req.ResultPath)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("promote failed: %v", err)}
	}
	return http.StatusOK, map[string]any{"success": true, "version": policy.Version}
}

// HandleRevert reverts baseline to a previous version.
func (h *Handlers) HandleRevert(r *http.Request) (int, any) {
	var req struct {
		Type         string `json:"type"`
		Version      int    `json:"version"`
		ExperimentID string `json:"experiment_id"`
		Reason       string `json:"reason"`
		DryRun       bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	target := baseline.RevertTarget{Type: baseline.RevertType(req.Type), Version: req.Version, ExperimentID: req.ExperimentID}
	mgr := baseline.NewManager(h.BaselinePath)
	result, err := mgr.Revert(target, req.Reason, req.DryRun)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("revert failed: %v", err)}
	}
	return http.StatusOK, result
}

// HandleHistory returns the promotion history.
func (h *Handlers) HandleHistory(r *http.Request) (int, any) {
	mgr := baseline.NewManager(h.BaselinePath)
	history, err := mgr.GetPromotionHistory()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load history: %v", err)}
	}
	return http.StatusOK, map[string]any{"history": promotionHistoryToAPI(history)}
}

// HandleJudge evaluates an experiment result.
func (h *Handlers) HandleJudge(r *http.Request) (int, any) {
	var req struct {
		ExperimentID string `json:"experiment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if req.ExperimentID == "" {
		return http.StatusBadRequest, map[string]string{"error": "experiment_id required"}
	}
	if err := shared.ValidateExperimentID(req.ExperimentID); err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}

	resultPath := filepath.Join(h.LedgerDir, "experiments", req.ExperimentID+".json")
	if _, err := os.Stat(resultPath); err != nil {
		return http.StatusNotFound, map[string]string{"error": "experiment result not found"}
	}

	replayPath := config.GetReplayDataPath(h.WorkDir)
	judge := experiment.NewJudge(ledger.NewStore(h.LedgerDir).(ledger.ExperimentStore), replayPath, h.BaselinePath)
	result, err := judge.Evaluate(resultPath)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("judge failed: %v", err)}
	}

	return http.StatusOK, map[string]any{
		"success":    true,
		"status":     result.Experiment.Status,
		"baseline":   result.Experiment.BaselineValue,
		"candidate":  result.Experiment.CandidateValue,
		"experiment": result.Experiment,
	}
}

// HandleDiff returns the prompt diff for an experiment.
func (h *Handlers) HandleDiff(r *http.Request) (int, any) {
	experimentID := strings.TrimSpace(r.URL.Query().Get("experiment_id"))
	if experimentID == "" {
		return http.StatusBadRequest, map[string]string{"error": "experiment_id required"}
	}
	if err := shared.ValidateExperimentID(experimentID); err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}

	resultPath := filepath.Join(h.LedgerDir, "experiments", experimentID+".json")
	bytes, err := os.ReadFile(resultPath)
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "experiment result not found"}
	}
	var result domain.PromptExperimentResult
	if err := json.Unmarshal(bytes, &result); err != nil {
		return http.StatusInternalServerError, map[string]string{"error": "invalid experiment result"}
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
		return http.StatusInternalServerError, map[string]string{"error": "cannot read candidate prompt"}
	}

	resp := map[string]any{
		"baseline_prompt":  baselinePrompt,
		"candidate_prompt": string(candidateBytes),
		"target_agent_id":  result.Experiment.TargetAgentID,
		"skill":            result.Experiment.Skill,
		// SK-22: judge-collected metrics are already stored in the same ledger
		// JSON; expose them so the endpoint matches its "metrics comparison"
		// contract instead of returning a prompt-only diff.
		"acceptance_metric": result.Experiment.AcceptanceMetric,
		"baseline_value":    result.Experiment.BaselineValue,
		"candidate_value":   result.Experiment.CandidateValue,
	}
	if result.EvalMetrics != nil {
		resp["eval_metrics"] = result.EvalMetrics
	}
	return http.StatusOK, resp
}

// HandleInbox returns the experiment inbox with pending judges, promotes, and recent history.
func (h *Handlers) HandleInbox(r *http.Request) (int, any) {
	policy, err := baseline.Load(h.BaselinePath)
	if err != nil {
		policy = baseline.DefaultPolicy()
	}

	experimentsDir := filepath.Join(h.LedgerDir, "experiments")
	if _, err := experiment.ExpireOldExperiments(experimentsDir, experiment.DefaultExperimentTTL); err != nil { //nolint:staticcheck
	}

	ledgerExperiments := loadLedgerExperiments(filepath.Join(h.LedgerDir, "experiments.jsonl"))

	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return http.StatusOK, ExperimentInboxResponse{BaselineVersion: policy.Version}
		}
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("read experiments dir: %v", err)}
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
			logging.Warn("experiment_handler", "read_experiment_file_failed", logging.Err(err))
			continue
		}
		var result domain.PromptExperimentResult
		if err := json.Unmarshal(bytes, &result); err != nil {
			logging.Warn("experiment_handler", "parse_experiment_file_failed", logging.Err(err))
			continue
		}

		item := ExperimentInboxItem{
			ExperimentID:         result.Experiment.ID,
			TargetAgentID:        result.Experiment.TargetAgentID,
			Skill:                result.Experiment.Skill,
			MutationType:         result.Experiment.MutationType,
			MutationSummary:      buildMutationSummary(policy, result),
			Status:               result.Experiment.Status,
			BaselineValue:        result.Experiment.BaselineValue,
			CandidateValue:       result.Experiment.CandidateValue,
			BaselineMonetaryNTD:  result.Experiment.BaselineMonetaryNTD,
			CandidateMonetaryNTD: result.Experiment.CandidateMonetaryNTD,
			CandidatePath:        result.CandidatePrompt,
			RejectReason:         result.Experiment.RevertReason,
			RecordedAt:           result.RecordedAt,
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

	for _, item := range ledgerExperiments {
		if item.Status == domain.ExperimentPlanned || item.Status == domain.ExperimentRunning {
			pendingJudges = append(pendingJudges, item)
		} else {
			recentHistory = append(recentHistory, item)
		}
	}
	// Deduplicate: keep only the latest experiment per agent from ledger
	ledgerByAgent := make(map[string]ExperimentInboxItem)
	for _, item := range pendingJudges {
		if _, ok := ledgerByAgent[item.TargetAgentID]; !ok {
			ledgerByAgent[item.TargetAgentID] = item
		}
	}
	pendingJudges = make([]ExperimentInboxItem, 0, len(ledgerByAgent))
	for _, item := range ledgerByAgent {
		pendingJudges = append(pendingJudges, item)
	}
	if len(pendingJudges) > 100 {
		pendingJudges = pendingJudges[:100]
	}

	allItems := make([]ExperimentInboxItem, 0, len(pendingJudges)+len(pendingPromotes)+len(recentHistory))
	allItems = append(allItems, pendingJudges...)
	allItems = append(allItems, pendingPromotes...)
	allItems = append(allItems, recentHistory...)

	return http.StatusOK, ExperimentInboxResponse{
		PendingJudges:   pendingJudges,
		PendingPromotes: pendingPromotes,
		RecentHistory:   recentHistory,
		BaselineVersion: policy.Version,
		Items:           allItems,
	}
}

func loadLedgerExperiments(path string) []ExperimentInboxItem {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var items []ExperimentInboxItem
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		var rec domain.ExperimentRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		items = append(items, ExperimentInboxItem{
			ExperimentID:   rec.ID,
			TargetAgentID:  rec.TargetAgentID,
			Skill:          rec.Skill,
			MutationType:   rec.MutationType,
			Status:         rec.Status,
			BaselineValue:  rec.BaselineValue,
			CandidateValue: rec.CandidateValue,
			RecordedAt:     time.Now(),
		})
	}
	return items
}
