package sectormap

import (
	"sort"
	"testing"
)

// TestETFRepresentatives_AllTargetsAreCanonical 是 PR-α guardrail：
// 宣告表內所有 L1 target 必須在 canonicalL1（canonical.go 的 20 個 L1）中。
func TestETFRepresentatives_AllTargetsAreCanonical(t *testing.T) {
	if err := ETFL1CoverageMappersAreAllCanonical(); err != nil {
		t.Fatalf("ETFL1CoverageMappersAreAllCanonical returned error: %v", err)
	}
}

// TestETFRepresentatives_L1TargetsSumToOne 確保每個 ETF 的 L1Targets 加總 = 1.0。
func TestETFRepresentatives_L1TargetsSumToOne(t *testing.T) {
	for _, r := range ETFRepresentatives() {
		sum := 0.0
		for _, w := range r.L1Targets {
			sum += w
		}
		if sum < 0.999999999 || sum > 1.000000001 {
			t.Errorf("ETF %s L1Targets sum = %.12f, want 1.0", r.Symbol, sum)
		}
	}
}

// TestETFRepresentatives_Count 是 PR-α acceptance metric：
// 任務要求 ETF L1 coverage ≥ 12。
func TestETFRepresentatives_Count(t *testing.T) {
	got := ETFL1CoverageCount()
	if got < 12 {
		t.Errorf("ETFL1CoverageCount = %d, want >= 12 (PR-α acceptance)", got)
	}
	t.Logf("ETFL1CoverageCount = %d (target >= 12)", got)
}

// TestETFRepresentatives_CoverageMatchesCanonicalL1 確保所有 L1 target 都來自
// canonicalL1（canonical.go 的 20 個 L1 IDs）。
func TestETFRepresentatives_CoverageMatchesCanonicalL1(t *testing.T) {
	coverage := ETFL1Coverage()
	canonical := map[string]struct{}{}
	for _, id := range CanonicalL1IDs() {
		canonical[id] = struct{}{}
	}
	for _, id := range coverage {
		if _, ok := canonical[id]; !ok {
			t.Errorf("ETFL1Coverage contains %q which is not in CanonicalL1IDs()", id)
		}
	}
	if !sort.StringsAreSorted(coverage) {
		t.Errorf("ETFL1Coverage not sorted: %v", coverage)
	}
}

// TestETFRepresentatives_SymbolsSorted 確保 ETFSymbols() 回傳 sorted 結果。
func TestETFRepresentatives_SymbolsSorted(t *testing.T) {
	syms := ETFSymbols()
	if !sort.StringsAreSorted(syms) {
		t.Errorf("ETFSymbols not sorted: %v", syms)
	}
	if len(syms) != len(ETFRepresentatives()) {
		t.Errorf("ETFSymbols length %d != ETFRepresentatives length %d", len(syms), len(ETFRepresentatives()))
	}
}

// TestETFRepresentatives_DoesNotMutateInternalState 確保呼叫者拿到的是 copy，
// 改返回值不會污染 SSOT。
func TestETFRepresentatives_DoesNotMutateInternalState(t *testing.T) {
	r1 := ETFRepresentatives()
	r1[0].L1Targets["hacked"] = 0.5
	r1[0].Symbol = "HACK"

	r2 := ETFRepresentatives()
	if _, leaked := r2[0].L1Targets["hacked"]; leaked {
		t.Error("ETFRepresentatives leaked L1Targets map: callers can mutate shared state")
	}
	if r2[0].Symbol == "HACK" {
		t.Error("ETFRepresentatives leaked Symbol field")
	}
}

// TestETFRepresentativeLookup_KnownAndUnknown 確認 lookup 行為。
func TestETFRepresentativeLookup_KnownAndUnknown(t *testing.T) {
	known := "0050.TW"
	m, ok := ETFRepresentativeLookup(known)
	if !ok {
		t.Fatalf("ETFRepresentativeLookup(%q) ok=false, want true", known)
	}
	if len(m) == 0 {
		t.Errorf("ETFRepresentativeLookup(%q) returned empty map", known)
	}
	if _, hasSemi := m["semiconductor"]; !hasSemi {
		t.Errorf("ETFRepresentativeLookup(%q) missing semiconductor target", known)
	}
	m["hacked"] = 1.0
	m2, ok := ETFRepresentativeLookup(known)
	if !ok {
		t.Fatal("second lookup failed")
	}
	if _, leaked := m2["hacked"]; leaked {
		t.Error("ETFRepresentativeLookup leaked internal state")
	}

	unknown := "9999.TW"
	if _, ok := ETFRepresentativeLookup(unknown); ok {
		t.Errorf("ETFRepresentativeLookup(%q) ok=true, want false", unknown)
	}
}

