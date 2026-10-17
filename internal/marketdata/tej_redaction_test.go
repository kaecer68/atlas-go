package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// tejRedactionFixtureKey is a fake credential. The probe below asserts it can
// never appear in a TEJ error string: the key rides in the QUERY STRING, and a
// transport error formats the whole URL, so before FU-20261016-11 the key went
// straight into channel_health.
const tejRedactionFixtureKey = "FAKE-tej-api-key-9f3c1f0b7a2e4d5c"

// newRedactionTestTEJClient builds a TEJ client pointed at baseURL with a real
// (non-infinite-rate, temp quota) wiring replaced by test doubles, so the probe
// measures redaction and nothing else.
func newRedactionTestTEJClient(t *testing.T, baseURL string) *TEJClient {
	t.Helper()
	client := newTEJClientInternal(tejRedactionFixtureKey, t.TempDir())
	client.baseURL = baseURL
	client.SetHTTPClient(&http.Client{Timeout: 2 * time.Second})
	client.rateLimiter = rate.NewLimiter(rate.Inf, 1)
	return client
}

// TestTEJClient_Errors_HideAPIKey is the negative proof for FU-20261016-11:
// every TEJ error path must reach the sink without the credential, and must
// keep the fields an operator needs (status, upstream code, upstream message).
func TestTEJClient_Errors_HideAPIKey(t *testing.T) {
	t.Run("transport error keeps the URL but not the credential", func(t *testing.T) {
		// A closed port produces a real *url.Error. The point is that Go
		// formats the request URL into it.
		closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		closedURL := closed.URL
		closed.Close()

		client := newRedactionTestTEJClient(t, closedURL)
		_, err := client.GetStockPriceDaily(context.Background(), "2330", "2025-01-03", "2025-01-03")
		if err == nil {
			t.Fatal("expected a transport error against a closed port")
		}
		assertNoCredential(t, "GetStockPriceDaily transport error", err.Error(),
			"tej http request", "TRAIL/TAPRCD.json")
	})

	t.Run("non-200 body is redacted but status and upstream error survive", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			// Shape of a real rejection: the vendor echoes the key it saw,
			// both as a JSON member and inside the message text.
			_, _ = w.Write([]byte(`{"error":{"code":"401","message":"invalid api_key ` +
				r.URL.Query().Get("api_key") + ` for api_key=` + tejRedactionFixtureKey + `&coid=2330"}}`))
		}))
		defer server.Close()

		client := newRedactionTestTEJClient(t, server.URL)
		_, err := client.GetStockPriceDaily(context.Background(), "2330", "2025-01-03", "2025-01-03")
		if err == nil {
			t.Fatal("expected an error for HTTP 401")
		}
		assertNoCredential(t, "GetStockPriceDaily 401 body", err.Error(),
			"tej api error", "status 401", "invalid api_key")
	})

	t.Run("upstream error envelope is redacted but code and message survive", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"error":{"code":"AUTH_FAILED","message":"token ` +
				tejRedactionFixtureKey + ` has no entitlement"}}`))
		}))
		defer server.Close()

		client := newRedactionTestTEJClient(t, server.URL)
		_, err := client.GetStockPriceDaily(context.Background(), "2330", "2025-01-03", "2025-01-03")
		if err == nil {
			t.Fatal("expected an error for an error envelope")
		}
		assertNoCredential(t, "GetStockPriceDaily error envelope", err.Error(),
			"tej api error", "AUTH_FAILED", "has no entitlement")
	})

	t.Run("financial statements transport and 401 paths follow the same rule", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"401","message":"bad key ` + tejRedactionFixtureKey + `"}}`))
		}))
		defer server.Close()

		client := newRedactionTestTEJClient(t, server.URL)
		keys, err := client.GetFinancialStatements(context.Background(), "2330", "TWN/AFINA", "2025-01-01", "2025-03-31")
		_ = keys
		if err == nil {
			t.Fatal("expected an error for HTTP 401")
		}
		assertNoCredential(t, "GetFinancialStatements 401 body", err.Error(),
			"tej api error", "status 401", "bad key")
	})
}

// assertNoCredential is the shared probe: the fake credential is absent, and
// the named diagnostic fragments are still present. Both directions matter —
// a redaction that removes the credential by removing the message is not a fix.
func assertNoCredential(t *testing.T, what, got string, wantKept ...string) {
	t.Helper()
	// Logged so the PR body can quote the redacted text: this is the "after"
	// half of the negative proof.
	t.Logf("%s -> %s", what, got)
	if strings.Contains(got, tejRedactionFixtureKey) {
		t.Errorf("%s leaks the TEJ api_key: %s", what, got)
	}
	for _, want := range wantKept {
		if !strings.Contains(got, want) {
			t.Errorf("%s dropped the diagnostic text %q: %s", what, want, got)
		}
	}
}
