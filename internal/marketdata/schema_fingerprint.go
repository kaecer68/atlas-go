package marketdata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// ─── P2-15: response schema fingerprints ────────────────────────────────────
//
// Goal: upstream column changes should surface as a WARN the moment they
// happen instead of only failing in production hours later. Every provider
// gets a "response fingerprint" — the required fields + JSON types for its
// main endpoint. The fingerprint is checked on every successful fetch and
// logs a warn on mismatch (變更即 warn log). Hard parse gates downstream
// (typed ErrSchema errors, breaker trips) decide whether the fetch fails;
// the fingerprint is the early, human-readable canary.
//
// Wiring per provider:
//   - FinMind: fetchDataset checks the envelope + first data row against the
//     dataset's required fields (registry below).
//   - Yahoo: UnmarshalYahooChart (the single shared parse point for all 8+
//     channels) checks chart.result[0].indicators.quote — a missing quote
//     array would otherwise PANIC consumers that index Quote[0].
//   - TWSE: GetQuotes checks the STOCK_DAY_ALL fields header + row width
//     (the fixed-index parser silently skips short rows).
//   - TAIFEX: FetchPCR re-checks the raw row keys (the struct-level
//     parseOK checks already hard-fail on missing fields; the fingerprint
//     adds the warn).
//
// Field-kind check is intentionally shallow: it validates the JSON type the
// parser depends on (string / number / array ...), not semantic ranges.

// fingerprintKind is the expected JSON type of a fingerprint field.
type fingerprintKind int

const (
	fingerprintAny fingerprintKind = iota
	fingerprintString
	fingerprintNumber
	fingerprintBool
	fingerprintArray
	fingerprintObject
)

func (k fingerprintKind) String() string {
	switch k {
	case fingerprintString:
		return "string"
	case fingerprintNumber:
		return "number"
	case fingerprintBool:
		return "bool"
	case fingerprintArray:
		return "array"
	case fingerprintObject:
		return "object"
	default:
		return "any"
	}
}

// fingerprintField is one required field in a response fingerprint.
type fingerprintField struct {
	name string // JSON key (top-level key of the checked object)
	kind fingerprintKind
}

// responseFingerprint is the expected shape of one endpoint's response.
type responseFingerprint struct {
	provider string // logging component ("finmind", "yahoo", "twse", "taifex")
	endpoint string // human-readable endpoint / dataset name
	fields   []fingerprintField
}

// checkFingerprint validates obj against the spec and returns human-readable
// problems ("missing field X", "field X has type string, want number").
// Pure function so tests can assert the exact problem list.
func checkFingerprint(fp responseFingerprint, obj map[string]any) []string {
	var problems []string
	for _, f := range fp.fields {
		v, ok := obj[f.name]
		if !ok {
			problems = append(problems, fmt.Sprintf("missing field %q", f.name))
			continue
		}
		if f.kind == fingerprintAny {
			continue
		}
		if !fingerprintKindMatches(f.kind, v) {
			problems = append(problems, fmt.Sprintf("field %q has type %T, want %s", f.name, v, f.kind))
		}
	}
	return problems
}

// fingerprintKindMatches reports whether v has the JSON type k expects.
func fingerprintKindMatches(k fingerprintKind, v any) bool {
	switch k {
	case fingerprintString:
		_, ok := v.(string)
		return ok
	case fingerprintNumber:
		switch v.(type) {
		case float64, float32, int, int64, int32, json.Number:
			return true
		}
		return false
	case fingerprintBool:
		_, ok := v.(bool)
		return ok
	case fingerprintArray:
		// Accept both []any and []map[string]any: FinMind's Data field is
		// []map[string]any after JSON decode (k3 audit 2026-08-24 — the
		// []any-only assertion made every successful fetchDataset log a
		// false schema-change warn, drowning out real drift alerts).
		switch v.(type) {
		case []any, []map[string]any:
			return true
		}
		return false
	case fingerprintObject:
		_, ok := v.(map[string]any)
		return ok
	}
	return true
}