// TestETFRepresentativeLookup_AllDeclaredSymbolsResolve 確保 ETFSymbols() 中的
// 每個 symbol 都能成功 lookup。
func TestETFRepresentativeLookup_AllDeclaredSymbolsResolve(t *testing.T) {
	for _, sym := range ETFSymbols() {
		if _, ok := ETFRepresentativeLookup(sym); !ok {
			t.Errorf("ETFRepresentativeLookup(%q) ok=false for declared symbol", sym)
		}
	}
}

// TestUnknownL1Error_Message 確保 error message 包含 ETF 與 L1 ID。
func TestUnknownL1Error_Message(t *testing.T) {
	e := &UnknownL1Error{ETF: "9999.TW", L1: "fake_sector"}
	want := "sectormap: ETF 9999.TW maps to unknown canonical L1 sector id fake_sector"
	if got := e.Error(); got != want {
		t.Errorf("UnknownL1Error.Error() = %q, want %q", got, want)
	}
}

// TestETFRepresentatives_NoDuplicateSymbols 確保沒有重複的 ETF symbol。
func TestETFRepresentatives_NoDuplicateSymbols(t *testing.T) {
	seen := map[string]struct{}{}
	for _, r := range ETFRepresentatives() {
		if _, dup := seen[r.Symbol]; dup {
			t.Errorf("duplicate ETF symbol %q in etfRepresentatives", r.Symbol)
		}
		seen[r.Symbol] = struct{}{}
	}
}

// TestETFRepresentatives_AllSymbolsHaveTW 是格式護欄。
func TestETFRepresentatives_AllSymbolsHaveTW(t *testing.T) {
	for _, r := range ETFRepresentatives() {
		if len(r.Symbol) < 4 || r.Symbol[len(r.Symbol)-3:] != ".TW" {
			t.Errorf("ETF %q does not end with .TW", r.Symbol)
		}
	}
}

// TestETFRepresentativeDisposition_KnownETF 確認 audit disposition helper。
func TestETFRepresentativeDisposition_KnownETF(t *testing.T) {
	m, ok := ETFRepresentativeDisposition("0050.TW")
	if !ok {
		t.Fatal("ETFRepresentativeDisposition(0050.TW) ok=false")
	}
	if m.Status != StatusMapped {
		t.Errorf("0050.TW status = %q, want %q", m.Status, StatusMapped)
	}
	if m.Namespace != NamespaceETFRepresentatives {
		t.Errorf("namespace = %q, want %q", m.Namespace, NamespaceETFRepresentatives)
	}
	if len(m.Targets) == 0 {
		t.Error("0050.TW should have targets")
	}
}

// TestETFRepresentativeDisposition_UnknownETF 確認未知 symbol 回 false。
func TestETFRepresentativeDisposition_UnknownETF(t *testing.T) {
	_, ok := ETFRepresentativeDisposition("9999.TW")
	if ok {
		t.Error("ETFRepresentativeDisposition(9999.TW) ok=true, want false")
	}
}

// TestETFRepresentatives_NamespaceRegistration 確保 11 個 ETF keys 已註冊到
// NamespaceETFRepresentatives namespace table 中（tables.go）。
func TestETFRepresentatives_NamespaceRegistration(t *testing.T) {
	table, ok := tables[NamespaceETFRepresentatives]
	if !ok {
		t.Fatalf("namespace %q not in tables map", NamespaceETFRepresentatives)
	}
	for _, r := range ETFRepresentatives() {
		if _, ok := table[r.Symbol]; !ok {
			t.Errorf("ETF %q not registered in namespace table", r.Symbol)
		}
	}
	if got, want := len(table), len(etfRepresentatives); got != want {
		t.Errorf("namespace table size = %d, want %d", got, want)
	}
}
