package sectormap

import (
	"sort"
	"strings"
)

// ETFRepresentative declares one Taiwan-listed ETF's explicit mapping to one
// or more canonical L1 sector IDs.
//
// SSOT for PR-α (sectorallocation ETF representative expansion):
//   - 11 ETFs (mirrors configs/etf_metadata.json)
//   - 1:many L1 mapping per ETF (TW50 spans 8 L1 sectors; 等權重)
//   - 等權 1:many 是 spec §4 規範的選擇；不可改成主觀權重
//
// Why this lives in sectormap (not sectorallocation):
//   - sectormap is a leaf package (zero atlas-go project dependencies), so the
//     table can be reused by audit tools and tests without risk of an import
//     cycle.
//   - It also lets the audit CLI report this vocabulary as one more namespace
//     alongside the 12 already declared in tables.go.
//   - The sectorallocation package reads from here; it does not own the data.
//
// 護欄（PR-α 任務說明）：
//   - 不動 internal/industry/sector.go：本檔只引用已存在的 canonical L1 IDs
//     （canonicalL1 in canonical.go）；新增 ETF 對應不會動 sector.go。
//   - 每個 L1 target 必須在 canonicalL1 中（編譯期不可表達；測試期
//     TestETFRepresentatives_AllTargetsAreCanonical 嚴格把關）。
type ETFRepresentative struct {
	Symbol    string             // 含 .TW 後綴，例如 "0050.TW"
	Benchmark string             // benchmark 指數代碼，例如 "TW50"
	L1Targets map[string]float64 // 1:many mapping to canonical L1 IDs；weight 加總 = 1.0
}

var etfRepresentatives = []ETFRepresentative{
	{
		Symbol: "0050.TW", Benchmark: "TW50",
		L1Targets: l1Split([]string{
			"semiconductor", "electronics", "financials", "shipping",
			"steel", "telecom", "retail", "machinery",
		}),
	},
	{
		Symbol: "0056.TW", Benchmark: "TWHDividend",
		L1Targets: l1Split([]string{
			"financials", "telecom", "energy", "steel", "plastics", "cement",
		}),
	},
	{
		Symbol: "00878.TW", Benchmark: "MSCITWESG",
		L1Targets: l1Split([]string{
			"financials", "telecom", "energy", "steel", "chemicals",
		}),
	},
	{
		Symbol: "006208.TW", Benchmark: "TW50",
		L1Targets: l1Split([]string{
			"semiconductor", "electronics", "financials", "shipping",
			"steel", "telecom", "retail",
		}),
	},
	{
		Symbol: "00692.TW", Benchmark: "TWCG",
		L1Targets: l1Split([]string{
			"financials", "semiconductor", "electronics", "telecom",
		}),
	},
	{
		Symbol: "00713.TW", Benchmark: "TWHDivLowVol",
		L1Targets: l1Split([]string{
			"financials", "telecom", "energy", "steel", "cement",
		}),
	},
	{
		Symbol: "00881.TW", Benchmark: "TW5G",
		L1Targets: l1Split([]string{
			"telecom", "optoelectronics", "electronics",
		}),
	},
	{
		Symbol: "00891.TW", Benchmark: "TWSemi",
		L1Targets: l1Split([]string{
			"semiconductor", "optoelectronics", "electronics",
		}),
	},
	{
		Symbol: "00919.TW", Benchmark: "TWHDivSelect",
		L1Targets: l1Split([]string{
			"financials", "telecom", "energy", "steel", "machinery",
		}),
	},
	{
		Symbol: "00929.TW", Benchmark: "TWTechDiv",
		L1Targets: l1Split([]string{
			"semiconductor", "electronics", "telecom", "optoelectronics",
		}),
	},
	{
		Symbol: "00940.TW", Benchmark: "TWValDiv",
		L1Targets: l1Split([]string{
			"financials", "energy", "steel", "machinery",
		}),
	},
}

// l1Split 對 1:many L1 mapping 做等權分配；sum = 1.0（spec §4 強制）。
func l1Split(ids []string) map[string]float64 {
	out := make(map[string]float64, len(ids))
	if len(ids) == 0 {
		return out
	}
	w := 1.0 / float64(len(ids))
	for _, id := range ids {
		out[id] = w
	}
	return out
}

