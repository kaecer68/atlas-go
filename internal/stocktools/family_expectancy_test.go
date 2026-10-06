package stocktools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// ─── family-expectancy endpoint tests ──────────────────────────────────────
//
// These exercise GET /api/stock/family-expectancy against a real in-memory
// stockpicker ledger (same fixture style as the industry_winrate tests) and
// pin that the new read-only route leaves the existing responses untouched.

func getFamilyExpectancy(t *testing.T, deps Deps, path string) (*httptest.ResponseRecorder, FamilyExpectancyResponse) {
	t.Helper()
	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var out FamilyExpectancyResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal response: %v (body=%s)", err, rec.Body.String())
		}
	}
	return rec, out
}

// familyExpectancyFixture seeds two families over four trigger dates, one of
// them carrying a regime tag (RISK_ON) and one break-even (miss-) row.
func familyExpectancyFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	dates := []string{"2026-05-11", "2026-05-12", "2026-05-13", "2026-05-14"}
	outcomes := []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: dates[0], Source: "stockpicker-momentum-20d-positive", ForwardReturn: 0.02, CostRate: 0.00585},
		{Symbol: "2454", TriggerDate: dates[1], Source: "stockpicker-momentum-20d-positive", ForwardReturn: -0.01, CostRate: 0.00585},
		{Symbol: "1301", TriggerDate: dates[2], Source: "stockpicker-momentum-20d-positive", ForwardReturn: 0.03, CostRate: 0.00585, Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: dates[3], Source: "stockpicker-momentum-20d-positive", ForwardReturn: -0.00585, CostRate: 0.00585},
		{Symbol: "2330", TriggerDate: dates[0], Source: "stockpicker-price-volume-top-divergence", ForwardReturn: -0.01, CostRate: 0.00585},
	}
	if err := stockpicker.RecordOutcomes(context.Background(), db, outcomes); err != nil {
		t.Fatalf("seed outcomes: %v", err)
	}
}

// TestHandleFamilyExpectancy_HappyPath answers "which family is still alive?"
// in one call: one row per family with the canonical caliber, the t statistic,
// the regime strata and the trailing window.
func TestHandleFamilyExpectancy_HappyPath(t *testing.T) {
	db := openWinRateTestDB(t)
	familyExpectancyFixture(t, db)

	rec, out := getFamilyExpectancy(t, Deps{FamilyExpectancy: NewSQLiteWinRateProvider(db)},
		"/api/stock/family-expectancy")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !out.Found {
		t.Fatalf("found=false: %+v", out)
	}
	if len(out.Families) != 2 {
		t.Fatalf("families = %d, want 2: %+v", len(out.Families), out.Families)
	}
	if out.Families[0].Source != "stockpicker-momentum-20d-positive" {
		t.Fatalf("families must be sorted by source, got %q first", out.Families[0].Source)
	}

	row := out.Families[0]
	if row.ConditionID != "momentum-20d-positive" || row.Direction != "buy" {
		t.Errorf("identity: condition=%q direction=%q", row.ConditionID, row.Direction)
	}
	if row.N != 4 || row.Hits != 2 {
		t.Errorf("n=%d hits=%d, want 4/2 (the break-even row is a miss)", row.N, row.Hits)
	}
	if row.NetCostRate != 0.00585 {
		t.Errorf("net_cost_rate = %v, want the params cost rate 0.00585", row.NetCostRate)
	}
	if row.NetExpectancyT == 0 {
		t.Error("net_expectancy_t must be computed for 4 non-identical rows")
	}
	if row.TrailingN != 4 {
		t.Errorf("trailing_n = %d, want 4 (four distinct dates, window not yet full)", row.TrailingN)
	}
	if out.TrailingWindowStart != "2026-05-11" || out.TrailingWindowEnd != "2026-05-14" {
		t.Errorf("window = %s..%s, want 2026-05-11..2026-05-14", out.TrailingWindowStart, out.TrailingWindowEnd)
	}
	if _, err := time.Parse(time.RFC3339, out.GeneratedAt); err != nil {
		t.Errorf("generated_at = %q, want RFC3339: %v", out.GeneratedAt, err)
	}
	if row.GeneratedAt != out.GeneratedAt {
		t.Errorf("row generated_at %q != report generated_at %q", row.GeneratedAt, out.GeneratedAt)
	}
	if len(row.ByRegime) != 2 {
		t.Fatalf("by_regime = %+v, want unknown + RISK_ON", row.ByRegime)
	}
	if row.ByRegime[0].Regime != stockpicker.UnknownRegime || row.ByRegime[0].N != 3 {
		t.Errorf("by_regime[0] = %+v, want unknown with n=3", row.ByRegime[0])
	}
	if row.ByRegime[1].Regime != "RISK_ON" || row.ByRegime[1].N != 1 {
		t.Errorf("by_regime[1] = %+v, want RISK_ON with n=1", row.ByRegime[1])
	}

	// The avoid-semantics condition keeps its inverted reading.
	if out.Families[1].Direction != "avoid" {
		t.Errorf("top-divergence direction = %q, want avoid", out.Families[1].Direction)
	}
}

// TestHandleFamilyExpectancy_TrailingDaysOverride honours the query parameter.
func TestHandleFamilyExpectancy_TrailingDaysOverride(t *testing.T) {
	db := openWinRateTestDB(t)
	familyExpectancyFixture(t, db)

	rec, out := getFamilyExpectancy(t, Deps{FamilyExpectancy: NewSQLiteWinRateProvider(db)},
		"/api/stock/family-expectancy?trailing_days=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if out.TrailingTradingDays != 1 {
		t.Errorf("trailing_trading_days = %d, want 1", out.TrailingTradingDays)
	}
	if out.TrailingWindowStart != "2026-05-14" || out.TrailingWindowEnd != "2026-05-14" {
		t.Errorf("window = %s..%s, want the latest date only", out.TrailingWindowStart, out.TrailingWindowEnd)
	}
	if out.Families[0].TrailingN != 1 {
		t.Errorf("trailing_n = %d, want 1", out.Families[0].TrailingN)
	}
	if out.Families[0].N != 4 {
		t.Errorf("n = %d, want the full period untouched by the trailing override", out.Families[0].N)
	}
}

