package marketdata

// upstream_body_redaction_test.go — FU-20261016-14.
//
// Every provider in this package copies the upstream error body into its error
// string, because that body is the only way an operator can tell "token
// rejected" from "quota exhausted" from "WAF page". The same bodies also echo
// credential material back: a proxy dumps the request headers it received, an
// upstream envelope carries `"token":"…"`, a vendor answers 401 with the key
// quoted in the message.
//
// The contract each test below pins down is two-sided, and a one-sided test
// would be worthless: the credential must NOT survive into the error, and the
// diagnosis (status code, upstream message) MUST survive. A redaction that ate
// the message would be reverted by the next on-call engineer, so "secret gone"
// alone is not a passing grade.
//
// Fixtures are obviously fake by construction (`FAKE-…`); no real credential
// appears in this file or in the errors it produces.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// a14JSONErrorBody renders an upstream error envelope that carries a
// credential member next to the message an operator needs.
func a14JSONErrorBody(msg, secret string, status int) string {
	return fmt.Sprintf(`{"msg":%q,"status":%d,"token":%q}`, msg, status, secret)
}

// a14AssertRedactedButDiagnosable enforces both halves of the rule: err must be
// non-nil, every want substring must still be present, and the fake credential
// must be gone. The surviving message is logged so the "still diagnosable"
// claim is visible in `go test -v` output instead of being asserted in the dark.
func a14AssertRedactedButDiagnosable(t *testing.T, err error, secret string, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error; got nil")
	}
	message := err.Error()
	for _, part := range want {
		if !strings.Contains(message, part) {
			t.Errorf("error lost diagnostic %q (diagnosability regressed): %s", part, message)
		}
	}
	if strings.Contains(message, secret) {
		t.Errorf("credential leaked into error string: %s", message)
	}
	t.Logf("redacted error: %s", message)
}

// ─── TWSE OpenAPI (twse_openapi.go: STOCK_DAY_ALL and STOCK_DAY) ────────────

func TestTWSEOpenAPI_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-twse-openapi-token-7a1b"
	const msg = "upstream schema changed"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusNotFound)))
	}))
	defer ts.Close()

	c := NewTWSEClient()
	c.baseURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.rateLimiter = newUnlimitedLimiter()

	_, err := c.GetQuotes(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "404", msg)
}

func TestTWSEOpenAPI_DailyQuoteUpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-twse-daily-token-3e2f"
	const msg = "invalid date parameter"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusForbidden)))
	}))
	defer ts.Close()

	c := NewTWSEClient()
	c.baseURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.rateLimiter = newUnlimitedLimiter()

	_, err := c.GetDailyQuote(context.Background(), "20260924", "2330")
	a14AssertRedactedButDiagnosable(t, err, secret, "403", msg)
}

// ─── TWSE MI_INDEX (twse_mi_index.go) ───────────────────────────────────────

func TestTWSEMIIndex_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-twse-mi-token-2c3d"
	const msg = "MI_INDEX upstream unavailable"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusForbidden)))
	}))
	defer ts.Close()

	c := NewTWSEClient()
	c.baseURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.rateLimiter = newUnlimitedLimiter()

	_, err := c.GetQuotesForDate(context.Background(), "20260924")
	a14AssertRedactedButDiagnosable(t, err, secret, "403", msg)
}

// ─── Fugle (fugle_client.go: body on 5xx, and the transport error) ──────────

func TestFugleClient_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-fugle-key-5c1d2e3f"
	const msg = "invalid timestamp header"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		// A proxy in front of Fugle dumps the request headers it received —
		// the API key travels in X-API-KEY, so this is a header-dump shape,
		// not JSON (the textual rule has to catch it).
		_, _ = fmt.Fprintf(w, "upstream failure: %s\nX-API-KEY: %s\n", msg, secret)
	}))
	defer ts.Close()

	c := newFugleClient(secret, t.TempDir())
	c.baseURL = ts.URL
	c.httpClient = ts.Client()
	c.rateLimiter = newUnlimitedLimiter()

	_, err := c.GetQuote(context.Background(), "2330")
	a14AssertRedactedButDiagnosable(t, err, secret, "500", msg)
}

