package stocktools

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// ─── win_rate handler tests (PR 3c, Part 1) ────────────────────────────────
//
// These exercise GET /api/stock/win_rate against a real in-memory
// stockpicker ledger (seeded via the package-level SaveWinRate /
// RecordOutcomes), covering the happy path, condition filtering, window
// selection, empty-data semantics (200 + found=false), and error paths
// (missing symbol, no provider).

// openWinRateTestDB opens an in-memory SQLite database with the ledger schema.
func openWinRateTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return db
}

// seedWinRate stores one (symbol, source, window) summary plus two signal
// outcomes for the date-range computation.
func seedWinRate(t *testing.T, db *sql.DB, summary stockpicker.StockWinRateSummary) {
	t.Helper()
	ctx := context.Background()
	if err := stockpicker.SaveWinRate(ctx, db, summary); err != nil {
		t.Fatalf("seed SaveWinRate: %v", err)
	}
	if err := stockpicker.RecordOutcomes(ctx, db, []stockpicker.SignalOutcome{
		{Symbol: summary.Symbol, TriggerDate: "2026-06-01", Source: summary.Source, ForwardReturn: 0.01, Hit: true},
		{Symbol: summary.Symbol, TriggerDate: "2026-08-20", Source: summary.Source, ForwardReturn: -0.02, Hit: false},
	}); err != nil {
		t.Fatalf("seed RecordOutcomes: %v", err)
	}
}

func sampleWinRateSummary(symbol, source, window string) stockpicker.StockWinRateSummary {
	return stockpicker.StockWinRateSummary{
		Symbol:            symbol,
		Source:            source,
		Window:            window,
		Observations:      40,
		Hits:              24,
		WinRate:           0.6,
		WilsonLower:       0.44,
		WilsonUpper:       0.74,
		Confidence:        0.95,
		CalibrationStatus: stockpicker.CalibrationEligible,
		NetCostRate:       0.00585,
		AvgForwardReturn:  0.012,
		UpdatedAt:         "2026-08-27T12:00:00Z",
	}
}

func getWinRate(t *testing.T, deps Deps, path string) (*httptest.ResponseRecorder, WinRateResponse) {
	t.Helper()
	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var out WinRateResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal response: %v (body=%s)", err, rec.Body.String())
		}
	}
	return rec, out
}

// TestHandleWinRate_HappyPath verifies the full read path: sources listing,
// summary load, and outcome date range all surface in the response.
func TestHandleWinRate_HappyPath(t *testing.T) {
	db := openWinRateTestDB(t)
	seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d"))

	rec, out := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)}, "/api/stock/win_rate?symbol=2330")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !out.Found {
		t.Fatalf("found = false, want true: %+v", out)
	}
	if out.Symbol != "2330" || out.RollingWindow != "120d" {
		t.Fatalf("unexpected header: %+v", out)
	}
	if len(out.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d: %+v", len(out.Conditions), out.Conditions)
	}
	cond := out.Conditions[0]
	if cond.ConditionID != "foreign-3d-net-buy" {
		t.Errorf("condition_id = %q, want foreign-3d-net-buy", cond.ConditionID)
	}
	if cond.Observations != 40 || cond.Hits != 24 || cond.WinRate != 0.6 {
		t.Errorf("summary fields wrong: %+v", cond)
	}
	if cond.WilsonLower != 0.44 || cond.WilsonUpper != 0.74 || cond.Confidence != 0.95 {
		t.Errorf("wilson fields wrong: %+v", cond)
	}
	if cond.CalibrationStatus != "eligible" {
		t.Errorf("calibration_status = %q, want eligible", cond.CalibrationStatus)
	}
	if cond.AvgForwardReturn != 0.012 || cond.NetCostRate != 0.00585 {
		t.Errorf("return fields wrong: %+v", cond)
	}
	if cond.UpdatedAt != "2026-08-27T12:00:00Z" {
		t.Errorf("updated_at = %q", cond.UpdatedAt)
	}
	if cond.DataStart != "2026-06-01" || cond.DataEnd != "2026-08-20" {
		t.Errorf("data range = %s..%s, want 2026-06-01..2026-08-20", cond.DataStart, cond.DataEnd)
	}
}

