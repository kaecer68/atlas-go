package industry_test

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

// repoRoot resolves the repository root from this test file's location.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(f), "..", "..")
}

// loadRepoParameters points the config singleton at the repository's real
// configs/parameters.json so the tests below exercise the production taxonomy
// instead of the compiled-in fallback.
func loadRepoParameters(t *testing.T) {
	t.Helper()
	config.SetParametersConfigPath(filepath.Join(repoRoot(t), "configs", "parameters.json"))
	config.ResetParametersConfig()
	t.Cleanup(func() {
		config.SetParametersConfigPath("configs/parameters.json")
		config.ResetParametersConfig()
	})
	if cfg := config.GetParametersConfig(); cfg == nil {
		t.Fatal("config.GetParametersConfig returned nil")
	}
}

// ---------------------------------------------------------------------------
// The typed taxonomy in this package and the leaf namespace package must agree.
// These assertions are what keeps sectormap from becoming a second source of
// truth for "what is a canonical sector".
// ---------------------------------------------------------------------------

func TestSectormapCanonicalListsAgreeWithIndustryTaxonomy(t *testing.T) {
	wantL1 := make([]string, 0, 20)
	for _, id := range industry.L1Sectors() {
		wantL1 = append(wantL1, string(id))
	}
	slices.Sort(wantL1)
	if got := sectormap.CanonicalL1IDs(); !slices.Equal(got, wantL1) {
		t.Errorf("sectormap L1 list disagrees with industry.L1Sectors():\n got %v\nwant %v", got, wantL1)
	}

	wantL2 := make([]string, 0, 18)
	for id := range industry.SubIndustryDisplayZHTw {
		wantL2 = append(wantL2, string(id))
	}
	slices.Sort(wantL2)
	if got := sectormap.CanonicalL2IDs(); !slices.Equal(got, wantL2) {
		t.Errorf("sectormap L2 list disagrees with industry.SubIndustryDisplayZHTw:\n got %v\nwant %v", got, wantL2)
	}

	for _, raw := range sectormap.CanonicalIDs() {
		id := industry.SectorID(raw)
		if !id.IsValid() {
			t.Errorf("sectormap claims %q is canonical but industry.SectorID(%q).IsValid() is false", raw, raw)
		}
	}
	for _, id := range industry.AllSectors() {
		if !sectormap.IsCanonical(string(id)) {
			t.Errorf("industry.AllSectors() has %q which sectormap does not know", id)
		}
	}
}

func TestSectorIDParentL1MatchesSectormap(t *testing.T) {
	for _, id := range industry.AllSectors() {
		want, wantOK := sectormap.ParentL1Of(string(id))
		got, gotOK := industry.ParentL1(id)
		if gotOK != wantOK {
			t.Errorf("ParentL1(%s) ok = %v, want %v", id, gotOK, wantOK)
			continue
		}
		if wantOK && got != industry.SectorID(want) {
			t.Errorf("ParentL1(%s) = %s, want %s", id, got, want)
		}
		if gotOK && !got.IsL1() {
			t.Errorf("ParentL1(%s) = %s which is not an L1 sector", id, got)
		}
	}
}

// TestDefaultCycleThresholdsVocabularyIsCanonical guards the compiled-in
// fallback vocabulary: when configs/parameters.json is missing,
// DefaultParametersConfig() supplies cycle thresholds keyed by industry ID. Those
// keys must be canonical IDs (plus the documented "_default" entry), otherwise
// the fallback path would reintroduce a foreign key space (#1943).
func TestDefaultCycleThresholdsVocabularyIsCanonical(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	if cfg == nil {
		t.Fatal("DefaultParametersConfig() returned nil")
	}
	keys := cfg.Industry.CycleThresholds.Value
	if len(keys) == 0 {
		t.Fatal("compiled-in cycle_thresholds is empty")
	}
	foreign := []string{}
	for k := range keys {
		if k == "_default" {
			continue
		}
		if !industry.SectorID(k).IsValid() {
			foreign = append(foreign, k)
		}
	}
	slices.Sort(foreign)
	if len(foreign) > 0 {
		t.Errorf("compiled-in cycle_thresholds contains non-canonical keys: %v", foreign)
	}
	t.Logf("compiled-in cycle_thresholds: %d keys (configs/parameters.json declares %d)",
		len(keys), len(sectormap.Keys(sectormap.NamespaceCycleThresholds)))
}

