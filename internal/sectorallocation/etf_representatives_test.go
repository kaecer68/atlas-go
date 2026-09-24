package sectorallocation

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

// The declared ETF → canonical L1 table in internal/sectormap is *derived* data.
// These tests re-run the derivation from the checked-in evidence snapshot, so
// the table cannot be hand-edited without the evidence and the evidence cannot
// go stale without the table failing. This is the same "test as specification"
// pattern the #1943 namespace tables use.

const (
	etfSnapshotPath  = "testdata/etf_holdings_20260924.json"
	etfCoverageFloor = 12
	// quantiseTolerance: the declared weights are quantised to the 1e-6 grid
	// (largest remainder, so they sum to exactly 1), which bounds the difference
	// against the exact renormalised derivation.
	quantiseTolerance = 1e-6 * 1.5
)

type etfSnapshot struct {
	SnapshotDate   string                      `json:"snapshot_date"`
	IndustrySource map[string]json.RawMessage  `json:"industry_source"`
	Notes          map[string]string           `json:"notes"`
	ETFs           map[string]etfSnapshotEntry `json:"etfs"`
}

type etfSnapshotEntry struct {
	Name       string            `json:"name"`
	Benchmark  string            `json:"benchmark"`
	Issuer     string            `json:"issuer"`
	SourceURL  string            `json:"source_url"`
	SourceKind string            `json:"source_kind"`
	AsOf       string            `json:"as_of"`
	Extraction string            `json:"extraction"`
	Holdings   []snapshotHolding `json:"holdings"`
}

type snapshotHolding struct {
	Symbol         string  `json:"symbol"`
	Name           string  `json:"name"`
	WeightPct      float64 `json:"weight_pct"`
	IndustryCode   string  `json:"industry_code"`
	IndustrySource string  `json:"industry_source"`
}

func loadETFSnapshot(t *testing.T) etfSnapshot {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), etfSnapshotPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var snap etfSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return snap
}

// deriveL1 reproduces the derivation that produced the declared table: sum the
// published weights per canonical L1 sector (via the holding's TWSE/TPEx
// industry code), drop positions with no canonical target, renormalise.
func deriveL1(hs []snapshotHolding) (weights map[string]float64, reported, mapped float64) {
	acc := map[string]float64{}
	for _, h := range hs {
		reported += h.WeightPct
		l1, ok := sectormap.TWSESIndustryCodeL1(h.IndustryCode)
		if !ok {
			continue
		}
		mapped += h.WeightPct
		acc[l1] += h.WeightPct
	}
	weights = make(map[string]float64, len(acc))
	for id, w := range acc {
		weights[id] = w / mapped
	}
	return weights, reported, mapped
}

func TestETFSnapshot_IsSelfConsistentEvidence(t *testing.T) {
	snap := loadETFSnapshot(t)
	if len(snap.ETFs) == 0 {
		t.Fatal("snapshot has no ETFs")
	}
	if snap.SnapshotDate == "" {
		t.Error("snapshot must carry the date the holdings were read")
	}
	for _, need := range []string{"listed", "otc", "code_legend"} {
		if _, ok := snap.IndustrySource[need]; !ok {
			t.Errorf("snapshot must record the %q industry source with its URL", need)
		}
	}
	if len(snap.Notes) == 0 {
		t.Error("snapshot must explain its own derivation and exclusions")
	}

	for symbol, e := range snap.ETFs {
		if e.AsOf == "" || e.Issuer == "" || e.SourceURL == "" || e.Extraction == "" {
			t.Errorf("%s: issuer/as_of/source_url/extraction must all be recorded", symbol)
		}
		if len(e.Holdings) == 0 {
			t.Errorf("%s: no holdings recorded", symbol)
			continue
		}
		seen := map[string]bool{}
		for _, h := range e.Holdings {
			if len(h.Symbol) < 4 || len(h.Symbol) > 6 {
				t.Errorf("%s: implausible symbol %q", symbol, h.Symbol)
			}
			if seen[h.Symbol] {
				t.Errorf("%s: duplicate holding %q", symbol, h.Symbol)
			}
			seen[h.Symbol] = true
			if h.WeightPct <= 0 {
				t.Errorf("%s/%s: non-positive weight %v", symbol, h.Symbol, h.WeightPct)
			}
			// Every industry code in the evidence must be a declared key of the
			// industry-code namespace. An undeclared code would be silent drift,
			// which is exactly what #1943 forbids.
			if h.IndustryCode == "" || h.IndustrySource == "" {
				t.Errorf("%s/%s: missing industry_code/industry_source", symbol, h.Symbol)
				continue
			}
			if !slices.Contains(sectormap.Keys(sectormap.NamespaceTWSESIndustryCode), h.IndustryCode) {
				t.Errorf("%s/%s: industry code %q is not declared in twse_industry_code",
					symbol, h.Symbol, h.IndustryCode)
			}
		}
	}
}

