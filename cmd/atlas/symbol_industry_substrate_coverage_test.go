package main

// symbol_industry_substrate_coverage_test.go — unit tests for the coverage
// accounting that feeds the honest universe coverage audit (issue #1943
// follow-up: the old audit printed ratio=1.00 by construction).
//
// The bucketing itself lives in symbolIndustryCoverageFromEntries, a pure
// function, so it is tested here (package main, next to the loader that owns it)
// without a database or a live first-party channel. The end-to-end wiring --
// that the substrate is installed and that the audit consumes Coverage() -- is
// covered by TestNewSymbolIndustrySubstrate_GateOnLoadsPerStockField and by
// internal/monitoring's coverage tests.

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/symbolindustry"
)

// ─── symbolIndustryCoverageFromEntries ────────────────────────────────────

// TestSymbolIndustryCoverageFromEntries pins the accounting that the audit
// divides. The invariant that matters: Upstream counts EVERY row the channel
// saw (including the rows that could not be classified), and Resolved counts
// exactly the rows the loader installs.
func TestSymbolIndustryCoverageFromEntries(t *testing.T) {
	t.Run("mixed_population_buckets_by_documented_statuses", func(t *testing.T) {
		entries := []symbolindustry.Entry{
			{
				Symbol: "1101", CanonicalL1: "cement", MappingStatus: symbolindustry.StatusMapped,
				MappingReason: "canonical L1 cement 為唯一對應",
			},
			{
				Symbol: "2330", CanonicalL1: "semiconductor", MappingStatus: symbolindustry.StatusMapped,
				MappingReason: "canonical L1 semiconductor 為唯一對應",
			},
			{
				Symbol: "2801", MappingStatus: symbolindustry.StatusUnmapped,
				MappingReason: "canonical taxonomy 無對應節點",
			},
			{
				Symbol: "910322", MappingStatus: symbolindustry.StatusUnknown,
				MappingReason: "code not declared in namespace twse_industry_code (upstream drift; report, do not map)",
			},
			{
				Symbol: "1234", MappingStatus: symbolindustry.StatusUnknown,
				MappingReason: "code not declared in namespace twse_industry_code (upstream drift; report, do not map)",
			},
		}

		got := symbolIndustryCoverageFromEntries(entries)

		if got.Upstream != 5 {
			t.Fatalf("Upstream = %d, want 5 (every row the channel saw)", got.Upstream)
		}
		if got.Resolved != 2 {
			t.Fatalf("Resolved = %d, want 2", got.Resolved)
		}
		if got.Unmapped != 1 {
			t.Fatalf("Unmapped = %d, want 1", got.Unmapped)
		}
		if got.Unknown != 2 {
			t.Fatalf("Unknown = %d, want 2", got.Unknown)
		}
		if got.Unmapped+got.Unknown+got.Resolved != got.Upstream {
			t.Fatalf("resolved+unmapped+unknown = %d, want Upstream = %d",
				got.Unmapped+got.Unknown+got.Resolved, got.Upstream)
		}
		// Reasons describe the GAPS: the resolved rows' reasons (which document
		// a sound mapping) must not be carried into the audit.
		want := []string{
			"canonical taxonomy 無對應節點",
			"code not declared in namespace twse_industry_code (upstream drift; report, do not map)",
		}
		if !reflect.DeepEqual(got.Reasons, want) {
			t.Fatalf("Reasons = %q, want %q (distinct, sorted, unresolved rows only)", got.Reasons, want)
		}
	})

	t.Run("empty_population_is_zero_not_full", func(t *testing.T) {
		got := symbolIndustryCoverageFromEntries(nil)
		if got.Upstream != 0 || got.Resolved != 0 || got.Unmapped != 0 || got.Unknown != 0 {
			t.Fatalf("empty entries must yield an all-zero accounting, got %+v", got)
		}
		if got.Reasons != nil {
			t.Fatalf("empty entries must yield no reasons, got %q", got.Reasons)
		}
	})

	t.Run("full_coverage_has_no_reasons", func(t *testing.T) {
		got := symbolIndustryCoverageFromEntries([]symbolindustry.Entry{
			{
				Symbol: "1101", CanonicalL1: "cement", MappingStatus: symbolindustry.StatusMapped,
				MappingReason: "canonical L1 cement 為唯一對應",
			},
			{
				Symbol: "2330", CanonicalL1: "semiconductor", MappingStatus: symbolindustry.StatusMapped,
				MappingReason: "canonical L1 semiconductor 為唯一對應",
			},
		})
		if got.Upstream != 2 || got.Resolved != 2 || got.Unmapped != 0 || got.Unknown != 0 {
			t.Fatalf("accounting = %+v, want 2/2/0/0", got)
		}
		if len(got.Reasons) != 0 {
			t.Fatalf("a fully resolved population has no gap reasons, got %q", got.Reasons)
		}
	})

	t.Run("canonical_l1_without_symbol_is_not_resolved", func(t *testing.T) {
		// Defensive: a row without a symbol cannot be installed, so it must not
		// inflate the numerator (Resolved == the pipeline population).
		got := symbolIndustryCoverageFromEntries([]symbolindustry.Entry{
			{Symbol: "", CanonicalL1: "cement", MappingStatus: symbolindustry.StatusMapped},
			{Symbol: "2330", CanonicalL1: "semiconductor", MappingStatus: symbolindustry.StatusMapped},
		})
		if got.Upstream != 2 || got.Resolved != 1 {
			t.Fatalf("accounting = %+v, want Upstream=2 Resolved=1", got)
		}
	})
}

