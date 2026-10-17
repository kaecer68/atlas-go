package risk

// Guard for FU-20261016-32: exported risk-semantic symbols that no production
// path consumes.
//
// The audit (docs/specs/exported-risk-symbols-audit-20261016.md) found eight
// exported symbols whose names claim risk control while nothing on the production
// path uses them. Six of them are now declared in their own doc comment as
// DECLARED ONLY / NOT ENFORCED, or as a duplicate that must not be called (see the
// per-row reason below). This test is the machine-checkable half of that
// declaration and couples it to the code in BOTH directions:
//
//   - a non-test production reference appears while wired == false ⇒ red. Wiring
//     one of these changes production risk behavior, so it must update the doc
//     comment, the re-certification conditions, this table and FU-20261016-32 in
//     the same commit.
//   - wired == true while nothing references the symbol ⇒ red. Claiming
//     enforcement that does not exist is the defect this guard exists for.
//   - wiredVia naming a row that is not wired ⇒ red. A symbol whose only
//     caller sits inside its declaring file cannot satisfy the external scan, so
//     it stays wired == false and records that caller instead.
//
// The scan is deliberately textual and deliberately over-triggering (the same
// choice as TestSiliconPhaseWeightMultiplierHasNoConsumer in
// internal/industry/silicon_cycle_wiring_test.go): a production file that merely
// mentions the name counts as a consumer. False-red is the safe direction here.
//
// Deliberately NOT in the table, because the semantic is already enforced
// elsewhere and only the symbol is unused:
//   - GetRegimeMultiplier (internal/sim/dynamic_threshold.go): read-only accessor;
//     the value is applied inside GetThreshold. The unwired half of that engine is
//     GetThreshold itself, which is a different (proposed) change.
//   - ShouldHaltTrading (internal/risk/macro_aware_drawdown.go): byte-identical
//     predicate to the inline `decision.Action >= risk.DrawdownSevere` checks in
//     orchestrator/strategy_evolver.go:200 and :183.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// declaredOnlyLever is one row of the FU-20261016-32 registry.
type declaredOnlyLever struct {
	symbol    string
	defining  string   // file that declares it; excluded from the consumer scan
	extraSkip []string // additional non-test files to skip (declaration-adjacent docs)
	wired     bool     // true = a production consumer is expected
	// wiredVia names the wired symbol this one is reached from, for symbols
	// whose only production caller lives inside the declaring file (and is
	// therefore invisible to the external-reference scan). Such a row keeps
	// wired == false — the scan cannot see the reference — but it must not be
	// reported as unenforced.
	wiredVia string
	reason   string
}

func declaredOnlyLevers() []declaredOnlyLever {
	return []declaredOnlyLever{
		{
			symbol:   "ModulatePosition",
			defining: "internal/orchestrator/industry_cycle_modulator.go",
			reason:   "rescaled position size; production modulates conviction through CollectModulationSteps instead, and ModulatorWiringActive is false",
		},
		{
			symbol:   "ModulateScore",
			defining: "internal/orchestrator/industry_cycle_modulator.go",
			reason:   "rescaled conviction score; same two gates as ModulatePosition",
		},
		{
			symbol:   "CycleAmplifiedMultiplier",
			defining: "internal/narrative/seasonal_bridge.go",
			reason:   "cycle-aware seasonal amplification; only SeasonalMultiplier is wired (via NarrativeSeasonalProvider), and SetCycleCard has no production caller so the card is always nil",
		},
		{
			symbol:   "ApplyRiskFilters",
			defining: "internal/live/agent_runner.go",
			reason:   "would apply the CRO/CIO control layer a second time; the live runner already receives ExecutionInput.FinalRecommendations with the control layer applied upstream",
		},
		{
			symbol:   "SetHaltTrading",
			defining: "internal/live/risk_gate.go",
			reason:   "operator-facing halt setter; the production halt writer is SyncRiskGateFromCircuitBreaker, and no manual-halt control surface or clearing rule exists yet",
		},
		{
			symbol:   "SyncHaltFromCircuitState",
			defining: "internal/live/risk_gate.go",
			wiredVia: "SyncRiskGateFromCircuitBreaker",
			reason:   "live, but its only caller is SyncRiskGateFromCircuitBreaker inside the declaring file, which the external-reference scan excludes; it is not an independent entry point",
		},
		{
			symbol:   "SyncRiskGateFromCircuitBreaker",
			defining: "internal/live/risk_gate.go",
			wired:    true,
			reason:   "Orchestrator.onIntradayCycle mirrors the breaker state after every circuit evaluation; the nil VaR provider keeps the uncalibrated estimate off the reject path",
		},
		{
			symbol:    "GetPositionSizeAdjustment",
			defining:  "internal/risk/macro_aware_drawdown.go",
			extraSkip: []string{"internal/risk/doc.go"},
			reason:    "the drawdown exposure cap never reaches position sizing; MaxExposure is read only by this accessor and CalculatePortfolioAdjustment, neither of which has a caller",
		},
	}
}

