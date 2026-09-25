package industry

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// ─── Per-stock industry substrate (issue #1943) ─────────────────────────────
//
// #1943 unified the industry vocabularies but left the DATA population broken:
// symbol -> canonical L1 resolution was sourced only from the hard-coded
// representative-stock tables (internal/industry/representative_stocks.go and
// the classification tree's representative_stocks), which cover ~27 symbols in
// production — roughly 3.2 % of the listed market. Every industry-level
// statistic (sector exposure, SmartUniverse population, coverage audit) was
// therefore computed on a population too small to be significant.
//
// The first-party `symbol_industry` channel removes that limit: TWSE
// opendata/t187ap03_L (上市, 產業別) + TPEx mopsfin_t187ap03_O (上櫃,
// SecuritiesIndustryCode) cover the whole listed market and map every 2-digit
// code through the declared namespace K table (#1958,
// internal/sectormap/twse_industry_code.go).
//
// This file is the read-side PORT, not the loader: the concrete substrate (a
// database-backed lookup) lives in the wiring layer, so this package stays free
// of storage dependencies. Wiring installs it only when the config gate
// configs/parameters.json -> industry.substrate_from_symbol_industry_enabled is
// true (default false), which is what makes "gate off" byte-identical instead
// of merely similar.

// SymbolIndustrySubstrate is the per-stock industry field.
//
// Implementations MUST report unmapped/unknown codes by returning ok=false
// rather than guessing a sector: the upstream vocabulary leaves 14 of its 36
// codes without a defensible canonical target (#1958), and those codes are
// reported by the channel's own snapshot, never imputed here.
type SymbolIndustrySubstrate interface {
	// ResolveL1 returns the canonical L1 sector of a symbol. The symbol is
	// normalized (trimmed, ".TW" stripped) before lookup. ok=false means the
	// symbol is unknown or its upstream code has no canonical L1 target.
	ResolveL1(symbol string) (SectorID, bool)
	// Symbols returns every symbol the substrate knows, sorted ascending and
	// normalized. It is the substrate's population — the number the issue
	// #1943 acceptance criterion measures.
	Symbols() []string
}

// SymbolIndustryCoverage is the first-party `symbol_industry` channel's own
// accounting of the population it saw, reported by the component that loads it.
//
// It exists because a coverage audit needs a denominator that is not derived
// from the numerator (the pre-existing audit set total = mapped, so its ratio
// was 1.00 by construction and could never alert). The numbers here come from
// the upstream rows themselves:
//
//   - Upstream is the upstream population the channel saw: every row the
//     first-party fetch produced (TWSE t187ap03_L 上市 + TPEx mopsfin_t187ap03_O
//     上櫃), whether or not it could be classified.
//   - Resolved is the part of Upstream that carries a canonical L1 answer. It is
//     exactly the population the pipeline builds from (the substrate's Symbols).
//   - Unmapped and Unknown are the two documented unresolved reasons: a declared
//     code without a defensible single canonical L1 target, and a code that is
//     not declared at all (upstream drift). They are reported, never imputed.
//   - Reasons carries the distinct reason strings the unresolved rows carry, so
//     an operator can see WHY the population is short without opening a file.
type SymbolIndustryCoverage struct {
	Upstream int
	Resolved int
	Unmapped int
	Unknown  int
	Reasons  []string
}

// SymbolIndustryCoverageReporter is an OPTIONAL extension of
// SymbolIndustrySubstrate: it is deliberately NOT part of the port, so no
// existing implementation has to grow a method and no consumer can depend on it
// by accident. Consumers type-assert:
//
//	if reporter, ok := substrate.(industry.SymbolIndustryCoverageReporter); ok { ... }
//
// Only a substrate that is aware of the FIRST-PARTY upstream population can
// answer it: reporting Upstream requires knowing the rows that the channel saw
// but could not resolve, and a table-driven or tree-driven implementation has
// never seen them. An implementation that cannot answer must simply not
// implement the interface -- the consumer then reports the coverage as
// unavailable instead of inventing a denominator.
type SymbolIndustryCoverageReporter interface {
	Coverage() SymbolIndustryCoverage
}

