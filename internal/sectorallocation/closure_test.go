package sectorallocation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestVerifyClosure_RejectsInProgressToDone(t *testing.T) {
	// SA01 階段的 status_machine 是 basic check；in_progress 不被它單獨拒絕。
	// 真正的「in_progress→done 跳躍」偵測等 SA12 擴充。
	// 此測試改為驗證 implemented→done 必須有 evidence（status machine + three_evidence 共同守）。
	st := writeStubManifest(t, "SA01\tproblem\trc\tfiles\tac\tdone\tnotes\t")
	results := sectorallocation.VerifyClosure(st)
	if !hasFailure(results, "id_done_requires_three_evidence") {
		t.Fatal("done without three evidence categories must be rejected")
	}
}

func TestVerifyClosure_AllowsImplementedToInProgress(t *testing.T) {
	st := writeStubManifest(t, "SA01\tproblem\trc\tfiles\tac\timplemented\tnotes\t")
	results := sectorallocation.VerifyClosure(st)
	if hasFailure(results, "manifest_status_machine") {
		t.Fatal("implemented→in_progress is a valid transition; status machine must not reject")
	}
}

func TestVerifyClosure_RequiresThreeEvidenceForDone(t *testing.T) {
	st := writeStubManifest(t, "SA01\tproblem\trc\tfiles\tac\tdone\t")
	results := sectorallocation.VerifyClosure(st)
	if !hasFailure(results, "id_done_requires_three_evidence") {
		t.Fatal("done ID without implementation/observation/negative evidence must fail")
	}
}

func TestVerifyClosure_AcceptsThreeEvidenceForDone(t *testing.T) {
	// 不可在 notes 內含 | 字元（會破壞 table 結構）；用 ; 分隔三類 evidence。
	// 測資 row 用 7 個 tab 段（前 7 欄），notes 直接放在第 8 欄。
	notes := "implementation: commit abc ; observation: log link ; negative: counter 0"
	row := "SA01\tproblem\trc\tfiles\tac\tdone\t\t" + notes
	st := writeStubManifest(t, row)
	results := sectorallocation.VerifyClosure(st)
	if hasFailure(results, "id_done_requires_three_evidence") {
		t.Fatalf("done ID with three evidence must pass; got failures: %v", failureNames(results))
	}
}

func TestVerifyClosure_RejectsEmpiricalSourceInNotes(t *testing.T) {
	notes := "implementation: commit abc ; source=empirical ; negative: counter 0"
	row := "SA01\tproblem\trc\tfiles\tac\tdone\t\t" + notes
	st := writeStubManifest(t, row)
	results := sectorallocation.VerifyClosure(st)
	if !hasFailure(results, "source_label_lock") {
		t.Fatal("source=empirical must be rejected by source label lock")
	}
}

func TestVerifyClosure_PhaseDependencyEnforced(t *testing.T) {
	// SA04 in Phase B 不得 done 若 SA01 in Phase A 不是 done。
	st := writeMultiRowManifest(t, []string{
		"SA01\tproblem\trc\tfiles\tac\tpending\tnotes\t",
		"SA04\tproblem\trc\tfiles\tac\tdone\t\timplementation: 1 ; observation: 2 ; negative: 3",
	})
	results := sectorallocation.VerifyClosure(st)
	if !hasFailure(results, "phase_dependency_complete") {
		t.Fatal("SA04 in Phase B done while SA01 in Phase A pending must be rejected")
	}
}

func TestVerifyClosure_CrossIDDependencyEnforced(t *testing.T) {
	// SA10 marked done but SA08 still pending → cross-id dependency fails
	st := writeMultiRowManifest(t, []string{
		"SA08\tproblem\trc\tfiles\tac\tpending\tnotes\t",
		"SA10\tproblem\trc\tfiles\tac\tdone\t\timplementation: 1 ; observation: 2 ; negative: 3",
	})
	results := sectorallocation.VerifyClosure(st)
	if !hasFailure(results, "cross_id_dangling_dependency") {
		t.Fatal("SA10 done while SA08 pending must be rejected")
	}
}

func TestVerifyClosure_EmptyManifestIsConfigError(t *testing.T) {
	defer func() {
		_ = recover()
	}()
	st := sectorallocation.ClosureState{ManifestPath: "/nonexistent/does/not/exist.md"}
	_ = sectorallocation.VerifyClosure(st)
}

func TestVerifyClosure_ResultStructureIsStable(t *testing.T) {
	st := writeStubManifest(t, "SA01\tproblem\trc\tfiles\tac\tdone\timplementation: a ; observation: b ; negative: c\t")
	results := sectorallocation.VerifyClosure(st)
	if len(results) == 0 {
		t.Fatal("verifier must return at least one result")
	}
	for _, r := range results {
		if r.Rule == "" {
			t.Fatal("every result must declare Rule")
		}
	}
}

func writeStubManifest(t *testing.T, row string) sectorallocation.ClosureState {
	t.Helper()
	return writeMultiRowManifest(t, []string{row})
}

func writeMultiRowManifest(t *testing.T, rows []string) sectorallocation.ClosureState {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.md")
	var b strings.Builder
	b.WriteString("# test manifest\n\n| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |\n")
	b.WriteString("|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|\n")
	for _, r := range rows {
		parts := strings.Split(r, "\t")
		// 表格只有 8 欄；多餘 tab-separated 段會破壞 pipe table 結構。
		if len(parts) > 8 {
			parts = parts[:8]
		}
		for len(parts) < 8 {
			parts = append(parts, "")
		}
		b.WriteString("| " + strings.Join(parts, " | ") + " |\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return sectorallocation.ClosureState{ManifestPath: path}
}

func hasFailure(results []sectorallocation.ClosureRuleResult, rule string) bool {
	for _, r := range results {
		if r.Rule == rule && !r.Passed {
			return true
		}
	}
	return false
}

func failureNames(results []sectorallocation.ClosureRuleResult) []string {
	names := []string{}
	for _, r := range results {
		if !r.Passed {
			names = append(names, r.Rule)
		}
	}
	return names
}