// a14FailingTransport makes http.Client.Do fail with an error that carries the
// credential, which is the shape a transport error would take if the key ever
// moved from the X-API-KEY header into the query string.
type a14FailingTransport struct{ err error }

func (f a14FailingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestFugleClient_TransportErrorRedacted(t *testing.T) {
	const secret = "FAKE-fugle-key-5c1d2e3f"
	transportErr := fmt.Errorf(`Get "https://api.fugle.tw/v1.0/intraday/quote/2330?api_key=%s": dial tcp: connection refused`, secret)

	c := newFugleClient(secret, t.TempDir())
	c.baseURL = "https://api.fugle.tw/v1.0"
	c.httpClient = &http.Client{Transport: a14FailingTransport{err: transportErr}}
	c.rateLimiter = newUnlimitedLimiter()

	_, err := c.GetQuote(context.Background(), "2330")
	a14AssertRedactedButDiagnosable(t, err, secret, "http request", "connection refused")
}

// ─── TAIFEX (taifex_provider.go: PCR + large trader, taifex_institutional.go) ─

func TestTAIFEXPCR_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-taifex-pcr-token-9f8e"
	const msg = "taifex maintenance window"

	// 403 rather than 5xx on purpose: 429/5xx belong to fetchWithRetry, which
	// already redacts its own snippet. The local site in FetchPCR only ever
	// sees the non-retryable codes, so that is the shape worth pinning.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusForbidden)))
	}))
	defer ts.Close()

	p := NewTAIFEXProvider()
	p.baseURL = ts.URL
	p.SetHTTPClient(ts.Client())
	p.rateLimiter = newUnlimitedLimiter()

	_, err := p.FetchPCR(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "403", msg)
}

func TestTAIFEXLargeTrader_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-taifex-oi-token-4d5c"
	const msg = "large trader endpoint retired"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Same reasoning as the PCR case: 4xx is the code that reaches the
		// local body-capture site instead of fetchWithRetry.
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusNotFound)))
	}))
	defer ts.Close()

	p := NewTAIFEXProvider()
	p.baseURL = ts.URL
	p.SetHTTPClient(ts.Client())
	p.rateLimiter = newUnlimitedLimiter()

	_, err := p.FetchRetailFuturesOI(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "404", msg)
}

func TestTAIFEXInstitutional_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-taifex-inst-token-6b7a"
	const msg = "institutional endpoint rejected the call"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusInternalServerError)))
	}))
	defer ts.Close()

	p := NewTAIFEXProvider()
	p.baseURL = ts.URL
	p.SetHTTPClient(ts.Client())
	p.rateLimiter = newUnlimitedLimiter()

	_, err := p.FetchInstitutionalFuturesDaily(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "500", msg)
}

// TestTAIFEXFuturesCSVHeaderPreviewRedacted covers the 200-with-HTML case: the
// schema sentinel quotes the first line of a body that is not CSV at all. That
// preview is upstream text on an error path, so it goes through the same rule.
func TestTAIFEXFuturesCSVHeaderPreviewRedacted(t *testing.T) {
	const secret = "FAKE-taifex-csv-bearer-2f3e4d5c"
	body := "Authorization: Bearer " + secret

	err := verifyFuturesCSVHeader(body)
	if err == nil {
		t.Fatal("expected a schema error for a non-CSV body")
	}
	a14AssertRedactedButDiagnosable(t, err, secret, "header sentinel missing", "non-CSV body")
	if !errors.Is(err, ErrTAIFEXFuturesBarsSchema) {
		t.Errorf("schema error identity lost: %v", err)
	}
}

// ─── TPEx daily close (tpex_daily_close.go) ─────────────────────────────────

func TestTPExDailyClose_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-tpex-token-1a2b3c"
	const msg = "table not published"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusNotFound)))
	}))
	defer ts.Close()

	client := newTPExTestClient(t, ts)
	_, err := client.Snapshot(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "404", msg)
}

// ─── BDI (bdi_provider.go, CNBC) ────────────────────────────────────────────

