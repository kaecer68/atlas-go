// Command industry-namespace-audit reports how every sector vocabulary in
// atlas-go maps to the canonical taxonomy (issue #1943).
//
// Run:
//
//	go run ./cmd/experimental/industry-namespace-audit [flags]
//
// Flags:
//
//	-params    path to parameters JSON (default configs/parameters.json)
//	-universe  optional newline-separated symbol file; universe coverage is
//	           reported for it (one symbol per line, comments with '#')
//	-json      emit machine-readable JSON only
//
// Exit codes:
//
//	0 — every declared representative symbol resolves to a canonical L1 sector
//	1 — at least one declared representative symbol is unresolved
//
// Read-only: it never writes configuration, state or production data.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

type report struct {
	Namespaces      []industry.NamespaceAudit   `json:"namespaces"`
	TreeSegments    int                         `json:"tree_segments"`
	UnmappedSegment map[string]string           `json:"unmapped_segments"`
	Conflicts       []industry.SymbolL1Conflict `json:"conflicts"`
	DeclaredL1      industry.CanonicalCoverage  `json:"declared_l1_coverage"`
	DeclaredAll     industry.CanonicalCoverage  `json:"declared_all_coverage"`
	Universe        *universeReport             `json:"universe_coverage,omitempty"`
	GICSBlocked     *gicsReport                 `json:"legacy_gics,omitempty"`
	ETF             *etfCoverageReport          `json:"etf_l1_coverage,omitempty"`
}

// etfCoverageReport is the delivery-vehicle coverage metric: how many canonical
// L1 sectors the declared ETFs can actually reach, and the per-ETF exposure the
// number is built from. Both sides come from internal/sectormap, where the rows
// carry the issuer page and data date they were derived from.
type etfCoverageReport struct {
	// Symbols is the number of declared ETFs.
	Symbols int `json:"symbols"`
	// L1Covered is the union of canonical L1 sectors reachable from them.
	L1Covered []string `json:"l1_covered"`
	// Count is len(L1Covered); the acceptance floor is 12.
	Count int `json:"count"`
	// CanonicalL1Total is the taxonomy size, so the reader does not have to know it.
	CanonicalL1Total int `json:"canonical_l1_total"`
	// Rows is the per-ETF exposure, sorted by symbol.
	Rows []etfCoverageRow `json:"rows"`
}

type etfCoverageRow struct {
	Symbol     string             `json:"symbol"`
	Name       string             `json:"name"`
	Benchmark  string             `json:"benchmark"`
	Issuer     string             `json:"issuer"`
	AsOf       string             `json:"as_of"`
	SourceURL  string             `json:"source_url"`
	Holdings   int                `json:"holdings"`
	L1Covered  []string           `json:"l1_covered"`
	L1Weights  map[string]float64 `json:"l1_weights"`
	UnmappedL1 int                `json:"unmapped_l1"`
}

type universeReport struct {
	Source string                     `json:"source"`
	Cov    industry.CanonicalCoverage `json:"coverage"`
}

type gicsReport struct {
	UnmappedKeys []string `json:"unmapped_keys"`
	BlockedWt    float64  `json:"blocked_weight"`
	Error        string   `json:"error,omitempty"`
}

func main() {
	params := flag.String("params", "configs/parameters.json", "parameters JSON path")
	universe := flag.String("universe", "", "optional symbol file for universe coverage")
	asJSON := flag.Bool("json", true, "emit JSON")
	flag.Parse()

	config.SetParametersConfigPath(*params)
	config.ResetParametersConfig()
	cfg := config.GetParametersConfig()
	if cfg == nil {
		fmt.Fprintln(os.Stderr, "industry-namespace-audit: parameters config unavailable")
		os.Exit(2)
	}

	tree := industry.DefaultClassification()
	mapper, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		fmt.Fprintf(os.Stderr, "industry-namespace-audit: mapper: %v\n", err)
		os.Exit(2)
	}

	rep := report{
		Namespaces:      industry.AuditAllNamespaces(),
		TreeSegments:    len(tree.GetAllSegments()),
		UnmappedSegment: mapper.UnmappedSegments(),
		Conflicts:       mapper.Conflicts(),
		DeclaredL1: industry.ComputeCanonicalCoverage(
			industry.DeclaredRepresentativeUniverse(tree, industry.Level1), mapper),
		DeclaredAll: industry.ComputeCanonicalCoverage(
			append(industry.DeclaredRepresentativeUniverse(tree, industry.Level1),
				industry.DeclaredRepresentativeUniverse(tree, industry.Level2)...), mapper),
	}

	if *universe != "" {
		symbols, err := readSymbols(*universe)
		if err != nil {
			fmt.Fprintf(os.Stderr, "industry-namespace-audit: read universe: %v\n", err)
			os.Exit(2)
		}
		rep.Universe = &universeReport{Source: *universe, Cov: industry.ComputeCanonicalCoverage(symbols, mapper)}
	}

	if cfg.SectorAllocation.BaseWeights != nil {
		proj, err := sectorallocation.ProjectLegacyGICSWeights(cfg.SectorAllocation.BaseWeights)
		g := &gicsReport{UnmappedKeys: proj.UnmappedKeys, BlockedWt: proj.BlockedWeight}
		if err != nil {
			g.Error = err.Error()
		}
		rep.GICSBlocked = g
	}

	rep.ETF = buildETFReport()

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "industry-namespace-audit: encode: %v\n", err)
			os.Exit(2)
		}
	} else {
		printHuman(rep)
	}

	if rep.DeclaredL1.Mapped != rep.DeclaredL1.Universe {
		os.Exit(1)
	}
}

