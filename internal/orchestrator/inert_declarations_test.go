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

// TestLLMSectorAgentLoop_IsPassThrough pins N-P1 (issue #1944): the plugin is a
// pass-through even when a driver IS wired, so no production path can carry an
// LLM sector opinion into a recommendation today.
//
// The source-level half matters more than the constant: the constant is a claim,
// and the claim is only honest while ProcessRecommendations cannot write back
// into its input. The test therefore reads the function's own source and fails
// if a future edit adds a write to `recs` (an element assignment or an append),
// or returns anything other than the input slice.
func TestLLMSectorAgentLoop_IsPassThrough(t *testing.T) {
	if LLMSectorAgentLoopActive {
		t.Fatal("LLMSectorAgentLoopActive must be false while ProcessRecommendations returns its input unchanged")
	}

	src, err := os.ReadFile("plugin_adapters.go")
	if err != nil {
		t.Fatalf("read plugin_adapters.go: %v", err)
	}
	const sig = "func (p *llmSectorAgentsPlugin) ProcessRecommendations("
	start := strings.Index(string(src), sig)
	if start < 0 {
		t.Fatalf("ProcessRecommendations not found in plugin_adapters.go — this test's subject moved; update it with the code")
	}
	rest := string(src)[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal("could not delimit ProcessRecommendations: gofmt keeps the closing brace at column 0")
	}
	body := rest[:end]

	// Every return in the function hands back the input slice, and nothing writes
	// into it.
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "return ") {
			continue
		}
		if trimmed != "return recs" {
			t.Errorf("ProcessRecommendations returns %q; while the loop is declared inactive every path must return the input unchanged", trimmed)
		}
	}
	if !strings.Contains(body, "return recs") {
		t.Error("ProcessRecommendations no longer returns the input slice at all")
	}
	for _, write := range []string{"recs[", "append(recs", "recs = "} {
		if strings.Contains(body, write) {
			t.Errorf("ProcessRecommendations contains %q: the plugin can now change its input, so LLMSectorAgentLoopActive must be revisited (and the #1971 observation window respected)", write)
		}
	}

	// The driver injection point must have no production caller either: with the
	// flag defaulting to false, WithLLMSectorAgents is only reachable from
	// factory.go behind LLM_SECTOR_AGENTS_ENABLED.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	const gatedCall = "WithLLMSectorAgents("
	// The injection point may exist in production, but only behind the opt-in
	// flag: LLM_SECTOR_AGENTS_ENABLED defaults to false
	// (internal/config/config.go), so a default deployment does not even register
	// the plugin. "A gated caller" is a different fact from "no caller", and
	// conflating the two is how a flag default gets mistaken for wiring.
	var ungated []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Base(path) == "system_plugins.go" {
			return nil // the definition itself
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		// Only real code counts: the file's own comments mention the call
		// (`system.WithLLMSectorAgents(driver)` in a doc comment), and matching
		// those would report a gate failure that does not exist.
		offset := 0
		for _, line := range strings.Split(string(data), "\n") {
			lineStart := offset
			offset += len(line) + 1
			if !strings.Contains(line, gatedCall) || strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			// The call must sit inside an LLMSectorAgentsEnabled block: look back a
			// bounded window for the gate (comments sit between the if and the call).
			from := lineStart - 800
			if from < 0 {
				from = 0
			}
			if !strings.Contains(string(data)[from:lineStart], "LLMSectorAgentsEnabled") {
				ungated = append(ungated, path)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(ungated) > 0 {
		t.Errorf("WithLLMSectorAgents is called WITHOUT the LLMSectorAgentsEnabled gate in %v; an ungated call would register the pass-through plugin in every deployment, so LLMSectorAgentLoopActive (and docs/reference/inert-registry.md) must be revisited in the same change", ungated)
	}
}
