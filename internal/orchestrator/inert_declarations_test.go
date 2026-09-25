package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestAgentPrimaryMetricsWired_IsFalse pins the inert-by-design status of
// domain.AgentSpec.PrimaryMetrics (issue #1944 / I15). The declarations in
// configs/agents.json and SeedRegistry are intent only; flip this expectation
// only together with a real consumer that scores agents on those metrics.
func TestAgentPrimaryMetricsWired_IsFalse(t *testing.T) {
	if AgentPrimaryMetricsWired {
		t.Fatal("AgentPrimaryMetricsWired must be false: no production code reads AgentSpec.PrimaryMetrics")
	}
}

// TestAgentPrimaryMetrics_HasNoNonTestReader is the evidence behind the flag
// above. It walks the repository's Go sources and asserts that PrimaryMetrics
// appears only in writer/clone paths and tests — never in a read that could
// influence a decision. If a real reader is added, this test fails and the flag
// must be revisited in the same change.
func TestAgentPrimaryMetrics_HasNoNonTestReader(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}

	// Allowed non-test occurrences: declarations, seed/clone writes, and the
	// JSON wiring in the config loader. None of them reads the value to make a
	// decision.
	allowedFiles := map[string]bool{
		filepath.Join(root, "internal", "domain", "recommendation", "recommendation.go"): true, // field declaration
		filepath.Join(root, "internal", "orchestrator", "registry.go"):                   true, // hardcoded seed write
		filepath.Join(root, "internal", "spawning", "agent_factory.go"):                  true, // clone/inherit write
	}

	var offenders []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil || !strings.Contains(string(src), "PrimaryMetrics") {
			return nil
		}
		if !allowedFiles[path] {
			offenders = append(offenders, path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(offenders) > 0 {
		t.Fatalf("PrimaryMetrics gained a non-test reader in %v; AgentPrimaryMetricsWired must be revisited", offenders)
	}
}

// TestDriverAdapterReserved_HasNoNonTestCaller is the evidence for
// LLMSectorAgentDriverWired (issue #1944 / N-P3). It parses the orchestrator
// package and asserts NewDriverAdapter is never called from production code.
func TestDriverAdapterReserved_HasNoNonTestCaller(t *testing.T) {
	if LLMSectorAgentDriverWired {
		t.Fatal("LLMSectorAgentDriverWired must be false while NewDriverAdapter has no production caller")
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}

	fset := token.NewFileSet()
	var offenders []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "NewDriverAdapter" {
				offenders = append(offenders, fset.Position(call.Pos()).String())
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(offenders) > 0 {
		t.Fatalf("NewDriverAdapter gained a production caller at %v; LLMSectorAgentDriverWired must be revisited", offenders)
	}
}

// TestSeedRegistryPrimaryMetricsAreDeclarationsOnly documents that the seed
// registry still carries the (unread) declarations, so removing the field is a
// deliberate schema decision rather than an accident.
func TestSeedRegistryPrimaryMetricsAreDeclarationsOnly(t *testing.T) {
	reg := SeedRegistry()
	withMetrics := 0
	for _, a := range reg.Agents {
		if len(a.PrimaryMetrics) > 0 {
			withMetrics++
		}
	}
	if withMetrics == 0 {
		t.Skip("SeedRegistry no longer declares PrimaryMetrics; remove the inert flag with the field")
	}
	var _ []string = domain.AgentSpec{}.PrimaryMetrics
}
