package marketdata_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestYahooStockProvider_DailyChange verifies the unified YahooStockProvider
// correctly computes daily change from a mocked 5-day Yahoo response.
func TestYahooStockProvider_DailyChange(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 5-day response: closes = [100, 101, 99, 102, 105]
		// Expected daily change: (105-102)/102*100 ≈ 2.94%
		w.Write([]byte(`{
			"chart": {
				"result": [{
					"meta": {"regularMarketTime": 1750000000, "regularMarketPrice": 105},
					"indicators": {"quote": [{"close": [100, 101, 99, 102, 105]}]}
				}]
			}
		}`))
	}))
	defer ts.Close()

	client := &http.Client{Transport: &rewriteHostTransport{target: ts.URL}}
	marketdata.SetYahooSessionClient(client)

	p := marketdata.NewNVDAProvider()
	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}
	if snap.NVDA.Value != 105 {
		t.Errorf("NVDA.Value = %f, want 105", snap.NVDA.Value)
	}
	wantChangePct := 2.94
	if snap.NVDA.ChangePct < wantChangePct-0.1 || snap.NVDA.ChangePct > wantChangePct+0.1 {
		t.Errorf("NVDA.ChangePct = %f, want ~%.2f", snap.NVDA.ChangePct, wantChangePct)
	}
}

// TestYahooStockProvider_BoundsCapRejection verifies that implausible daily
// changes (>30%) are rejected as errors rather than returned to consumers.
func TestYahooStockProvider_BoundsCapRejection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// closes = [100, 200] → daily change = +100% (> 30% cap)
		w.Write([]byte(`{
			"chart": {
				"result": [{
					"meta": {"regularMarketTime": 1750000000},
					"indicators": {"quote": [{"close": [100, 200]}]}
				}]
			}
		}`))
	}))
	defer ts.Close()

	client := &http.Client{Transport: &rewriteHostTransport{target: ts.URL}}
	marketdata.SetYahooSessionClient(client)

	p := marketdata.NewTSMADRProvider()
	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error for implausible daily change >30%, got nil")
	}
}

// rewriteHostTransport redirects all HTTP requests to the test server URL.
type rewriteHostTransport struct{ target string }

func (t *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the request URL to point to the test server.
	// The Yahoo session tries multiple hosts, but they'll all be redirected here.
	return http.DefaultTransport.RoundTrip(cloneRequestForURL(req, t.target))
}

func cloneRequestForURL(req *http.Request, target string) *http.Request {
	r := req.Clone(req.Context())
	r.URL.Scheme = "http"
	r.URL.Host = target[7:] // strip "http://"
	return r
}
