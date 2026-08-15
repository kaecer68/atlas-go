package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestTWSEOddLotProvider_UpstreamRemoved verifies the provider fails fast with
// ErrOddLotUpstreamRemoved when TWSE returns the repurposed flat response
// (BFI84U now serves the 停券預告表 report with no "tables" array).
func TestTWSEOddLotProvider_UpstreamRemoved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stat":"OK","title":"得為融資融券有價證券停券預告表","fields":["股票代號","股票名稱","停券起日","停券迄日","原因"],"data":[["2330","台積電","115.08.12","115.08.17","分配收益"]]}`))
	}))
	defer server.Close()

	p := NewTWSEOddLotProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	_, err := p.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for removed odd-lot upstream")
	}
	if !errors.Is(err, ErrOddLotUpstreamRemoved) {
		t.Fatalf("expected ErrOddLotUpstreamRemoved, got %v", err)
	}
}