// warnFingerprint logs a single WARN when the response no longer matches the
// expected fingerprint. Warn-only by design (P2-15) — the hard gates decide
// whether the fetch actually fails; this is the early detection signal.
func warnFingerprint(fp responseFingerprint, obj map[string]any) {
	problems := checkFingerprint(fp, obj)
	if len(problems) == 0 {
		return
	}
	logging.Warn(fp.provider, "schema_fingerprint_change",
		"endpoint", fp.endpoint,
		"problems", strings.Join(problems, "; "),
	)
}

// ─── FinMind dataset registry ───────────────────────────────────────────────

// finmindDatasetFields maps FinMind dataset name → required row fields that
// the callers in this file depend on (GetMonthRevenue → revenue, GetStockPrice
// → open/max/min/close, ...). A renamed field now logs a warn at fetch time
// instead of surfacing later as an obscure type-assertion error.
var finmindDatasetFields = map[string][]fingerprintField{
	"TaiwanStockMonthRevenue": {
		{name: "date", kind: fingerprintString},
		{name: "revenue", kind: fingerprintNumber},
	},
	"TaiwanStockFinancialStatements": {
		{name: "date", kind: fingerprintString},
		{name: "value", kind: fingerprintNumber},
		{name: "origin_name", kind: fingerprintString},
	},
	"TaiwanStockInstitutionalInvestorsBuySell": {
		{name: "name", kind: fingerprintString},
		{name: "buy", kind: fingerprintNumber},
		{name: "sell", kind: fingerprintNumber},
	},
	"TaiwanStockPrice": {
		{name: "date", kind: fingerprintString},
		{name: "open", kind: fingerprintNumber},
		{name: "max", kind: fingerprintNumber},
		{name: "min", kind: fingerprintNumber},
		{name: "close", kind: fingerprintNumber},
	},
	"TaiwanStockGovernmentBankBuySell": {
		{name: "date", kind: fingerprintString},
		{name: "stock_id", kind: fingerprintString},
		{name: "bank_name", kind: fingerprintString},
		{name: "buy", kind: fingerprintNumber},
		{name: "sell", kind: fingerprintNumber},
		{name: "buy_amount", kind: fingerprintNumber},
		{name: "sell_amount", kind: fingerprintNumber},
	},
	"TaiwanStockInfo": {
		{name: "stock_id", kind: fingerprintString},
		{name: "stock_name", kind: fingerprintString},
		{name: "industry_category", kind: fingerprintString},
		{name: "type", kind: fingerprintString},
	},
	// D08: sector index 2021 backfill (每5秒指數統計). The provider keeps
	// kind=twse rows, takes the last print per series as the daily close, and
	// maps stock_id onto the canonical 18 industries.
	"TaiwanStockEvery5SecondsIndex": {
		{name: "date", kind: fingerprintString},
		{name: "time", kind: fingerprintString},
		{name: "stock_id", kind: fingerprintString},
		{name: "price", kind: fingerprintNumber},
		{name: "kind", kind: fingerprintString},
	},
}

// warnFinMindDatasetFingerprint checks the first data row (if any) against
// the dataset's required fields and warns on mismatch. No-op for datasets
// without a registered fingerprint or empty responses.
func warnFinMindDatasetFingerprint(dataset string, data []map[string]any) {
	fields, ok := finmindDatasetFields[dataset]
	if !ok || len(data) == 0 {
		return
	}
	warnFingerprint(responseFingerprint{
		provider: "finmind",
		endpoint: dataset,
		fields:   fields,
	}, data[0])
}

// finmindEnvelopeFingerprint is the top-level FinMindResponse shape. The
// Status/Data fields are already hard-checked in fetchDataset; the
// fingerprint adds the warn for the type contract the parser relies on.
var finmindEnvelopeFingerprint = responseFingerprint{
	provider: "finmind",
	endpoint: "data (envelope)",
	fields: []fingerprintField{
		{name: "msg", kind: fingerprintString},
		{name: "status", kind: fingerprintNumber},
		{name: "data", kind: fingerprintArray},
	},
}

