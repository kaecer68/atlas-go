package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ─── P1-7: shared providerBreaker semantics ─────────────────────────────────
//
// The providerBreaker component (circuit_breaker.go) is shared across
// FinMind / TWSE / TAIFEX / Yahoo / GovernmentBroker. These tests pin the
// shared semantics: (1) injectable threshold, (2) consecutive upstream
// failures trip the breaker, (3) no-data / holiday / quota conditions do
// NOT count as failures.

func TestProviderBreaker_InjectableThreshold(t *testing.T) {
	// threshold=1: a single failure opens the circuit.
	b1 := newProviderBreaker("t1", circuitBreakerConfig{
		failureThreshold: 1,
		recoveryTimeout:  100 * time.Millisecond,
		halfOpenMaxCalls: 1,
	})
	b1.recordFailure()
	if b1.stateSnapshot().State != ProviderCircuitOpen {
		t.Fatal("threshold=1 breaker must open after 1 failure")
	}

	// threshold=5: five failures required.
	b5 := newProviderBreaker("t5", circuitBreakerConfig{
		failureThreshold: 5,
		recoveryTimeout:  100 * time.Millisecond,
		halfOpenMaxCalls: 1,
	})
	for i := range 4 {
		b5.recordFailure()
		if b5.stateSnapshot().State != ProviderCircuitClosed {
			t.Fatalf("threshold=5 breaker opened after %d failures", i+1)
		}
	}
	b5.recordFailure()
	if b5.stateSnapshot().State != ProviderCircuitOpen {
		t.Fatal("threshold=5 breaker must open after 5 failures")
	}
	if got := b5.stateSnapshot().Threshold; got != 5 {
		t.Fatalf("Threshold() = %d, want 5", got)
	}
}

func TestProviderBreaker_NoDataDoesNotCountAsFailure(t *testing.T) {
	// A provider that classifies its no-data condition via recordSuccess
	// must never trip: interleave no-data events with the failure counter.
	b := newProviderBreaker("nodata", fastTestConfig())
	for range 10 {
		b.recordFailure()
		// "no-data/holiday" conditions reset instead of counting.
		b.recordSuccess()
	}
	if b.stateSnapshot().State != ProviderCircuitClosed {
		t.Fatal("no-data events must not accumulate as breaker failures")
	}
	if b.stateSnapshot().FailureCount != 0 {
		t.Fatalf("FailureCount = %d, want 0", b.stateSnapshot().FailureCount)
	}
}

// ─── FinMind ────────────────────────────────────────────────────────────────

func TestFinMindBreaker_OpensAfterUpstreamFailures(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream boom", http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}
	c.rateLimiter = newUnlimitedLimiter()
	c.retryCfg = retryConfig{maxAttempts: 1, baseBackoff: time.Millisecond}

	ctx := context.Background()
	for i := range 3 {
		if _, err := c.fetchDataset(ctx, "TaiwanStockPrice", "2330", "2026-01-01", "2026-01-01"); err == nil {
			t.Fatalf("attempt %d: expected error", i+1)
		}
	}
	if got := c.BreakerInfo().State; got != ProviderCircuitOpen {
		t.Fatalf("breaker state = %s, want open", got)
	}

	// Next call short-circuits without HTTP (returns breaker-open error).
	if _, err := c.fetchDataset(ctx, "TaiwanStockPrice", "2330", "2026-01-01", "2026-01-01"); err == nil ||
		!strings.Contains(err.Error(), "breaker open") {
		t.Fatalf("expected breaker-open error, got %v", err)
	}
}

func TestFinMindBreaker_QuotaExhaustionDoesNotTrip(t *testing.T) {
	// Daily-quota exhaustion is a budget condition (auto-resets at 00:00 TW),
	// not an outage: it must NOT trip the client breaker.
	c := &FinMindClient{
		quotaTracker: &DailyQuotaTracker{
			dailyLimit: 0,
			lastReset:  time.Now().Truncate(24 * time.Hour),
		},
		rateLimiter: newUnlimitedLimiter(),
		breaker:     newProviderBreaker("finmind", fastTestConfig()),
		retryCfg:    retryConfig{maxAttempts: 1},
	}
	ctx := context.Background()
	for i := range 5 {
		_, err := c.fetchDataset(ctx, "TaiwanStockPrice", "2330", "2026-01-01", "2026-01-01")
		if err == nil || !strings.Contains(err.Error(), ErrQuotaExhausted.Error()) {
			t.Fatalf("call %d: expected ErrQuotaExhausted, got %v", i+1, err)
		}
	}
	if got := c.BreakerInfo().State; got != ProviderCircuitClosed {
		t.Fatalf("breaker state = %s, want closed (quota must not trip)", got)
	}
}

