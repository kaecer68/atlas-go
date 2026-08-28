package server

import (
	"strings"
	"testing"
)

// TestMCPAutoDescConformance verifies MCP tool description invariants
// against the auto-generated description map (auto-desc.gen.go/json).
// These checks prevent the kind of description drift, ambiguous naming,
// and missing fields that caused E-01 through E-06 (#1265 master).
//
// Adding a new tool? go generate then check this test still passes.
func TestMCPAutoDescConformance(t *testing.T) {
	keys := autoDescKeys()
	if len(keys) == 0 {
		t.Fatal("autoDescKeys() returned 0 tools — auto-desc.gen.json may be empty or unparsed")
	}

	// Invariant 1: every auto-desc entry must have a non-empty description.
	for _, name := range keys {
		d := autoDesc(name)
		if d == nil {
			t.Errorf("autoDesc(%q) returned nil — tool exists in keys but not in map", name)
			continue
		}
		if strings.TrimSpace(d.Description) == "" {
			t.Errorf("tool %q has empty description in auto-desc.gen.json — every tool must document its domain and data contract", name)
		}
	}

	// Invariant 2: tool count must be stable. Drift > 5 from the
	// expected ~119 base suggests missing go generate.
	if n := len(keys); n < 107 || n > 123 {
		t.Errorf("auto-desc tool count %d outside expected range [107, 123]; run go generate ./cmd/atlas-mcp/... if intentional", n)
	}

	// Invariant 3: domain disambiguation cross-references (#1266).
	descBy := func(name string) string {
		if d := autoDesc(name); d != nil {
			return d.Description
		}
		return ""
	}

	recDesc := descBy("get_recommendations")
	if recDesc != "" {
		if !strings.Contains(strings.ToLower(recDesc), "portfolio") {
			t.Errorf("get_recommendations description must mention 'portfolio'; got: %q", recDesc)
		}
		if !strings.Contains(recDesc, "strategy_list_active") {
			t.Errorf("get_recommendations description must cross-reference strategy_list_active; got: %q", recDesc)
		}
	}

	for _, name := range []string{"strategy_list_active", "strategy_ranker"} {
		d := descBy(name)
		if d != "" {
			if !strings.Contains(strings.ToLower(d), "technique") && !strings.Contains(strings.ToLower(d), "signal") {
				t.Errorf("%s description must mention 'technique' or 'signal'; got: %q", name, d)
			}
			if !strings.Contains(d, "get_recommendations") {
				t.Errorf("%s description must cross-reference get_recommendations; got: %q", name, d)
			}
		}
	}

	// Invariant 4: quickstart tools must exist and have descriptions.
	quickstart := []string{
		"system_get_health",
		"macro_get_snapshot_latest",
		"get_recommendations",
		"mcp_quickstart",
	}
	toolSet := make(map[string]bool, len(keys))
	for _, k := range keys {
		toolSet[k] = true
	}
	for _, qs := range quickstart {
		if !toolSet[qs] {
			t.Errorf("quickstart tool %q not found in auto-desc map", qs)
		} else if d := autoDesc(qs); d != nil && strings.TrimSpace(d.Description) == "" {
			t.Errorf("quickstart tool %q has empty description", qs)
		}
	}

	// Invariant 5: tool names use snake_case (no uppercase).
	for _, name := range keys {
		if strings.ToLower(name) != name {
			t.Errorf("tool %q has upper-case characters — use snake_case", name)
		}
	}

	// Invariant 4: narrative_get_models must describe the actual payload —
	// investment models with sector bets — not a detector/forecaster list.
	// Guards against regressing to the misleading "regime detector, flow
	// forecaster" description (capital_models audit, 2026-08-13).
	if mDesc := descBy("narrative_get_models"); mDesc != "" {
		low := strings.ToLower(mDesc)
		if !strings.Contains(low, "favored") && !strings.Contains(low, "avoided") &&
			!strings.Contains(low, "investment") && !strings.Contains(low, "sector") {
			t.Errorf("narrative_get_models description must describe investment models / sector bets; got: %q", mDesc)
		}
		if strings.Contains(low, "regime detector") || strings.Contains(low, "flow forecaster") {
			t.Errorf("narrative_get_models description must not claim to be a detector/forecaster list; got: %q", mDesc)
		}
	}

	t.Logf("conformance: %d tools in auto-desc map, all invariants passed", len(keys))
}
