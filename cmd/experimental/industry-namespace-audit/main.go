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
	ETFL1Coverage   *etfL1CoverageReport        `json:"etf_l1_coverage,omitempty"`
}

// etfL1CoverageReport 把 sectorallocation.ETFL1Coverage() 的結果與 sectormap 的
// namespace 報表結合輸出。PR-α 新增 metric。
type etfL1CoverageReport struct {
	Count     int                `json:"count"`
	Symbols   int                `json:"symbols"`
	L1Covered []string           `json:"l1_covered"`
	BySymbol  []etfBySymbolEntry `json:"by_symbol"`
}

// etfBySymbolEntry 把一個 ETF symbol 與其 L1 targets 列出，方便人工抽檢。
type etfBySymbolEntry struct {
	Symbol    string             `json:"symbol"`
	Benchmark string             `json:"benchmark"`
	L1Targets map[string]float64 `json:"l1_targets"`
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

	// PR-α: ETF L1 coverage metric.
	// SSOT 在 internal/sectormap/etf_representatives.go；sectorallocation 是 wrapper。
	etfCov := &etfL1CoverageReport{
		Count:     sectorallocation.ETFL1CoverageCount(),
		Symbols:   len(sectorallocation.ETFRepresentatives()),
		L1Covered: stringSliceFromSectorIDs(sectorallocation.ETFL1Coverage()),
	}
	for _, r := range sectorallocation.ETFRepresentatives() {
		m := make(map[string]float64, len(r.L1Targets))
		for k, v := range r.L1Targets {
			m[string(k)] = v
		}
		etfCov.BySymbol = append(etfCov.BySymbol, etfBySymbolEntry{
			Symbol:    r.Symbol,
			Benchmark: r.Benchmark,
			L1Targets: m,
		})
	}
	rep.ETFL1Coverage = etfCov

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

// stringSliceFromSectorIDs 把 industry.SectorID slice 轉成 string slice。
func stringSliceFromSectorIDs(ids []industry.SectorID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
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
	if rep.ETFL1Coverage != nil {
		fmt.Printf("ETF L1 coverage: %d/%d symbols cover %d L1 sectors\n",
			rep.ETFL1Coverage.Symbols, len(rep.ETFL1Coverage.BySymbol), rep.ETFL1Coverage.Count)
		fmt.Printf("  L1 covered: %v\n", rep.ETFL1Coverage.L1Covered)
	}
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
