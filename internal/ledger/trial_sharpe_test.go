package ledger

import (
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
)

// TestSQLiteStore_LoadTrialSharpes 讀回歷史 trial 的 Sharpe：同範圍多筆、
// 他範圍一筆、無 EvalMetrics 者跳過。
func TestSQLiteStore_LoadTrialSharpes(t *testing.T) {
	store, err := newSQLiteFullStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	seed := []struct {
		id       string
		agent    string
		mutation string
		sharpe   float64
		hasEval  bool
	}{
		{"exp-a", "agent-1", "prompt_tightening", 1.5, true},
		{"exp-b", "agent-1", "prompt_tightening", 2.5, true},
		{"exp-c", "agent-2", "prompt_tightening", 0.5, true},
		{"exp-d", "agent-1", "prompt_tightening", 0, false},
	}
	for _, s := range seed {
		result := domain.PromptExperimentResult{
			Experiment: domain.ExperimentRecord{
				ID:            s.id,
				TargetAgentID: s.agent,
				MutationType:  s.mutation,
			},
		}
		if s.hasEval {
			result.EvalMetrics = &eval.EvalResult{Sharpe: s.sharpe}
		}
		if err := store.RecordPromptExperimentResult(s.id, result); err != nil {
			t.Fatalf("record %s: %v", s.id, err)
		}
	}

	got, err := store.LoadTrialSharpes()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("trial sharpes = %d, want 3 (exp-d without EvalMetrics skipped): %+v", len(got), got)
	}
	byID := map[string]TrialSharpe{}
	for _, ts := range got {
		byID[ts.ExperimentID] = ts
	}
	if byID["exp-a"].Sharpe != 1.5 || byID["exp-a"].TargetAgentID != "agent-1" || byID["exp-a"].MutationType != "prompt_tightening" {
		t.Errorf("exp-a = %+v, want sharpe 1.5 agent-1 prompt_tightening", byID["exp-a"])
	}
	if _, ok := byID["exp-d"]; ok {
		t.Error("exp-d without EvalMetrics must be skipped")
	}
}
