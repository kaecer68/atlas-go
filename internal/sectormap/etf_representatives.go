package sectormap

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ETFRepresentative is one declared ETF → canonical L1 exposure row.
//
// Why this vocabulary exists: sector allocation needs to know which equity
// industries a Taiwan-listed ETF spans, because ETFs are the vehicle the
// allocation layer can trade. Before this table every consumer assembled that
// answer by hand, so the same ETF had a different L1 set per call site — the
// failure mode issue #1943 banned for sector keys.
//
// Every row is *derived*, not authored:
//
//  1. holdings come from the issuer's own published portfolio page or API
//     (SourceURL / AsOf); nothing here is copied from a blog or an estimate;
//  2. each holding's industry comes from the first-party TWSE/TPEx listed
//     company industry code (產業別 / SecuritiesIndustryCode);
//  3. code → canonical L1 is the declared twse_industry_code table.
//
// internal/sectorallocation re-runs that derivation from the checked-in
// snapshot (internal/sectorallocation/testdata/etf_holdings_20260924.json) on
// every test run, so a hand-edited row or a stale snapshot fails the build
// instead of silently rotting.
type ETFRepresentative struct {
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
	// Benchmark is the tracked index code from configs/etf_metadata.json.
	Benchmark string `json:"benchmark"`
	Issuer    string `json:"issuer"`
	// AsOf is the data date printed on the issuer's holdings page.
	AsOf string `json:"as_of"`
	// SourceURL is the exact first-party page or API the holdings came from.
	SourceURL string `json:"source_url"`
	// Holdings is the number of equity positions the issuer published.
	Holdings int `json:"holdings"`
	// ReportedWeightPct is the sum of the published per-position weights. It is
	// below 100 whenever the fund also holds futures, cash or margin.
	ReportedWeightPct float64 `json:"reported_weight_pct"`
	// MappedWeightPct is the part of ReportedWeightPct whose industry code
	// resolves to a canonical L1 sector.
	MappedWeightPct float64 `json:"mapped_weight_pct"`
	// L1Targets is the renormalised canonical L1 exposure; it sums to 1.
	L1Targets map[string]float64 `json:"l1_targets"`
}

// Evidence renders the audit trail of one row: issuer page, data date, the
// per-symbol industry source, and how the uncovered weight was handled.
func (r ETFRepresentative) Evidence() string {
	unmapped := r.ReportedWeightPct - r.MappedWeightPct
	if unmapped < 0 {
		unmapped = 0
	}
	return r.Issuer + " published holdings " + r.SourceURL +
		" (data date " + r.AsOf + ", " + strconv.Itoa(r.Holdings) + " equity positions, " +
		percent(r.ReportedWeightPct) + "% of NAV) -> per-symbol TWSE/TPEx listed-company" +
		" industry code (t187ap03_L / mopsfin_t187ap03_O) -> canonical L1 via the" +
		" twse_industry_code table; positions without a canonical L1 target (" +
		percent(unmapped) + "% of NAV) are reported and excluded, the remainder renormalised to 1"
}

// percent renders a weight percentage with up to two decimals.
func percent(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" {
		return "0"
	}
	return s
}

