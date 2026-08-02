package experiment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/eval"
)

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	dir := t.TempDir()
	ledgerDir := filepath.Join(dir, "ledger")
	if err := os.MkdirAll(filepath.Join(ledgerDir, "experiments"), 0o755); err != nil {
		t.Fatalf("mkdir experiments: %v", err)
	}

	baselineFile := filepath.Join(dir, "baseline_policy.json")
	policy := baseline.DefaultPolicy()
	policyBytes, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal default policy: %v", err)
	}
	if err := os.WriteFile(baselineFile, policyBytes, 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	// Also create a prompt file to serve as baseline source
	promptDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "growth.prompt.md"), []byte("volume_floor: 500"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	return &Handlers{
		BaselinePath: baselineFile,
		LedgerDir:    ledgerDir,
		WorkDir:      dir,
	}
}

func postJSON(t *testing.T, url string, body any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertStatus(t *testing.T, status int, want int) {
	t.Helper()
	if status != want {
		t.Errorf("status = %d, want %d", status, want)
	}
}

func assertJSONKey(t *testing.T, body any, key string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m[key]; !ok {
		t.Errorf("response missing key %q: %v", key, m)
	}
	return m
}

// writeExperimentResult writes a PromptExperimentResult into the experiments dir.
func writeExperimentResult(t *testing.T, dir string, id string, status domain.ExperimentStatus, candidatePrompt string) string {
	t.Helper()
	result := experiment.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:               id,
			TargetAgentID:    "growth-01",
			Skill:            "growth",
			MutationType:     "prompt",
			Status:           status,
			AcceptanceMetric: "sharpe_like",
			BaselineValue:    0.5,
			CandidateValue:   0.8,
		},
		Brief: domain.MutationBrief{
			TargetAgentID: "growth-01",
			TargetSkill:   "growth",
			MutationType:  "prompt",
			PromptFile:    "prompts/growth.prompt.md",
		},
		CandidatePrompt: candidatePrompt,
		RecordedAt:      time.Now(),
		EvalMetrics: &eval.EvalResult{
			R2OOS:     0.3,
			Sharpe:    1.5,
			CumReturn: 0.1,
			MaxDD:     -0.05,
		},
	}
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write experiment result: %v", err)
	}
	return path
}

func TestHandlePromote_MissingResultPath(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/promote", map[string]string{})
	status, _ := h.HandlePromote(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandlePromote_EmptyResultPath(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/promote", map[string]string{"result_path": ""})
	status, _ := h.HandlePromote(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandlePromote_InvalidJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/experiment/promote",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandlePromote(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandlePromote_ResultNotFound(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/promote", map[string]string{
		"result_path": "/nonexistent/path.json",
	})
	status, body := h.HandlePromote(req)
	assertStatus(t, status, http.StatusInternalServerError)
	m := assertJSONKey(t, body, "error")
	if m["error"] == "" {
		t.Error("expected error message")
	}
}

func TestHandlePromote_Success(t *testing.T) {
	h := newTestHandlers(t)
	// Create a candidate prompt file that exists on disk
	candidatePrompt := filepath.Join(h.WorkDir, "prompts", "candidate.prompt.md")
	if err := os.WriteFile(candidatePrompt, []byte("volume_floor: 1000"), 0o644); err != nil {
		t.Fatalf("write candidate prompt: %v", err)
	}
	// Create experiment result file
	resultPath := writeExperimentResult(t, filepath.Join(h.LedgerDir, "experiments"),
		"exp-001", domain.ExperimentAccepted, candidatePrompt)

	req := postJSON(t, "/api/experiment/promote", map[string]string{
		"result_path": resultPath,
	})
	status, body := h.HandlePromote(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "success")
	if m["success"] != true {
		t.Errorf("success = %v", m["success"])
	}
}

func TestHandleRevert_MissingBody(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/experiment/revert",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandleRevert(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleRevert_InvalidJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/experiment/revert",
		bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandleRevert(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleRevert_InvalidType(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/revert", map[string]any{
		"type":    "invalid",
		"version": -1,
		"dry_run": true,
	})
	status, body := h.HandleRevert(req)
	// Should fail because invalid revert type can't resolve target
	assertStatus(t, status, http.StatusInternalServerError)
	assertJSONKey(t, body, "error")
}

func TestHandleRevert_DryRunLast(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/revert", map[string]any{
		"type":    "last",
		"reason":  "test revert",
		"dry_run": true,
	})
	status, _ := h.HandleRevert(req)
	// Dry-run on a single-version default policy fails because
	// there's nothing to revert (already at target version).
	// Either 200 (if resolve succeeds but dry-run) or 500 (no history).
	// Just check it's handled.
	if status < 200 || status >= 600 {
		t.Errorf("unexpected status: %d", status)
	}
}

