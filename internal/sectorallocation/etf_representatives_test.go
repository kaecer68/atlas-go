package sectorallocation

import (
	"sort"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

// TestETFRepresentatives_AllTargetsAreCanonical 是 PR-α guardrail：
// 宣告表內所有 L1 target 必須在 industry.L1Sectors() 中。
// 任務說明：「不動 internal/industry/sector.go」；本測試確保即使 sector.go 之後
// 被擴充/縮減，本檔仍會紅燈直到人工對齊。
func TestETFRepresentatives_AllTargetsAreCanonical(t *testing.T) {
	if err := ETFL1CoverageMappersAreAllCanonical(); err != nil {
		t.Fatalf("ETFL1CoverageMappersAreAllCanonical returned error: %v", err)
	}
}

// TestETFRepresentatives_L1TargetsSumToOne 確保每個 ETF 的 L1Targets map 加總 = 1.0。
// 1:many mapping 在 spec §4 中明確要求 weight 總和 = 1.0。
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

// TestETFRepresentatives_CoverageMatchesL1Sectors 確保所有 L1 target 都來自
// industry.L1Sectors() 的 20 個 ID，並以 sorted 順序回傳。
func TestETFRepresentatives_CoverageMatchesL1Sectors(t *testing.T) {
	coverage := ETFL1Coverage()
	want := industry.L1Sectors()
	// 只比對 ETFL1Coverage 內的 IDs 必須是 L1Sectors 的子集
	validL1 := map[industry.SectorID]struct{}{}
	for _, id := range want {
		validL1[id] = struct{}{}
	}
	for _, id := range coverage {
		if _, ok := validL1[id]; !ok {
			t.Errorf("ETFL1Coverage contains %q which is not in industry.L1Sectors()", id)
		}
	}
	// 也確認 sorted
	if !sort.SliceIsSorted(coverage, func(i, j int) bool { return coverage[i] < coverage[j] }) {
		t.Errorf("ETFL1Coverage is not sorted: %v", coverage)
	}
}

// TestETFRepresentatives_SymbolsSorted 確保 ETFSymbols() 回傳 sorted 結果。
func TestETFRepresentatives_SymbolsSorted(t *testing.T) {
	syms := ETFSymbols()
	if !sort.SliceIsSorted(syms, func(i, j int) bool { return syms[i] < syms[j] }) {
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

// TestETFRepresentativeLookup_KnownAndUnknown 確認 lookup 行為：
// 已知 symbol 回 (map, true)；未知 symbol 回 (nil, false)。
func TestETFRepresentativeLookup_KnownAndUnknown(t *testing.T) {
	known := "0050.TW"
	m, ok := ETFRepresentativeLookup(known)
	if !ok {
		t.Fatalf("ETFRepresentativeLookup(%q) ok=false, want true", known)
	}
	if len(m) == 0 {
		t.Errorf("ETFRepresentativeLookup(%q) returned empty map", known)
	}
	if _, hasSemi := m[industry.SectorSemiconductor]; !hasSemi {
		t.Errorf("ETFRepresentativeLookup(%q) missing semiconductor target", known)
	}
	// 改 map 不污染 SSOT
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
// 每個 symbol 都能成功 lookup，避免宣告不一致。
func TestETFRepresentativeLookup_AllDeclaredSymbolsResolve(t *testing.T) {
	for _, sym := range ETFSymbols() {
		if _, ok := ETFRepresentativeLookup(sym); !ok {
			t.Errorf("ETFRepresentativeLookup(%q) ok=false for declared symbol", sym)
		}
	}
}

// TestUnknownL1Error_Message 確保 error message 包含 ETF 與 L1 ID，方便排查。
func TestUnknownL1Error_Message(t *testing.T) {
	e := &UnknownL1Error{ETF: "9999.TW", L1: "fake_sector"}
	want := "ETF 9999.TW maps to unknown L1 sector id fake_sector"
	if got := e.Error(); got != "sectorallocation: "+want {
		t.Errorf("UnknownL1Error.Error() = %q, want %q", got, "sectorallocation: "+want)
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

// TestETFRepresentatives_AllSymbolsHaveTW 是格式護欄：所有 ETF symbol 都必須
// 以 ".TW" 後綴結尾，與 configs/etf_metadata.json 一致。
func TestETFRepresentatives_AllSymbolsHaveTW(t *testing.T) {
	for _, r := range ETFRepresentatives() {
		if len(r.Symbol) < 4 || r.Symbol[len(r.Symbol)-3:] != ".TW" {
			t.Errorf("ETF %q does not end with .TW", r.Symbol)
		}
	}
}

// TestETFRepresentatives_AllL1TargetsCoveredByAudit 是 PR-α 與 audit 工具的
// 整合測試：每個 ETF 至少有一個 L1 target 出現在 ETFL1Coverage()。
func TestETFRepresentatives_AllL1TargetsCoveredByAudit(t *testing.T) {
	coverage := map[industry.SectorID]struct{}{}
	for _, id := range ETFL1Coverage() {
		coverage[id] = struct{}{}
	}
	for _, r := range ETFRepresentatives() {
		for id := range r.L1Targets {
			if _, ok := coverage[id]; !ok {
				t.Errorf("ETF %s targets %q but ETFL1Coverage() does not list it", r.Symbol, id)
			}
		}
	}
}

// TestETFRepresentatives_AlignsWithSectormap 確保 sectorallocation 與 sectormap
// 兩邊的 SSOT 對齊：symbols 數、L1 coverage 數、個別 symbol 的 L1 mapping 都必須一致。
//
// 任何不一致都會紅燈，強制人工對齊（PR-α 不容許兩份 SSOT）。
func TestETFRepresentatives_AlignsWithSectormap(t *testing.T) {
	// 1) Symbol 數對齊
	symsSalloc := ETFSymbols()
	symsSector := sectormap.ETFSymbols()
	if len(symsSalloc) != len(symsSector) {
		t.Errorf("symbol count mismatch: sectorallocation=%d sectormap=%d", len(symsSalloc), len(symsSector))
	}
	for i := range symsSalloc {
		if symsSalloc[i] != symsSector[i] {
			t.Errorf("symbol[%d] mismatch: sectorallocation=%q sectormap=%q", i, symsSalloc[i], symsSector[i])
		}
	}

	// 2) L1 coverage 對齊
	covSalloc := ETFL1Coverage()
	covSector := sectormap.ETFL1Coverage()
	if len(covSalloc) != len(covSector) {
		t.Errorf("L1 coverage count mismatch: sectorallocation=%d sectormap=%d", len(covSalloc), len(covSector))
	}
	for i := range covSalloc {
		if string(covSalloc[i]) != covSector[i] {
			t.Errorf("L1[%d] mismatch: sectorallocation=%q sectormap=%q", i, covSalloc[i], covSector[i])
		}
	}

	// 3) 個別 symbol 的 L1 mapping 對齊
	for _, sym := range symsSalloc {
		mSalloc, okSalloc := ETFRepresentativeLookup(sym)
		mSector, okSector := sectormap.ETFRepresentativeLookup(sym)
		if okSalloc != okSector {
			t.Errorf("symbol %q lookup ok mismatch: sectorallocation=%v sectormap=%v", sym, okSalloc, okSector)
		}
		if !okSalloc {
			continue
		}
		if len(mSalloc) != len(mSector) {
			t.Errorf("symbol %q mapping count mismatch: sectorallocation=%d sectormap=%d", sym, len(mSalloc), len(mSector))
		}
		for id, w := range mSalloc {
			if wSector, ok := mSector[string(id)]; !ok || w != wSector {
				t.Errorf("symbol %q L1 %q weight mismatch: sectorallocation=%v sectormap=%v", sym, id, w, wSector)
			}
		}
	}

	// 4) Canonical guard 對齊
	if errSalloc := ETFL1CoverageMappersAreAllCanonical(); errSalloc != nil {
		if errSector := sectormap.ETFL1CoverageMappersAreAllCanonical(); errSector == nil {
			t.Errorf("sectorallocation canonical guard failed but sectormap passes: %v", errSalloc)
		}
	}
}
