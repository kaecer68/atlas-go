package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func createTestReplayCSV(t *testing.T, path string) {
	content := `Date,Code,Name,TradeVolume,Open,High,Low,Close
2026-03-25,0050,Test,1000000,100,110,90,105
2026-03-26,0050,Test,1000000,105,115,95,110
2026-03-27,0050,Test,1000000,110,120,100,115
2026-03-30,0050,Test,1000000,115,125,105,120
2026-03-31,0050,Test,1000000,120,130,110,125
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test replay: %v", err)
	}
}

func TestExecuteCreatesCandidatePrompt(t *testing.T) {
	dir := t.TempDir()
	executor := NewExecutor(ledger.NewStore(dir).(ledger.FullStore), "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	promptPath := filepath.Join(root, "prompts/agents/growth_momentum.md")
	briefPath := filepath.Join(dir, "brief.json")
	replayPath := filepath.Join(dir, "test-replay.csv")
	createTestReplayCSV(t, replayPath)

	brief := domain.MutationBrief{
		WindowID:            "window-20260325-20260331",
		TargetAgentID:       "growth-momentum-01",
		TargetSkill:         "growth_momentum",
		TargetLayer:         domain.LayerStyle,
		PromptFile:          promptPath,
		MutationType:        "prompt_tightening",
		FailurePattern:      "Repeated negative outcomes.",
		Hypothesis:          "Tightening the prompt should improve quality.",
		AcceptanceMetric:    "sharpe_like",
		AcceptanceGates:     []string{"improve_sharpe_like"},
		ForbiddenActions:    []string{"illiquid_breakout_chasing"},
		RequiredSkills:      []string{"growth_momentum", "technical_breakout"},
		ObservedWindowCount: 2,
		MaturityLevel:       "level_1_exploratory",
		IterationGuidance:   []string{"Change one bounded behavior only."},
		RecommendedWindow:   "test-window",
		GeneratedAt:         time.Now(),
	}
	bytes, err := json.Marshal(brief)
	if err != nil {
		t.Fatalf("marshal brief: %v", err)
	}
	if err := os.WriteFile(briefPath, bytes, 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	result, err := executor.Run(briefPath, replayPath)
	if err != nil {
		t.Fatalf("run experiment: %v", err)
	}
	if result.CandidatePrompt == "" {
		t.Fatalf("expected candidate prompt path")
	}
	if filepath.Base(result.CandidatePrompt) != "v2.md" {
		t.Fatalf("expected v2 prompt file")
	}
	promptBytes, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		t.Fatalf("read candidate prompt: %v", err)
	}
	if string(promptBytes) == "" {
		t.Fatalf("expected candidate prompt contents")
	}
}

func TestExecuteUsesRiskRuleTemplate(t *testing.T) {
	dir := t.TempDir()
	executor := NewExecutor(ledger.NewStore(dir).(ledger.FullStore), "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	promptPath := filepath.Join(root, "prompts/agents/cro_risk.md")
	briefPath := filepath.Join(dir, "brief-risk.json")
	replayPath := filepath.Join(dir, "test-replay.csv")
	createTestReplayCSV(t, replayPath)

	brief := domain.MutationBrief{
		WindowID:            "window-20260325-20260331",
		TargetAgentID:       "cro-01",
		TargetSkill:         "cro_risk",
		TargetLayer:         domain.LayerControl,
		PromptFile:          promptPath,
		MutationType:        "risk_rule_change",
		Hypothesis:          "Tighten control rules.",
		AcceptanceMetric:    "sharpe_like",
		AcceptanceGates:     []string{"improve_sharpe_like"},
		ForbiddenActions:    []string{"risk_limit_override"},
		RequiredSkills:      []string{"cro_risk"},
		ObservedWindowCount: 4,
		MaturityLevel:       "level_2_window_validated",
		IterationGuidance:   []string{"Treat control-layer mutations as conservative governance changes."},
		RecommendedWindow:   "test-window",
		GeneratedAt:         time.Now(),
	}
	bytes, err := json.Marshal(brief)
	if err != nil {
		t.Fatalf("marshal brief: %v", err)
	}
	if err := os.WriteFile(briefPath, bytes, 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	result, err := executor.Run(briefPath, replayPath)
	if err != nil {
		t.Fatalf("run experiment: %v", err)
	}
	promptBytes, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		t.Fatalf("read candidate prompt: %v", err)
	}
	if !strings.Contains(string(promptBytes), "Risk Rule Change Proposal") {
		t.Fatalf("expected risk rule template artifact")
	}
}

func TestExecuteUsesPortfolioConstraintTemplate(t *testing.T) {
	dir := t.TempDir()
	executor := NewExecutor(ledger.NewStore(dir).(ledger.FullStore), "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	promptPath := filepath.Join(root, "prompts/agents/cio_seed.md")
	briefPath := filepath.Join(dir, "brief-portfolio.json")
	replayPath := filepath.Join(dir, "test-replay.csv")
	createTestReplayCSV(t, replayPath)

	brief := domain.MutationBrief{
		WindowID:            "window-20260325-20260331",
		TargetAgentID:       "cio-01",
		TargetSkill:         "cio_portfolio",
		TargetLayer:         domain.LayerControl,
		PromptFile:          promptPath,
		MutationType:        "portfolio_constraint_revision",
		Hypothesis:          "Tighten portfolio governance.",
		AcceptanceMetric:    "sharpe_like",
		AcceptanceGates:     []string{"improve_sharpe_like"},
		ForbiddenActions:    []string{"cro_bypass"},
		RequiredSkills:      []string{"cio_portfolio"},
		ObservedWindowCount: 5,
		MaturityLevel:       "level_3_regime_aware",
		IterationGuidance:   []string{"Do not widen risk limits unless replay evidence is unusually strong."},
		RecommendedWindow:   "test-window",
		GeneratedAt:         time.Now(),
	}
	bytes, err := json.Marshal(brief)
	if err != nil {
		t.Fatalf("marshal brief: %v", err)
	}
	if err := os.WriteFile(briefPath, bytes, 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	result, err := executor.Run(briefPath, replayPath)
	if err != nil {
		t.Fatalf("run experiment: %v", err)
	}
	promptBytes, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		t.Fatalf("read candidate prompt: %v", err)
	}
	if !strings.Contains(string(promptBytes), "Portfolio Constraint Optimization Proposal") {
		t.Fatalf("expected portfolio constraint template artifact")
	}
}

func TestExecuteRejectsInvalidBriefContract(t *testing.T) {
	dir := t.TempDir()
	executor := NewExecutor(ledger.NewStore(dir).(ledger.FullStore), "")
	briefPath := filepath.Join(dir, "brief-invalid.json")
	replayPath := filepath.Join(dir, "test-replay.csv")
	createTestReplayCSV(t, replayPath)

	brief := domain.MutationBrief{
		WindowID:         "window-20260325-20260331",
		TargetAgentID:    "growth-momentum-01",
		TargetSkill:      "growth_momentum",
		TargetLayer:      domain.LayerStyle,
		PromptFile:       "prompts/agents/growth_momentum.md",
		MutationType:     "not_supported",
		AcceptanceMetric: "sharpe_like",
		AcceptanceGates:  []string{"improve_sharpe_like"},
	}
	bytes, err := json.Marshal(brief)
	if err != nil {
		t.Fatalf("marshal brief: %v", err)
	}
	if err := os.WriteFile(briefPath, bytes, 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	if _, err := executor.Run(briefPath, replayPath); err == nil {
		t.Fatalf("expected invalid brief contract to fail")
	}
}
