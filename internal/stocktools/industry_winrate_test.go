package stocktools

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// ─── industry_winrate handler tests (issue #1942) ──────────────────────────
//
// These exercise GET /api/stock/industry_winrate against a real in-memory
// stockpicker ledger. Symbol -> industry attribution uses the production
// canonical L1 taxonomy (industry.ClassifyBySymbol), so the fixtures pin the
// real mapping (2330/2454 -> semiconductor, 1301 -> plastics) plus an
// unmapped symbol that must be reported, never dropped.

// industryTestDates returns three recent trigger dates inside the default
// 120d rolling window, relative to now so the test does not rot.
func industryTestDates() [3]string {
	now := time.Now().UTC()
	var out [3]string
	for i := range out {
		out[i] = now.AddDate(0, 0, -7+i).Format("2006-01-02")
	}
	return out
}

func seedIndustryOutcomes(t *testing.T, db *sql.DB, outcomes []stockpicker.SignalOutcome) {
	t.Helper()
	if err := stockpicker.RecordOutcomes(context.Background(), db, outcomes); err != nil {
		t.Fatalf("seed outcomes: %v", err)
	}
}

func getIndustryWinRate(t *testing.T, deps Deps, path string) (*httptest.ResponseRecorder, IndustryWinRateResponse) {
	t.Helper()
	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var out IndustryWinRateResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal response: %v (body=%s)", err, rec.Body.String())
		}
	}
	return rec, out
}

// TestHandleIndustryWinRate_HappyPath answers "半導體在某期間的命中率＝？"
// in one call and reports coverage + the unmapped list.
func TestHandleIndustryWinRate_HappyPath(t *testing.T) {
	db := openWinRateTestDB(t)
	dates := industryTestDates()
	const source = "stockpicker-momentum-20d-positive"
	seedIndustryOutcomes(t, db, []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: dates[0], Source: source, ForwardReturn: 0.02, CostRate: 0.00585},
		{Symbol: "2454", TriggerDate: dates[1], Source: source, ForwardReturn: 0.001, CostRate: 0.00585},
		{Symbol: "1301", TriggerDate: dates[1], Source: source, ForwardReturn: -0.01, CostRate: 0.00585},
		{Symbol: "9999", TriggerDate: dates[2], Source: source, ForwardReturn: 0.05, CostRate: 0.00585},
	})

	rec, out := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/industry_winrate?condition_id=momentum-20d-positive")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !out.Found {
		t.Fatalf("found=false: %+v", out)
	}
	if out.ConditionID != "momentum-20d-positive" || out.Direction != "buy" || out.Window != "120d" {
		t.Errorf("identity fields: source=%q condition=%q direction=%q window=%q",
			out.Source, out.ConditionID, out.Direction, out.Window)
	}
	if len(out.Industries) != 2 {
		t.Fatalf("industries = %d, want 2 (plastics, semiconductor): %+v", len(out.Industries), out.Industries)
	}
	semis, ok := stockpicker.IndustryWinRateFor(out.IndustryWinRateReport, "semiconductor")
	if !ok {
		t.Fatalf("semiconductor row missing: %+v", out.Industries)
	}
	if semis.IndustryNameZH != "半導體" {
		t.Errorf("industry_name_zh = %q, want 半導體", semis.IndustryNameZH)
	}
	if semis.Observations != 2 || semis.Hits != 1 || semis.Symbols != 2 {
		t.Errorf("semiconductor row: %+v", semis)
	}
	if semis.NetCostRate != 0.00585 {
		t.Errorf("net_cost_rate = %v, want the params cost rate 0.00585", semis.NetCostRate)
	}
	if semis.AvgNetForwardReturn == 0 {
		t.Error("avg_net_forward_return must be computed")
	}
	if len(out.Coverage.UnmappedSymbols) != 1 || out.Coverage.UnmappedSymbols[0].Symbol != "9999" {
		t.Errorf("unmapped symbols: %+v", out.Coverage.UnmappedSymbols)
	}
	if out.Coverage.TotalObservations != 4 || out.Coverage.MappedObservations != 3 {
		t.Errorf("coverage: %+v", out.Coverage)
	}
}

