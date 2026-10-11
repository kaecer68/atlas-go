package experiment

import (
	"math"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// scriptableTrialStore 注入歷史 trial Sharpe；內嵌 nil ExperimentStore
// （passesAcceptance 不呼叫 store 方法，見 judge.go j.store 唯一使用點）。
type scriptableTrialStore struct {
	ledger.ExperimentStore
	sharpes []ledger.TrialSharpe
	loadErr error
}

func (m *scriptableTrialStore) LoadTrialSharpes() ([]ledger.TrialSharpe, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.sharpes, nil
}

// alternatingSharpes 建 N 個 ±5 交替的歷史 Sharpe（截面變異數 ≈25）：
// 過去試驗起伏極大時，當前的好 Sharpe 極可能是運氣。
func alternatingSharpes(target, mutation string, n int) []ledger.TrialSharpe {
	out := make([]ledger.TrialSharpe, 0, n)
	for i := 0; i < n; i++ {
		v := 5.0
		if i%2 == 1 {
			v = -5.0
		}
		out = append(out, ledger.TrialSharpe{ExperimentID: "hist", TargetAgentID: target, MutationType: mutation, Sharpe: v})
	}
	return out
}

// sharpesOf 萃取歷史 Sharpe 序列（dsrVerdict 吃純序列，保持可測性）。
func sharpesOf(trials []ledger.TrialSharpe) []float64 {
	out := make([]float64, 0, len(trials))
	for _, ts := range trials {
		out = append(out, ts.Sharpe)
	}
	return out
}

// tightCandidateReturns 建 63 筆均值 ≈0.0123、標準差 ≈0.02 的候選報酬
// （Sharpe ≈ 9.7；welchTTest 要求每腿 ≥63 筆，見 judge.go）。
func tightCandidateReturns() []float64 {
	out := make([]float64, 0, 63)
	for i := 0; i < 31; i++ {
		out = append(out, 0.032, -0.008)
	}
	return append(out, 0.032)
}

func flatBaselineReturns() []float64 {
	out := make([]float64, 0, 63)
	for i := 0; i < 31; i++ {
		out = append(out, 0.001, -0.001)
	}
	return append(out, 0.001)
}

func dsrTestResult(target, mutation string) domain.PromptExperimentResult {
	return domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			BaselineValue:   0.0100,
			CandidateValue:  0.0105,
			MutationType:    mutation,
			TargetAgentID:   target,
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_1_exploratory",
			TargetAgentID: target,
			MutationType:  mutation,
		},
		BaselineObservations:  63,
		CandidateObservations: 63,
		BaselineReturns:       flatBaselineReturns(),
		CandidateReturns:      tightCandidateReturns(),
		JudgeChecks:           []string{"a", "b", "c", "d", "e", "f"},
	}
}

func dsrTestJudge(sharpes []ledger.TrialSharpe) *Judge {
	j := testJudge()
	j.store = &scriptableTrialStore{sharpes: sharpes}
	return j
}

// TestDSRVerdict_RejectsLuckWhenHistoryVariesWildly 歷史起伏大時當前高
// Sharpe 判為運氣（DSR≈0）；歷史穩定時放行（DSR≈1）；無歷史回 NaN。
func TestDSRVerdict_RejectsLuckWhenHistoryVariesWildly(t *testing.T) {
	returns := tightCandidateReturns()
	dsr, trials := dsrVerdict(returns, sharpesOf(alternatingSharpes("agent-1", "prompt_tightening", 30)))
	if trials != 31 {
		t.Fatalf("trials = %d, want 31 (30 history + current)", trials)
	}
	if !(dsr < 0.05) {
		t.Fatalf("wild-history DSR = %v, want ≈0 (reject)", dsr)
	}

	dsr, trials = dsrVerdict(returns, []float64{1.0, 1.2})
	if trials != 3 {
		t.Fatalf("trials = %d, want 3", trials)
	}
	if !(dsr > 0.95) {
		t.Fatalf("stable-history DSR = %v, want ≈1 (pass)", dsr)
	}

	if dsr, _ := dsrVerdict(returns, nil); !math.IsNaN(dsr) {
		t.Fatalf("empty history DSR = %v, want NaN (skip)", dsr)
	}
}

// TestPassesAcceptance_DSRRejectsLuckInHighVarianceRegime 端到端：
// K=31、歷史變異大 → 即使 Welch 通過也拒絕。
func TestPassesAcceptance_DSRRejectsLuckInHighVarianceRegime(t *testing.T) {
	j := dsrTestJudge(alternatingSharpes("agent-1", "prompt_tightening", 30))
	accepted, note := j.passesAcceptance(dsrTestResult("agent-1", "prompt_tightening"), nil)
	if accepted {
		t.Fatal("high-variance history must reject despite passing Welch")
	}
	if !strings.Contains(note, "deflated Sharpe") {
		t.Fatalf("rejection must come from the DSR gate, got note %q", note)
	}
}

// TestPassesAcceptance_DSRFailOpen 端到端：無歷史（K=1）或 store 無
// loader 時跳過 DSR（既有行為不變）。
func TestPassesAcceptance_DSRFailOpen(t *testing.T) {
	j := dsrTestJudge(nil)
	accepted, note := j.passesAcceptance(dsrTestResult("agent-1", "prompt_tightening"), nil)
	if !accepted {
		t.Fatalf("empty history must fail open, got note %q", note)
	}

	accepted, note = testJudge().passesAcceptance(dsrTestResult("agent-1", "prompt_tightening"), nil)
	if !accepted {
		t.Fatalf("nil store must fail open, got note %q", note)
	}
}
