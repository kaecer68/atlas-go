package marketdata

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/config"
)

// ---------------------------------------------------------------------------
// 2026-09-24: TWSE STOCK_DAY_ALL retry.
//
// Production evidence (Mac Mini, 2026-09-24): 20/20 probes of
// www.twse.com.tw/exchangeReport/STOCK_DAY_ALL returned 200 in 70–170ms, yet
// twse_replay_sync recorded
// `context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
// on 2026-09-23T15:30:23Z with consecutive_failures=1 — i.e. ONE 20s
// per-attempt timeout lost the whole daily replay sync, because GetQuotes had
// no retry at all. These tests pin the fix: bounded retries with exponential
// backoff, transport timeouts retried, and a caller budget that actually
// covers the policy.
// ---------------------------------------------------------------------------

// twseOKQuotesJSON is a minimal valid STOCK_DAY_ALL payload (JSON variant).
const twseOKQuotesJSON = `{
	"stat": "OK",
	"date": "20260924",
	"title": "上市個股日成交資訊",
	"fields": ["Code","Name","TradeVolume","TradeValue","OpeningPrice","HighestPrice","LowestPrice","ClosingPrice","Change","Transaction"],
	"data": [
		["2330","台積電","81160741","15450000000","190","191.23","189.07","190.64","+0.50","35000"]
	]
}`

// newTWSERetryTestClient builds a client pointed at srv with an explicit,
// fast retry policy (real backoff defaults would make the tests sleep).
func newTWSERetryTestClient(srv *httptest.Server, timeout time.Duration, attempts int) *TWSEClient {
	c := &TWSEClient{
		httpClient:  &http.Client{Timeout: timeout, Transport: srv.Client().Transport},
		baseURL:     srv.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}
	c.SetRetryPolicyForTest(attempts, time.Millisecond, 0)
	c.SetPerAttemptTimeoutForTest(timeout)
	return c
}

func TestTWSEClient_GetQuotes_RetriesSlowResponseThenSucceeds(t *testing.T) {
	// Two responses slower than the per-attempt timeout, then a normal one:
	// exactly the production shape (a transient slow window), where a single
	// attempt used to lose the whole day's sync.
	srv, calls := slowThenFastServer(t, 2, 300*time.Millisecond, twseOKQuotesJSON)
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 40*time.Millisecond, 3)
	quotes, err := c.GetQuotes(t.Context())
	if err != nil {
		t.Fatalf("GetQuotes after 2 slow responses = error %v, want success", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("HTTP attempts = %d, want 3 (2 timeouts + 1 success)", got)
	}
	if len(quotes) != 1 || quotes[0].Symbol != "2330" {
		t.Errorf("quotes = %+v, want the single 2330 row", quotes)
	}
}

func TestTWSEClient_GetQuotes_ExhaustsAttemptsOnPersistentlySlowUpstream(t *testing.T) {
	srv, calls := slowThenFastServer(t, math.MaxInt32, 300*time.Millisecond, twseOKQuotesJSON)
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 40*time.Millisecond, 3)
	_, err := c.GetQuotes(t.Context())
	if err == nil {
		t.Fatal("GetQuotes with a permanently slow upstream = nil error, want a timeout error")
	}
	if !strings.Contains(err.Error(), "Client.Timeout exceeded") {
		t.Errorf("error %q must keep naming the upstream timeout (channel LastError contract)", err.Error())
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("HTTP attempts = %d, want exactly 3 (bounded, never unbounded)", got)
	}
}

func TestTWSEClient_GetQuotes_NoRetryWhenAttemptsIsOne(t *testing.T) {
	srv, calls := slowThenFastServer(t, math.MaxInt32, 300*time.Millisecond, twseOKQuotesJSON)
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 40*time.Millisecond, 1)
	if _, err := c.GetQuotes(t.Context()); err == nil {
		t.Fatal("GetQuotes = nil error, want the timeout")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("HTTP attempts = %d, want 1 (retries explicitly disabled)", got)
	}
}