// TestHandleIndustryWinRate_IndustryFilter: industry_id narrows the answer to
// one row, accepting both the canonical id and the Chinese label.
func TestHandleIndustryWinRate_IndustryFilter(t *testing.T) {
	db := openWinRateTestDB(t)
	dates := industryTestDates()
	const source = "stockpicker-momentum-20d-positive"
	seedIndustryOutcomes(t, db, []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: dates[0], Source: source, ForwardReturn: 0.02, CostRate: 0.00585},
		{Symbol: "1301", TriggerDate: dates[1], Source: source, ForwardReturn: 0.03, CostRate: 0.00585},
	})

	for _, query := range []string{"industry_id=semiconductor", "industry_id=半導體"} {
		rec, out := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
			"/api/stock/industry_winrate?condition_id=momentum-20d-positive&"+query)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", query, rec.Code, rec.Body.String())
		}
		if !out.Found || len(out.Industries) != 1 {
			t.Fatalf("%s: expected exactly one row, got %+v", query, out)
		}
		if out.Industries[0].IndustryID != "semiconductor" {
			t.Errorf("%s: industry_id = %q", query, out.Industries[0].IndustryID)
		}
	}
}

// TestHandleIndustryWinRate_FilterNoRow: a valid industry with no mapped
// observations answers found=false + an explanatory message, and still
// carries the coverage block (the gap is part of the answer).
func TestHandleIndustryWinRate_FilterNoRow(t *testing.T) {
	db := openWinRateTestDB(t)
	dates := industryTestDates()
	const source = "stockpicker-momentum-20d-positive"
	seedIndustryOutcomes(t, db, []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: dates[0], Source: source, ForwardReturn: 0.02, CostRate: 0.00585},
		{Symbol: "9999", TriggerDate: dates[1], Source: source, ForwardReturn: 0.02, CostRate: 0.00585},
	})

	rec, out := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/industry_winrate?condition_id=momentum-20d-positive&industry_id=steel")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if out.Found {
		t.Fatalf("found=true for an industry with no observations: %+v", out)
	}
	if !strings.Contains(out.Message, "steel") || !strings.Contains(out.Message, "coverage") {
		t.Errorf("message = %q, want the industry id and the coverage caveat", out.Message)
	}
	if out.Coverage.TotalObservations != 2 || out.Coverage.UnmappedObservations != 1 {
		t.Errorf("coverage must stay populated on found=false: %+v", out.Coverage)
	}
}

// TestHandleIndustryWinRate_InvalidIndustryID: unknown ids and L2
// sub-industries are client errors, not silent empty answers.
func TestHandleIndustryWinRate_InvalidIndustryID(t *testing.T) {
	db := openWinRateTestDB(t)
	dates := industryTestDates()
	seedIndustryOutcomes(t, db, []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: dates[0], Source: "stockpicker-momentum-20d-positive", ForwardReturn: 0.02, CostRate: 0.00585},
	})

	for _, bad := range []string{"not-a-sector", "robotics"} {
		rec, _ := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
			"/api/stock/industry_winrate?condition_id=momentum-20d-positive&industry_id="+bad)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("industry_id=%s: expected 400, got %d", bad, rec.Code)
		}
	}
}

// TestHandleIndustryWinRate_RequiresConditionID: conditions are never pooled.
func TestHandleIndustryWinRate_RequiresConditionID(t *testing.T) {
	rec, _ := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(openWinRateTestDB(t))},
		"/api/stock/industry_winrate")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestHandleIndustryWinRate_NoProvider: the endpoint is optional wiring.
func TestHandleIndustryWinRate_NoProvider(t *testing.T) {
	rec, _ := getIndustryWinRate(t, Deps{}, "/api/stock/industry_winrate?condition_id=momentum-20d-positive")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// TestHandleIndustryWinRate_NoData: an empty ledger answers 200 + found=false.
func TestHandleIndustryWinRate_NoData(t *testing.T) {
	rec, out := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(openWinRateTestDB(t))},
		"/api/stock/industry_winrate?condition_id=nonexistent")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if out.Found || out.Message == "" {
		t.Fatalf("want found=false with a message, got %+v", out)
	}
}

