package sectormap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// These tests are the drift alarm for #1943: they read the live configuration
// files and fail when a vocabulary gains or loses a key without the explicit
// mapping table in this package being updated. Without them the tables decay
// into documentation.

func repoFile(t *testing.T, rel string) []byte {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/sectormap/<this file> → repo root
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return data
}

func loadJSON(t *testing.T, rel string, v any) {
	t.Helper()
	if err := json.Unmarshal(repoFile(t, rel), v); err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
}

func assertSameKeys(t *testing.T, what string, got, want []string) {
	t.Helper()
	got = slices.Clone(got)
	want = slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("%s key set drifted loose from internal/sectormap:\n declared-only: %v\n config-only:   %v",
			what, missing(want, got), missing(got, want))
	}
}

// missing returns the elements of a that are absent from b.
func missing(a, b []string) []string {
	out := []string{}
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

// industrySection returns configs/parameters/industry.json (split form) and the
// same section of the configs/parameters.json monolith, asserting they agree —
// the repo already guarantees that equivalence for the merged config.
func industrySections(t *testing.T) (split, mono map[string]json.RawMessage) {
	t.Helper()
	loadJSON(t, "configs/parameters/industry.json", &split)

	var all map[string]json.RawMessage
	loadJSON(t, "configs/parameters.json", &all)
	if err := json.Unmarshal(all["industry"], &mono); err != nil {
		t.Fatalf("parse parameters.json industry section: %v", err)
	}
	return split, mono
}

type metaValue[T any] struct {
	Value T `json:"value"`
}

func TestDrift_ClassificationTreeKeys(t *testing.T) {
	split, mono := industrySections(t)

	type segment struct {
		ID    string `json:"id"`
		Level int    `json:"level"`
	}
	var splitTree metaValue[struct {
		Segments []segment `json:"segments"`
	}]
	if err := json.Unmarshal(split["classification_tree"], &splitTree); err != nil {
		t.Fatalf("parse classification_tree: %v", err)
	}
	ids := make([]string, 0, len(splitTree.Value.Segments))
	violations := []string{}
	for _, s := range splitTree.Value.Segments {
		ids = append(ids, s.ID)
		if s.Level < 1 || s.Level > 3 {
			violations = append(violations, s.ID)
		}
	}
	assertSameKeys(t, "configs/parameters/industry.json classification_tree",
		ids, Keys(NamespaceClassificationTree))

	var monoTree metaValue[struct {
		Segments []segment `json:"segments"`
	}]
	if err := json.Unmarshal(mono["classification_tree"], &monoTree); err != nil {
		t.Fatalf("parse monolith classification_tree: %v", err)
	}
	monoIDs := make([]string, 0, len(monoTree.Value.Segments))
	for _, s := range monoTree.Value.Segments {
		monoIDs = append(monoIDs, s.ID)
	}
	assertSameKeys(t, "configs/parameters.json industry.classification_tree",
		monoIDs, Keys(NamespaceClassificationTree))
}

func TestDrift_DefaultMetricsAndCycleThresholds(t *testing.T) {
	split, mono := industrySections(t)

	for _, tc := range []struct {
		section string
		ns      Namespace
	}{
		{"default_metrics", NamespaceDefaultMetrics},
		{"cycle_thresholds", NamespaceCycleThresholds},
	} {
		var splitV metaValue[map[string]json.RawMessage]
		if err := json.Unmarshal(split[tc.section], &splitV); err != nil {
			t.Fatalf("parse %s: %v", tc.section, err)
		}
		assertSameKeys(t, "configs/parameters/industry.json "+tc.section,
			sortedKeys(splitV.Value), Keys(tc.ns))

		var monoV metaValue[map[string]json.RawMessage]
		if err := json.Unmarshal(mono[tc.section], &monoV); err != nil {
			t.Fatalf("parse monolith %s: %v", tc.section, err)
		}
		assertSameKeys(t, "configs/parameters.json industry."+tc.section,
			sortedKeys(monoV.Value), Keys(tc.ns))
	}
}

func TestDrift_SectorSymbolsKeys(t *testing.T) {
	var doc map[string]json.RawMessage
	loadJSON(t, "configs/sector_symbols.json", &doc)
	assertSameKeys(t, "configs/sector_symbols.json", sortedKeys(doc), Keys(NamespaceSectorSymbols))
}

func TestDrift_GICSBaseWeightsKeys(t *testing.T) {
	var split struct {
		BaseWeights map[string]float64 `json:"base_weights"`
	}
	loadJSON(t, "configs/parameters/sector_allocation.json", &split)
	assertSameKeys(t, "configs/parameters/sector_allocation.json base_weights",
		sortedKeys(split.BaseWeights), Keys(NamespaceGICSBaseWeights))

	var all struct {
		SectorAllocation struct {
			BaseWeights map[string]float64 `json:"base_weights"`
		} `json:"sector_allocation"`
	}
	loadJSON(t, "configs/parameters.json", &all)
	assertSameKeys(t, "configs/parameters.json sector_allocation.base_weights",
		sortedKeys(all.SectorAllocation.BaseWeights), Keys(NamespaceGICSBaseWeights))
}

func TestDrift_StrategyTechniqueSectorTags(t *testing.T) {
	var doc []struct {
		Sectors []string `json:"sectors"`
	}
	raw := repoFile(t, "data/seeds/strategy_techniques.json")
	if err := json.Unmarshal(raw, &doc); err != nil {
		// The file may be an object with a techniques array.
		var wrapper struct {
			Techniques []struct {
				Sectors []string `json:"sectors"`
			} `json:"techniques"`
		}
		if err2 := json.Unmarshal(raw, &wrapper); err2 != nil {
			t.Fatalf("parse data/seeds/strategy_techniques.json: %v / %v", err, err2)
		}
		for _, tw := range wrapper.Techniques {
			doc = append(doc, struct {
				Sectors []string `json:"sectors"`
			}{Sectors: tw.Sectors})
		}
	}
	tags := []string{}
	for _, tpl := range doc {
		tags = append(tags, tpl.Sectors...)
	}
	slices.Sort(tags)
	tags = slices.Compact(tags)
	assertSameKeys(t, "data/seeds/strategy_techniques.json sectors", tags,
		Keys(NamespaceStrategyTechniqueSectors))
}

func TestDrift_SectorIndexReaderAcceptsEveryCanonicalL1(t *testing.T) {
	keys := Keys(NamespaceSectorIndexReader)
	for _, id := range CanonicalL1IDs() {
		if !slices.Contains(keys, id) {
			t.Errorf("sector_index reader must accept canonical L1 %q", id)
		}
	}
	for _, legacy := range []string{"ai_supply_chain", "robotics"} {
		got, ok := ResolveL1(NamespaceSectorIndexReader, legacy)
		if !ok {
			t.Errorf("legacy sector_index ID %q must resolve to an L1 sector", legacy)
		}
		if !IsCanonicalL1(got) {
			t.Errorf("legacy sector_index ID %q resolved to non-L1 %q", legacy, got)
		}
	}
}

func TestDrift_TWSELegacyAndCanonicalMapsAgree(t *testing.T) {
	// Pre-#1943 the two TWSE maps disagreed: 電腦及週邊設備類 → ai_supply_chain in
	// the legacy map but → electronics in the canonical map. Both must now
	// resolve to the same canonical L1 sector.
	for _, key := range Keys(NamespaceTWSESectorIndexLegacy) {
		twse, ok := ResolveL1(NamespaceTWSESectorIndex, key)
		if !ok {
			t.Errorf("TWSE name %q missing from the canonical TWSE table", key)
			continue
		}
		legacy, ok := ResolveL1(NamespaceTWSESectorIndexLegacy, key)
		if !ok {
			t.Errorf("TWSE name %q unresolved in the legacy table", key)
			continue
		}
		if twse != legacy {
			t.Errorf("TWSE name %q resolves to %q canonically but %q via the legacy map", key, twse, legacy)
		}
	}
}

func TestDrift_L2ParentL1AgreesWithAuthoredTree(t *testing.T) {
	split, _ := industrySections(t)

	type segment struct {
		ID       string `json:"id"`
		Level    int    `json:"level"`
		ParentID string `json:"parent_id"`
	}
	var tree metaValue[struct {
		Segments []segment `json:"segments"`
	}]
	if err := json.Unmarshal(split["classification_tree"], &tree); err != nil {
		t.Fatalf("parse classification_tree: %v", err)
	}

	byID := make(map[string]segment, len(tree.Value.Segments))
	for _, seg := range tree.Value.Segments {
		byID[seg.ID] = seg
	}

	// Issue #1943: a key must not resolve to two different L1 sectors depending
	// on whether it arrives as a tree segment or as a raw key. The declared
	// L2 → L1 table therefore has to agree with the tree's authored parent chain.
	for _, seg := range tree.Value.Segments {
		if seg.Level == 1 || !IsCanonicalL2(seg.ID) {
			continue
		}
		root := seg.ID
		for {
			parent, ok := byID[byID[root].ParentID]
			if !ok {
				break
			}
			root = parent.ID
		}
		wantL1, ok := ParentL1Of(root)
		if !ok {
			// The whole subtree rolls up to nothing (e.g. under an asset-class
			// bucket); nothing to compare.
			continue
		}
		gotL1, ok := ParentL1Of(seg.ID)
		if !ok {
			t.Errorf("tree L2 %s (root %s) rolls up to %s but the declared table has no L1 parent for it",
				seg.ID, root, wantL1)
			continue
		}
		if gotL1 != wantL1 {
			t.Errorf("tree L2 %s: authored tree chain gives %s (root %s) but the declared L2→L1 table gives %s — the same key would resolve to two different L1 sectors",
				seg.ID, wantL1, root, gotL1)
		}
	}
}

func TestDrift_TWSETableClosesTheCanonicalGaps(t *testing.T) {
	// The live TWSE MI_INDEX response (2026-09-24) exposes 化學類指數 and
	// 觀光餐旅類指數, which is how chemicals and tourism become reachable at all.
	for _, want := range []string{"chemicals", "tourism"} {
		found := false
		for _, key := range Keys(NamespaceTWSESectorIndex) {
			if id, ok := ResolveL1(NamespaceTWSESectorIndex, key); ok && id == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no declared TWSE index name maps to %s; the canonical gap would reopen", want)
		}
	}
}

func TestDrift_RepresentativeStocksNamespaceIsCanonicalL1(t *testing.T) {
	// The Go table (industry.DefaultRepresentativeStocks) is keyed by SectorID,
	// so this namespace must declare exactly the 20 canonical L1 IDs. Combined
	// with industry.TestDefaultRepresentativeStocksUseCanonicalL1Keys (which
	// asserts the Go map's keys are canonical L1) the two sides cannot diverge.
	want := CanonicalL1IDs()
	got := Keys(NamespaceRepresentativeStocks)
	if !slices.Equal(got, want) {
		t.Errorf("industry_representative_stocks declares %v, want the 20 canonical L1 IDs", got)
	}
}

// sortedKeys returns the keys of m in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