// ─── Yahoo chart fingerprint ────────────────────────────────────────────────

// yahooChartFingerprintProblems returns problems for the chart response
// shape. The consumers index Result[0].Indicators.Quote[0].Close directly,
// so a missing/empty quote array is a panic risk (only taiex_return_calculator
// guards it) — UnmarshalYahooChart converts that into a typed ErrSchema error
// plus a warn. Pure function for tests.
func yahooChartFingerprintProblems(result *yahooChartResult) []string {
	if result == nil || len(result.Chart.Result) == 0 {
		return nil
	}
	var problems []string
	if len(result.Chart.Result[0].Indicators.Quote) == 0 {
		problems = append(problems, "chart.result[0].indicators.quote is empty or missing")
	}
	return problems
}

// ─── TWSE STOCK_DAY_ALL fingerprint ─────────────────────────────────────────

// twseStockDayAllFingerprintProblems warns when the STOCK_DAY_ALL JSON no
// longer matches the shape the fixed-index parser expects: >= 9 columns in
// the fields header (when present) and in every data row. Hard gates above
// (Stat != OK, all-rows-parse-failed) catch total breakage; this catches the
// partial drift (e.g. a trailing column added/removed) before values shift.
// Pure function for tests.
func twseStockDayAllFingerprintProblems(resp *TWSEDailyResponse) []string {
	if resp == nil {
		return nil
	}
	var problems []string
	if len(resp.Fields) > 0 && len(resp.Fields) < 9 {
		problems = append(problems, fmt.Sprintf("fields header has %d columns, want >= 9", len(resp.Fields)))
	}
	short := 0
	for _, row := range resp.Data {
		if len(row) < 9 {
			short++
		}
	}
	if short > 0 {
		problems = append(problems, fmt.Sprintf("%d of %d data rows have < 9 columns", short, len(resp.Data)))
	}
	return problems
}

// warnTWSEStockDayAllFingerprint logs the TWSE fingerprint problems (if any).
func warnTWSEStockDayAllFingerprint(resp *TWSEDailyResponse) {
	problems := twseStockDayAllFingerprintProblems(resp)
	if len(problems) == 0 {
		return
	}
	logging.Warn("twse", "schema_fingerprint_change",
		"endpoint", "STOCK_DAY_ALL",
		"problems", strings.Join(problems, "; "))
}

// ─── TAIFEX PutCallRatio fingerprint ────────────────────────────────────────

// taifexPCRFingerprint is the raw PutCallRatio row shape. The struct-level
// parseOK checks hard-fail on a renamed field; the fingerprint adds a warn
// with the exact missing key. Values are strings in the raw API.
var taifexPCRFingerprint = responseFingerprint{
	provider: "taifex",
	endpoint: "PutCallRatio",
	fields: []fingerprintField{
		{name: "Date", kind: fingerprintString},
		{name: "PutVolume", kind: fingerprintString},
		{name: "CallVolume", kind: fingerprintString},
		{name: "PutCallVolumeRatio%", kind: fingerprintString},
		{name: "PutOI", kind: fingerprintString},
		{name: "CallOI", kind: fingerprintString},
		{name: "PutCallOIRatio%", kind: fingerprintString},
	},
}

// warnTAIFEXPCRFingerprint re-parses the raw PCR payload as generic JSON
// objects and warns when a required column key is missing or wrong-typed.
// The body is tiny (a few dozen rows), so the extra decode is negligible.
// Decode errors are ignored — the struct decode below surfaces them.
func warnTAIFEXPCRFingerprint(body []byte, contentType string) {
	var rows []map[string]any
	if err := DecodeJSON(bytes.NewReader(body), contentType, &rows); err != nil {
		return
	}
	if len(rows) == 0 {
		return
	}
	warnFingerprint(taifexPCRFingerprint, rows[0])
}
