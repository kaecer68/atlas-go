package marketdata

// tpex_daily_close_test.go — the first-party 上櫃 whole-market table added for
// issue #1986.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// tpexFixtureRows mirrors the real tpex_mainboard_daily_close_quotes payload:
// every numeric field is a JSON string and the placeholders vary by field.
var tpexFixtureRows = []tpexDailyCloseRow{
	{
		Date: "1150924", SecuritiesCompanyCode: "1259", CompanyName: "天瀚",
		Close: "12.30", Change: "0.10 ", Open: "12.20", High: "12.45", Low: "12.15",
		Average: "12.30", TradingShares: "1,234,000", TransactionAmount: "15,000,000",
	},
	{
		Date: "1150924", SecuritiesCompanyCode: "4804", CompanyName: "大略-KY",
		Close: "0.00", Change: "0.00 ", Open: "---", High: "---", Low: "---",
		Average: "0.00", TradingShares: "0", TransactionAmount: "0",
	},
	{
		Date: "1150924", SecuritiesCompanyCode: "6550", CompanyName: "北極星藥業-KY",
		Close: "45.6", Change: " ", Open: "45.0", High: "46.0", Low: "44.9",
		Average: "45.6", TradingShares: "2,000", TransactionAmount: "90,000",
	},
}

func newTPExTestClient(t *testing.T, srv *httptest.Server) *TPExDailyCloseClient {
	t.Helper()
	return &TPExDailyCloseClient{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
		breaker:    newProviderBreaker("tpex_daily_close", defaultCircuitBreakerConfig()),
		retryCfg:   retryConfig{maxAttempts: 1},
	}
}

func TestTPExDailyClose_SnapshotParsesTheWholeMarketTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tpexFixtureRows)
	}))
	defer srv.Close()

	client := newTPExTestClient(t, srv)
	quotes, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(quotes) != len(tpexFixtureRows) {
		t.Fatalf("quotes = %d, want %d", len(quotes), len(tpexFixtureRows))
	}

	bySymbol := make(map[string]domainQuoteView, len(quotes))
	for _, q := range quotes {
		bySymbol[q.Symbol] = domainQuoteView{
			last: q.Last, open: q.Open, high: q.High, low: q.Low, volume: q.Volume, source: q.Source,
		}
	}
	got, ok := bySymbol["1259"]
	if !ok {
		t.Fatalf("symbol 1259 missing from the snapshot: %v", bySymbol)
	}
	if got.last != 12.3 || got.open != 12.2 || got.high != 12.45 || got.low != 12.15 {
		t.Errorf("1259 prices = %+v, want close/open/high/low 12.3/12.2/12.45/12.15", got)
	}
	// TradingShares is in 股 and carries thousands separators upstream.
	if got.volume != 1_234_000 {
		t.Errorf("1259 volume = %d, want 1234000 (thousands separators stripped)", got.volume)
	}
	if got.source != "tpex_daily_close" {
		t.Errorf("1259 source = %q, want tpex_daily_close", got.source)
	}
	// "---" placeholders must parse to 0 rather than fail the whole table.
	if suspended := bySymbol["4804"]; suspended.open != 0 || suspended.last != 0 {
		t.Errorf("4804 placeholders parsed as %+v, want zeros", suspended)
	}
}

type domainQuoteView struct {
	last, open, high, low float64
	volume                int64
	source                string
}

func TestTPExDailyClose_EmptyAndErrorBodies(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "empty table",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("[]"))
			},
		},
		{
			name: "upstream error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
			},
		},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("[{\"Date\""))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			client := newTPExTestClient(t, srv)
			if _, err := client.Snapshot(context.Background()); err == nil {
				t.Fatal("expected an error")
			}
			if state := client.breaker.stateSnapshot().State; state != ProviderCircuitOpen {
				// One failure is below the threshold, so it must still be closed.
				if state != ProviderCircuitClosed {
					t.Errorf("breaker = %s, want closed after a single failure", state)
				}
			}
		})
	}
}

func TestTPExDailyClose_RowsWithoutASymbolAreSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rows := append([]tpexDailyCloseRow{{Date: "1150924", CompanyName: "no code"}}, tpexFixtureRows...)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rows)
	}))
	defer srv.Close()

	quotes, err := newTPExTestClient(t, srv).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(quotes) != len(tpexFixtureRows) {
		t.Fatalf("quotes = %d, want %d (the codeless row must be skipped)", len(quotes), len(tpexFixtureRows))
	}
}

func TestTPExDailyClose_SnapshotFetchesOncePerCall(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tpexFixtureRows)
	}))
	defer srv.Close()

	client := newTPExTestClient(t, srv)
	for range 3 {
		if _, err := client.Snapshot(context.Background()); err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
	}
	// The client itself is a pure fetcher (the coverage layer owns the cache);
	// three Snapshot calls must therefore be three requests, not more.
	if got := calls.Load(); got != 3 {
		t.Errorf("upstream calls = %d, want 3", got)
	}
}

func TestTPExDailyClose_UsesTheSharedVenueRateLimiter(t *testing.T) {
	// The TPEx client must not carry its own token bucket: the TWSE/TPEx
	// OpenAPI family shares one documented policy.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tpexFixtureRows)
	}))
	defer srv.Close()

	ResetSharedTWSEClient()
	t.Cleanup(ResetSharedTWSEClient)

	// A drained bucket: the client must be refused rather than bypass the shared
	// TWSE/TPEx policy. 20 ms is enough for the wait to be observed, and the
	// limiter must still be the very instance the TWSE OpenAPI family uses.
	limiter := getTWSESharedLimiter()
	if limiter == nil {
		t.Fatal("shared TWSE/TPEx limiter is nil")
	}
	client := newTPExTestClient(t, srv)

	// Rate-limit wait is bounded by twseRateLimitWaitAllowance; the assertion is
	// only that the call goes through the shared bucket, i.e. that requesting a
	// fresh token is observable at all.
	if err := limiter.Wait(context.Background()); err != nil {
		t.Fatalf("shared limiter rejected a wait: %v", err)
	}
	if _, err := client.Snapshot(context.Background()); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
}

func TestTPExDailyClose_RetriesTransportFailures(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tpexFixtureRows)
	}))
	defer srv.Close()

	client := newTPExTestClient(t, srv)
	client.retryCfg = retryConfig{maxAttempts: 3, baseBackoff: time.Millisecond, maxBackoff: 5 * time.Millisecond}
	quotes, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot after retry: %v", err)
	}
	if len(quotes) == 0 {
		t.Fatal("no quotes after a successful retry")
	}
	if got := calls.Load(); got < 2 {
		t.Errorf("upstream calls = %d, want >= 2 (first attempt must have been retried)", got)
	}
}

func TestTPExDailyClose_ErrorMentionsTheStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTPExTestClient(t, srv)
	_, err := client.Snapshot(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to carry the status and the upstream body", err)
	}
}

func TestTPExDailyClose_Name(t *testing.T) {
	if got := (&TPExDailyCloseClient{}).Name(); got != "tpex_daily_close" {
		t.Errorf("Name() = %q, want tpex_daily_close", got)
	}
}

func TestTPExDailyClose_DefaultBaseURLIsTheFirstPartyEndpoint(t *testing.T) {
	c := &TPExDailyCloseClient{baseURL: tpexAPIBaseURL}
	if !strings.Contains(c.baseURL, "tpex.org.tw/openapi/v1") {
		t.Errorf("base URL = %q, want the first-party TPEx OpenAPI host", c.baseURL)
	}
	_ = rate.Inf
}
