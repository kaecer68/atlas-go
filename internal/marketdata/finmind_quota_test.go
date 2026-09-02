package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

// FinMind free-tier throttling returns HTTP 200 with a non-success msg and an
// empty data array. The client must classify it as ErrQuotaExhausted — not as
// legitimate empty data — so history walk-backs stop instead of burning quota
// (observed 2026-09-02 on the tdcc channel).
func TestFetchDatasetRawFreeTierThrottle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"Your level is free. Please update your user level.","status":200,"data":[]}`))
	}))
	defer srv.Close()

	c := NewFinMindClient("test-key")
	c.SetBaseURL(srv.URL)
	c.SetRateLimiter(rateInf())

	rows, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockHoldingSharesPer", "", "2026-09-01", "2026-09-01")
	if err == nil {
		t.Fatalf("expected quota error, got %d rows", len(rows))
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("expected ErrQuotaExhausted, got: %v", err)
	}
}

// Legitimate no-data responses keep msg="success" and must stay empty-data.
func TestFetchDatasetRawLegitEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer srv.Close()

	c := NewFinMindClient("test-key")
	c.SetBaseURL(srv.URL)
	c.SetRateLimiter(rateInf())

	rows, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockHoldingSharesPer", "", "2026-01-01", "2026-01-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func rateInf() *rate.Limiter {
	return rate.NewLimiter(rate.Inf, 1)
}
