package sectormap

import (
	"math"
	"slices"
	"strings"
	"testing"
)

func TestResolve_UnknownNamespaceAndKeyAreExplicit(t *testing.T) {
	m := Resolve(Namespace("does_not_exist"), "semiconductor")
	if m.Status != StatusUnknown {
		t.Errorf("unknown namespace status = %q, want %q", m.Status, StatusUnknown)
	}
	if m.IsMapped() {
		t.Error("unknown namespace must not produce a mapping")
	}

	m = Resolve(NamespaceClassificationTree, "not_a_declared_segment")
	if m.Status != StatusUnknown {
		t.Errorf("undeclared key status = %q, want %q (drift must be distinguishable from unmapped)", m.Status, StatusUnknown)
	}
	if !strings.Contains(m.Reason, "drift") {
		t.Errorf("undeclared key reason should mention drift, got %q", m.Reason)
	}
}

func TestResolve_IdentityKeyIsCanonical(t *testing.T) {
	m := Resolve(NamespaceClassificationTree, "semiconductor")
	if m.Status != StatusCanonical {
		t.Fatalf("status = %q, want %q", m.Status, StatusCanonical)
	}
	if got, ok := m.Primary(); !ok || got != "semiconductor" {
		t.Errorf("Primary() = (%q, %v), want (semiconductor, true)", got, ok)
	}
}

func TestResolve_L2IdentityRollsUpToL1(t *testing.T) {
	got, ok := ResolveL1(NamespaceClassificationTree, "ai_supply_chain")
	if !ok || got != "electronics" {
		t.Errorf("ResolveL1(classification_tree, ai_supply_chain) = (%q, %v), want (electronics, true)", got, ok)
	}
	if got, ok := ResolveL1(NamespaceClassificationTree, "etf_rotation"); ok {
		t.Errorf("ResolveL1(etf_rotation) = %q, want no L1 target (asset-class bucket)", got)
	}
}

func TestResolve_UnmappedCarriesReasonAndCandidates(t *testing.T) {
	for _, key := range []string{"defensive", "high_dividend", "small_cap", "tech", "pcb", "thermal"} {
		m := Resolve(NamespaceClassificationTree, key)
		if m.Status != StatusUnmapped {
			t.Errorf("%s status = %q, want %q", key, m.Status, StatusUnmapped)
			continue
		}
		if m.IsMapped() {
			t.Errorf("%s must not have a target", key)
		}
		if m.Reason == "" {
			t.Errorf("%s must explain why it is unmapped", key)
		}
	}
	if m := Resolve(NamespaceClassificationTree, "tech"); len(m.Candidates) == 0 {
		t.Error("tech should list candidate canonical L1 sectors for the operator decision")
	}
	if m := Resolve(NamespaceGICSBaseWeights, "_cash_reserve"); len(m.Candidates) != 0 {
		t.Errorf("_cash_reserve must not suggest an equity sector, got %v", m.Candidates)
	}
}

func TestResolve_ReturnsDefensiveCopy(t *testing.T) {
	m := Resolve(NamespaceClassificationTree, "semiconductor")
	m.Targets["hacked"] = 1
	again := Resolve(NamespaceClassificationTree, "semiconductor")
	if _, bad := again.Targets["hacked"]; bad {
		t.Error("Resolve leaked the package table: callers can mutate shared state")
	}
}

func TestEveryNamespace_MapsOnlyToCanonicalIDs(t *testing.T) {
	for _, ns := range Namespaces() {
		for _, m := range Mappings(ns) {
			sum := 0.0
			for id, w := range m.Targets {
				if !IsCanonical(id) {
					t.Errorf("%s/%s maps to non-canonical ID %q", ns, m.Key, id)
				}
				if w <= 0 {
					t.Errorf("%s/%s has non-positive weight %v for %q", ns, m.Key, w, id)
				}
				sum += w
			}
			if len(m.Targets) > 0 && math.Abs(sum-1.0) > 1e-9 {
				t.Errorf("%s/%s target weights sum to %v, want 1.0", ns, m.Key, sum)
			}
			for _, c := range m.Candidates {
				if !IsCanonical(c) {
					t.Errorf("%s/%s candidate %q is not a canonical ID", ns, m.Key, c)
				}
			}
			if len(m.Targets) > 0 && len(m.Candidates) > 0 {
				t.Errorf("%s/%s declares both targets and candidates", ns, m.Key)
			}
		}
	}
}