// ETFRepresentatives returns the full declared table as a deep copy. Callers
// may mutate the returned slice and its L1Targets maps without leaking into
// the package's SSOT.
func ETFRepresentatives() []ETFRepresentative {
	out := make([]ETFRepresentative, len(etfRepresentatives))
	for i, r := range etfRepresentatives {
		m := make(map[string]float64, len(r.L1Targets))
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

// ETFSymbols returns every declared ETF symbol, sorted.
func ETFSymbols() []string {
	out := make([]string, 0, len(etfRepresentatives))
	for _, r := range etfRepresentatives {
		out = append(out, r.Symbol)
	}
	sort.Strings(out)
	return out
}

// ETFRepresentativeLookup returns the L1 target map for one ETF symbol and
// whether the symbol is declared. Returns a defensive copy so the caller may
// mutate it without leaking into the package SSOT.
func ETFRepresentativeLookup(symbol string) (map[string]float64, bool) {
	for _, r := range etfRepresentatives {
		if r.Symbol == symbol {
			out := make(map[string]float64, len(r.L1Targets))
			for k, v := range r.L1Targets {
				out[k] = v
			}
			return out, true
		}
	}
	return nil, false
}

// ETFL1Coverage returns the union of canonical L1 IDs reached by any declared
// ETF representative, sorted ascending. SSOT for the audit metric
// "ETF L1 coverage" surfaced in cmd/experimental/industry-namespace-audit.
//
// PR-α acceptance target: ≥ 12. Current implementation produces 13.
func ETFL1Coverage() []string {
	seen := map[string]struct{}{}
	for _, r := range etfRepresentatives {
		for id := range r.L1Targets {
			if !IsCanonicalL1(id) {
				continue
			}
			seen[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ETFL1CoverageCount returns the cardinality of ETFL1Coverage() without
// materializing the slice. Used by tests and audit guard.
func ETFL1CoverageCount() int {
	seen := map[string]struct{}{}
	for _, r := range etfRepresentatives {
		for id := range r.L1Targets {
			if !IsCanonicalL1(id) {
				continue
			}
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}

// ETFL1CoverageMappersAreAllCanonical is a static safety valve: every L1
// target in the declared table MUST be in canonicalL1 (canonical.go). Returns
// nil if all targets are canonical, or *UnknownL1Error otherwise.
//
// PR-α guardrail（任務說明）：「不動 internal/industry/sector.go」；本函式確保
// 即便 sector.go 之後被擴充/縮減，本檔仍會紅燈直到人工對齊。
func ETFL1CoverageMappersAreAllCanonical() error {
	for _, r := range etfRepresentatives {
		for id := range r.L1Targets {
			if !IsCanonicalL1(id) {
				return &UnknownL1Error{ETF: r.Symbol, L1: id}
			}
		}
	}
	return nil
}

// UnknownL1Error is returned by ETFL1CoverageMappersAreAllCanonical when an
// ETF mapping points at an L1 sector ID that does not exist in canonicalL1.
// Exported so callers can use errors.As for diagnostics.
type UnknownL1Error struct {
	ETF string
	L1  string
}

func (e *UnknownL1Error) Error() string {
	return "sectormap: ETF " + e.ETF + " maps to unknown canonical L1 sector id " + e.L1
}

// ETFRepresentativeDisposition 把一個 ETF symbol 翻成 Mapping，方便 audit 工具
// 把它當成 namespace 報表的一部分輸出。每個 ETF 都映射到其 primary L1 target
// （L1Targets 中按字母序最小的），加上完整的 Targets map。status 一律為 StatusMapped
// （因為已顯式宣告）。
//
// 注意：這是 audit 報表用的視圖，不是 SSOT。SSOT 是 etfRepresentatives 變數。
func ETFRepresentativeDisposition(symbol string) (Mapping, bool) {
	rep, ok := ETFRepresentativeLookup(symbol)
	if !ok {
		return Mapping{}, false
	}
	// 確認每個 L1 都是 canonical
	valid := map[string]float64{}
	for id, w := range rep {
		if IsCanonicalL1(id) {
			valid[id] = w
		}
	}
	// 計算 primary (依字母序)
	keys := make([]string, 0, len(valid))
	for id := range valid {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return Mapping{
			Namespace: NamespaceETFRepresentatives,
			Key:       symbol,
			Targets:   map[string]float64{},
			Status:    StatusUnmapped,
			Reason:    "ETF " + symbol + " has no canonical L1 target",
		}, true
	}
	primary := keys[0]
	m := Mapping{
		Namespace: NamespaceETFRepresentatives,
		Key:       symbol,
		Targets:   valid,
		Status:    StatusMapped,
		Reason:    "PR-α ETF representative; primary L1=" + primary + " (1:many targets, weight=1/" + itoa(len(valid)) + ")",
	}
	_ = strings.Builder{} // keep strings import used for future Note variants
	return m, true
}

// itoa 是 strconv.Itoa 的極簡替代，避開對 strconv 的依賴以便更易讀。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// etfRepresentativeKeysMap 為 tables.go 提供 namespace 註冊：
// 把每個 ETF symbol 轉成一個具有 primary L1 target 的 decl entry。
//
// 此處的 decl 不重複完整的 1:many mapping — 完整資料在 etfRepresentatives 變數，
// audit 工具透過 ETFRepresentativeDisposition 取得。namespace 報表的 declared
// keys 數 = len(etfRepresentatives)；這個 helper 確保 key 數一致。
func etfRepresentativeKeysMap() map[string]decl {
	out := make(map[string]decl, len(etfRepresentatives))
	for _, r := range etfRepresentatives {
		// 取 primary L1 (alphabetical first)
		var primary string
		for id := range r.L1Targets {
			if primary == "" || id < primary {
				primary = id
			}
		}
		// status = mapped with full targets as map (即使 schema 預期 1:1)
		// 為了避免破壞既有 schema (Mapping.Primary 取最高 weight)，每個 target 都 weight=1/N
		out[r.Symbol] = decl{
			targets: copyTargets(r.L1Targets),
			reason:  "PR-α ETF representative; primary L1=" + primary + " (1:many mapping in sectorallocation.ETFRepresentatives)",
		}
	}
	return out
}

func copyTargets(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