func TestExportedRiskLeversHaveNoProductionConsumer(t *testing.T) {
	levers := declaredOnlyLevers()
	root := moduleRoot(t)

	// wiredVia must point at a row that is really wired, otherwise a symbol can
	// claim transitive reachability from a symbol that is itself dead.
	bySymbol := make(map[string]declaredOnlyLever, len(levers))
	for _, lv := range levers {
		bySymbol[lv.symbol] = lv
	}
	for _, lv := range levers {
		if lv.wiredVia == "" {
			continue
		}
		if lv.wired {
			t.Fatalf("%s: wired and wiredVia are mutually exclusive — either the scan sees a consumer, or it does not", lv.symbol)
		}
		via, ok := bySymbol[lv.wiredVia]
		if !ok {
			t.Fatalf("%s: wiredVia names %s, which is not a row of this table (FU-20261016-32)", lv.symbol, lv.wiredVia)
		}
		if !via.wired {
			t.Fatalf("%s: wiredVia names %s, whose own row is not declared wired — the reachability claim is stale (FU-20261016-32)", lv.symbol, lv.wiredVia)
		}
	}

	seen := make(map[string]string, len(levers))
	for _, lv := range levers {
		if prev, dup := seen[lv.symbol]; dup {
			t.Fatalf("duplicate guard row for %s (declared in %s and %s)", lv.symbol, prev, lv.defining)
		}
		seen[lv.symbol] = lv.defining

		// The row must still describe a real declaration; otherwise the guard
		// silently stops guarding after a rename or deletion.
		decl, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(lv.defining)))
		if err != nil {
			t.Fatalf("%s: cannot read declaring file %s: %v — update or drop this guard row (FU-20261016-32)", lv.symbol, lv.defining, err)
		}
		if !declaresSymbol(string(decl), lv.symbol) {
			t.Fatalf("%s: %s no longer declares it — update or drop this guard row (FU-20261016-32)", lv.symbol, lv.defining)
		}

		consumers, err := productionReferences(root, lv)
		if err != nil {
			t.Fatalf("%s: scan failed: %v", lv.symbol, err)
		}

		switch {
		case lv.wired && len(consumers) == 0:
			t.Fatalf("%s is declared wired but no non-test production file references it — wire it, or set wired=false together with the DECLARED ONLY comment and the re-certification conditions (FU-20261016-32)", lv.symbol)
		case !lv.wired && len(consumers) > 0:
			t.Fatalf("%s now has production consumer(s): %v — it is declared %s (FU-20261016-32), so a wiring change must update its doc comment, satisfy its re-certification conditions, add the end-to-end test it needs, flip this guard row and close FU-20261016-32 in the same commit", lv.symbol, consumers, leverState(lv))
		}
		t.Logf("%s: consumers=%d wired=%t (%s)", lv.symbol, len(consumers), lv.wired, lv.reason)
	}
}

func leverState(lv declaredOnlyLever) string {
	if lv.wired {
		return "wired"
	}
	if lv.wiredVia != "" {
		return "reached only via " + lv.wiredVia
	}
	return "DECLARED ONLY / NOT ENFORCED"
}

// declaresSymbol reports whether src contains a declaration of symbol, either as
// a package-level func (func Symbol(...) or a method (func (r *T) Symbol(...).
// The check exists so a renamed or deleted symbol cannot leave a guard row that
// silently guards nothing.
func declaresSymbol(src, symbol string) bool {
	return strings.Contains(src, "func "+symbol+"(") ||
		strings.Contains(src, ") "+symbol+"(")
}

// productionReferences returns the non-test .go files under internal/ and cmd/
// that mention the symbol, excluding the declaring file and the declaration-
// adjacent documentation files listed on the row.
func productionReferences(root string, lv declaredOnlyLever) ([]string, error) {
	skip := map[string]bool{lv.defining: true}
	for _, s := range lv.extraSkip {
		skip[s] = true
	}

	var consumers []string
	for _, sub := range []string{"internal", "cmd"} {
		base := filepath.Join(root, sub)
		walkErr := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case ".git", ".worktrees", "node_modules", "vendor":
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			if skip[rel] {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if strings.Contains(string(data), lv.symbol) {
				consumers = append(consumers, rel)
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return consumers, nil
}

// moduleRoot walks up from the test's working directory until it finds go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("module root (go.mod) not found above %s", dir)
		}
		dir = parent
	}
}