// etfRepresentatives is the authoritative list, ordered by ETF symbol.
var etfRepresentatives = []ETFRepresentative{
	{
		Symbol:            "0050.TW",
		Name:              "元大台灣50",
		Benchmark:         "TW50",
		Issuer:            "元大投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://www.yuantaetfs.com/product/detail/0050/ratio",
		Holdings:          50,
		ReportedWeightPct: 99.71,
		MappedWeightPct:   99.71,
		L1Targets: map[string]float64{
			"biotech":           0.00331,
			"electronics":       0.137398,
			"energy":            0.000903,
			"financials":        0.083241,
			"food":              0.00341,
			"optoelectronics":   0.005416,
			"other_electronics": 0.040116,
			"plastics":          0.010932,
			"semiconductor":     0.695617,
			"shipping":          0.002507,
			"telecom":           0.01715,
		},
	},
	{
		Symbol:            "0056.TW",
		Name:              "元大高股息",
		Benchmark:         "TWHDividend",
		Issuer:            "元大投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://www.yuantaetfs.com/product/detail/0056/ratio",
		Holdings:          50,
		ReportedWeightPct: 98.36,
		MappedWeightPct:   93.94,
		L1Targets: map[string]float64{
			"cement":            0.007026,
			"electronics":       0.243986,
			"financials":        0.288588,
			"food":              0.025229,
			"machinery":         0.005003,
			"other_electronics": 0.029274,
			"plastics":          0.083138,
			"semiconductor":     0.210347,
			"shipping":          0.048329,
			"steel":             0.006813,
			"telecom":           0.052267,
		},
	},
	{
		Symbol:            "006208.TW",
		Name:              "富邦台50",
		Benchmark:         "TW50",
		Issuer:            "富邦投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://websys.fsit.com.tw/FubonETF/Fund/Assets.aspx?stkId=006208",
		Holdings:          50,
		ReportedWeightPct: 99.6368,
		MappedWeightPct:   99.6368,
		L1Targets: map[string]float64{
			"biotech":           0.003357,
			"electronics":       0.137096,
			"energy":            0.000928,
			"financials":        0.083318,
			"food":              0.003416,
			"optoelectronics":   0.005452,
			"other_electronics": 0.04013,
			"plastics":          0.010882,
			"semiconductor":     0.695776,
			"shipping":          0.00245,
			"telecom":           0.017195,
		},
	},
	{
		Symbol:            "00692.TW",
		Name:              "富邦公司治理",
		Benchmark:         "TWCG",
		Issuer:            "富邦投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://websys.fsit.com.tw/FubonETF/Fund/Assets.aspx?stkId=00692",
		Holdings:          100,
		ReportedWeightPct: 99.639,
		MappedWeightPct:   98.7978,
		L1Targets: map[string]float64{
			"biotech":           0.000437,
			"cement":            0.001035,
			"chemicals":         0.00034,
			"construction":      0.000645,
			"electronics":       0.133602,
			"financials":        0.105557,
			"food":              0.000601,
			"machinery":         0.00356,
			"optoelectronics":   0.002111,
			"other_electronics": 0.034102,
			"plastics":          0.015448,
			"retail":            0.001808,
			"semiconductor":     0.676465,
			"shipping":          0.005124,
			"steel":             0.000796,
			"telecom":           0.016133,
			"textiles":          0.002236,
		},
	},
	{
		Symbol:            "00713.TW",
		Name:              "元大高股息低波動",
		Benchmark:         "TWHDivLowVol",
		Issuer:            "元大投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://www.yuantaetfs.com/product/detail/00713/ratio",
		Holdings:          50,
		ReportedWeightPct: 97.7,
		MappedWeightPct:   90.7,
		L1Targets: map[string]float64{
			"auto":          0.023705,
			"chemicals":     0.012459,
			"construction":  0.007828,
			"electronics":   0.082139,
			"energy":        0.007607,
			"financials":    0.306064,
			"food":          0.116538,
			"retail":        0.076516,
			"semiconductor": 0.058434,
			"shipping":      0.057883,
			"steel":         0.023043,
			"telecom":       0.169901,
			"textiles":      0.057883,
		},
	},
	{
		Symbol:            "00878.TW",
		Name:              "國泰永續高股息",
		Benchmark:         "MSCITWESG",
		Issuer:            "國泰投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://cwapi.cathaysite.com.tw/api/ETF/GetETFDetailStockList?FundCode=CN&SearchDate=2026-09-24&status=1",
		Holdings:          29,
		ReportedWeightPct: 97.33,
		MappedWeightPct:   95.87,
		L1Targets: map[string]float64{
			"electronics":   0.271931,
			"financials":    0.345885,
			"food":          0.023261,
			"retail":        0.018045,
			"semiconductor": 0.207781,
			"shipping":      0.06029,
			"telecom":       0.072807,
		},
	},
	{
		Symbol:            "00881.TW",
		Name:              "國泰台灣5G+",
		Benchmark:         "TW5G",
		Issuer:            "國泰投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://cwapi.cathaysite.com.tw/api/ETF/GetETFDetailStockList?FundCode=CR&SearchDate=2026-09-24&status=1",
		Holdings:          30,
		ReportedWeightPct: 99.09,
		MappedWeightPct:   98.58,
		L1Targets: map[string]float64{
			"electronics":       0.263441,
			"optoelectronics":   0.0211,
			"other_electronics": 0.080138,
			"semiconductor":     0.602353,
			"telecom":           0.032968,
		},
	},
	{
		Symbol:            "00891.TW",
		Name:              "中信關鍵半導體",
		Benchmark:         "TWSemi",
		Issuer:            "中國信託投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://www.ctbcinvestments.com.tw/API/etf/ETFHoldingWeight?FID=E0017&StartDate=2026-09-24",
		Holdings:          30,
		ReportedWeightPct: 98.95,
		MappedWeightPct:   96.98,
		L1Targets: map[string]float64{
			"electronics":   0.037018,
			"semiconductor": 0.94999,
			"telecom":       0.012992,
		},
	},
	{
		Symbol:            "00919.TW",
		Name:              "群益台灣精選高息",
		Benchmark:         "TWHDivSelect",
		Issuer:            "群益投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://www.capitalfund.com.tw/CFWeb/api/etf/buyback (POST JSON {\"fundId\":\"195\",\"date\":null})",
		Holdings:          40,
		ReportedWeightPct: 99.8477,
		MappedWeightPct:   93.0242,
		L1Targets: map[string]float64{
			"auto":              0.003921,
			"chemicals":         0.003385,
			"construction":      0.023851,
			"electronics":       0.218702,
			"financials":        0.573344,
			"food":              0.003811,
			"other_electronics": 0.009471,
			"semiconductor":     0.070348,
			"shipping":          0.075063,
			"steel":             0.002899,
			"textiles":          0.015205,
		},
	},
	{
		Symbol:            "00929.TW",
		Name:              "復華台灣科技優息",
		Benchmark:         "TWTechDiv",
		Issuer:            "復華投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://www.fhtrust.com.tw/api/assets?fundID=ETF21&qDate=2026/09/24",
		Holdings:          50,
		ReportedWeightPct: 96.587,
		MappedWeightPct:   85.597,
		L1Targets: map[string]float64{
			"electronics":       0.36284,
			"optoelectronics":   0.08061,
			"other_electronics": 0.170286,
			"semiconductor":     0.291541,
			"telecom":           0.094723,
		},
	},
	{
		Symbol:            "00940.TW",
		Name:              "元大台灣價值高息",
		Benchmark:         "TWValDiv",
		Issuer:            "元大投信",
		AsOf:              "2026-09-24",
		SourceURL:         "https://www.yuantaetfs.com/product/detail/00940/ratio",
		Holdings:          50,
		ReportedWeightPct: 97.46,
		MappedWeightPct:   90.75,
		L1Targets: map[string]float64{
			"auto":              0.011019,
			"cement":            0.016859,
			"electronics":       0.252452,
			"financials":        0.213554,
			"food":              0.023361,
			"machinery":         0.027328,
			"optoelectronics":   0.08595,
			"other_electronics": 0.071515,
			"retail":            0.027328,
			"semiconductor":     0.15427,
			"shipping":          0.090138,
			"telecom":           0.026226,
		},
	},
}