// buildETFReport assembles the ETF L1 coverage block. The ETF representative
// table is declared in internal/sectormap, so every number here is read, not
// computed: the audit is checking what the system claims, and the derivation
// itself is verified by internal/sectorallocation against the holdings snapshot.
func buildETFReport() *etfCoverageReport {
	etf := &etfCoverageReport{
		Symbols:          len(sectorallocation.ETFRepresentativeSymbols()),
		L1Covered:        sectorIDStrings(sectorallocation.ETFL1Coverage()),
		Count:            sectorallocation.ETFL1CoverageCount(),
		CanonicalL1Total: len(sectormap.CanonicalL1IDs()),
		Rows:             []etfCoverageRow{},
	}
	for _, r := range sectorallocation.ETFRepresentatives() {
		l1 := make([]industry.SectorID, 0, len(r.L1Weights))
		weights := make(map[string]float64, len(r.L1Weights))
		unmapped := 0
		for id, w := range r.L1Weights {
			l1 = append(l1, id)
			weights[string(id)] = w
			if !sectormap.IsCanonicalL1(string(id)) {
				unmapped++
			}
		}
		slices.Sort(l1)
		etf.Rows = append(etf.Rows, etfCoverageRow{
			Symbol:     r.Symbol,
			Name:       r.Name,
			Benchmark:  r.Benchmark,
			Issuer:     r.Issuer,
			AsOf:       r.AsOf,
			SourceURL:  r.SourceURL,
			Holdings:   r.Holdings,
			L1Covered:  sectorIDStrings(l1),
			L1Weights:  weights,
			UnmappedL1: unmapped,
		})
	}
	slices.SortFunc(etf.Rows, func(a, b etfCoverageRow) int { return strings.Compare(a.Symbol, b.Symbol) })
	return etf
}

func printHuman(rep report) {
	fmt.Printf("canonical taxonomy: %d L1 + %d L2 sectors\n",
		len(sectormap.CanonicalL1IDs()), len(sectormap.CanonicalL2IDs()))
	fmt.Printf("classification tree: %d segments, %d unmapped\n", rep.TreeSegments, len(rep.UnmappedSegment))
	for _, ns := range rep.Namespaces {
		fmt.Printf("  %-38s declared=%-3d canonical=%-3d mapped=%-3d unmapped=%-3d covers %d/20 L1\n",
			ns.Namespace, ns.DeclaredKeys, ns.CanonicalKey, ns.MappedKeys, ns.UnmappedKeys, len(ns.CoveredL1))
	}
	fmt.Printf("declared L1 representative symbols: %d/%d mapped\n", rep.DeclaredL1.Mapped, rep.DeclaredL1.Universe)
	fmt.Printf("declared symbols (L1+L2)          : %d/%d mapped\n", rep.DeclaredAll.Mapped, rep.DeclaredAll.Universe)
	if rep.Universe != nil {
		fmt.Printf("universe %s: %d/%d mapped (%.4f)\n",
			rep.Universe.Source, rep.Universe.Cov.Mapped, rep.Universe.Cov.Universe, rep.Universe.Cov.Ratio)
	}
	if rep.GICSBlocked != nil {
		fmt.Printf("legacy GICS weights: blocked=%.4f unmapped=%v\n",
			rep.GICSBlocked.BlockedWt, rep.GICSBlocked.UnmappedKeys)
	}
	if rep.ETF != nil {
		fmt.Printf("ETF L1 coverage: %d/%d canonical L1 reached by %d declared ETFs (floor 12)\n",
			rep.ETF.Count, rep.ETF.CanonicalL1Total, rep.ETF.Symbols)
		fmt.Printf("  L1 covered: %v\n", rep.ETF.L1Covered)
		for _, r := range rep.ETF.Rows {
			fmt.Printf("  %-10s %-14s %-22s as_of=%s holdings=%-3d L1=%d %v\n",
				r.Symbol, r.Name, r.Issuer, r.AsOf, r.Holdings, len(r.L1Covered), r.L1Covered)
		}
	}
}

// sectorIDStrings renders SectorIDs as plain strings.
func sectorIDStrings(ids []industry.SectorID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

// readSymbols reads a newline-separated symbol list; blank lines and lines
// starting with '#' are ignored.
func readSymbols(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	sort.Strings(out)
	return out, nil
}
