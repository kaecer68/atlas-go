package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWorktreeWorkflowSidecar validates .opencode/worktree-workflow.json exists
// and matches the schema sven1103-agent/opencode-worktree-plugin v0.6.3 expects.
//
// References:
//   - Plugin DEFAULTS:        plugin src/index.js lines 8-13
//   - Sidecar merge:          plugin src/index.js line 518
//   - worktreeRoot template:  plugin src/index.js line 529
//   - protectedBranches set:  plugin src/index.js line 710
func TestWorktreeWorkflowSidecar(t *testing.T) {
	// Locate the sidecar by walking up from this test file's directory.
	// internal/config/worktree_workflow_test.go → ../../.opencode/worktree-workflow.json
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	sidecarPath := filepath.Join(repoRoot, ".opencode", "worktree-workflow.json")

	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("sidecar MUST exist at %s (sven1103 isolation contract): %v", sidecarPath, err)
	}

	var sidecar struct {
		Remote            string   `json:"remote"`
		BranchPrefix      string   `json:"branchPrefix"`
		BaseBranch        string   `json:"baseBranch"`
		WorktreeRoot      string   `json:"worktreeRoot"`
		CleanupMode       string   `json:"cleanupMode"`
		ProtectedBranches []string `json:"protectedBranches"`
	}
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		t.Fatalf("unmarshal sidecar at %s: %v", sidecarPath, err)
	}

	// Field presence checks
	if sidecar.Remote == "" {
		t.Error("remote is required")
	}
	if sidecar.BaseBranch == "" {
		t.Error("baseBranch is required")
	}
	if sidecar.WorktreeRoot == "" {
		t.Error("worktreeRoot is required")
	}
	if sidecar.CleanupMode == "" {
		t.Error("cleanupMode is required")
	}
	if len(sidecar.ProtectedBranches) == 0 {
		t.Error("protectedBranches must not be empty")
	}

	// Enum validation — plugin accepts exactly these two values (src/index.js:383, 525)
	switch sidecar.CleanupMode {
	case "preview", "apply":
	default:
		t.Errorf("cleanupMode must be 'preview' or 'apply', got %q", sidecar.CleanupMode)
	}

	// Path validation: worktreeRoot must be a template (starts with $) or relative.
	// Plugin resolves via path.resolve(repoRoot, worktreeRoot) — an absolute path
	// would escape the repo sibling structure.
	if sidecar.WorktreeRoot != "" && filepath.IsAbs(sidecar.WorktreeRoot) {
		t.Errorf("worktreeRoot must be a template ($REPO/$ROOT/$ROOT_PARENT) or relative path, got absolute %q", sidecar.WorktreeRoot)
	}

	// Safety: baseBranch MUST be in protectedBranches.
	// Plugin src/index.js:710 builds protectedBranches from
	// [defaultBranch, baseBranch, ...config.protectedBranches].
	// If baseBranch isn't listed explicitly, it gets added anyway, but listing
	// it makes the contract visible in the sidecar itself.
	hasBase := false
	for _, b := range sidecar.ProtectedBranches {
		if b == sidecar.BaseBranch {
			hasBase = true
			break
		}
	}
	if !hasBase {
		t.Errorf("protectedBranches must include baseBranch %q (got %v)", sidecar.BaseBranch, sidecar.ProtectedBranches)
	}

	// Documentation: branchPrefix should be set explicitly to make the
	// `wt/<slug>` naming contract visible (plugin defaults to "wt/" if unset,
	// which is the same value but harder to discover).
	if sidecar.BranchPrefix == "" {
		t.Log("branchPrefix not set in sidecar; plugin will fall back to default 'wt/' — explicit is recommended for documentation")
	}
}