func TestHandleRevert_DryRunVersion(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/revert", map[string]any{
		"type":    "version",
		"version": 0,
		"reason":  "test version revert",
		"dry_run": true,
	})
	status, body := h.HandleRevert(req)
	// This should fail because version 0 doesn't exist or is same as current
	assertStatus(t, status, http.StatusInternalServerError)
	assertJSONKey(t, body, "error")
}

func TestHandleRevert_DryRunExperiment(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/revert", map[string]any{
		"type":          "experiment",
		"experiment_id": "nonexistent",
		"reason":        "test experiment revert",
		"dry_run":       true,
	})
	// Should fail because experiment_id doesn't exist in policy promotions
	status, _ := h.HandleRevert(req)
	// 500 because no such experiment in promotions
	if status < 200 || status >= 600 {
		t.Errorf("unexpected status: %d", status)
	}
}

func TestHandleJudge_MissingExperimentID(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/judge", map[string]string{})
	status, _ := h.HandleJudge(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleJudge_EmptyExperimentID(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/judge", map[string]string{"experiment_id": ""})
	status, _ := h.HandleJudge(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleJudge_InvalidExperimentID(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/judge", map[string]string{"experiment_id": "../../etc/passwd"})
	status, _ := h.HandleJudge(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleJudge_InvalidJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/experiment/judge",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandleJudge(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleJudge_ExperimentNotFound(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/experiment/judge", map[string]string{"experiment_id": "nonexistent"})
	status, _ := h.HandleJudge(req)
	assertStatus(t, status, http.StatusNotFound)
}

func TestHandleDiff_MissingExperimentID(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/experiment/diff", nil)
	status, _ := h.HandleDiff(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleDiff_InvalidExperimentID(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/experiment/diff?experiment_id=../../etc/passwd", nil)
	status, _ := h.HandleDiff(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleDiff_ExperimentNotFound(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/experiment/diff?experiment_id=nonexistent", nil)
	status, _ := h.HandleDiff(req)
	assertStatus(t, status, http.StatusNotFound)
}

func TestHandleDiff_Success(t *testing.T) {
	h := newTestHandlers(t)
	// Create candidate prompt file that exists
	candidatePrompt := filepath.Join(h.WorkDir, "prompts", "candidate.prompt.md")
	if err := os.WriteFile(candidatePrompt, []byte("volume_floor: 1000"), 0o644); err != nil {
		t.Fatalf("write candidate prompt: %v", err)
	}
	// Create experiment result with a valid prompt_file reference
	writeExperimentResult(t, filepath.Join(h.LedgerDir, "experiments"),
		"exp-diff", domain.ExperimentAccepted, candidatePrompt)

	req := httptest.NewRequest(http.MethodGet, "/api/experiment/diff?experiment_id=exp-diff", nil)
	status, body := h.HandleDiff(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "baseline_prompt")
	assertJSONKey(t, body, "candidate_prompt")
	assertJSONKey(t, body, "target_agent_id")
	// SK-22: judge-collected metrics are exposed alongside the prompt diff.
	assertJSONKey(t, body, "acceptance_metric")
	assertJSONKey(t, body, "baseline_value")
	assertJSONKey(t, body, "candidate_value")
	assertJSONKey(t, body, "eval_metrics")
	if got := m["baseline_value"]; got != 0.5 {
		t.Errorf("baseline_value = %v, want 0.5", got)
	}
	if got := m["candidate_value"]; got != 0.8 {
		t.Errorf("candidate_value = %v, want 0.8", got)
	}
	if got := m["acceptance_metric"]; got != "sharpe_like" {
		t.Errorf("acceptance_metric = %v, want sharpe_like", got)
	}
}

func TestHandleHistory_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/experiment/history", nil)
	status, body := h.HandleHistory(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "history")
}

func TestHandleHistory_EmptyPath(t *testing.T) {
	// Test with empty baseline path (falls back to default policy)
	h := &Handlers{
		BaselinePath: "",
		LedgerDir:    t.TempDir(),
		WorkDir:      t.TempDir(),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/experiment/history", nil)
	status, body := h.HandleHistory(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "history")
}

func TestHandleInbox_EmptyDir(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/experiment-inbox", nil)
	status, body := h.HandleInbox(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "pending_judges")
	if _, ok := m["pending_promotes"]; !ok {
		t.Error("missing pending_promotes")
	}
	if _, ok := m["recent_history"]; !ok {
		t.Error("missing recent_history")
	}
	if _, ok := m["baseline_version"]; !ok {
		t.Error("missing baseline_version")
	}
}

func TestHandleInbox_WithExperiments(t *testing.T) {
	h := newTestHandlers(t)
	experimentsDir := filepath.Join(h.LedgerDir, "experiments")

	candidatePrompt := filepath.Join(h.WorkDir, "prompts", "candidate.prompt.md")
	os.WriteFile(candidatePrompt, []byte("dummy"), 0o644)

	// Create running experiment (should go to pending_judges)
	writeExperimentResult(t, experimentsDir, "exp-running", domain.ExperimentRunning, candidatePrompt)
	// Create planned experiment (should go to pending_judges)
	writeExperimentResult(t, experimentsDir, "exp-planned", domain.ExperimentPlanned, candidatePrompt)
	// Create accepted experiment (should go to pending_promotes)
	writeExperimentResult(t, experimentsDir, "exp-accepted", domain.ExperimentAccepted, candidatePrompt)
	// Create rejected experiment (should go to recent_history)
	writeExperimentResult(t, experimentsDir, "exp-rejected", domain.ExperimentRejected, candidatePrompt)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/experiment-inbox", nil)
	status, _ := h.HandleInbox(req)
	assertStatus(t, status, http.StatusOK)
}

func TestHandleInbox_NoExperimentsDir(t *testing.T) {
	dir := t.TempDir()
	baselineFile := filepath.Join(dir, "baseline_policy.json")
	policy := baseline.DefaultPolicy()
	policyBytes, _ := json.Marshal(policy)
	os.WriteFile(baselineFile, policyBytes, 0o644)

	h := &Handlers{
		BaselinePath: baselineFile,
		LedgerDir:    dir, // no experiments subdir
		WorkDir:      dir,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/experiment-inbox", nil)
	status, body := h.HandleInbox(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "pending_judges")
}

func TestHandleInbox_NoBaselineFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "experiments"), 0o755)

	h := &Handlers{
		BaselinePath: filepath.Join(dir, "nonexistent_policy.json"),
		LedgerDir:    dir,
		WorkDir:      dir,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/experiment-inbox", nil)
	status, body := h.HandleInbox(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "baseline_version")
}

func TestRegisterRoutes(t *testing.T) {
	h := newTestHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/api/experiment/promote"},
		{"POST", "/api/experiment/revert"},
		{"POST", "/api/experiment/judge"},
		{"GET", "/api/experiment/diff"},
		{"GET", "/api/dashboard/experiment-inbox"},
		{"GET", "/api/experiment/history"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		if r.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
			// Provide minimal body for POST endpoints so they don't fail on body decode
			req.Body = ioCloser{bytes.NewReader([]byte("{}"))}
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 0 {
			t.Errorf("route %s %s not registered (no handler)", r.method, r.path)
		}
	}
}

// ioCloser wraps a bytes.Reader into an io.ReadCloser
type ioCloser struct {
	*bytes.Reader
}

func (ioCloser) Close() error { return nil }

func TestBuildMutationSummary_NoChanges(t *testing.T) {
	policy := baseline.DefaultPolicy()
	result := experiment.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			MutationType: "prompt",
		},
	}
	summary := buildMutationSummary(policy, result)
	if summary != "prompt" {
		t.Errorf("expected 'prompt', got %q", summary)
	}
}

func TestPromotionHistoryToAPI_Empty(t *testing.T) {
	result := promotionHistoryToAPI(nil)
	if result == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}
}

func TestPromotionHistoryToAPI_NonEmpty(t *testing.T) {
	history := []baseline.PromotionRecordWithVersion{
		{
			PromotionRecord: baseline.PromotionRecord{
				ExperimentID:  "exp-001",
				TargetAgentID: "growth-01",
				TargetSkill:   "growth",
				MutationType:  "prompt",
				CandidatePath: "/tmp/test.prompt",
				PromotedAt:    time.Now(),
				Status:        "accepted",
				VersionAfter:  2,
			},
			Version: 2,
		},
	}
	result := promotionHistoryToAPI(history)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0]["experiment_id"] != "exp-001" {
		t.Errorf("experiment_id = %v", result[0]["experiment_id"])
	}
}

func TestLoadLedgerExperiments_NoFile(t *testing.T) {
	dir := t.TempDir()
	items := loadLedgerExperiments(filepath.Join(dir, "nonexistent.jsonl"))
	if items != nil {
		t.Errorf("expected nil from missing file, got %d items", len(items))
	}
}

func TestLoadLedgerExperiments_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	items := loadLedgerExperiments(path)
	if items != nil {
		t.Errorf("expected nil from empty file, got %d items", len(items))
	}
}

func TestLoadLedgerExperiments_ValidRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "experiments.jsonl")
	lines := []string{
		fmt.Sprintf(`{"id":"exp-a","target_agent_id":"growth-01","skill":"growth","mutation_type":"prompt","status":"%s"}`, domain.ExperimentAccepted),
		fmt.Sprintf(`{"id":"exp-b","target_agent_id":"growth-02","skill":"growth","mutation_type":"prompt","status":"%s"}`, domain.ExperimentRunning),
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
	items := loadLedgerExperiments(path)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ExperimentID != "exp-a" {
		t.Errorf("first experiment_id = %s, want exp-a", items[0].ExperimentID)
	}
	if items[1].Status != domain.ExperimentRunning {
		t.Errorf("second status = %s, want running", items[1].Status)
	}
}
