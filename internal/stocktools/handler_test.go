package stocktools

import (
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
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"apiVersion":"v0.3","data":{"info":{"symbol":"2330","name":"台積電","date":"2026-07-07","time":"14:30:00","countryCode":"TW","timeZone":"Asia/Taipei"},"quote":{"trade":{"price":680},"priceOpen":{"price":670},"priceHigh":{"price":685},"priceLow":{"price":668},"total":{"tradeVolume":12345}}}}`))
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
	bars := []domain.DailyBar{
		{Date: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Symbol: "2330.TW", Close: 650, Volume: 1000},
		{Date: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), Symbol: "2330.TW", Close: 660, Volume: 1100},
		{Date: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), Symbol: "2330.TW", Close: 670, Volume: 1200},
		{Date: time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), Symbol: "2330.TW", Close: 680, Volume: 1300},
		{Date: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC), Symbol: "2330.TW", Close: 690, Volume: 1400},
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