func TestFinMindBreaker_NoDataDoesNotTrip(t *testing.T) {
	// Empty data (dataset exists but no rows — holiday/weekend) is a normal
	// FinMind response, not a failure.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}
	c.rateLimiter = newUnlimitedLimiter()
	c.retryCfg = retryConfig{maxAttempts: 1}

	ctx := context.Background()
	for i := range 5 {
		data, err := c.fetchDataset(ctx, "TaiwanStockPrice", "2330", "2026-01-01", "2026-01-01")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
		if len(data) != 0 {
			t.Fatalf("call %d: expected empty data", i+1)
		}
	}
	if got := c.BreakerInfo().State; got != ProviderCircuitClosed {
		t.Fatalf("breaker state = %s, want closed (empty data must not trip)", got)
	}
}

// ─── TWSE ──────────────────────────────────────────────────────────────────

func TestTWSEBreaker_OpensAfterUpstreamFailures(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := GetSharedTWSEClient()
	c.baseURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.rateLimiter = newUnlimitedLimiter()
	c.breaker = newProviderBreaker("twse", fastTestConfig())

	ctx := context.Background()
	for i := range 3 {
		if _, err := c.GetQuotes(ctx); err == nil {
			t.Fatalf("attempt %d: expected error", i+1)
		}
	}
	if got := c.BreakerInfo().State; got != ProviderCircuitOpen {
		t.Fatalf("breaker state = %s, want open", got)
	}
}

func TestTWSEBreaker_EmptyDataDoesNotTrip(t *testing.T) {
	// stat=OK with zero rows = holiday / market-closed: typed ErrTWSEEmptyData
	// must NOT trip the breaker.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"stat":"OK","date":"20260101","title":"t","fields":[],"data":[]}`))
	}))
	defer ts.Close()

	c := GetSharedTWSEClient()
	c.baseURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.rateLimiter = newUnlimitedLimiter()
	c.breaker = newProviderBreaker("twse", fastTestConfig())

	ctx := context.Background()
	for i := range 5 {
		if _, err := c.GetQuotes(ctx); err == nil || !isNoDataErr(err) {
			t.Fatalf("call %d: expected ErrTWSEEmptyData, got %v", i+1, err)
		}
	}
	if got := c.BreakerInfo().State; got != ProviderCircuitClosed {
		t.Fatalf("breaker state = %s, want closed (empty data must not trip)", got)
	}
}

// ─── TAIFEX ─────────────────────────────────────────────────────────────────

func TestTAIFEXBreaker_SchemaFailureTrips(t *testing.T) {
	// Upstream schema change (missing field) must trip the breaker, unlike
	// a plain empty-list response.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"Date":"2026-01-01"}]`)) // missing all numeric fields
	}))
	defer ts.Close()

	p := NewTAIFEXProvider()
	p.baseURL = ts.URL
	p.SetHTTPClient(ts.Client())
	p.rateLimiter = newUnlimitedLimiter()
	p.retryCfg = retryConfig{maxAttempts: 1}
	p.breaker = newProviderBreaker("taifex", fastTestConfig())

	ctx := context.Background()
	for i := range 3 {
		if _, err := p.FetchPCR(ctx); err == nil {
			t.Fatalf("attempt %d: expected ErrTAIFEXSchema", i+1)
		}
	}
	if got := p.BreakerInfo().State; got != ProviderCircuitOpen {
		t.Fatalf("breaker state = %s, want open (schema change must trip)", got)
	}
}

func TestTAIFEXBreaker_EmptyListDoesNotTrip(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	p := NewTAIFEXProvider()
	p.baseURL = ts.URL
	p.SetHTTPClient(ts.Client())
	p.rateLimiter = newUnlimitedLimiter()
	p.retryCfg = retryConfig{maxAttempts: 1}
	p.breaker = newProviderBreaker("taifex", fastTestConfig())

	ctx := context.Background()
	for i := range 5 {
		if _, err := p.FetchPCR(ctx); err == nil {
			t.Fatalf("call %d: expected empty-list error", i+1)
		}
	}
	if got := p.BreakerInfo().State; got != ProviderCircuitClosed {
		t.Fatalf("breaker state = %s, want closed (empty list must not trip)", got)
	}
}

// ─── Yahoo ──────────────────────────────────────────────────────────────────

func TestYahooBreaker_AllHostsFailTrips(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())
	s := getYahooSession()
	oldBreaker := s.breaker
	s.breaker = newProviderBreaker("yahoo", fastTestConfig())
	defer func() { s.breaker = oldBreaker }() // restore so later Yahoo tests start clean

	ctx := context.Background()
	for i := range 3 {
		if _, err := s.fetchWithFallback(ctx, "^TWII", map[string]string{"interval": "1d", "range": "5d"}); err == nil {
			t.Fatalf("attempt %d: expected error", i+1)
		}
	}
	if got := s.BreakerInfo().State; got != ProviderCircuitOpen {
		t.Fatalf("breaker state = %s, want open", got)
	}

	// Open breaker short-circuits all Yahoo channels (single shared session).
	if _, err := s.fetchWithFallback(ctx, "AAPL", map[string]string{"interval": "1d", "range": "5d"}); err == nil ||
		!strings.Contains(err.Error(), "breaker open") {
		t.Fatalf("expected breaker-open error for any symbol, got %v", err)
	}
}

