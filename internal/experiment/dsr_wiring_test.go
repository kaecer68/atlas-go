package experiment

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// TestDSRGate_ReachableThroughJSONStore is the test that was MISSING while the
// gate was dead in production. Every pre-existing DSR test injects
// scriptableTrialStore — a mock that DOES implement ledger.TrialSharpeLoader —
// so they stayed green while cmd/atlas wired the Judge with
// `ledger.NewStore(cfg.LedgerDir)`, the JSON-file store, which did not
// implement that interface. The type assertion therefore failed on every
// candidate and deflatedSharpeGate returned "skip" for its entire life, even
// though 282 real trials (10 (agent,mutation) groups, all K>=2, max K=56) sat
// in <baseDir>/experiments/*.json unread. Fail-open here was a wiring hole.
func TestDSRGate_ReachableThroughJSONStore(t *testing.T) {
	dir := t.TempDir()
	raw := ledger.NewStore(dir)

	// 1. The production store must satisfy the loader interface. Without this
	//    method the DSR gate is silently dead — fail-open by omission.
	loader, ok := raw.(ledger.TrialSharpeLoader)
	if !ok {
		t.Fatal("ledger.Store must implement ledger.TrialSharpeLoader: cmd/atlas builds the Judge from this store, so a missing method makes the DSR gate silently dead")
	}
	expStore, ok := raw.(ledger.ExperimentStore)
	if !ok {
		t.Fatal("ledger.Store must implement ledger.ExperimentStore")
	}

	// 2. Two same-scope historical trials whose Sharpes swing wildly: a
	//    qualified-looking candidate on top of them is luck, not skill.
	const target, mutation = "agent-1", "prompt_tightening"
	for i, sr := range []float64{-5, 5, -5, 5} {
		id := fmt.Sprintf("hist-%d", i)
		if err := expStore.RecordPromptExperimentResult(id, domain.PromptExperimentResult{
			Experiment: domain.ExperimentRecord{
				ID:            id,
				TargetAgentID: target,
				MutationType:  mutation,
			},
			EvalMetrics: &eval.EvalResult{Sharpe: sr},
		}); err != nil {
			t.Fatalf("record trial %d: %v", i, err)
		}
	}

	trials, err := loader.LoadTrialSharpes()
	if err != nil {
		t.Fatalf("LoadTrialSharpes: %v", err)
	}
	if len(trials) != 4 {
		t.Fatalf("LoadTrialSharpes returned %d trials, want 4 (all carry EvalMetrics)", len(trials))
	}

	// 3. End-to-end through the real Judge: the gate must EVALUATE (not skip)
	//    and must reject the lucky candidate.
	j := testJudge()
	j.store = expStore
	accepted, note := j.passesAcceptance(dsrTestResult(target, mutation), nil)
	if accepted {
		t.Fatalf("DSR gate must reject a lucky candidate once the loader is reachable; note=%q", note)
	}
	if !strings.Contains(note, "deflated Sharpe") {
		t.Fatalf("rejection must come from the DSR gate, got %q", note)
	}
}
