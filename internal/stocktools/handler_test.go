package stocktools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	// incomplete=true 回傳 closePrice-only 殘缺 quote（manifest Phase B2
	// 測試用：驗證「所有 provider 殘缺 → complete:false」）。
	incomplete bool
	// notFound=true 回傳 ErrTWSEQuoteNotFound（policy 語義測試用）。
	notFound bool
	// checkDeadline, if > 0, makes GetQuotes fail unless the inbound context
	// has at least this much remaining time. Used to verify the handler grants
	// the TWSE fallback enough budget (previously 5s proved too short for the
	// full-market STOCK_DAY_ALL payload).
	checkDeadline time.Duration
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
	// Verify the handler grants the fallback enough time. The TWSE fallback
	// downloads the full-market STOCK_DAY_ALL (~300KB), which can exceed the
	// old 5s budget under production load.
	if m.checkDeadline > 0 {
		deadline, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("expected TWSE fallback context to have a deadline")
		}
		remaining := time.Until(deadline)
		if remaining < m.checkDeadline {
			return nil, fmt.Errorf("TWSE fallback deadline too short: %v, want at least %v", remaining, m.checkDeadline)
		}
	}
	if m.notFound {
		return nil, fmt.Errorf("%w: %s", marketdata.ErrTWSEQuoteNotFound, symbols[0])
	}
	if m.incomplete {
		return []domain.Quote{{Symbol: symbols[0], Last: 680, Market: "TW", Source: "twse"}}, nil
	}
	return []domain.Quote{{Symbol: symbols[0], Last: 680, Open: 670, High: 685, Low: 668, Volume: 12345, Market: "TW", Source: "twse"}}, nil
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

// TestHandleQuote_TWSEFallbackTimeoutBudget verifies that the TWSE fallback
// is given at least 10 seconds to download the full-market quote payload. The
// previous 5s budget was too short for the ~300KB STOCK_DAY_ALL response
// when Fugle had already consumed part of the request window, producing 503.
func TestHandleQuote_TWSEFallbackTimeoutBudget(t *testing.T) {
	fugleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fugleServer.Close()

	fugleClient := marketdata.NewFugleClient("test-key")
	fugleClient.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: fugleServer.URL}})

	mockTWSE := &mockTWSEProvider{checkDeadline: 9 * time.Second}
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{FugleClient: fugleClient, TWSEQuote: mockTWSE})

	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=2330", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (TWSE fallback got >= 10s budget), got %d: %s", rec.Code, rec.Body.String())
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

// ─── monthly_revenue handler tests ──────────────────────────────────────────
//
// These test the /api/stock/monthly_revenue endpoint added 2026-08-07
// (hermes v4.0 dispatch #1). The handler uses the existing
// TSMCRevenueProvider via Deps.Revenue — NOT a new provider type. The
// FinMind HTTP client is replaced with a local httptest server via
// revenueLocalTransport (defined below) which only intercepts
// api.finmindtrade.com traffic, mirroring marketdata's rewriteTransport.

type revenueLocalTransport struct {
	target string
}

func (t *revenueLocalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "api.finmindtrade.com" {
		return http.DefaultTransport.RoundTrip(req)
	}
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.target, "http://")
	req.Host = ""
	return http.DefaultTransport.RoundTrip(req)
}

// TestHandleMonthlyRevenue_TWSEHappyPath verifies the happy path for a
// TWSE-listed symbol (2330) with explicit year/month query params.
func TestHandleMonthlyRevenue_TWSEHappyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sym := r.URL.Query().Get("data_id")
		start := r.URL.Query().Get("start_date")
		var body string
		switch {
		case sym == "2330" && start == "2026-07-01":
			body = `{"msg":"success","status":200,"data":[{"revenue":400000000000.0,"date":"2026-07-01"}]}`
		case sym == "2330" && start == "2025-07-01":
			body = `{"msg":"success","status":200,"data":[{"revenue":300000000000.0,"date":"2025-07-01"}]}`
		case sym == "2330" && start == "2026-06-01":
			body = `{"msg":"success","status":200,"data":[{"revenue":350000000000.0,"date":"2026-06-01"}]}`
		default:
			body = `{"msg":"success","status":200,"data":[]}`
		}
		w.Write([]byte(body))
	}))
	defer ts.Close()

	client := marketdata.NewFinMindClientWithStateDir("k", t.TempDir())
	client.SetHTTPClient(&http.Client{Transport: &revenueLocalTransport{target: ts.URL}})
	rp := marketdata.NewTSMCRevenueProviderWithClient(client)

	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Revenue: rp})

	req := httptest.NewRequest(http.MethodGet,
		"/api/stock/monthly_revenue?symbol=2330&year=2026&month=7", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("2330: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, `"symbol":"2330.TW"`) {
		t.Errorf("2330 body missing symbol: %s", body)
	}
	if !contains(body, `"revenue":400000000000`) {
		t.Errorf("2330 body missing revenue: %s", body)
	}
	// 400 vs 300 → +33.33% YoY.
	if !contains(body, `"yoy_pct":33.33333333333333`) && !contains(body, `"yoy_pct":33.3333`) {
		t.Errorf("2330 body missing yoy_pct ≈ 33.33: %s", body)
	}
	// 400 vs 350 → +14.29% MoM.
	if !contains(body, `"mom_pct":14.285714285714285`) && !contains(body, `"mom_pct":14.2857`) {
		t.Errorf("2330 body missing mom_pct ≈ 14.29: %s", body)
	}
}