func TestETFRepresentatives_MatchesTheMetadataSSOT(t *testing.T) {
	snap := loadETFSnapshot(t)
	snapSymbols := make([]string, 0, len(snap.ETFs))
	for s := range snap.ETFs {
		snapSymbols = append(snapSymbols, s)
	}
	slices.Sort(snapSymbols)
	if got := ETFRepresentativeSymbols(); !slices.Equal(got, snapSymbols) {
		t.Errorf("declared ETF symbols %v != snapshot symbols %v", got, snapSymbols)
	}
	for symbol, e := range snap.ETFs {
		row, ok := findETF(t, symbol)
		if !ok {
			continue
		}
		if row.AsOf != e.AsOf {
			t.Errorf("%s: declared as_of %q but snapshot says %q", symbol, row.AsOf, e.AsOf)
		}
		if row.SourceURL != e.SourceURL {
			t.Errorf("%s: declared source_url %q but snapshot says %q", symbol, row.SourceURL, e.SourceURL)
		}
		if row.Issuer != e.Issuer {
			t.Errorf("%s: declared issuer %q but snapshot says %q", symbol, row.Issuer, e.Issuer)
		}
		if row.Holdings != len(e.Holdings) {
			t.Errorf("%s: declared %d holdings but snapshot has %d", symbol, row.Holdings, len(e.Holdings))
		}
	}
}

func TestETFRepresentatives_DerivationMatchesDeclaredTable(t *testing.T) {
	snap := loadETFSnapshot(t)
	for symbol, e := range snap.ETFs {
		want, reported, mapped := deriveL1(e.Holdings)
		stored, ok := sectormap.Resolve(sectormap.NamespaceETFRepresentatives, symbol).Targets, true
		if !ok || len(stored) == 0 {
			t.Errorf("%s: no declared L1 targets", symbol)
			continue
		}
		if len(stored) != len(want) {
			t.Errorf("%s: declared %d L1 targets %v but the holdings imply %d %v",
				symbol, len(stored), sortedIDs(stored), len(want), sortedIDs(want))
		}
		for id, w := range want {
			got, ok := stored[id]
			if !ok {
				t.Errorf("%s: holdings imply L1 %q (%.6f) but the declared table omits it", symbol, id, w)
				continue
			}
			if math.Abs(got-w) > quantiseTolerance {
				t.Errorf("%s/%s: declared %.6f but holdings imply %.6f (tolerance %g)", symbol, id, got, w, quantiseTolerance)
			}
		}
		row, _ := findETF(t, symbol)
		if math.Abs(row.ReportedWeightPct-reported) > 1e-6 {
			t.Errorf("%s: declared reported weight %v but holdings sum to %v", symbol, row.ReportedWeightPct, reported)
		}
		if math.Abs(row.MappedWeightPct-mapped) > 1e-6 {
			t.Errorf("%s: declared mapped weight %v but holdings imply %v", symbol, row.MappedWeightPct, mapped)
		}
	}
}

func TestETFL1Coverage_FloorIsBackedByHoldings(t *testing.T) {
	snap := loadETFSnapshot(t)

	// Independent of the declared table: compute the union straight from the
	// evidence, so weakening the declared rows cannot weaken this floor.
	fromEvidence := map[string]bool{}
	for _, e := range snap.ETFs {
		w, _, _ := deriveL1(e.Holdings)
		for id := range w {
			fromEvidence[id] = true
		}
	}
	if len(fromEvidence) < etfCoverageFloor {
		t.Fatalf("ETF holdings cover %d canonical L1 sectors (%v), floor is %d",
			len(fromEvidence), sortedSet(fromEvidence), etfCoverageFloor)
	}

	declared := ETFL1Coverage()
	if len(declared) < etfCoverageFloor {
		t.Fatalf("declared ETF L1 coverage = %d, floor is %d", len(declared), etfCoverageFloor)
	}
	if ETFL1CoverageCount() != len(declared) {
		t.Errorf("ETFL1CoverageCount() = %d but ETFL1Coverage() has %d entries", ETFL1CoverageCount(), len(declared))
	}
	for _, id := range declared {
		if !industry.IsL1(id) {
			t.Errorf("ETF coverage contains non-L1 sector %q", id)
		}
		if !fromEvidence[string(id)] {
			t.Errorf("declared coverage claims %q but no holding in the evidence snapshot implies it", id)
		}
	}
}

func TestETFRepresentativeLookup_TypedViewMatchesTable(t *testing.T) {
	for _, row := range ETFRepresentatives() {
		weights, ok := ETFRepresentativeLookup(row.Symbol)
		if !ok {
			t.Errorf("%s: lookup failed although the row is declared", row.Symbol)
			continue
		}
		if len(weights) != len(row.L1Weights) {
			t.Errorf("%s: typed view has %d keys, row has %d", row.Symbol, len(weights), len(row.L1Weights))
		}
		for id, w := range row.L1Weights {
			if got, ok := weights[id]; !ok || math.Abs(got-w) > 1e-12 {
				t.Errorf("%s/%s: typed view %v, row %v", row.Symbol, id, weights[id], w)
			}
		}
	}
	if _, ok := ETFRepresentativeLookup("0050"); ok {
		t.Error("bare code 0050 must not resolve; the SOOT keys are .TW-suffixed")
	}
	if _, ok := ETFRepresentativeLookup("9999.TW"); ok {
		t.Error("undeclared ETF must not resolve")
	}
}

func findETF(t *testing.T, symbol string) (sectormap.ETFRepresentative, bool) {
	t.Helper()
	for _, r := range sectormap.ETFRepresentatives() {
		if r.Symbol == symbol {
			return r, true
		}
	}
	t.Errorf("%s is not declared in sectormap.NamespaceETFRepresentatives", symbol)
	return sectormap.ETFRepresentative{}, false
}

func sortedIDs(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
