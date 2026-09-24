// Package sectorallocation 的 ETF representatives 表 — thin wrapper over sectormap SSOT.
//
// 完整 PR-α 單一真相來源（SSOT）在 internal/sectormap/etf_representatives.go
// （leaf package，方便 audit 工具直接呼叫、避免 import cycle）。
//
// 本檔的角色：
//
//   - 把 sectormap.ETFRepresentative（key 為 string）轉成 ETFRepresentative
//     （key 為 industry.SectorID），方便 sectorallocation 內 typed call sites 使用。
//   - 對外提供 sectorallocation.ETFRepresentatives / ETFL1Coverage /
//     ETFL1CoverageCount / ETFL1CoverageMappersAreAllCanonical /
//     ETFRepresentativeLookup 等 API；呼叫端不直接知道 SSOT 在 sectormap。
//
// 護欄（PR-α 任務說明）：
//   - 不動 internal/industry/sector.go：本檔與 sectormap SSOT 都只引用已存在的
//     canonical L1 IDs（industry.L1Sectors() = sectormap.CanonicalL1IDs()）。
//   - 兩套 API 的 SSOT 對齊由 etf_representatives_test.go 的
//     TestETFRepresentatives_AlignsWithSectormap 把關；任何不一致 → 紅燈。
package sectorallocation

import (
	"sort"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

// ETFRepresentative 把一個 ETF symbol 顯式掛到多個 canonical L1 sector IDs。
//
// 等權重分配：若一支 ETF 同時跨 5 個 L1 sectors，則每個 L1 sector 分配 1/5 = 0.2。
// 「等權」是明確選擇（spec §4: 1:many mapping 必須帶 weight；無可考證時用等權）。
//
// Benchmark 與公開追蹤指數請參考 sectormap.ETFRepresentative 與 configs/etf_metadata.json。
type ETFRepresentative struct {
	Symbol    string
	Benchmark string
	L1Targets map[industry.SectorID]float64 // 1:many L1 mapping；每個 L1 weight 加總 = 1.0
}

// loadSSOT 把 sectormap SSOT 轉成 industry.SectorID-keyed 的 slice。
//
// 在 init 期一次性轉換；後續所有讀取都走 etfRepresentatives 這個 package-level slice。
// 若 sectormap 端有變更 → 重新編譯後此處會更新。
var etfRepresentatives = loadSSOTFromSectormap()

func loadSSOTFromSectormap() []ETFRepresentative {
	src := sectormap.ETFRepresentatives()
	out := make([]ETFRepresentative, 0, len(src))
	for _, r := range src {
		conv := make(map[industry.SectorID]float64, len(r.L1Targets))
		for k, v := range r.L1Targets {
			conv[industry.SectorID(k)] = v
		}
		out = append(out, ETFRepresentative{
			Symbol:    r.Symbol,
			Benchmark: r.Benchmark,
			L1Targets: conv,
		})
	}
	return out
}

// ETFRepresentatives returns the full set of declared ETF representatives as
// a deep copy. The returned slice is safe to mutate by callers; the underlying
// L1Targets maps are independent copies so internal state cannot leak.
func ETFRepresentatives() []ETFRepresentative {
	out := make([]ETFRepresentative, len(etfRepresentatives))
	for i, r := range etfRepresentatives {
		m := make(map[industry.SectorID]float64, len(r.L1Targets))
		for k, v := range r.L1Targets {
			m[k] = v
		}
		out[i] = ETFRepresentative{
			Symbol:    r.Symbol,
			Benchmark: r.Benchmark,
			L1Targets: m,
		}
	}
	return out
}

// ETFSymbols returns every declared ETF symbol, sorted ascending.
func ETFSymbols() []string {
	out := make([]string, 0, len(etfRepresentatives))
	for _, r := range etfRepresentatives {
		out = append(out, r.Symbol)
	}
	sort.Strings(out)
	return out
}

// ETFRepresentativeLookup returns the L1 target map for one ETF symbol and
// whether the symbol is declared. ok=false means the symbol has no canonical
// mapping (it is not necessarily a bug — caller decides).
//
// Returned map is a defensive copy so callers may mutate it freely.
func ETFRepresentativeLookup(symbol string) (map[industry.SectorID]float64, bool) {
	for _, r := range etfRepresentatives {
		if r.Symbol == symbol {
			out := make(map[industry.SectorID]float64, len(r.L1Targets))
			for k, v := range r.L1Targets {
				out[k] = v
			}
			return out, true
		}
	}
	return nil, false
}

// ETFL1Coverage returns the union of canonical L1 sector IDs reached by any
// declared ETF representative, sorted ascending. This is the SSOT for the
// "ETF L1 coverage" metric surfaced in the industry-namespace-audit CLI.
//
// PR-α acceptance target: coverage ≥ 12 L1 sectors.
func ETFL1Coverage() []industry.SectorID {
	seen := map[industry.SectorID]struct{}{}
	for _, r := range etfRepresentatives {
		for id := range r.L1Targets {
			if !industry.IsL1(id) {
				continue
			}
			seen[id] = struct{}{}
		}
	}
	out := make([]industry.SectorID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ETFL1CoverageCount returns len(ETFL1Coverage()) without materializing the
// slice. Useful for assertion paths and CI guard tests.
func ETFL1CoverageCount() int {
	n := 0
	seen := map[industry.SectorID]struct{}{}
	for _, r := range etfRepresentatives {
		for id := range r.L1Targets {
			if !industry.IsL1(id) {
				continue
			}
			seen[id] = struct{}{}
		}
	}
	for range seen {
		n++
	}
	return n
}

// ETFL1CoverageMappersAreAllCanonical 是一個靜態安全閥：宣告表內任何 L1 target
// 必須在 industry.L1Sectors() 中（即 sector.go 的 20 個 L1），否則回 error。
//
// 實作上直接委派給 sectormap 版本（SSOT 在 sectormap）。
//
// PR-α guardrail（任務說明）：「不動 internal/industry/sector.go」；本函式確保
// 即便 sector.go 之後被擴充/縮減，本檔仍會紅燈直到人工對齊。
func ETFL1CoverageMappersAreAllCanonical() error {
	err := sectormap.ETFL1CoverageMappersAreAllCanonical()
	if err == nil {
		return nil
	}
	// 把 sectormap.UnknownL1Error 包裝成 sectorallocation 版本以保留對外 API
	if e, ok := err.(*sectormap.UnknownL1Error); ok {
		return &UnknownL1Error{ETF: e.ETF, L1: e.L1}
	}
	return err
}

// UnknownL1Error is returned by ETFL1CoverageMappersAreAllCanonical when an
// ETF mapping points at an L1 sector ID that does not exist in industry.L1Sectors().
// It is exported so callers can use errors.As for diagnostics.
type UnknownL1Error struct {
	ETF string
	L1  string
}

func (e *UnknownL1Error) Error() string {
	return "sectorallocation: ETF " + e.ETF + " maps to unknown L1 sector id " + e.L1
}