// SymbolIndustryCoverageAsOfReporter is a SECOND, independently optional
// extension of SymbolIndustrySubstrate: the ability to DATE the accounting
// Coverage() returns.
//
// It is a separate interface, not a method added to
// SymbolIndustryCoverageReporter, for the same reason that one is separate from
// the port: adding a method to an existing interface breaks every implementation
// that already satisfies it (and every test that pins its method set). A
// reporter that cannot date its accounting must not be forced to invent a
// timestamp -- it simply does not implement this interface, and the consumer
// reports the age as unknown. Consumers type-assert independently:
//
//	if asOf, ok := substrate.(industry.SymbolIndustryCoverageAsOfReporter); ok { ... }
//
// Why the age matters: Coverage() answers from the last SUCCESSFUL load, and a
// failed reload leaves that view in place instead of emptying it (failing closed
// to the pre-#1943 behavior). So a substrate whose reloads have all failed since
// the last success keeps reporting a healthy-looking ratio computed from rows it
// loaded days ago, and the reload TTL does not bound it. Publishing the instant
// does not fix that; it is what turns "the check returned something" into "the
// check returned something dated N hours ago" (the #1944 / #1953 / false-green
// family: a check that reports having data instead of whether the data is
// right).
type SymbolIndustryCoverageAsOfReporter interface {
	// CoverageAsOf returns when the accounting reported by Coverage() was
	// loaded, and whether such a load has ever succeeded.
	//
	// The instant is the load's own wall-clock time, NOT the time the data
	// itself claims to be from: the upstream company-industry table carries no
	// per-row timestamp, and the channel snapshot's UpdatedAt describes the
	// FETCH, not the in-memory view that Coverage() divides. Callers must read
	// the pair as "how long has this view been serving": ok=false means no
	// successful load has ever happened (so Upstream is 0 and coverage is not
	// measurable), NOT that the data is fresh.
	CoverageAsOf() (time.Time, bool)
}

// SymbolIndustryReloadErrorReporter is a THIRD, independently optional
// extension of SymbolIndustrySubstrate: the last reload FAILURE, so an operator
// can tell "the load failed" apart from "the load succeeded and the population
// was empty".
//
// Both states surface as Upstream == 0 through Coverage(), and they call for
// opposite responses (fix the store vs. investigate the upstream channel), which
// is the exact ambiguity the coverage audit cannot resolve on its own. Separate
// interface, same compatibility rule as above; consumers type-assert
// independently:
//
//	if errReporter, ok := substrate.(industry.SymbolIndustryReloadErrorReporter); ok { ... }
type SymbolIndustryReloadErrorReporter interface {
	// LastReloadError returns the message of the most recent FAILED reload, or
	// "" when the most recent reload succeeded (or none has been attempted).
	// Implementations must NOT retry a load from this accessor: it reports the
	// evidence of the last attempt, and a getter that repairs the state it is
	// asked about erases that evidence.
	LastReloadError() string
}

// Resolution sources for SymbolL1Mapper.ResolveL1WithSource.
const (
	// L1SourceRepresentativeStocks: the answer came from the hard-coded
	// representative-stock tables (the pre-#1943 behavior).
	L1SourceRepresentativeStocks = "representative_stocks"
	// L1SourceSymbolIndustry: the answer came from the installed per-stock
	// industry substrate.
	L1SourceSymbolIndustry = "symbol_industry"
)

// ─── Consumption evidence (issue #1944 lesson) ──────────────────────────────
//
// #1944 found a whole write path with no reader: the data was stored, the
// status claimed it was in effect, and nothing consumed it. The substrate gate
// has the same failure shape (a loader that runs while every resolver ignores
// it), so wiring MUST announce its consumers here and the outward-facing
// evidence (PR body, audit output) can distinguish "installed" from "installed
// and consumed". Same contract as sectorallocation.RegisterPolicyConsumer.

var (
	substrateConsumerMu sync.RWMutex
	substrateConsumers  []string
)

// RegisterSymbolIndustryConsumer announces a downstream consumer that resolves
// symbols through the installed substrate. label must be a stable identifier
// (e.g. "monitoring.universe_builder"). Empty labels are ignored, duplicates are
// idempotent.
func RegisterSymbolIndustryConsumer(label string) {
	label = strings.TrimSpace(label)
	if label == "" {
		return
	}
	substrateConsumerMu.Lock()
	defer substrateConsumerMu.Unlock()
	if slices.Contains(substrateConsumers, label) {
		return
	}
	substrateConsumers = append(substrateConsumers, label)
}

// ResetSymbolIndustryConsumers clears the registry. Wiring owners call it on
// teardown/rollback; tests call it between cases so a process-global
// registration cannot leak (registration is a process-global fact).
func ResetSymbolIndustryConsumers() {
	substrateConsumerMu.Lock()
	defer substrateConsumerMu.Unlock()
	substrateConsumers = nil
}

// RegisteredSymbolIndustryConsumers returns the labels registered so far,
// sorted, as a copy.
func RegisteredSymbolIndustryConsumers() []string {
	substrateConsumerMu.RLock()
	defer substrateConsumerMu.RUnlock()
	out := slices.Clone(substrateConsumers)
	slices.Sort(out)
	return out
}

// SymbolIndustryConsumerWired reports whether any consumer announced itself.
func SymbolIndustryConsumerWired() bool {
	substrateConsumerMu.RLock()
	defer substrateConsumerMu.RUnlock()
	return len(substrateConsumers) > 0
}