// TestHandleIndustryWinRate_RegimeFilter: regime-stratified industry rows use
// the same stratum rule as the condition-level path (untagged rows excluded).
func TestHandleIndustryWinRate_RegimeFilter(t *testing.T) {
	db := openWinRateTestDB(t)
	dates := industryTestDates()
	const source = "stockpicker-momentum-20d-positive"
	seedIndustryOutcomes(t, db, []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: dates[0], Source: source, ForwardReturn: 0.02, CostRate: 0.00585, Regime: "RISK_ON"},
		{Symbol: "2454", TriggerDate: dates[1], Source: source, ForwardReturn: -0.02, CostRate: 0.00585, Regime: "RISK_OFF"},
	})

	rec, out := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/industry_winrate?condition_id=momentum-20d-positive&regime=RISK_ON")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !out.Found || out.Regime != "RISK_ON" {
		t.Fatalf("regime echo: %+v", out)
	}
	semis, ok := stockpicker.IndustryWinRateFor(out.IndustryWinRateReport, "semiconductor")
	if !ok {
		t.Fatalf("semiconductor row missing: %+v", out.Industries)
	}
	if semis.Observations != 1 || semis.Hits != 1 {
		t.Errorf("RISK_ON row = %+v, want 1 observation / 1 hit", semis)
	}
	if out.Coverage.TotalObservations != 1 {
		t.Errorf("coverage must reflect the filtered stratum: %+v", out.Coverage)
	}
}

// TestCanonicalL1IndustryID documents the accepted industry_id vocabulary.
func TestCanonicalL1IndustryID(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"semiconductor", "semiconductor", true},
		{" 半導體 ", "semiconductor", true},
		{"金融", "financials", true},
		{"robotics", "", false}, // L2 sub-industry: no aggregate yet
		{"not-a-sector", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := CanonicalL1IndustryID(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("CanonicalL1IndustryID(%q) = %q,%v want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestHandleIndustryWinRate_RegimeStratumEmpty (review E4): data exists but
// none of it carries the requested regime tag. The answer must be
// found=false with a message that says so (not "no stored outcomes"), and the
// coverage block must still describe the unfiltered read so the caller can see
// that the source does hold data.
func TestHandleIndustryWinRate_RegimeStratumEmpty(t *testing.T) {
	db := openWinRateTestDB(t)
	dates := industryTestDates()
	const source = "stockpicker-momentum-20d-positive"
	seedIndustryOutcomes(t, db, []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: dates[0], Source: source, ForwardReturn: 0.02, CostRate: 0.00585}, // untagged
		{Symbol: "2454", TriggerDate: dates[1], Source: source, ForwardReturn: -0.02, CostRate: 0.00585, Regime: "RISK_OFF"},
	})

	rec, out := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/industry_winrate?condition_id=momentum-20d-positive&regime=RISK_ON")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if out.Found {
		t.Fatalf("found=true for an empty regime stratum: %+v", out)
	}
	if !strings.Contains(out.Message, "RISK_ON") || strings.Contains(out.Message, "no stored outcomes for condition") {
		t.Errorf("message must name the empty regime and not claim there is no data: %q", out.Message)
	}
	if out.Regime != "RISK_ON" {
		t.Errorf("regime echo = %q, want RISK_ON", out.Regime)
	}
	if len(out.Industries) != 0 {
		t.Errorf("industries must stay empty (other regimes are not pooled): %+v", out.Industries)
	}
	if out.Coverage.TotalObservations != 2 || out.Coverage.MappedObservations != 2 {
		t.Errorf("coverage must describe the unfiltered read: %+v", out.Coverage)
	}
	// JSON stability: empty collections, never null.
	if !strings.Contains(rec.Body.String(), `"industries":[]`) || !strings.Contains(rec.Body.String(), `"unmapped_symbols":[]`) {
		t.Errorf("empty slices must serialise as []: %s", rec.Body.String())
	}
}

