package sectormap

import (
	"math"
	"slices"
	"strings"
	"testing"
)

// ETF L1 coverage is a *product* metric, not a code metric: it answers "how many
// equity industries can sector allocation actually reach through the ETF
// vehicles it owns?". These tests pin the floor and the provenance of every row
// so neither can be softened without failing the build.

const etfCoverageFloor = 12

func TestETFRepresentatives_CoverageFloor(t *testing.T) {
	got := ETFL1Coverage()
	if len(got) < etfCoverageFloor {
		t.Fatalf("ETF L1 coverage = %d (%v), floor is %d", len(got), got, etfCoverageFloor)
	}
	for _, id := range got {
		if !IsCanonicalL1(id) {
			t.Errorf("ETF coverage contains non-L1 ID %q", id)
		}
	}
	if n := ETFL1CoverageCount(); n != len(got) {
		t.Errorf("ETFL1CoverageCount() = %d but ETFL1Coverage() has %d entries", n, len(got))
	}
}

func TestETFRepresentatives_KeysComeFromTheETFMetadataSSOT(t *testing.T) {
	var meta map[string]struct {
		Name      string `json:"name"`
		Benchmark string `json:"benchmark"`
	}
	loadJSON(t, "configs/etf_metadata.json", &meta)

	assertSameKeys(t, "configs/etf_metadata.json ETF symbols",
		sortedKeys(meta), ETFRepresentativeSymbols())

	for _, r := range ETFRepresentatives() {
		m, ok := meta[r.Symbol]
		if !ok {
			continue
		}
		if r.Name != m.Name {
			t.Errorf("%s: declared name %q but configs/etf_metadata.json says %q", r.Symbol, r.Name, m.Name)
		}
		if r.Benchmark != m.Benchmark {
			t.Errorf("%s: declared benchmark %q but configs/etf_metadata.json says %q", r.Symbol, r.Benchmark, m.Benchmark)
		}
	}
}

func TestETFRepresentatives_NamespaceHasNoUnmappedKey(t *testing.T) {
	rep := Report(NamespaceETFRepresentatives)
	if rep.Unmapped != 0 {
		t.Errorf("ETF namespace has %d unmapped keys %v; every ETF must resolve or be reported as unknown, not left dangling",
			rep.Unmapped, rep.UnmappedKeys)
	}
	if rep.DeclaredKeys == 0 || rep.Mapped != rep.DeclaredKeys {
		t.Errorf("ETF namespace declared=%d mapped=%d canonical=%d, want every key mapped",
			rep.DeclaredKeys, rep.Mapped, rep.Canonical)
	}
	if len(rep.CoveredL1) != ETFL1CoverageCount() {
		t.Errorf("namespace report covers %d L1 but ETFL1Coverage() says %d", len(rep.CoveredL1), ETFL1CoverageCount())
	}
}

func TestETFRepresentatives_EveryRowCarriesFirstPartyEvidence(t *testing.T) {
	for _, r := range ETFRepresentatives() {
		if !strings.HasPrefix(r.SourceURL, "https://") {
			t.Errorf("%s: source_url %q is not an https first-party URL", r.Symbol, r.SourceURL)
		}
		if r.AsOf == "" || r.Issuer == "" {
			t.Errorf("%s: issuer/as_of must be recorded (got issuer=%q as_of=%q)", r.Symbol, r.Issuer, r.AsOf)
		}
		if r.Holdings < 3 {
			t.Errorf("%s: only %d holdings recorded; the L1 vector cannot be credible", r.Symbol, r.Holdings)
		}
		if r.ReportedWeightPct <= 0 || r.MappedWeightPct <= 0 {
			t.Errorf("%s: weights must be positive (reported=%v mapped=%v)", r.Symbol, r.ReportedWeightPct, r.MappedWeightPct)
		}
		if r.MappedWeightPct > r.ReportedWeightPct+1e-9 {
			t.Errorf("%s: mapped weight %v exceeds reported weight %v", r.Symbol, r.MappedWeightPct, r.ReportedWeightPct)
		}

		m := Resolve(NamespaceETFRepresentatives, r.Symbol)
		if m.Status != StatusMapped {
			t.Errorf("%s: resolved status %q, want %q", r.Symbol, m.Status, StatusMapped)
			continue
		}
		for _, want := range []string{r.Issuer, r.SourceURL, r.AsOf, "twse_industry_code"} {
			if !strings.Contains(m.Reason, want) {
				t.Errorf("%s: mapping reason omits %q:\n%s", r.Symbol, want, m.Reason)
			}
		}
	}
}

func TestETFRepresentatives_TargetsAreCanonicalL1AndSumToOne(t *testing.T) {
	for _, r := range ETFRepresentatives() {
		if len(r.L1Targets) == 0 {
			t.Errorf("%s has an empty L1 vector", r.Symbol)
		}
		sum := 0.0
		for id, w := range r.L1Targets {
			if !IsCanonicalL1(id) {
				t.Errorf("%s maps to %q which is not a canonical L1 sector", r.Symbol, id)
			}
			if w <= 0 {
				t.Errorf("%s has non-positive weight %v for %q", r.Symbol, w, id)
			}
			sum += w
		}
		if math.Abs(sum-1.0) > 1e-9 {
			t.Errorf("%s L1 weights sum to %v, want 1", r.Symbol, sum)
		}
	}
}

func TestETFRepresentatives_SortedAndDeepCopy(t *testing.T) {
	rows := ETFRepresentatives()
	if !slices.IsSorted(ETFRepresentativeSymbols()) {
		t.Error("ETFRepresentativeSymbols() is not sorted")
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Symbol >= rows[i].Symbol {
			t.Errorf("rows are not ordered by symbol: %s before %s", rows[i-1].Symbol, rows[i].Symbol)
		}
	}
	if len(rows) == 0 {
		t.Fatal("no ETF rows declared")
	}
	rows[0].L1Targets["semiconductor"] = 99
	again := ETFRepresentatives()
	if again[0].L1Targets["semiconductor"] == 99 {
		t.Error("ETFRepresentatives() leaks the package table to callers")
	}
}
