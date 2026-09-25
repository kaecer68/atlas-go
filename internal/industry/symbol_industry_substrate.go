package industry

import (
	"slices"
	"strings"
	"sync"
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