// ---------------------------------------------------------------------------
// Namespace C: the hard-coded representative-stock table is already canonical.
// ---------------------------------------------------------------------------

func TestDefaultRepresentativeStocksUseCanonicalL1Keys(t *testing.T) {
	reps := industry.DefaultRepresentativeStocks()
	if len(reps) != 20 {
		t.Errorf("DefaultRepresentativeStocks has %d sectors, want 20", len(reps))
	}
	owner := map[string]industry.SectorID{}
	for id, symbols := range reps {
		if !id.IsL1() {
			t.Errorf("DefaultRepresentativeStocks key %q is not a canonical L1 sector", id)
		}
		for _, sym := range symbols {
			if other, dup := owner[sym]; dup {
				t.Errorf("symbol %s is listed under both %s and %s", sym, other, id)
			}
			owner[sym] = id
		}
	}
}

// ---------------------------------------------------------------------------
// The production classification tree: every declared symbol must resolve.
// ---------------------------------------------------------------------------

func TestProductionClassificationTree_SymbolCoverage(t *testing.T) {
	loadRepoParameters(t)

	tree := industry.DefaultClassification()
	m, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper(DefaultClassification()): %v", err)
	}

	universe := industry.DeclaredRepresentativeUniverse(tree, industry.Level1)
	if len(universe) == 0 {
		t.Fatal("tree declares no Level-1 representative stocks")
	}
	cov := industry.ComputeCanonicalCoverage(universe, m)
	t.Logf("production tree: %d L1 representative symbols, %d mapped to %d canonical L1 sectors (ratio %.4f)",
		cov.Universe, cov.Mapped, cov.L1SectorsCovered, cov.Ratio)

	if cov.Mapped != cov.Universe {
		t.Fatalf("%d of %d declared Level-1 representative symbols have no canonical L1 sector; unmapped: %v",
			cov.Universe-cov.Mapped, cov.Universe, cov.UnmappedSymbols)
	}
	if cov.L1SectorsCovered < 5 {
		t.Errorf("only %d canonical L1 sectors are reachable from the tree: %v", cov.L1SectorsCovered, cov.ByL1)
	}

	// Every declared symbol in the whole tree (L1 + L2) must resolve too.
	all := industry.DeclaredRepresentativeUniverse(tree, industry.Level1)
	all = append(all, industry.DeclaredRepresentativeUniverse(tree, industry.Level2)...)
	full := industry.ComputeCanonicalCoverage(all, m)
	t.Logf("production tree: %d declared symbols overall, %d mapped (ratio %.4f)", full.Universe, full.Mapped, full.Ratio)
	if full.Mapped != full.Universe {
		t.Errorf("%d declared symbols have no canonical L1 sector: %v", full.Universe-full.Mapped, full.UnmappedSymbols)
	}
}

func TestProductionClassificationTree_UnmappedSegmentsAreReported(t *testing.T) {
	loadRepoParameters(t)

	m, err := industry.NewSymbolL1Mapper(industry.DefaultClassification())
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}
	unmapped := m.UnmappedSegments()
	want := []string{"defensive", "etf_rotation", "high_dividend", "small_cap", "tech"}
	got := make([]string, 0, len(unmapped))
	for k := range unmapped {
		got = append(got, k)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("unmapped tree segments = %v, want %v (update this test and the spec together when the taxonomy changes)", got, want)
	}
	for _, id := range got {
		if unmapped[id] == "" {
			t.Errorf("unmapped segment %s has no reason", id)
		}
	}
	// pcb/thermal have no canonical taxonomy node of their own, but they sit
	// under L1 electronics, so the structural (rank 0) resolution still maps
	// them. That is intended: the segment is usable, the key is not canonical.
	for _, id := range []string{"pcb", "thermal"} {
		if _, reported := unmapped[id]; reported {
			t.Errorf("%s sits under a canonical L1 segment and must resolve structurally", id)
		}
	}
}