// ─── storeSymbolIndustrySubstrate.Coverage ────────────────────────────────

// coverageFakeStore is a minimal symbolindustry.Store: loadAll fails when
// loadErr is set, which is how the "failed load must not look green" case is
// exercised.
type coverageFakeStore struct {
	entries []symbolindustry.Entry
	loadErr error
}

func (s *coverageFakeStore) UpsertAll(context.Context, []symbolindustry.Entry) (int, error) {
	return 0, errors.New("not implemented")
}

func (s *coverageFakeStore) LoadAll(context.Context) ([]symbolindustry.Entry, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return append([]symbolindustry.Entry(nil), s.entries...), nil
}

func (s *coverageFakeStore) Lookup(context.Context, string) (symbolindustry.Entry, bool, error) {
	return symbolindustry.Entry{}, false, errors.New("not implemented")
}

func (s *coverageFakeStore) Count(context.Context) (int, error) {
	return len(s.entries), nil
}

// TestStoreSubstrateCoverage_MatchesInstalledPopulation is the invariant the
// audit depends on: the numerator the audit publishes is the population the
// pipeline uses, and the denominator is larger than it whenever upstream rows
// stayed unresolved.
func TestStoreSubstrateCoverage_MatchesInstalledPopulation(t *testing.T) {
	sub := &storeSymbolIndustrySubstrate{
		ttl: time.Hour,
		now: time.Now,
		store: &coverageFakeStore{entries: []symbolindustry.Entry{
			{Symbol: "1101", CanonicalL1: "cement", MappingStatus: symbolindustry.StatusMapped},
			{Symbol: "2330", CanonicalL1: "semiconductor", MappingStatus: symbolindustry.StatusMapped},
			{Symbol: "2603", CanonicalL1: "shipping", MappingStatus: symbolindustry.StatusMapped},
			{Symbol: "2801", MappingStatus: symbolindustry.StatusUnmapped, MappingReason: "residual bucket"},
			{Symbol: "910322", MappingStatus: symbolindustry.StatusUnknown, MappingReason: "upstream drift"},
		}},
	}

	cov := sub.Coverage()
	if cov.Upstream != 5 || cov.Resolved != 3 || cov.Unmapped != 1 || cov.Unknown != 1 {
		t.Fatalf("Coverage() = %+v, want 5/3/1/1", cov)
	}
	if got := len(sub.Symbols()); got != cov.Resolved {
		t.Fatalf("installed population = %d, want Resolved = %d", got, cov.Resolved)
	}
	reporter, ok := industry.SymbolIndustrySubstrate(sub).(industry.SymbolIndustryCoverageReporter)
	if !ok {
		t.Fatal("the store substrate must implement the optional coverage reporter")
	}
	if got := reporter.Coverage(); got.Upstream != cov.Upstream {
		t.Fatalf("reporter.Coverage() upstream = %d, want %d", got.Upstream, cov.Upstream)
	}
}

// TestStoreSubstrateCoverage_FailedLoadReportsNothing pins the failure
// semantics: a load failure leaves Upstream at 0, which the audit reads as
// "coverage not measurable" -- never as 100 % coverage.
func TestStoreSubstrateCoverage_FailedLoadReportsNothing(t *testing.T) {
	sub := &storeSymbolIndustrySubstrate{
		ttl:   time.Hour,
		now:   time.Now,
		store: &coverageFakeStore{loadErr: errors.New("db down")},
	}

	cov := sub.Coverage()
	if cov.Upstream != 0 || cov.Resolved != 0 {
		t.Fatalf("a failed load must report nothing, got %+v", cov)
	}
	if got := len(sub.Symbols()); got != 0 {
		t.Fatalf("a failed load must install no symbols, got %d", got)
	}
}