// ─── GovernmentBroker ───────────────────────────────────────────────────────

func TestGovernmentBrokerBreaker_NoDataDoesNotTrip(t *testing.T) {
	// A real no-data upstream: bsMenu.aspx returns a valid ASP.NET form
	// (form tokens present) and the POST query returns a page with no
	// broker table rows (holiday / empty trading day). Each symbol parses
	// successfully into an empty result — no transport/parse failure — so
	// AggregateDate yields (nil, nil) and the breaker must NOT accumulate
	// failures. (k3 audit: the previous mock returned token-less HTML,
	// which actually exercised the failure path and only passed because the
	// buggy unconditional recordSuccess masked it.)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.Method == http.MethodGet {
			// bsMenu.aspx — valid form with the three ASP.NET tokens.
			_, _ = w.Write([]byte(`<html><body><form>
<input type="hidden" id="__VIEWSTATE" value="abc123">
<input type="hidden" id="__VIEWSTATEGENERATOR" value="gen123">
<input type="hidden" id="__EVENTVALIDATION" value="ev123">
</form></body></html>`))
			return
		}
		// POST query — valid response, but no broker table rows for this date.
		_, _ = w.Write([]byte(`<html><body><table><tr><td>no rows</td></tr></table></body></html>`))
	}))
	defer ts.Close()

	agg := NewGovernmentBrokerAggregator(t.TempDir())
	agg.SetHTTPClient(ts.Client())
	agg.SetBaseURL(ts.URL)
	agg.limiter = newUnlimitedLimiter()
	agg.SetFetchSource(GovSourceLegacyScraper)
	agg.breaker = newProviderBreaker("government_broker", fastTestConfig())

	ctx := context.Background()
	for i := range 5 {
		res, err := agg.AggregateDate(ctx, time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i+1, err)
		}
		// A holiday/empty page parses into an empty reading (TotalNet 0),
		// NOT (nil, nil): the (nil, nil) contract is reserved for the
		// all-failure path (see AllTransportFail test), which is exactly
		// the path the old unconditional recordSuccess was masking.
		if res != nil && (res.TotalNet != 0 || res.Source != "broker-aggregate") {
			t.Fatalf("run %d: unexpected reading %+v", i+1, res)
		}
	}
	if got := agg.BreakerInfo().State; got != ProviderCircuitClosed {
		t.Fatalf("breaker state = %s, want closed (no-data must not trip)", got)
	}
	if got := agg.BreakerInfo().FailureCount; got != 0 {
		t.Fatalf("FailureCount = %d, want 0 (no-data must not accumulate failures)", got)
	}
}

func TestGovernmentBrokerBreaker_AllTransportFail_OpensBreaker(t *testing.T) {
	// Upstream completely down: every symbol fetch fails at the transport
	// layer. The breaker must OPEN (threshold 3, one run of 50 symbols
	// exceeds it) and stay open — the run must NOT reset it via the
	// no-data path (k3 audit regression guard). The next run is gated by
	// the open breaker without sending any request.
	reqs := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs++
		http.Error(w, "upstream unavailable", http.StatusInternalServerError)
	}))
	defer ts.Close()

	agg := NewGovernmentBrokerAggregator(t.TempDir())
	agg.SetHTTPClient(ts.Client())
	agg.SetBaseURL(ts.URL)
	agg.limiter = newUnlimitedLimiter()
	agg.SetFetchSource(GovSourceLegacyScraper)
	agg.breaker = newProviderBreaker("government_broker", fastTestConfig())

	ctx := context.Background()
	date := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

	// Run 1: all symbols fail -> error surfaced + breaker opens.
	res, err := agg.AggregateDate(ctx, date)
	if err == nil {
		t.Fatal("expected error when all symbols fail, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result on total failure, got %+v", res)
	}
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("error = %v, want wrapped ErrUpstream", err)
	}
	if got := agg.BreakerInfo().State; got != ProviderCircuitOpen {
		t.Fatalf("breaker state = %s, want open after all-fail run", got)
	}

	// Run 2: gated by the open breaker — no requests sent, open error.
	reqsBefore := reqs
	_, err = agg.AggregateDate(ctx, date)
	if err == nil {
		t.Fatal("expected error from open breaker, got nil")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("open-breaker error = %v, want wrapped ErrUpstream", err)
	}
	if reqs != reqsBefore {
		t.Fatalf("open breaker must not send requests (sent %d new)", reqs-reqsBefore)
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

func newUnlimitedLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Inf, 0)
}

func isNoDataErr(err error) bool {
	return strings.Contains(err.Error(), ErrTWSEEmptyData.Error())
}