func TestTWSEClient_GetQuotes_Retries5xxThenSucceeds(t *testing.T) {
	srv, calls := newRetryTestServer(t, []struct {
		status     int
		body       string
		retryAfter string
	}{
		{status: http.StatusServiceUnavailable, body: "upstream busy"},
		{status: http.StatusOK, body: twseOKQuotesJSON},
	})
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 5*time.Second, 3)
	quotes, err := c.GetQuotes(t.Context())
	if err != nil {
		t.Fatalf("GetQuotes after 503 retry = error %v, want success", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("HTTP attempts = %d, want 2 (503 + 1 retry)", got)
	}
	if len(quotes) != 1 {
		t.Errorf("quotes len = %d, want 1", len(quotes))
	}
}

func TestTWSEClient_GetQuotes_HandBuiltClientRetriesByDefault(t *testing.T) {
	// A client assembled as a struct literal (no retryCfg) must still get the
	// production policy — otherwise the fix would silently do nothing on any
	// future call site that forgets to wire the field.
	srv, calls := slowThenFastServer(t, 2, 300*time.Millisecond, twseOKQuotesJSON)
	defer srv.Close()

	c := &TWSEClient{
		httpClient:  &http.Client{Timeout: 40 * time.Millisecond, Transport: srv.Client().Transport},
		baseURL:     srv.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}
	if c.retryCfg.maxAttempts != 0 {
		t.Fatalf("precondition: literal client retryCfg = %+v, want the zero value", c.retryCfg)
	}
	// Real default backoff (1s + 2s) is exercised here, so keep it to the
	// shipment default of 3 attempts.
	quotes, err := c.GetQuotes(t.Context())
	if err != nil {
		t.Fatalf("GetQuotes with the fallback retry policy = error %v, want success", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("HTTP attempts = %d, want 3 (marketdata.max_retry_attempts default)", got)
	}
	if len(quotes) != 1 {
		t.Errorf("quotes len = %d, want 1", len(quotes))
	}
}

func TestTWSEClient_RetryPolicy_TransportErrorsRetryable(t *testing.T) {
	c := &TWSEClient{}
	pol := c.retryPolicy()
	if !pol.retryTransportErrors {
		t.Error("TWSE policy must retry transport errors (timeouts are how TWSE fails)")
	}
	if pol.maxBackoff != twseMaxRetryBackoff {
		t.Errorf("maxBackoff = %v, want %v", pol.maxBackoff, twseMaxRetryBackoff)
	}
	if pol.maxAttempts != 3 {
		t.Errorf("maxAttempts = %d, want 3 (marketdata.max_retry_attempts)", pol.maxAttempts)
	}
	// An explicitly configured client wins over the fallback.
	c.SetRetryPolicyForTest(7, 250*time.Millisecond, 3*time.Second)
	if got := c.retryPolicy().maxAttempts; got != 7 {
		t.Errorf("maxAttempts = %d, want 7 (explicit policy must win)", got)
	}
}

func TestTWSEClient_FetchBudget_CoversTheRetryPolicy(t *testing.T) {
	// Root cause guard: daily-replay-sync used a hardcoded 60s context while
	// the retry policy needs more, so the deadline expired mid-retry. The
	// budget must describe the whole policy, including backoff.
	ResetSharedTWSEClient()
	c := GetSharedTWSEClient()

	params := config.GetParametersConfig()
	perAttempt := time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value) * time.Second
	pol := c.retryPolicy()
	want := twseRateLimitWaitAllowance + time.Duration(pol.maxAttempts)*perAttempt
	for attempt := range pol.maxAttempts - 1 {
		want += retryWait(pol, attempt)
	}
	if got := c.FetchBudget(); got != want {
		t.Errorf("FetchBudget() = %v, want %v (attempts*timeout + backoff + limiter slack)", got, want)
	}
	if c.FetchBudget() <= 60*time.Second {
		t.Errorf("FetchBudget() = %v must exceed the old hardcoded 60s context, otherwise the retry loop is cut short", c.FetchBudget())
	}
}

func TestTWSEClient_FetchBudget_HandBuiltClientUsesParameterTimeout(t *testing.T) {
	c := &TWSEClient{}
	params := config.GetParametersConfig()
	pol := defaultRetryConfig()
	want := twseRateLimitWaitAllowance +
		time.Duration(pol.maxAttempts)*time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value)*time.Second
	for attempt := range pol.maxAttempts - 1 {
		want += retryWait(pol, attempt)
	}
	if got := c.FetchBudget(); got != want {
		t.Errorf("FetchBudget() = %v, want %v (falls back to marketdata.twse_api_timeout_sec)", got, want)
	}
}
