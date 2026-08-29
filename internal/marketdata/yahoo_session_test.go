package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestYahooSession_ConcurrentEnsureCrumb_SingleFlight verifies that when the
// shared Yahoo session has no cached crumb, many concurrent FetchSnapshot calls
// perform exactly one cookie+crumb handshake instead of N. This prevents the
// "thundering herd" that exacerbated the production lock-cascade hang.
func TestYahooSession_ConcurrentEnsureCrumb_SingleFlight(t *testing.T) {
	var cookieCalls atomic.Int32
	var crumbCalls atomic.Int32
	var chartCalls atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			cookieCalls.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "A3", Value: "test-session"})
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/v1/test/getcrumb"):
			crumbCalls.Add(1)
			// Deliberately slow to make the thundering-herd window wide.
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("test-crumb"))
		case strings.HasPrefix(r.URL.Path, "/v8/finance/chart/"):
			chartCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1700000000},"indicators":{"quote":[{"close":[100,101]}]}}]}}`))
		default:
			t.Logf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{"query1.finance.yahoo.com"}
	defer func() { yahooHosts = origHosts }()

	// Point the singleton session at the mock server and clear any cached crumb
	// so the first FetchSnapshot is forced to do a handshake.
	SetYahooSessionClient(&http.Client{Transport: &rewriteHostTransport{target: ts.URL}})
	globalYahooSession.s.crumb = ""
	globalYahooSession.s.cookie = ""
	globalYahooSession.s.lastFetch = time.Time{}

	const n = 20
	var wg sync.WaitGroup
	ctx := context.Background()
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := NewNVDAProvider()
			_, _ = p.FetchSnapshot(ctx)
		}()
	}
	wg.Wait()

	if got := cookieCalls.Load(); got != 1 {
		t.Errorf("cookie handshake calls = %d, want 1", got)
	}
	if got := crumbCalls.Load(); got != 1 {
		t.Errorf("crumb handshake calls = %d, want 1", got)
	}
	if got := chartCalls.Load(); got != n {
		t.Errorf("chart fetches = %d, want %d", got, n)
	}
}

// TestYahooSession_ConcurrentEnsureCrumb_NoDeadlock runs the same concurrency
// surface under the race detector to catch lock-ordering or data races in the
// crumb refresh path. Uses a mock server so the test does not hit the network.
func TestYahooSession_ConcurrentEnsureCrumb_NoDeadlock(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case r.URL.Path == "/":
			http.SetCookie(w, &http.Cookie{Name: "A3", Value: "x"})
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/v1/test/getcrumb"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("crumb"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{"query1.finance.yahoo.com"}
	defer func() { yahooHosts = origHosts }()

	SetYahooSessionClient(&http.Client{Transport: &rewriteHostTransport{target: ts.URL}})
	globalYahooSession.s.crumb = ""
	globalYahooSession.s.cookie = ""
	globalYahooSession.s.lastFetch = time.Time{}

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = globalYahooSession.s.ensureCrumb(context.Background())
		}()
	}
	wg.Wait()

	if calls.Load() < 2 {
		t.Errorf("expected at least one cookie and one crumb call, got %d", calls.Load())
	}
}

// rewriteHostTransport redirects all HTTP requests to the test server URL.
type rewriteHostTransport struct{ target string }

func (t *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(cloneRequestForURL(req, t.target))
}

func cloneRequestForURL(req *http.Request, target string) *http.Request {
	r := req.Clone(req.Context())
	r.URL.Scheme = "http"
	r.URL.Host = target[7:] // strip "http://"
	return r
}