func TestKeysAndMappingsAreSortedAndConsistent(t *testing.T) {
	for _, ns := range Namespaces() {
		keys := Keys(ns)
		if !slices.IsSorted(keys) {
			t.Errorf("%s: Keys() not sorted", ns)
		}
		if got := len(Mappings(ns)); got != len(keys) {
			t.Errorf("%s: Mappings() len %d != Keys() len %d", ns, got, len(keys))
		}
	}
	if got := Keys(Namespace("nope")); got != nil {
		t.Errorf("Keys(unknown) = %v, want nil", got)
	}
	if got := Mappings(Namespace("nope")); len(got) != 0 {
		t.Errorf("Mappings(unknown) = %v, want empty", got)
	}
}

func TestReport_CountsAddUp(t *testing.T) {
	for _, rep := range Reports() {
		if rep.DeclaredKeys != rep.Canonical+rep.Mapped+rep.Unmapped {
			t.Errorf("%s: declared %d != canonical %d + mapped %d + unmapped %d",
				rep.Namespace, rep.DeclaredKeys, rep.Canonical, rep.Mapped, rep.Unmapped)
		}
		if len(rep.UnmappedKeys) != rep.Unmapped {
			t.Errorf("%s: unmapped list len %d != count %d", rep.Namespace, len(rep.UnmappedKeys), rep.Unmapped)
		}
		if len(rep.CoveredL1) > 20 || len(rep.CoveredL2) > 18 {
			t.Errorf("%s: coverage exceeds the canonical taxonomy", rep.Namespace)
		}
	}
}

func TestNamespaces_CoversEveryDeclaredVocabulary(t *testing.T) {
	want := []Namespace{
		NamespaceCanonical,
		NamespaceRepresentativeStocks,
		NamespaceClassificationTree,
		NamespaceSectorSymbols,
		NamespaceGICSBaseWeights,
		NamespaceDefaultMetrics,
		NamespaceCycleThresholds,
		NamespaceTWSESectorIndex,
		NamespaceTWSESectorIndexLegacy,
		NamespaceSectorIndexReader,
		NamespaceStrategyTechniqueSectors,
		NamespaceFinMindSectorSeries,
		NamespaceTWSESIndustryCode,
		NamespaceETFRepresentatives,
	}
	got := Namespaces()
	if !slices.Equal(got, []Namespace{
		"canonical_l1_l2",
		"config_classification_tree",
		"config_industry_cycle_thresholds",
		"config_industry_default_metrics",
		"config_sector_allocation_base_weights",
		"config_sector_symbols",
		"finmind_sector_series",
		"industry_representative_stocks",
		"marketdata_sector_index_reader_ids",
		"sectorallocation_etf_representatives",
		"strategy_technique_sectors",
		"twse_industry_code",
		"twse_sector_index_name",
		"twse_sector_index_name_legacy",
	}) {
		t.Errorf("Namespaces() = %v", got)
	}
	if len(got) != len(want) {
		t.Errorf("namespace count = %d, want %d", len(got), len(want))
	}
}

func TestCoveredL1_TWSEIndexNamesCoverTheWholeTaxonomy(t *testing.T) {
	direct := CoveredL1(NamespaceTWSESectorIndex, false)
	if len(direct) != 20 {
		t.Errorf("TWSE sector-index names cover %d canonical L1 sectors, want all 20: %v", len(direct), direct)
	}
}

func TestCoveredL1_TreeAndSymbolsArePartial(t *testing.T) {
	tree := CoveredL1(NamespaceClassificationTree, true)
	if len(tree) == 0 {
		t.Fatal("classification tree reaches no canonical L1 sector")
	}
	if len(tree) >= 20 {
		t.Errorf("classification tree unexpectedly covers all 20 L1 sectors; re-check the table: %v", tree)
	}
}