// TestHandleWinRate_MultipleConditions verifies all persisted stockpicker
// sources for a symbol are returned, ordered by source.
func TestHandleWinRate_MultipleConditions(t *testing.T) {
	db := openWinRateTestDB(t)
	seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d"))
	seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-momentum-20d-positive", "120d"))

	_, out := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)}, "/api/stock/win_rate?symbol=2330")
	if !out.Found || len(out.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, found=%v conditions=%d", out.Found, len(out.Conditions))
	}
	if out.Conditions[0].ConditionID != "foreign-3d-net-buy" || out.Conditions[1].ConditionID != "momentum-20d-positive" {
		t.Errorf("conditions not ordered by source: %+v", out.Conditions)
	}
}

// TestHandleWinRate_ConditionFilter verifies ?condition_id= restricts the
// response to that condition; an unknown condition id yields found=false.
func TestHandleWinRate_ConditionFilter(t *testing.T) {
	db := openWinRateTestDB(t)
	seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d"))
	seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-momentum-20d-positive", "120d"))

	_, out := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/win_rate?symbol=2330&condition_id=momentum-20d-positive")
	if !out.Found || len(out.Conditions) != 1 {
		t.Fatalf("expected 1 filtered condition, got %+v", out)
	}
	if out.Conditions[0].ConditionID != "momentum-20d-positive" {
		t.Errorf("condition_id = %q, want momentum-20d-positive", out.Conditions[0].ConditionID)
	}

	rec, out := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/win_rate?symbol=2330&condition_id=no-such-condition")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (found=false), got %d", rec.Code)
	}
	if out.Found || len(out.Conditions) != 0 {
		t.Fatalf("unknown condition: found=%v conditions=%d, want false/0", out.Found, len(out.Conditions))
	}
	if !strings.Contains(out.Message, "no stockpicker win-rate data") {
		t.Errorf("message = %q, want no-data message", out.Message)
	}
}

// TestHandleWinRate_RollingWindow verifies the rolling_window query param
// selects the stored window (default 120d).
func TestHandleWinRate_RollingWindow(t *testing.T) {
	db := openWinRateTestDB(t)
	seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "60d"))

	// Explicit window finds the row.
	_, out := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)},
		"/api/stock/win_rate?symbol=2330&rolling_window=60d")
	if !out.Found || out.RollingWindow != "60d" {
		t.Fatalf("60d window not found: %+v", out)
	}

	// Default window (120d) has no row → found=false.
	rec, out := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)}, "/api/stock/win_rate?symbol=2330")
	if rec.Code != http.StatusOK || out.Found {
		t.Fatalf("default window: code=%d found=%v, want 200/false", rec.Code, out.Found)
	}
	if out.RollingWindow != "120d" {
		t.Errorf("rolling_window default = %q, want 120d", out.RollingWindow)
	}
}

// TestHandleWinRate_NoData verifies 200 + found=false + message when the
// symbol has no stored data at all.
func TestHandleWinRate_NoData(t *testing.T) {
	db := openWinRateTestDB(t)
	rec, out := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)}, "/api/stock/win_rate?symbol=9999")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (found=false), got %d: %s", rec.Code, rec.Body.String())
	}
	if out.Found {
		t.Fatal("found = true, want false")
	}
	if !strings.Contains(out.Message, "9999") {
		t.Errorf("message should mention symbol: %q", out.Message)
	}
}

