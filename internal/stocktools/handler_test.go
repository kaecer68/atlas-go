package stocktools

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

type redirectRoundTripper struct {
	serverURL string
}

func (rt *redirectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	parsed, err := url.Parse(rt.serverURL)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = parsed.Scheme
	req.URL.Host = parsed.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestHandleQuote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("X-API-KEY header missing or wrong: %q", r.Header.Get("X-API-KEY"))
		}
		w.Header().Set("Content-Type", "application/json")
		// v1.0 扁平結構（2026-08-03 遷移）
		w.Write([]byte(`{"date":"2026-07-07","type":"EQUITY","exchange":"TWSE","market":"TSE","symbol":"2330","name":"台積電","closePrice":680,"openPrice":670,"highPrice":685,"lowPrice":668,"lastPrice":680,"total":{"tradeVolume":12345}}`))
	}))
	defer server.Close()

	client := marketdata.NewFugleClient("test-key")
	client.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: server.URL}})

	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{FugleClient: client})

	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=2330", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, `"symbol":"2330"`) {
		t.Fatalf("expected symbol in response: %s", body)
	}
}

// mockTWSEProvider is a marketdata.Provider that records the context it
// received and returns a canned quote. Used to verify the TWSE fallback
// receives an independent timeout budget (not the parent request deadline
// that Fugle already consumed).
type mockTWSEProvider struct {
	gotCtx context.Context
}

func (m *mockTWSEProvider) Name() string { return "mock-twse" }

func (m *mockTWSEProvider) GetQuotes(ctx context.Context, _ time.Time, symbols []string) ([]domain.Quote, error) {
	m.gotCtx = ctx
	if len(symbols) == 0 {
		return nil, errors.New("no symbols")
	}
	// Mimic a real HTTP-backed provider: if the context is already expired
	// (e.g. an inherited parent deadline that Fugle consumed), the upstream
	// call fails with context deadline exceeded — exactly what happened in
	// the SK-22 endpoint-2 audit.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []domain.Quote{{Symbol: symbols[0], Last: 680, Market: "TW", Source: "twse"}}, nil
}

// TestHandleQuote_TWSEFallbackGetsIndependentTimeoutBudget verifies the
// SK-22 endpoint-2 fix: when Fugle fails and the parent request deadline is
// already exhausted, the TWSE fallback must still receive its own 5s budget
// (context.WithoutCancel) instead of inheriting the expired parent deadline.
// Without the fix, TWSE fails with context deadline exceeded before it can
// contact the upstream.
func TestHandleQuote_TWSEFallbackGetsIndependentTimeoutBudget(t *testing.T) {
	// Fugle always fails (HTTP 500) so the handler falls back to TWSE.
	fugleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fugleServer.Close()

	fugleClient := marketdata.NewFugleClient("test-key")
	fugleClient.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: fugleServer.URL}})

	mockTWSE := &mockTWSEProvider{}
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{FugleClient: fugleClient, TWSEQuote: mockTWSE})

	// Parent request deadline expires in 10ms; we sleep past it before serving
	// so the deadline is already exhausted when the handler runs. If the TWSE
	// fallback inherits this expired deadline (old behavior) it fails with
	// context deadline exceeded before reaching the provider.
	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=2330", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	time.Sleep(20 * time.Millisecond)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (TWSE fallback got full budget), got %d: %s", rec.Code, rec.Body.String())
	}
	if mockTWSE.gotCtx == nil {
		t.Fatal("TWSE provider was not called")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestHandleQuoteMissingSymbol(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleQuoteNoProvidersReturns503(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=2330", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no providers), got %d", rec.Code)
	}
}

func TestHandleFundamentals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fundamentals.json")
	if err := os.WriteFile(path, []byte(`{"2330.TW":{"PE":25,"PB":6,"PS":8,"DividendYield":1.5,"Sector":"semiconductor"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON(path); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Fundamentals: fp})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/fundamentals?symbol=2330.TW", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, `"PE":25`) {
		t.Fatalf("expected PE in response: %s", body)
	}
}

func TestHandleFundamentalsRawSymbolNormalized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fundamentals.json")
	if err := os.WriteFile(path, []byte(`{"2330.TW":{"PE":25,"PB":6,"PS":8,"DividendYield":1.5,"Sector":"semiconductor"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON(path); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Fundamentals: fp})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/fundamentals?symbol=2330", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, `"PE":25`) {
		t.Fatalf("expected PE in response after raw-symbol normalization: %s", body)
	}
}

