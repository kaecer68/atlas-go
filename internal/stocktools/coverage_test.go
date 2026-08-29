// Tests for the coverage lookup and the /api/stock/coverage endpoint.
//
// Test surface:
//   - LookupCoverage: nil/empty/3-stock/empty-symbol cases
//   - HandleCoverage: 400 bad request, 200 with body for in-scope / out-of-scope symbols
//
// These tests must remain GREEN forever — they encode the stocktools scope
// contract. See docs/manifests/2026-08-06-stock-coverage-notice.md §6.
package stocktools

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// writeSnapshot creates a temp directory with a fundamentals.json containing
// only the (symbol → data) pairs the caller specifies. Each call gets a
// fresh temp dir to keep tests independent.
func writeSnapshot(t *testing.T, entries map[string]portfolio.FundamentalData) *portfolio.FundamentalProvider {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fundamentals.json")
	raw := map[string]portfolio.FundamentalData{}
	maps.Copy(raw, entries)
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON(path); err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	return fp
}

// LookupCoverage variant: a symbol present in the snapshot is covered.
func TestLookupCoverage_InSnapshot(t *testing.T) {
	fp := writeSnapshot(t, map[string]portfolio.FundamentalData{
		"6641.TW": {PE: 39.67, PB: 0.45, DividendYield: 2.19, Sector: ""},
	})
	// Empty Sector — explicit guarantee that "covered" still holds.
	got := LookupCoverage("6641", fp)
	if !got.Covered {
		t.Errorf("expected Covered=true for 6641, got %+v", got)
	}
	if got.Listing != ListingTWSE {
		t.Errorf("expected Listing=%q, got %q", ListingTWSE, got.Listing)
	}
	if got.Symbol != "6641.TW" {
		t.Errorf("canonical symbol want 6641.TW, got %q", got.Symbol)
	}
	if !got.QuoteCovered {
		t.Errorf("QuoteCovered must always be true (Fugle covers everything)")
	}
	if got.Reason != "" {
		t.Errorf("Reason should be empty for covered symbols, got %q", got.Reason)
	}
}

// LookupCoverage variant: 3131 (上櫃 TPEX) is not in the snapshot → uncovered.
func TestLookupCoverage_OOTC(t *testing.T) {
	fp := writeSnapshot(t, map[string]portfolio.FundamentalData{
		"6641.TW": {PE: 39.67},
	})
	got := LookupCoverage("3131", fp)
	if got.Covered {
		t.Errorf("expected Covered=false for 3131 (上櫃), got %+v", got)
	}
	if got.Listing != ListingUnknown {
		t.Errorf("expected Listing=%q, got %q", ListingUnknown, got.Listing)
	}
	if !got.QuoteCovered {
		t.Errorf("QuoteCovered must remain true even for out-of-scope symbols")
	}
	if got.Reason == "" {
		t.Errorf("Reason must explain out-of-scope to human callers")
	}
}

// LookupCoverage variant: nil provider → treated as "no coverage info"
// (the guard cannot short-circuit, so the caller proceeds and may or may
// not produce useful data depending on other deps).
func TestLookupCoverage_NilProvider(t *testing.T) {
	got := LookupCoverage("6641", nil)
	if got.Covered {
		t.Errorf("nil provider must yield Covered=false (no data to verify against), got %+v", got)
	}
	if got.Symbol != "6641.TW" {
		t.Errorf("canonical key must still normalize, got %q", got.Symbol)
	}
}

// LookupCoverage variant: explicit suffix (e.g. caller passed `2330.TW`)
// passes through unchanged.
func TestLookupCoverage_PassThroughSuffix(t *testing.T) {
	fp := writeSnapshot(t, map[string]portfolio.FundamentalData{
		"2330.TW": {PE: 25},
	})
	got := LookupCoverage("2330.TW", fp)
	if !got.Covered {
		t.Errorf("expected Covered=true for 2330.TW, got %+v", got)
	}
	if got.Symbol != "2330.TW" {
		t.Errorf("canonical symbol want 2330.TW, got %q", got.Symbol)
	}
}

// LookupCoverage variant: empty symbol returns Covered=false with a reason.
func TestLookupCoverage_EmptySymbol(t *testing.T) {
	fp := writeSnapshot(t, map[string]portfolio.FundamentalData{
		"6641.TW": {PE: 39.67},
	})
	got := LookupCoverage("", fp)
	if got.Covered {
		t.Errorf("empty symbol must yield Covered=false, got %+v", got)
	}
	if got.Reason == "" {
		t.Errorf("Reason must explain why empty symbol is not covered")
	}
}

// LookupCoverage variant: loss-making company with all-zero PE/PB in the
// snapshot must still be reported as Covered=true (key presence, not data
// magnitude, is the contract).
func TestLookupCoverage_ZeroDataSymbol(t *testing.T) {
	fp := writeSnapshot(t, map[string]portfolio.FundamentalData{
		"1234.TW": {PE: 0, PB: 0, PS: nil, DividendYield: 0, Sector: ""},
	})
	got := LookupCoverage("1234", fp)
	if !got.Covered {
		t.Errorf("Covered must be based on key presence, not field magnitude. got %+v", got)
	}
}

// HandleCoverage: 400 for missing symbol.
func TestHandleCoverage_MissingSymbol(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/coverage", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// HandleCoverage: 200 + structured body for an in-scope symbol.
func TestHandleCoverage_InScope(t *testing.T) {
	fp := writeSnapshot(t, map[string]portfolio.FundamentalData{
		"6641.TW": {PE: 39.67},
	})
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Fundamentals: fp})

	req := httptest.NewRequest(http.MethodGet, "/api/stock/coverage?symbol=6641", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body CoverageEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.Covered {
		t.Errorf("expected Covered=true, got %+v", body)
	}
	if body.Symbol != "6641.TW" {
		t.Errorf("canonical symbol: want 6641.TW, got %q", body.Symbol)
	}
}

// HandleCoverage: 200 + structured body for an out-of-scope symbol.
func TestHandleCoverage_OutOfScope(t *testing.T) {
	fp := writeSnapshot(t, map[string]portfolio.FundamentalData{
		"6641.TW": {PE: 39.67},
	})
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Fundamentals: fp})

	req := httptest.NewRequest(http.MethodGet, "/api/stock/coverage?symbol=3131", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (NOT 503), got %d", rec.Code)
	}
	var body CoverageEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Covered {
		t.Errorf("expected Covered=false for 3131, got %+v", body)
	}
	if body.Listing != ListingUnknown {
		t.Errorf("expected Listing=%q, got %q", ListingUnknown, body.Listing)
	}
	if body.Reason == "" {
		t.Errorf("out-of-scope response must include a human-readable reason")
	}
	if !body.QuoteCovered {
		t.Errorf("QuoteCovered should still be true (Fugle covers)")
	}
}