func TestBDIProvider_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-bdi-token-8d9e0f"
	const msg = "rate limit exceeded by CNBC"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusTooManyRequests)))
	}))
	defer ts.Close()

	SetBDILimiterForTest(newUnlimitedLimiter())

	p := NewBDIProvider()
	p.endpoint = ts.URL
	p.client = ts.Client()

	_, err := p.FetchSnapshot(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "429", msg)
}

// ─── BLS CPI (bls_cpi_provider.go) ──────────────────────────────────────────

func TestBLSCPIProvider_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-bls-token-3f4e5d"
	const msg = "invalid series id"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusUnauthorized)))
	}))
	defer ts.Close()

	p := NewBLSCPIProvider()
	p.endpoint = ts.URL
	p.client = ts.Client()

	_, err := p.FetchSnapshot(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "401", msg)
}

// ─── Customs export statistics (export_provider.go) ─────────────────────────

func TestExportStatistics_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-customs-token-7c8b9a"
	const msg = "customs portal maintenance window"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusNotFound)))
	}))
	defer ts.Close()

	p := NewExportStatisticsProvider(t.TempDir())
	p.client = ts.Client()
	p.limiter = newUnlimitedLimiter()
	p.baseURL = ts.URL

	_, err := p.FetchSnapshot(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "404", msg)
}

// ─── Yahoo session (yahoo_session.go) ───────────────────────────────────────

func TestYahooSession_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-yahoo-crumb-9d8e7f6a"
	const msg = "Invalid Cookie"

	// buildChartURL always uses https, so the stub has to be a TLS server and
	// the host list is handed over without its scheme.
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusInternalServerError)))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	t.Cleanup(func() { yahooHosts = origHosts })

	SetYahooSessionClient(ts.Client())

	session := getYahooSession()
	_, err := session.fetchFromHost(context.Background(), yahooHosts[0], "2330", map[string]string{
		"interval": "1d",
		"range":    yahooStockRange,
	})
	a14AssertRedactedButDiagnosable(t, err, secret, "500", msg)
}

// ─── Fubon proxy (fubon_client.go: quote, batch, market status) ─────────────

func TestFubonClient_QuoteUpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-fubon-token-5a6b7c"
	const msg = "upstream quote provider unreachable"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusBadGateway)))
	}))
	defer ts.Close()

	c := NewFubonClient()
	c.proxyURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.intradayLimiter = newUnlimitedLimiter()

	_, err := c.GetQuote(context.Background(), "2330")
	a14AssertRedactedButDiagnosable(t, err, secret, "502", msg)
}

func TestFubonClient_BatchQuotesUpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-fubon-batch-token-7d8e"
	const msg = "batch quote rejected"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusInternalServerError)))
	}))
	defer ts.Close()

	c := NewFubonClient()
	c.proxyURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.intradayLimiter = newUnlimitedLimiter()

	_, err := c.GetQuotes(context.Background(), []string{"2330", "2317"})
	a14AssertRedactedButDiagnosable(t, err, secret, "500", msg)
}

func TestFubonClient_MarketStatusUpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-fubon-status-token-9c0d"
	const msg = "market status endpoint unavailable"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusServiceUnavailable)))
	}))
	defer ts.Close()

	c := NewFubonClient()
	c.proxyURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.intradayLimiter = newUnlimitedLimiter()

	_, err := c.CheckMarketStatus(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "503", msg)
}

// ─── TAIEX TWSE fallback (taiex_twse_fallback.go) ───────────────────────────

func TestTAIEXFallback_UpstreamBodyRedacted(t *testing.T) {
	const secret = "FAKE-taiex-twse-token-2b3c4d"
	const msg = "MI_INDEX unavailable"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(a14JSONErrorBody(msg, secret, http.StatusServiceUnavailable)))
	}))
	defer ts.Close()

	ResetSharedTWSEClient()
	shared := GetSharedTWSEClient()
	shared.SetHTTPClient(ts.Client())
	shared.rateLimiter = newUnlimitedLimiter()

	origURL := taiexTWSEBaseURL
	taiexTWSEBaseURL = ts.URL
	t.Cleanup(func() { taiexTWSEBaseURL = origURL })

	_, err := fetchTWSETAIEXFallback(context.Background())
	a14AssertRedactedButDiagnosable(t, err, secret, "503", msg)
}