// TestHandleFamilyExpectancy_InvalidTrailingDays: a bad value is a client error
// (new parameter, so the endpoint returns 400 rather than the legacy 503 used
// by the rolling_window parsers).
func TestHandleFamilyExpectancy_InvalidTrailingDays(t *testing.T) {
	db := openWinRateTestDB(t)
	familyExpectancyFixture(t, db)

	for _, bad := range []string{"abc", "0", "-5", "1.5"} {
		rec, _ := getFamilyExpectancy(t, Deps{FamilyExpectancy: NewSQLiteWinRateProvider(db)},
			"/api/stock/family-expectancy?trailing_days="+bad)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("trailing_days=%s: expected 400, got %d (%s)", bad, rec.Code, rec.Body.String())
		}
	}
}

// TestHandleFamilyExpectancy_NoProvider: the endpoint is optional wiring.
func TestHandleFamilyExpectancy_NoProvider(t *testing.T) {
	rec, _ := getFamilyExpectancy(t, Deps{}, "/api/stock/family-expectancy")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// TestHandleFamilyExpectancy_NoData: an empty ledger answers 200 + found=false.
func TestHandleFamilyExpectancy_NoData(t *testing.T) {
	rec, out := getFamilyExpectancy(t, Deps{FamilyExpectancy: NewSQLiteWinRateProvider(openWinRateTestDB(t))},
		"/api/stock/family-expectancy")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if out.Found || out.Message == "" {
		t.Fatalf("want found=false with a message, got %+v", out)
	}
	if len(out.Families) != 0 {
		t.Errorf("families = %+v, want none", out.Families)
	}
}

// failingFamilyExpectancyProvider exercises the store-error path.
type failingFamilyExpectancyProvider struct{}

func (failingFamilyExpectancyProvider) LoadFamilyExpectancy(context.Context, int, string) (stockpicker.FamilyExpectancyReport, bool, error) {
	return stockpicker.FamilyExpectancyReport{}, false, errors.New("ledger unreadable")
}

// TestHandleFamilyExpectancy_StoreError: a read failure is 503, not a 200 with
// an empty body.
func TestHandleFamilyExpectancy_StoreError(t *testing.T) {
	rec, _ := getFamilyExpectancy(t, Deps{FamilyExpectancy: failingFamilyExpectancyProvider{}},
		"/api/stock/family-expectancy")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ledger unreadable") {
		t.Errorf("body = %s, want the provider error", rec.Body.String())
	}
}

// TestStockRoutes_AdditiveRegistration pins the route table: every pre-existing
// stocktools pattern is still registered, and the new one is additive.
func TestStockRoutes_AdditiveRegistration(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{})

	patterns := []string{
		"GET /api/stock/quote",
		"GET /api/stock/fundamentals",
		"GET /api/stock/chips",
		"GET /api/stock/technical",
		"GET /api/stock/sector-median-pe",
		"GET /api/stock/coverage",
		"GET /api/stock/monthly_revenue",
		"GET /api/stock/win_rate",
		"GET /api/stock/volume_divergence",
		"GET /api/stock/condition_winrate",
		"GET /api/stock/industry_winrate",
		"GET /api/stock/family-expectancy",
	}
	for _, pattern := range patterns {
		method, path, _ := strings.Cut(pattern, " ")
		req := httptest.NewRequest(method, path, nil)
		if _, got := mux.Handler(req); got != pattern {
			t.Errorf("route %q resolves to %q, want itself", pattern, got)
		}
	}
}

// TestHandleFamilyExpectancy_DoesNotChangeExistingResponses is the
// compatibility guard: adding the instrument must not alter one byte of the
// existing /api/stock/* responses.
func TestHandleFamilyExpectancy_DoesNotChangeExistingResponses(t *testing.T) {
	db := openWinRateTestDB(t)
	provider := NewSQLiteWinRateProvider(db)
	seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-momentum-20d-positive", defaultWinRateWindow))
	familyExpectancyFixture(t, db)

	legacyPaths := []string{
		"/api/stock/win_rate?symbol=2330",
		"/api/stock/condition_winrate?condition_id=momentum-20d-positive",
		"/api/stock/industry_winrate?condition_id=momentum-20d-positive",
	}
	before := Deps{WinRate: provider, ConditionWinRate: provider, IndustryWinRate: provider}
	after := Deps{WinRate: provider, ConditionWinRate: provider, IndustryWinRate: provider, FamilyExpectancy: provider}

	for _, path := range legacyPaths {
		recBefore, bodyBefore := getRawResponse(t, before, path)
		recAfter, bodyAfter := getRawResponse(t, after, path)
		if recBefore.Code != http.StatusOK || recAfter.Code != http.StatusOK {
			t.Fatalf("%s: status before=%d after=%d", path, recBefore.Code, recAfter.Code)
		}
		if bodyBefore != bodyAfter {
			t.Errorf("%s: response changed after registering the instrument:\n before=%s\n after =%s",
				path, bodyBefore, bodyAfter)
		}
		if strings.Contains(bodyBefore, "family") {
			t.Errorf("%s: existing response gained family fields: %s", path, bodyBefore)
		}
	}
}

// getRawResponse returns the recorder and the raw body for a path.
func getRawResponse(t *testing.T, deps Deps, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec, rec.Body.String()
}