func TestProductionClassificationTree_ConflictsAreResolvedAndReported(t *testing.T) {
	loadRepoParameters(t)

	m, err := industry.NewSymbolL1Mapper(industry.DefaultClassification())
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}
	conflicts := m.Conflicts()
	if len(conflicts) == 0 {
		t.Fatal("the production tree claims two symbols under two different canonical L1 sectors; expected them to be reported")
	}
	bySymbol := map[string]industry.SymbolL1Conflict{}
	for _, c := range conflicts {
		bySymbol[c.Symbol] = c
		if len(c.Rejected) == 0 {
			t.Errorf("%s: conflict without rejected claims", c.Symbol)
		}
		for _, r := range c.Rejected {
			if r.Target == c.Chosen.Target {
				t.Errorf("%s: rejected claim %s has the same target as the chosen claim", c.Symbol, r.Segment)
			}
		}
	}

	// 2356 is declared by server_assembly (L2 under L1 semiconductor) and by
	// robotics (tree-only L1 that rolls up to machinery). The structural
	// resolution wins, deterministically.
	c, ok := bySymbol["2356"]
	if !ok {
		t.Fatalf("expected 2356 to be reported as a conflict, got %v", conflicts)
	}
	if c.Chosen.Target != industry.SectorSemiconductor {
		t.Errorf("2356 resolved to %s, want semiconductor (structural rank 0 beats declared rank 1)", c.Chosen.Target)
	}
	if id, _ := m.ResolveL1("2356"); id != industry.SectorSemiconductor {
		t.Errorf("ResolveL1(2356) = %s, want semiconductor", id)
	}
}

// ---------------------------------------------------------------------------
// Namespace D/E/F and the ad-hoc vocabularies.
// ---------------------------------------------------------------------------

func TestResolveForeign_GICSBaseWeights(t *testing.T) {
	healthcare := industry.ResolveForeign(industry.NamespaceGICSBaseWeights, "healthcare")
	if !healthcare.Materialized() {
		t.Fatal("healthcare must map to a canonical sector")
	}
	if got := healthcare.Targets[industry.SectorBiotech]; got != 1.0 {
		t.Errorf("healthcare → biotech weight = %v, want 1.0", got)
	}
	if healthcare.L1 != industry.SectorBiotech {
		t.Errorf("healthcare L1 = %s, want biotech", healthcare.L1)
	}

	for _, key := range []string{"industrials", "materials", "real_estate", "utilities", "_cash_reserve"} {
		m := industry.ResolveForeign(industry.NamespaceGICSBaseWeights, key)
		if m.Materialized() {
			t.Errorf("%s must not be mapped silently, got %v", key, m.Targets)
		}
		if m.Status != sectormap.StatusUnmapped {
			t.Errorf("%s status = %q, want unmapped", key, m.Status)
		}
		if m.Reason == "" {
			t.Errorf("%s must carry a reason", key)
		}
	}
	if m := industry.ResolveForeign(industry.NamespaceGICSBaseWeights, "_cash_reserve"); len(m.Candidates) != 0 {
		t.Errorf("_cash_reserve is an asset class and must not suggest an equity sector: %v", m.Candidates)
	}
}