// TestHandleWinRate_RejectsEmptySymbol verifies 400 on missing symbol.
func TestHandleWinRate_RejectsEmptySymbol(t *testing.T) {
	db := openWinRateTestDB(t)
	rec, _ := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)}, "/api/stock/win_rate")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWinRate_NoProviderReturns503 verifies the fail-soft path when
// the store is not configured.
func TestHandleWinRate_NoProviderReturns503(t *testing.T) {
	rec, _ := getWinRate(t, Deps{}, "/api/stock/win_rate?symbol=2330")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWinRate_SymbolSuffixStripped verifies .TW/.TWO suffixes are
// normalized to the bare ledger code (mirroring monthly_revenue).
func TestHandleWinRate_SymbolSuffixStripped(t *testing.T) {
	db := openWinRateTestDB(t)
	seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d"))

	_, out := getWinRate(t, Deps{WinRate: NewSQLiteWinRateProvider(db)}, "/api/stock/win_rate?symbol=2330.TW")
	if !out.Found || out.Symbol != "2330" {
		t.Fatalf(".TW suffix not normalized: %+v", out)
	}
}

// TestHandleWinRate_ProviderErrorSurface verifies provider errors surface
// as 503 (via a fake that fails on Sources).
type failingWinRateProvider struct{}

func (f *failingWinRateProvider) Sources(context.Context, string, string) ([]string, error) {
	return nil, errFakeWinRate
}

func (f *failingWinRateProvider) LoadWinRate(context.Context, string, string, string) (stockpicker.StockWinRateSummary, bool, error) {
	return stockpicker.StockWinRateSummary{}, false, nil
}

func (f *failingWinRateProvider) OutcomeDateRange(context.Context, string, string) (string, string, bool) {
	return "", "", false
}

var errFakeWinRate = &testWinRateError{}

type testWinRateError struct{}

func (e *testWinRateError) Error() string { return "fake win-rate provider error" }

func TestHandleWinRate_ProviderErrorReturns503(t *testing.T) {
	rec, _ := getWinRate(t, Deps{WinRate: &failingWinRateProvider{}}, "/api/stock/win_rate?symbol=2330")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fake win-rate provider error") {
		t.Errorf("503 body should surface provider error, got: %s", rec.Body.String())
	}
}

// TestOpenWinRateDB_ReadOnly verifies OpenWinRateDB forces mode=ro at the
// driver level: after opening, any write (INSERT) must fail. This promotes
// the read-only guarantee from a DSN convention to a regression-tested
// assertion (k3 review M7) — a future refactor that drops `?mode=ro` from
// the DSN breaks this test instead of silently allowing writes.
func TestOpenWinRateDB_ReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")

	// Build a real file-backed ledger with the schema + one row so the
	// read-only handle has something to read and something to refuse.
	w, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open writable ledger: %v", err)
	}
	if err := ledger.InitSchema(w); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	ctx := context.Background()
	if err := stockpicker.SaveWinRate(ctx, w, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d")); err != nil {
		t.Fatalf("seed SaveWinRate: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writable ledger: %v", err)
	}

	ro, err := OpenWinRateDB(path)
	if err != nil {
		t.Fatalf("OpenWinRateDB: %v", err)
	}
	defer func() { _ = ro.Close() }()

	// Reads still work on the read-only handle.
	summary, found, err := stockpicker.LoadWinRate(ctx, ro, "2330", "stockpicker-foreign-3d-net-buy", "120d")
	if err != nil || !found {
		t.Fatalf("read-only LoadWinRate: found=%v err=%v", found, err)
	}
	if summary.Observations != 40 {
		t.Errorf("read-only LoadWinRate observations = %d, want 40", summary.Observations)
	}

	// Writes must fail at the driver level (mode=ro).
	if err := stockpicker.SaveWinRate(ctx, ro, sampleWinRateSummary("2317", "stockpicker-foreign-3d-net-buy", "120d")); err == nil {
		t.Fatal("INSERT on read-only handle succeeded; mode=ro is not enforced")
	}
}