func TestNormalizeFundamentalsSymbol(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"2330", "2330.TW"},
		{"2330.TW", "2330.TW"},
		{"AAPL.US", "AAPL.US"},
		{"00700.HK", "00700.HK"},
		{"", ""},
		{"TW", "TW.TW"},
	}
	for _, tc := range cases {
		if got := normalizeFundamentalsSymbol(tc.in); got != tc.want {
			t.Errorf("normalizeFundamentalsSymbol(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHandleChips(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"stat":"OK","data":[["2330","台積電","1000","500","500","0","0","0","800","300","500","400","0","0","0","0","0","0","1400"]]}`))
	}))
	defer server.Close()

	cf := marketdata.NewTWSECapitalFlowProvider("")
	cf.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: server.URL}})

	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{CapitalFlow: cf})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/chips?symbol=2330&date=20260701", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, `"symbol":"2330"`) {
		t.Fatalf("expected symbol in response: %s", body)
	}
}

func TestHandleTechnical(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewJSONLQuoteStore(dir)
	// Relative dates: bars are created at (now - i) days so they always fall
	// inside the handler's 30-day window regardless of when the test runs.
	// Fixed July dates broke after ~Aug (window start moved past them) — the
	// date.Before(start) filter dropped all but one bar → 503 (SK-22 fix).
	now := time.Now()
	bars := []domain.DailyBar{
		{Date: now.AddDate(0, 0, -4), Symbol: "2330.TW", Close: 650, Volume: 1000},
		{Date: now.AddDate(0, 0, -3), Symbol: "2330.TW", Close: 660, Volume: 1100},
		{Date: now.AddDate(0, 0, -2), Symbol: "2330.TW", Close: 670, Volume: 1200},
		{Date: now.AddDate(0, 0, -1), Symbol: "2330.TW", Close: 680, Volume: 1300},
		{Date: now, Symbol: "2330.TW", Close: 690, Volume: 1400},
	}
	if err := store.RecordQuotes(bars); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{QuoteStore: store})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/technical?symbol=2330&days=30", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, `"close":690`) {
		t.Fatalf("expected close in response: %s", body)
	}
}

func TestComputeTechnical(t *testing.T) {
	bars := make([]domain.DailyBar, 25)
	for i := 0; i < 25; i++ {
		bars[i] = domain.DailyBar{
			Date:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Symbol: "2330.TW",
			Close:  float64(10 + i),
		}
	}
	out := computeTechnical(bars)
	if out["sma20"].(float64) == 0 {
		t.Fatal("expected non-zero sma20")
	}
	if out["rsi14"].(float64) == 0 {
		t.Fatal("expected non-zero rsi14")
	}
}

func TestRSI(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		n      int
		want   float64
	}{
		{"equal gains/losses (RS=1) → 50", []float64{10, 11, 10, 11, 10, 11, 10, 11, 10, 11, 10, 11, 10, 11, 10}, 14, 50},
		{"all losses (RS=0) → 0", []float64{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6}, 14, 0},
		{"all gains (losses=0) → 100", []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}, 14, 100},
	}
	for _, tc := range cases {
		got := rsi(tc.values, tc.n)
		if got != tc.want {
			t.Errorf("rsi(%v, %d) = %v, want %v", tc.name, tc.n, got, tc.want)
		}
	}
}

func TestRSIBugRegression(t *testing.T) {
	values := []float64{10, 11, 10, 11, 10, 11, 10, 11, 10, 11, 10, 11, 10, 11, 10}
	got := rsi(values, 14)
	if got < 0 || got > 100 {
		t.Fatalf("rsi out of [0, 100] range: got %v (pre-fix bug returned negative values)", got)
	}
	if got != 50 {
		t.Fatalf("rsi(equal gains/losses) = %v, want 50 (standard formula)", got)
	}
}

// Helpers for the coverage-guard tests below. Duplicated from
// coverage_test.go intentionally to keep handler_test.go self-contained
// (no exported test-helpers across this package's two _test.go files).
func writeSnapshotHandlerTest(t *testing.T, entries string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fundamentals.json")
	if err := os.WriteFile(path, []byte(entries), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	return path
}

func loadFundamentalsForTest(t *testing.T, entries string) *portfolio.FundamentalProvider {
	t.Helper()
	path := writeSnapshotHandlerTest(t, entries)
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON(path); err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	return fp
}

// TestHandleFundamentals_OutOfScopeReturns200WithNote verifies that an
// out-of-scope symbol (e.g. 3131 上櫃) gets a 200 response carrying a
// `coverage_note` field instead of the misleading 200 with all-zero
// data seen before this PR. Frontend/MCP callers branch on this field.
func TestHandleFundamentals_OutOfScopeReturns200WithNote(t *testing.T) {
	fp := loadFundamentalsForTest(t, `{"6641.TW":{"PE":39.67,"PB":0.45,"DividendYield":2.19}}`)
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Fundamentals: fp})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/fundamentals?symbol=3131", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !contains(rec.Body.String(), `"coverage_note":"NOT_COVERED"`) {
		t.Fatalf("expected NOT_COVERED note in body, got: %s", rec.Body.String())
	}
}

