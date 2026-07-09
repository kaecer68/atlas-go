package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// rewriteTransport redirects HTTPS requests to a local httptest server.
// FinMindClient constructs URLs from a package-level finmindBaseURL const
// that cannot be overridden, so we intercept at http.RoundTripper instead.
type rewriteTransport struct {
	target *url.URL
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = t.target.Scheme
	cloned.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(cloned)
}

func newTestFinMindClient(t *testing.T, srv *httptest.Server) *marketdata.FinMindClient {
	t.Helper()
	serverURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	c := marketdata.NewFinMindClient("test-key")
	c.SetHTTPClient(&http.Client{Transport: &rewriteTransport{target: serverURL}})
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))
	return c
}

func TestBackfillSymbol_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"msg":    "success",
			"status": 200,
			"data": []map[string]any{
				{
					"date":           r.URL.Query().Get("start_date"),
					"stock_id":       r.URL.Query().Get("data_id"),
					"close":          1000.0,
					"open":           1000.0,
					"max":            1010.0,
					"min":            990.0,
					"Trading_Volume": 1000000.0,
				},
			},
		})
	}))
	defer srv.Close()

	store := ledger.NewJSONLQuoteStore(t.TempDir())
	client := newTestFinMindClient(t, srv)

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	n, err := backfillSymbol(context.Background(), client, store, "2330", start, end, false)
	if err != nil {
		t.Fatalf("backfillSymbol failed: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 bars (3 days), got %d", n)
	}

	loaded, err := store.LoadQuotes("2330", start, end)
	if err != nil {
		t.Fatalf("LoadQuotes failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Errorf("expected 3 loaded bars, got %d", len(loaded))
	}
	for _, bar := range loaded {
		if bar.Close != 1000 {
			t.Errorf("expected Close=1000, got %f", bar.Close)
		}
	}
}

func TestBackfillSymbol_NoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"msg":    "success",
			"status": 200,
			"data":   []map[string]any{},
		})
	}))
	defer srv.Close()

	store := ledger.NewJSONLQuoteStore(t.TempDir())
	client := newTestFinMindClient(t, srv)

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	_, err := backfillSymbol(context.Background(), client, store, "2330", start, end, false)
	if err == nil {
		t.Fatal("expected error from empty data, got nil")
	}
}

func TestBackfillSymbol_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := ledger.NewJSONLQuoteStore(t.TempDir())
	client := newTestFinMindClient(t, srv)

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	_, err := backfillSymbol(context.Background(), client, store, "2330", start, end, false)
	if err == nil {
		t.Fatal("expected error from HTTP 500, got nil")
	}
}