// etfRepresentativeKeys adapts the list above to the namespace table format.
// The Reason carries the full evidence chain, so an operator can audit one row
// without opening this file.
var etfRepresentativeKeys = func() map[string]decl {
	out := make(map[string]decl, len(etfRepresentatives))
	for _, r := range etfRepresentatives {
		out[r.Symbol] = decl{targets: maps.Clone(r.L1Targets), reason: r.Evidence()}
	}
	return out
}()

// ETFRepresentatives returns every declared row as a deep copy, ordered by
// symbol. Callers may mutate the result.
func ETFRepresentatives() []ETFRepresentative {
	out := make([]ETFRepresentative, len(etfRepresentatives))
	for i, r := range etfRepresentatives {
		out[i] = r
		out[i].L1Targets = maps.Clone(r.L1Targets)
	}
	return out
}

// ETFRepresentativeSymbols returns the declared ETF symbols, sorted.
func ETFRepresentativeSymbols() []string {
	out := make([]string, 0, len(etfRepresentatives))
	for _, r := range etfRepresentatives {
		out = append(out, r.Symbol)
	}
	return slices.Sorted(slices.Values(out))
}

// ETFL1Coverage returns the canonical L1 sectors reachable from any declared
// ETF, sorted. Single source of truth for the "ETF L1 coverage" metric that
// cmd/experimental/industry-namespace-audit publishes.
func ETFL1Coverage() []string { return CoveredL1(NamespaceETFRepresentatives, true) }

// ETFL1CoverageCount is the cardinality of ETFL1Coverage().
func ETFL1CoverageCount() int { return len(ETFL1Coverage()) }