// TestHandleIndustryWinRate_EmptyLedgerJSONTypes (review E5): the no-data path
// must not emit null collections either.
func TestHandleIndustryWinRate_EmptyLedgerJSONTypes(t *testing.T) {
	rec, out := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(openWinRateTestDB(t))},
		"/api/stock/industry_winrate?condition_id=momentum-20d-positive")
	if rec.Code != http.StatusOK || out.Found {
		t.Fatalf("expected 200 + found=false, got %d %+v", rec.Code, out)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"industries":[]`) || !strings.Contains(body, `"unmapped_symbols":[]`) {
		t.Errorf("empty slices must serialise as []: %s", body)
	}
	if out.Direction != "buy" || out.ConditionID != "momentum-20d-positive" || out.Window != "120d" {
		t.Errorf("identity fields must be preserved on the empty path: %+v", out)
	}
}

// TestHandleIndustryWinRate_AvoidSemanticsAndNetReturn covers the payload
// fields the pure tests cannot: direction=avoid at HTTP level, the net average
// return, unmapped ordering and symbol-level coverage.
func TestHandleIndustryWinRate_AvoidSemanticsAndNetReturn(t *testing.T) {
	db := openWinRateTestDB(t)
	dates := industryTestDates()
	const source = "stockpicker-price-volume-top-divergence"
	seedIndustryOutcomes(t, db, []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: dates[0], Source: source, ForwardReturn: -0.05, CostRate: 0.00585},
		{Symbol: "2454", TriggerDate: dates[1], Source: source, ForwardReturn: 0.001, CostRate: 0.00585},
		{Symbol: "9999", TriggerDate: dates[1], Source: source, ForwardReturn: 0.5, CostRate: 0.00585},
		{Symbol: "9998", TriggerDate: dates[2], Source: source, ForwardReturn: 0.5, CostRate: 0.00585},
	})

	rec, out := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/industry_winrate?condition_id=price-volume-top-divergence")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if out.Direction != "avoid" {
		t.Errorf("direction = %q, want avoid", out.Direction)
	}
	// 2 hits total: 9999/9998 (+0.5 - 0.00585 > 0); 2330 (-0.05) and 2454
	// (0.001 - 0.00585 < 0) miss.
	if out.Coverage.MappedObservations != 2 || out.Coverage.UnmappedObservations != 2 {
		t.Fatalf("coverage: %+v", out.Coverage)
	}
	if out.Coverage.SymbolCoveragePct != 50 {
		t.Errorf("symbol_coverage_pct = %v, want 50 (2 of 4 symbols mapped)", out.Coverage.SymbolCoveragePct)
	}
	// Unmapped symbols carry equal evidence -> symbol ascending.
	if len(out.Coverage.UnmappedSymbols) != 2 ||
		out.Coverage.UnmappedSymbols[0].Symbol != "9998" || out.Coverage.UnmappedSymbols[1].Symbol != "9999" {
		t.Errorf("unmapped ordering: %+v", out.Coverage.UnmappedSymbols)
	}
	semis, ok := stockpicker.IndustryWinRateFor(out.IndustryWinRateReport, "semiconductor")
	if !ok {
		t.Fatalf("semiconductor row missing: %+v", out.Industries)
	}
	if semis.Observations != 2 || semis.Hits != 0 {
		t.Errorf("semiconductor row: %+v", semis)
	}
	wantNet := (-0.05+0.001)/2 - 0.00585
	if math.Abs(semis.AvgNetForwardReturn-wantNet) > 1e-9 {
		t.Errorf("avg_net_forward_return = %v, want %v", semis.AvgNetForwardReturn, wantNet)
	}
}

// TestHandleIndustryWinRate_InvalidWindow documents the existing sibling
// contract: a malformed rolling_window surfaces as 503 (same as
// /api/stock/win_rate and /api/stock/condition_winrate), not 400.
func TestHandleIndustryWinRate_InvalidWindow(t *testing.T) {
	db := openWinRateTestDB(t)
	seedIndustryOutcomes(t, db, []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: industryTestDates()[0], Source: "stockpicker-momentum-20d-positive", ForwardReturn: 0.02, CostRate: 0.00585},
	})
	rec, _ := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/industry_winrate?condition_id=momentum-20d-positive&rolling_window=abc")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandleIndustryWinRate_LedgerUnavailable: a broken ledger handle is a 503
// (the same "no data configured" contract as the sibling endpoints).
func TestHandleIndustryWinRate_LedgerUnavailable(t *testing.T) {
	db := openWinRateTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	rec, _ := getIndustryWinRate(t, Deps{IndustryWinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/industry_winrate?condition_id=momentum-20d-positive")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}