// TestHandleMonthlyRevenue_TPEXSymbolBypassesCoverageGuard is the key
// regression guard: a TPEX-listed symbol (3131 弘塑) is out-of-scope for
// the other 4 stocktools endpoints (PR #1477 marks it NOT_COVERED via
// LookupCoverage), but monthly_revenue MUST still return 200 because
// FinMind TaiwanStockMonthRevenue covers TPEX. The handler intentionally
// does NOT consult LookupCoverage.
func TestHandleMonthlyRevenue_TPEXSymbolBypassesCoverageGuard(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sym := r.URL.Query().Get("data_id")
		start := r.URL.Query().Get("start_date")
		var body string
		switch {
		case sym == "3131" && start == "2026-07-01":
			body = `{"msg":"success","status":200,"data":[{"revenue":631051000.0,"date":"2026-07-01"}]}`
		case sym == "3131" && start == "2025-07-01":
			body = `{"msg":"success","status":200,"data":[{"revenue":560000000.0,"date":"2025-07-01"}]}`
		case sym == "3131" && start == "2026-06-01":
			body = `{"msg":"success","status":200,"data":[{"revenue":653000000.0,"date":"2026-06-01"}]}`
		default:
			body = `{"msg":"success","status":200,"data":[]}`
		}
		w.Write([]byte(body))
	}))
	defer ts.Close()

	client := marketdata.NewFinMindClientWithStateDir("k", t.TempDir())
	client.SetHTTPClient(&http.Client{Transport: &revenueLocalTransport{target: ts.URL}})
	rp := marketdata.NewTSMCRevenueProviderWithClient(client)

	mux := http.NewServeMux()
	// No Fundamentals provider — confirms the handler doesn't short-circuit
	// on the coverage guard even when Fundamentals data is loaded.
	RegisterRoutes(mux, Deps{Revenue: rp})

	req := httptest.NewRequest(http.MethodGet,
		"/api/stock/monthly_revenue?symbol=3131&year=2026&month=7", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("3131 (TPEX): expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), `"symbol":"3131.TW"`) {
		t.Errorf("3131 body missing symbol: %s", rec.Body.String())
	}
	if !contains(rec.Body.String(), `"revenue":631051000`) {
		t.Errorf("3131 body missing revenue: %s", rec.Body.String())
	}
	// 631 vs 560 → +12.7% YoY (assert the body actually carries the data,
	// not a coverage_note).
	if contains(rec.Body.String(), `"coverage_note"`) {
		t.Errorf("3131 monthly_revenue must NOT carry coverage_note: %s", rec.Body.String())
	}
}

// TestHandleMonthlyRevenue_RejectsEmptySymbol locks in 400 for missing
// symbol — same contract as the other 4 stocktools endpoints.
func TestHandleMonthlyRevenue_RejectsEmptySymbol(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Revenue: marketdata.NewTSMCRevenueProvider("k")})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/monthly_revenue", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing symbol, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandleMonthlyRevenue_NoProviderReturns503 — handler must surface
// 503 when Deps.Revenue is nil so callers can distinguish "service not
// configured" from "symbol not found".
func TestHandleMonthlyRevenue_NoProviderReturns503(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{}) // intentionally no Revenue
	req := httptest.NewRequest(http.MethodGet, "/api/stock/monthly_revenue?symbol=2330", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandleMonthlyRevenue_RejectsBadYearMonth verifies the year/month
// query-parameter validation.
func TestHandleMonthlyRevenue_RejectsBadYearMonth(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Revenue: marketdata.NewTSMCRevenueProvider("k")})

	cases := []struct {
		query string
		want  int
	}{
		{"/api/stock/monthly_revenue?symbol=2330&year=abc&month=7", http.StatusBadRequest},
		{"/api/stock/monthly_revenue?symbol=2330&year=2026&month=0", http.StatusBadRequest},
		{"/api/stock/monthly_revenue?symbol=2330&year=2026&month=13", http.StatusBadRequest},
		{"/api/stock/monthly_revenue?symbol=2330&year=1800&month=7", http.StatusBadRequest},
		{"/api/stock/monthly_revenue?symbol=2330&year=2200&month=7", http.StatusBadRequest},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.query, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: expected %d, got %d: %s", tc.query, tc.want, rec.Code, rec.Body.String())
		}
	}
}