// TestHandleChips_OutOfScopeShortCircuits verifies that the chips guard
// returns 200 + coverage_note BEFORE invoking the CapitalFlow provider's
// 7-day fallback loop. This prevents the 15s context budget collision we
// observed for 3131/3587 chips (see docs/manifests/2026-08-06-stock-coverage-notice.md).
func TestHandleChips_OutOfScopeShortCircuits(t *testing.T) {
	fp := loadFundamentalsForTest(t, `{"6641.TW":{"PE":39.67}}`)
	mux := http.NewServeMux()
	// CapitalFlow intentionally NOT provided — guard short-circuits before
	// the CapitalFlow nil-check fires, so we see coverage_note, not 503.
	RegisterRoutes(mux, Deps{Fundamentals: fp})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/chips?symbol=3131", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (guard short-circuits), got %d: %s", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), `"coverage_note":"NOT_COVERED"`) {
		t.Fatalf("expected coverage_note, got: %s", rec.Body.String())
	}
}

// TestHandleChips_InScopeStillReachesCapitalFlow regression guard — an
// in-scope symbol must still reach the CapitalFlow provider and produce
// the existing JSON shape with no coverage_note leakage.
func TestHandleChips_InScopeStillReachesCapitalFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"stat":"OK","data":[["6641","基士德-KY","17000","5000","12000","0","0","0","0","0","0","-5106","0","5106","-5106","0","0","0","6894"]]}`))
	}))
	defer server.Close()

	cf := marketdata.NewTWSECapitalFlowProvider("")
	cf.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: server.URL}})
	fp := loadFundamentalsForTest(t, `{"6641.TW":{"PE":39.67}}`)

	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{CapitalFlow: cf, Fundamentals: fp})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/chips?symbol=6641", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if contains(rec.Body.String(), `"coverage_note"`) {
		t.Fatalf("in-scope chips should not carry coverage_note, got: %s", rec.Body.String())
	}
	if !contains(rec.Body.String(), `"symbol":"6641"`) {
		t.Fatalf("expected existing chips body shape, got: %s", rec.Body.String())
	}
}

// TestHandleFundamentals_InScopeKeepsExistingFormat regression guard:
// in-scope symbols continue to return the FundamentalData shape unchanged.
func TestHandleFundamentals_InScopeKeepsExistingFormat(t *testing.T) {
	fp := loadFundamentalsForTest(t, `{"2330.TW":{"PE":25,"PB":6,"PS":8,"DividendYield":1.5,"Sector":"semiconductor"}}`)
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Fundamentals: fp})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/fundamentals?symbol=2330", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if contains(rec.Body.String(), `"coverage_note"`) {
		t.Fatalf("in-scope fundamentals should not carry coverage_note, got: %s", rec.Body.String())
	}
	if !contains(rec.Body.String(), `"PE":25`) {
		t.Fatalf("expected PE=25 in body, got: %s", rec.Body.String())
	}
}

// TestHandleChips_NoFundamentalsLoadSkipsGuard confirms the guard is
// fully transparent when Fundamentals is nil — preserves the existing
// TestHandleChips fixture (Deps{CapitalFlow: cf}, no Fundamentals).
func TestHandleChips_NoFundamentalsLoadSkipsGuard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"stat":"OK","data":[["2330","台積電","1000","500","500","0","0","0","800","300","500","400","0","0","0","0","0","0","1400"]]}`))
	}))
	defer server.Close()

	cf := marketdata.NewTWSECapitalFlowProvider("")
	cf.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: server.URL}})

	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{CapitalFlow: cf}) // intentionally no Fundamentals
	req := httptest.NewRequest(http.MethodGet, "/api/stock/chips?symbol=2330&date=20260701", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (guard skipped), got %d: %s", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), `"symbol":"2330"`) {
		t.Fatalf("expected existing chips body shape, got: %s", rec.Body.String())
	}
}
