// Coverage lookup for stocktools 4 endpoints.
//
// Purpose: Provide a single, cheap, deterministic source of truth for which
// TW symbols the data pipeline actually covers. The 4 endpoints (quote /
// fundamentals / chips / technical) currently accept any symbol and either
// silently return zero/fake data or fail with a misleading
// `context canceled` error (see docs/investigations/2026-08-06-equipment-stocks-chips-gaps.md).
//
// Sole data source: portfolio.FundamentalProvider (backed by
// data/fundamentals.json, ~1070 TWSE-listed common stocks). Coverage is
// intentionally narrow by design — TPEX-listed symbols are not in scope
// (see docs/manifests/2026-08-06-stock-coverage-notice.md §4 out-of-scope).
//
// Quote coverage (Fugle) is broader than fundamentals/chips coverage
// (TWSE T86), so `QuoteCovered` is reported as a static hint only — the
// actual quote handler keeps its Fugle-first/TWSE-fallback behavior intact.
package stocktools

import (
	"net/http"
	"strings"

	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// Listing tags returned in CoverageEntry.Listing. Stable string values;
// rename requires touching frontend render + MCP tool descriptions.
const (
	ListingTWSE         = "TWSE"            // snapshot includes this symbol
	ListingUnknown      = "UNKNOWN"         // snapshot exists but symbol absent
	ListingOTCQouteOnly = "TPEX_QUOTE_ONLY" // Fugle covers quote, no chips/fundamentals
)

// CoverageNoteNotCovered is the value placed in the `coverage_note` field
// of all 4 stocktools endpoint responses when the requested symbol is
// out-of-scope. Frontend and MCP callers branch on this exact value.
const CoverageNoteNotCovered = "NOT_COVERED"

// CoverageEntry describes whether a symbol is in stocktools' data scope.
type CoverageEntry struct {
	Symbol       string `json:"symbol"`
	Covered      bool   `json:"covered"`       // chips/fundamentals/technical are reachable
	Listing      string `json:"listing"`       // ListingTWSE / ListingUnknown / etc.
	QuoteCovered bool   `json:"quote_covered"` // Fugle covers (looser than Covered)
	Reason       string `json:"reason"`        // human-readable Chinese explanation
}

// canonicalKey applies the same `.TW`-suffix canonicalization the
// fundamentals handler does before consulting the snapshot. Pulled out as
// a separate function so /api/stock/coverage and the 4 stocktools handlers
// share one definition (avoids drift if the suffix rule ever changes).
func canonicalKey(rawSymbol string) string {
	return normalizeFundamentalsSymbol(rawSymbol)
}

// snapshotHas reports whether the symbol key is present in the snapshot.
// We use FundamentalProvider.HasSymbol directly (added 2026-08-06 to
// support the coverage guard) rather than Get() — Get() returns the zero
// value for both missing keys and present-but-empty entries, which would
// mis-classify loss-making companies (real PE=0 entries) as out-of-scope.
//
// Returns false (not panic) when fp is nil — the caller treats this as
// "coverage service unavailable" and skips the guard.
func snapshotHas(fp *portfolio.FundamentalProvider, canonical string) bool {
	if fp == nil || !fp.HasData() {
		return false
	}
	return fp.HasSymbol(canonical)
}

// LookupCoverage returns the coverage entry for a raw symbol (4–6 digit
// Taiwan code, optionally with `.TWO`/`.TW` suffix). It is cheap; safe to
// call per-request. Never returns nil.
//
// Cost: one map lookup in portfolio.FundamentalProvider.data (~1µs).
func LookupCoverage(rawSymbol string, fp *portfolio.FundamentalProvider) CoverageEntry {
	canonical := canonicalKey(rawSymbol)
	e := CoverageEntry{
		Symbol:       canonical,
		Covered:      false,
		Listing:      ListingUnknown,
		QuoteCovered: true, // Fugle quotes are non-discriminating; assume covered.
	}

	// Empty symbol: defensive return — handled at handler boundary usually,
	// but keep LookupCoverage self-contained.
	if strings.TrimSpace(rawSymbol) == "" {
		e.Reason = "symbol 不可為空"
		return e
	}

	if snapshotHas(fp, canonical) {
		e.Covered = true
		e.Listing = ListingTWSE
		return e
	}
	e.Reason = "本系統 chips/fundamentals 涵蓋台灣上市普通股；此股票代號不在資料範圍內"
	return e
}

// notCoveredResponse builds the 200 OK body returned by the 4 stocktools
// handlers when a symbol is out-of-scope. Returning 200 (not 503) avoids
// cascading "server failure" signals to LLM callers (atlas-mcp) and
// keeps the JSON shape uniform: a `coverage_note` field reliably tells
// the frontend/MCP render layer to show a coverage badge instead of an
// error banner.
//
// extra carries endpoint-specific payload (quote data, technical bars
// etc.). When non-nil, it is merged into the response so callers that
// can partially answer (e.g. quote via Fugle for OTC symbols) still get
// useful data alongside the coverage_note.
func notCoveredResponse(symbol string, cov CoverageEntry, extra map[string]any) (int, any) {
	body := map[string]any{
		"symbol":        symbol,
		"coverage_note": CoverageNoteNotCovered,
		"listing":       cov.Listing,
		"covered":       false,
		"reason":        cov.Reason,
	}
	for k, v := range extra {
		if _, exists := body[k]; !exists {
			body[k] = v
		}
	}
	return http.StatusOK, body
}

// HandleCoverage implements `GET /api/stock/coverage?symbol=X`.
// Returns 200 always (even for out-of-scope symbols); the structured
// `covered` boolean tells callers whether the 4 stocktools endpoints will
// return real data. This avoids 4xx storms when the frontend pre-checks
// an arbitrary symbol during autocomplete / search.
func (h *Handler) HandleCoverage(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	return http.StatusOK, LookupCoverage(symbol, h.deps.Fundamentals)
}