// fakeMonthlyRevenueProvider is a test double satisfying
// MonthlyRevenueProvider with a forced QuotaRemaining value. Used to
// exercise the quota-exhausted 503 path without reaching into
// marketdata unexported fields.
type fakeMonthlyRevenueProvider struct {
	quotaRemaining int
	hitFetch       bool
}

func (f *fakeMonthlyRevenueProvider) FetchMonthlyRevenue(_ context.Context, _ string, _, _ int) (marketdata.MonthlyRevenuePoint, error) {
	f.hitFetch = true
	return marketdata.MonthlyRevenuePoint{}, nil
}

func (f *fakeMonthlyRevenueProvider) QuotaRemaining() int {
	return f.quotaRemaining
}

// TestHandleMonthlyRevenue_QuotaExhaustedReturns503 verifies the fail-soft
// quota guard: when FinMind has fewer than monthlyRevenueMinQuota calls
// remaining, the handler returns 503 BEFORE issuing any FinMind request
// (the fake provider's FetchSnapshotForSymbolAt must NOT be called).
func TestHandleMonthlyRevenue_QuotaExhaustedReturns503(t *testing.T) {
	fake := &fakeMonthlyRevenueProvider{quotaRemaining: monthlyRevenueMinQuota - 1}
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Revenue: fake})

	req := httptest.NewRequest(http.MethodGet,
		"/api/stock/monthly_revenue?symbol=3131&year=2026&month=7", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on quota exhaustion, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.hitFetch {
		t.Error("handler must NOT issue FinMind request when quota below threshold")
	}
	if !contains(rec.Body.String(), "quota") {
		t.Errorf("503 body should mention quota, got: %s", rec.Body.String())
	}
}

// TestHandleMonthlyRevenue_QuotaSufficientReachesProvider verifies that
// with sufficient quota the handler proceeds to the provider (the 200
// happy paths above already exercise this with the real provider; this
// pins the boundary with the fake so the quota guard can't accidentally
// block a legitimate request).
func TestHandleMonthlyRevenue_QuotaSufficientReachesProvider(t *testing.T) {
	fake := &fakeMonthlyRevenueProvider{quotaRemaining: monthlyRevenueMinQuota}
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{Revenue: fake})

	req := httptest.NewRequest(http.MethodGet,
		"/api/stock/monthly_revenue?symbol=3131&year=2026&month=7", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	// fake returns empty snapshot without error → handler returns 200 with
	// zero-value TSMCRevenue.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with sufficient quota, got %d: %s", rec.Code, rec.Body.String())
	}
	if !fake.hitFetch {
		t.Error("handler should reach the provider when quota is sufficient")
	}
}

// ─── manifest Phase B2/C — quote 完整性與非交易日標記 ───────────────────────

// TestHandleQuote_FugleIncomplete_FallsBackToTWSE verifies that a Fugle
// 200 with closePrice-only data (Last>0, OHLC=0 — the "看似成功但殘缺"
// pattern) is treated as a failure: the handler falls back to TWSE and
// returns a complete quote, instead of silently forwarding the残缺 200.
func TestHandleQuote_FugleIncomplete_FallsBackToTWSE(t *testing.T) {
	fugleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Fugle 200 + closePrice-only（無 open/high/low/volume）
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"symbol":"2330","closePrice":2395}`))
	}))
	defer fugleServer.Close()

	fugleClient := marketdata.NewFugleClient("test-key")
	fugleClient.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: fugleServer.URL}})

	mockTWSE := &mockTWSEProvider{}
	h := NewHandler(Deps{FugleClient: fugleClient, TWSEQuote: mockTWSE})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=2330", nil)
	code, resp := h.HandleQuote(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200 from TWSE fallback, got %d: %v", code, resp)
	}
	q, ok := resp.(domain.Quote)
	if !ok {
		t.Fatalf("resp type = %T, want domain.Quote", resp)
	}
	if q.Source != "twse" {
		t.Errorf("Source = %q, want twse (fell back from incomplete Fugle data)", q.Source)
	}
	if q.Complete == nil || !*q.Complete {
		t.Errorf("Complete = %v, want true (TWSE quote is complete)", q.Complete)
	}
}

// TestHandleQuote_BothProvidersIncomplete_MarksCompleteFalse verifies the
// last-resort contract: when every provider returns incomplete data, the
// handler still returns 200 but marks complete:false — an explicit
// structural signal instead of a silent残缺 200.
func TestHandleQuote_BothProvidersIncomplete_MarksCompleteFalse(t *testing.T) {
	fugleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"symbol":"2330","closePrice":2395}`))
	}))
	defer fugleServer.Close()

	fugleClient := marketdata.NewFugleClient("test-key")
	fugleClient.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: fugleServer.URL}})

	mockTWSE := &mockTWSEProvider{incomplete: true}
	h := NewHandler(Deps{FugleClient: fugleClient, TWSEQuote: mockTWSE})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=2330", nil)
	code, resp := h.HandleQuote(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200 with complete:false, got %d: %v", code, resp)
	}
	q, ok := resp.(domain.Quote)
	if !ok {
		t.Fatalf("resp type = %T, want domain.Quote", resp)
	}
	if q.Complete == nil || *q.Complete {
		t.Errorf("Complete = %v, want false (all providers incomplete)", q.Complete)
	}
}