func TestResolveForeign_TWSEIndexNamesCoverCanonicalL1(t *testing.T) {
	seen := map[industry.SectorID]bool{}
	unmapped := []string{}
	for _, key := range sectormap.Keys(sectormap.NamespaceTWSESectorIndex) {
		l1, ok := industry.CanonicalL1FromForeign(industry.NamespaceTWSESectorIndex, key)
		if !ok {
			unmapped = append(unmapped, key)
			continue
		}
		seen[l1] = true
	}
	if len(unmapped) == 0 {
		t.Error("the live TWSE vocabulary contains names with no canonical L1 target; they must stay declared-unmapped rather than be mapped by guesswork")
	}
	t.Logf("TWSE sector-index names: %d mapped, %d declared-unmapped", len(seen), len(unmapped))
	for _, id := range industry.L1Sectors() {
		if !seen[id] {
			t.Errorf("canonical L1 %s has no TWSE sector-index name", id)
		}
	}
}

func TestResolveForeign_StrategyTechniqueTagsMixNamespaces(t *testing.T) {
	mapped, unmapped := 0, 0
	for _, key := range sectormap.Keys(sectormap.NamespaceStrategyTechniqueSectors) {
		if industry.ResolveForeign(industry.NamespaceStrategyTechniqueSectors, key).Materialized() {
			mapped++
			continue
		}
		unmapped++
	}
	if mapped == 0 || unmapped == 0 {
		t.Fatalf("strategy-technique tags: mapped=%d unmapped=%d; the vocabulary is expected to mix equity sectors with size/style/asset-class buckets", mapped, unmapped)
	}
	t.Logf("strategy-technique sector tags: %d equity-sector tags, %d non-equity buckets", mapped, unmapped)
}

func TestCanonicalCoverage_ReportsUnmappedSymbols(t *testing.T) {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "semiconductor",
		Name:                 "半導體",
		Level:                industry.Level1,
		RepresentativeStocks: []string{"2330"},
	})

	m, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}

	cov := industry.ComputeCanonicalCoverage([]string{"2330", "2330.TW", "", "9999"}, m)
	if cov.Universe != 2 {
		t.Errorf("universe = %d, want 2 (normalized and de-duplicated)", cov.Universe)
	}
	if cov.Mapped != 1 {
		t.Errorf("mapped = %d, want 1", cov.Mapped)
	}
	if len(cov.UnmappedSymbols) != 1 || cov.UnmappedSymbols[0] != "9999" {
		t.Errorf("unmapped = %v, want [9999]", cov.UnmappedSymbols)
	}
	if cov.Ratio != 0.5 {
		t.Errorf("ratio = %v, want 0.5", cov.Ratio)
	}
	if cov.ByL1[industry.SectorSemiconductor] != 1 {
		t.Errorf("ByL1[semiconductor] = %d, want 1", cov.ByL1[industry.SectorSemiconductor])
	}

	// A nil mapper must degrade to "nothing mapped", not panic.
	nilCov := industry.ComputeCanonicalCoverage([]string{"2330"}, nil)
	if nilCov.Mapped != 0 || nilCov.Universe != 1 || len(nilCov.UnmappedSymbols) != 1 {
		t.Errorf("nil mapper coverage = %+v", nilCov)
	}
}

func TestAuditAllNamespaces_MentionsEveryVocabulary(t *testing.T) {
	audits := industry.AuditAllNamespaces()
	if len(audits) != len(industry.AuditedNamespaces()) {
		t.Fatalf("AuditAllNamespaces len %d != AuditedNamespaces len %d", len(audits), len(industry.AuditedNamespaces()))
	}
	for _, a := range audits {
		if a.DeclaredKeys == 0 {
			t.Errorf("%s declares no keys", a.Namespace)
		}
		if a.DeclaredKeys != a.CanonicalKey+a.MappedKeys+a.UnmappedKeys {
			t.Errorf("%s: %d != %d + %d + %d", a.Namespace, a.DeclaredKeys, a.CanonicalKey, a.MappedKeys, a.UnmappedKeys)
		}
		if len(a.UnmappedList) != a.UnmappedKeys {
			t.Errorf("%s: unmapped list len %d != %d", a.Namespace, len(a.UnmappedList), a.UnmappedKeys)
		}
	}
}