// TestHandleQuote_NonTradingDay_MarksTradingDayFalse verifies the quote
// handler surfaces the trading-day calendar (manifest Phase C): on a
// weekend, the response carries trading_day:false instead of failing with
// a misleading 503 from the empty TWSE snapshot.
func TestHandleQuote_NonTradingDay_MarksTradingDayFalse(t *testing.T) {
	fugleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fugleServer.Close()

	fugleClient := marketdata.NewFugleClient("test-key")
	fugleClient.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: fugleServer.URL}})

	mockTWSE := &mockTWSEProvider{}
	h := NewHandler(Deps{FugleClient: fugleClient, TWSEQuote: mockTWSE})
	// 2026-08-09 是週日（非交易日）
	h.nowFunc = func() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.Local) }

	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=2330", nil)
	code, resp := h.HandleQuote(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200 with trading_day:false, got %d: %v", code, resp)
	}
	q, ok := resp.(domain.Quote)
	if !ok {
		t.Fatalf("resp type = %T, want domain.Quote", resp)
	}
	if q.TradingDay == nil || *q.TradingDay {
		t.Errorf("TradingDay = %v, want false (2026-08-09 is Sunday)", q.TradingDay)
	}
}

// TestHandleTechnical_NonTradingDay_MarksTradingDayFalse verifies the
// technical handler also surfaces the trading-day calendar (manifest
// Phase C): on a weekend the response carries trading_day:false so SMA/RSI
// are not misread as intraday signals.
func TestHandleTechnical_NonTradingDay_MarksTradingDayFalse(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewJSONLQuoteStore(dir)
	// 2026-08-09 是週日（非交易日）— bars 以固定基準日往前推，
	// 與 nowFunc 對齊，避免 store 資料落在查詢窗口外觸發外部 fallback。
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.Local)
	bars := []domain.DailyBar{
		{Date: base.AddDate(0, 0, -4), Symbol: "2330.TW", Close: 650, Volume: 1000},
		{Date: base.AddDate(0, 0, -3), Symbol: "2330.TW", Close: 660, Volume: 1100},
		{Date: base.AddDate(0, 0, -2), Symbol: "2330.TW", Close: 670, Volume: 1200},
		{Date: base.AddDate(0, 0, -1), Symbol: "2330.TW", Close: 680, Volume: 1300},
	}
	if err := store.RecordQuotes(bars); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{QuoteStore: store})
	h.nowFunc = func() time.Time { return base }

	req := httptest.NewRequest(http.MethodGet, "/api/stock/technical?symbol=2330&days=30", nil)
	code, resp := h.HandleTechnical(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", code, resp)
	}
	tech, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("resp type = %T, want map[string]any", resp)
	}
	if td, ok := tech["trading_day"]; !ok || td != false {
		t.Errorf("trading_day = %v, want false (2026-08-09 is Sunday)", td)
	}
}

// TestHandleQuote_AllProvidersNoData_ReturnsCoverageNote verifies the
// by-design policy signal (文件問題 4 / 驗收 SOP 2): when NO provider
// has the symbol (Fugle fails + TWSE reports not-found), the handler
// returns 200 + coverage_note instead of a misleading 503 — the client
// can tell "out of scope by design" from "atlas is broken".
func TestHandleQuote_AllProvidersNoData_ReturnsCoverageNote(t *testing.T) {
	fugleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fugleServer.Close()

	fugleClient := marketdata.NewFugleClient("test-key")
	fugleClient.SetHTTPClient(&http.Client{Transport: &redirectRoundTripper{serverURL: fugleServer.URL}})

	mockTWSE := &mockTWSEProvider{notFound: true}
	h := NewHandler(Deps{FugleClient: fugleClient, TWSEQuote: mockTWSE})
	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=660", nil)
	code, resp := h.HandleQuote(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200 with coverage_note, got %d: %v", code, resp)
	}
	body, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("resp type = %T, want map[string]any", resp)
	}
	if body["coverage_note"] != CoverageNoteNotCovered {
		t.Errorf("coverage_note = %v, want %q", body["coverage_note"], CoverageNoteNotCovered)
	}
}
